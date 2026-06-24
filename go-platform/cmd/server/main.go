// Enterprise Agent Platform — Go Backend 入口。
//
// 启动流程：
//  1. 加载环境变量配置
//  2. 创建 PostgreSQL 连接池
//  3. 自动执行数据库迁移（migrations/*.up.sql）
//  4. 初始化 auth 模块
//  5. 初始化 workflow 模块
//  6. 初始化 agent 模块（Registry + Gateway）
//  7. 初始化 tool 模块（Registry）
//  8. 将 Gateway 注入 Workflow Worker（连接 Workflow → Agent）
//  9. 启动 Asynq worker 服务端
//
// 10. 注册所有 API 路由
// 11. 启动 HTTP 服务器，优雅退出
//
// 依赖注入图（Phase 1-3 完整版）：
//
//	config.Load()
//	→ database.NewPool()
//	→ auth.NewRepository() → auth.NewService()
//	→ workflow.NewRepository() → workflow.NewEngine()
//	→ workflow.NewWorker() → workflow.NewService() → SetWorker()
//	→ agent.NewRepository() → agent.NewGateway()
//	→ tool.NewRepository()
//	→ workflowWorker.SetGateway(agentGateway, agentRepo)  ← 关键连线
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/auth"
	"github.com/enterprise-agent-platform/go-platform/internal/config"
	"github.com/enterprise-agent-platform/go-platform/internal/database"
	platformfile "github.com/enterprise-agent-platform/go-platform/internal/file"
	"github.com/enterprise-agent-platform/go-platform/internal/tool"
	"github.com/enterprise-agent-platform/go-platform/internal/workflow"
)

func main() {
	// ── 第 1 步：加载配置 ──
	cfg := config.Load()

	if cfg.ServerMode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx := context.Background()

	// ── 第 2 步：连接数据库 ──
	pool, err := database.NewPool(ctx, cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		log.Fatalf("database pool: %v", err)
	}
	defer pool.Close()

	// ── 第 3 步：验证数据库可达 + 执行迁移 ──
	if err := pool.Ping(ctx); err != nil {
		log.Printf("WARNING: database not reachable at %s:%s — %v", cfg.DBHost, cfg.DBPort, err)
		log.Println("Server will start but DB-dependent routes will fail.")
	} else {
		log.Printf("Connected to PostgreSQL at %s:%s/%s", cfg.DBHost, cfg.DBPort, cfg.DBName)
		if err := database.RunMigrations(ctx, pool); err != nil {
			log.Printf("WARNING: migration error: %v", err)
		}
	}

	// ── 第 4 步：组装 auth 依赖链 ──
	authRepo := auth.NewRepository(pool)
	authSvc := auth.NewService(authRepo, cfg.JWTSecret, cfg.JWTExpirationHours)
	authHandler := auth.NewHandler(authSvc)
	authMiddleware := auth.AuthMiddleware(authSvc)

	// ── 第 5 步：组装 audit 模块（提前创建，workflow/agent 模块需要注入） ──
	auditRepo := audit.NewRepository(pool)
	auditHandler := audit.NewHandler(auditRepo)

	// ── 第 6 步：组装 workflow 依赖链 ──
	workflowRepo := workflow.NewRepository(pool)
	workflowEngine := workflow.NewEngine()
	workflowSvc := workflow.NewService(workflowRepo, auditRepo, workflowEngine, nil)
	workflowWorker := workflow.NewWorker(
		cfg.RedisHost+":"+cfg.RedisPort,
		workflowSvc,
	)
	workflowSvc.SetWorker(workflowWorker)
	workflowHandler := workflow.NewHandler(workflowSvc)

	// ── 第 7 步：组装 agent 模块 ──
	// Agent Registry + Gateway（调用 Python Agent Service 的统一入口）
	agentRepo := agent.NewRepository(pool)
	agentGateway := agent.NewGateway(agentRepo, auditRepo, cfg.AgentServiceURL, cfg.StrictDomainPolicy)
	agentHandler := agent.NewHandler(agentRepo, auditRepo)
	agentHandler.SetWorkflowService(workflowSvc)

	// ── 第 8 步：组装 tool 模块 ──
	toolRepo := tool.NewRepository(pool)
	toolHandler := tool.NewHandler(toolRepo)

	fileRepo := platformfile.NewRepository(pool)
	fileHandler := platformfile.NewHandler(fileRepo, auditRepo, cfg.MinIOBucket, cfg.FileStorageDir)

	// ── 第 9 步：关键连线 — Gateway 注入 Workflow Worker ──
	// agent_graph 节点执行时，Worker 通过 Gateway 调用 Python Agent Service
	workflowWorker.SetGateway(agentGateway, agentRepo)

	// ── 第 10 步：启动 Asynq worker 服务端 ──
	redisAddr := cfg.RedisHost + ":" + cfg.RedisPort
	go func() {
		if err := workflowWorker.StartServer(context.Background(), redisAddr); err != nil {
			log.Printf("WARNING: Asynq worker error: %v", err)
		}
	}()
	log.Printf("Asynq worker started, connected to Redis at %s", redisAddr)
	log.Printf("Agent Gateway configured: Python Agent Service at %s", cfg.AgentServiceURL)

	// ── 第 11 步：创建 Gin 路由并注册所有端点 ──
	router := gin.New()

	router.Use(gin.Logger(), gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Trace-Id"},
		ExposeHeaders:    []string{"X-Trace-Id"},
		AllowCredentials: true,
	}))

	// 健康检查
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/internal/v1/files/:storage_key/content", fileHandler.GetContent)

	v1 := router.Group("/api/v1")

	// ── 公开 API ──
	{
		v1.POST("/auth/login", authHandler.Login)
	}

	// ── 受保护 API ──
	protected := v1.Group("")
	protected.Use(authMiddleware)
	{
		// Auth
		protected.GET("/auth/me", authHandler.Me)
		protected.GET("/business-apps", authHandler.GetBusinessApps)

		// Workflow
		protected.GET("/business-apps/:code/workflow-templates", workflowHandler.GetTemplates)
		protected.POST("/workflow-instances", workflowHandler.CreateInstance)
		protected.GET("/workflow-instances", workflowHandler.ListInstances)
		protected.GET("/workflow-instances/:id", workflowHandler.GetInstance)
		protected.POST("/workflow-instances/:id/start", workflowHandler.StartInstance)
		protected.POST("/workflow-instances/:id/cancel", workflowHandler.CancelInstance)
		protected.POST("/workflow-instances/:id/retry", workflowHandler.RetryNode)
		protected.GET("/workflow-instances/:id/nodes", workflowHandler.GetNodes)

		// Agent Registry
		protected.GET("/agents", agentHandler.ListAgents)
		protected.POST("/agents", agentHandler.CreateAgent)

		// Agent Run Logs
		protected.GET("/agent-run-logs", agentHandler.ListRunLogs)

		// Tool Registry
		protected.GET("/tools", toolHandler.ListTools)

		// Files
		protected.POST("/files", fileHandler.Upload)
		protected.GET("/files/:id", fileHandler.Get)

		// Approval Tasks
		protected.GET("/approval-tasks", agentHandler.ListApprovalTasks)
		protected.GET("/approval-tasks/:id", agentHandler.GetApprovalTask)
		protected.POST("/approval-tasks/:id/approve", agentHandler.ApproveTask)
		protected.POST("/approval-tasks/:id/reject", agentHandler.RejectTask)

		// Audit Logs
		protected.GET("/audit-logs", auditHandler.ListAuditLogs)
	}

	// ── 第 11 步：启动 HTTP 服务器 ──
	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: router,
	}

	go func() {
		log.Printf("Go Backend listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// ── 优雅退出 ──
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workflowWorker.Close()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}
