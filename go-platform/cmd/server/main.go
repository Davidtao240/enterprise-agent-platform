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
	"github.com/joho/godotenv"
	"golang.org/x/time/rate"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
	"github.com/enterprise-agent-platform/go-platform/internal/agent_gallery"
	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/auth"
	"github.com/enterprise-agent-platform/go-platform/internal/business"
	"github.com/enterprise-agent-platform/go-platform/internal/config"
	"github.com/enterprise-agent-platform/go-platform/internal/contextbuilder"
	"github.com/enterprise-agent-platform/go-platform/internal/conversation"
	"github.com/enterprise-agent-platform/go-platform/internal/database"
	"github.com/enterprise-agent-platform/go-platform/internal/eval"
	"github.com/enterprise-agent-platform/go-platform/internal/experiment"
	platformfile "github.com/enterprise-agent-platform/go-platform/internal/file"
	"github.com/enterprise-agent-platform/go-platform/internal/governance"
	"github.com/enterprise-agent-platform/go-platform/internal/knowledge"
	"github.com/enterprise-agent-platform/go-platform/internal/memory"
	"github.com/enterprise-agent-platform/go-platform/internal/middleware"
	"github.com/enterprise-agent-platform/go-platform/internal/observability"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/internal/policy"
	"github.com/enterprise-agent-platform/go-platform/internal/skill"
	"github.com/enterprise-agent-platform/go-platform/internal/tool"
	"github.com/enterprise-agent-platform/go-platform/internal/trace"
	"github.com/enterprise-agent-platform/go-platform/internal/workflow"
)

func main() {
	// ── 第 0 步：加载根目录 .env 文件（开发模式下使用） ──
	_ = godotenv.Load("../../.env", "../.env", ".env")

	// ── 第 1 步：加载配置 ──
	cfg := config.Load()

	// 关键安全检查：告警/阻止使用不安全的默认密钥启动生产服务
	if warnings := cfg.Validate(); len(warnings) > 0 {
		for _, w := range warnings {
			log.Printf("CONFIG: %s", w)
		}
		if cfg.ServerMode == "release" {
			log.Fatal("Refusing to start in release mode with insecure defaults. " +
				"Set the required environment variables or switch GO_SERVER_MODE to 'debug'.")
		}
	}

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

	// ── 第 5 步：组装 audit 模块（提前创建，workflow/agent 模块需要注入） ──
	auditRepo := audit.NewRepository(pool)
	auditHandler := audit.NewHandler(auditRepo)
	authHandler.SetAuditLogger(auditRepo)
	authMiddleware := auth.AuthMiddlewareWithAudit(authSvc, auditRepo)

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
	agentGateway.SetHTTPTimeout(cfg.AgentServiceTimeout)
	agentGateway.ConfigureRuntimeV2(cfg.InternalServiceToken)
	// Worker 执行路径开关(M2-A):true 时 agent_graph 节点走 Runtime V2 异步
	// (StartV2 + 事件推进),false 时保持 V1 同步桥。默认关闭,部署环境经
	// WORKER_RUNTIME_V2 显式开启。
	if cfg.WorkerRuntimeV2 {
		agentGateway.EnableWorkerRuntimeV2()
		log.Println("[main] workflow worker agent_graph path: runtime v2 async")
	} else {
		log.Println("[main] workflow worker agent_graph path: v1 sync bridge")
	}
	agentHandler := agent.NewHandler(agentRepo, auditRepo)
	durableService := agent.NewDurableRunService(agentRepo)
	runtimeHandler := agent.NewRuntimeHandler(durableService)
	agentHandler.SetWorkflowService(workflowSvc)

	// 事件驱动完成:Runtime 终态/中断事件 → 推进 Workflow 节点。
	runEventBridge := workflow.NewRunEventBridge(workflowSvc, agentRepo)
	runtimeHandler.SetEventSink(runEventBridge, agentRepo)

	// Resume/Cancel 控制面:工作流取消联动取消 Run;审批决策恢复中断 Run。
	runtimeController := agent.NewRuntimeController(agentGateway.RuntimeV2Client(), agentRepo)
	workflowSvc.SetRunCanceller(runtimeController)
	agentHandler.SetRunResumer(runtimeController)

	// ── M5-A: 六层 Trace 系统(异步 Recorder:缓冲 + 批量写,不阻塞主链路) ──
	traceRepo := trace.NewRepository(pool)
	traceRecorder := trace.NewRecorder(traceRepo, 4096, 2*time.Second)
	runtimeHandler.SetTraceSink(traceRecorder) // L2 Run / L5 Checkpoint / L6 Interrupt
	workflowSvc.SetTraceSink(traceRecorder)    // L1 Workflow

	// ── M5-B: Eval 评估体系(Trace/Run/ToolCall/Approval 按窗口聚合) ──
	evalRepo := eval.NewRepository(pool)
	evalSvc := eval.NewService(evalRepo)
	evalHandler := eval.NewHandler(evalSvc)

	// ── M5-C: Replay / Shadow / Canary 实验机制 ──
	// Router 注入 Gateway 后,V1 Execute 与 V2 StartV2 双路径统一走实验分流;
	// Service 的 Replay 经 Gateway.StartExperimentRun 发起独立实验 Run。
	experimentRepo := experiment.NewRepository(pool)
	experimentRouter := experiment.NewRouter(experimentRepo)
	agentGateway.SetExperimentRouter(experimentRouter)
	experimentSvc := experiment.NewService(experimentRepo, agentGateway)
	experimentHandler := experiment.NewHandler(experimentSvc)

	// ── 第 8 步：组装 tool 模块 ──
	toolRepo := tool.NewRepository(pool)
	toolHandler := tool.NewHandler(toolRepo)
	policyRepo := policy.NewRepository(pool)
	// M2-B: Tool Execution Service + ToolCall 持久化 + 域策略/权限注入
	// M2-C: 注入审批任务仓储(高风险自动建审批 + 决定绑定)与域策略(执行前重检)
	// M2-C 收尾: 熔断器 + 超时/DLQ 可靠性策略
	// M2-D: Connector Binding 校验注入
	toolCallRepo := tool.NewToolCallRepository(pool)
	toolApprovalRepo := tool.NewApprovalRepository(pool)
	toolCircuitRepo := tool.NewCircuitBreakerRepository(pool)
	toolCircuit := tool.NewCircuitBreaker(toolCircuitRepo, cfg.ToolCircuitThreshold, cfg.ToolCircuitCooldown)
	credentialSvc, err := tool.NewCredentialService(pool, cfg.ToolSecretEncryptionKey, auditRepo)
	if err != nil {
		log.Fatalf("Failed to create credential service: %v", err)
	}
	// M3-A: Connector Runtime(注册表 + 门禁 + 进程内凭证解析)
	connectorRegistry := tool.NewConnectorRegistryRepository(pool)
	connectorRuntime := tool.NewConnectorRuntime(connectorRegistry, credentialSvc)
	if err := connectorRuntime.RegisterConnector(context.Background(), tool.NewMockDBReadConnector()); err != nil {
		log.Fatalf("Failed to register mock db-read connector: %v", err)
	}
	// M3-B: Mock Ticket Connector(ticket_create_or_update:幂等/乐观锁/状态验证)
	ticketConnector := tool.NewMockTicketConnector()
	if err := connectorRuntime.RegisterConnector(context.Background(), ticketConnector); err != nil {
		log.Fatalf("Failed to register mock ticket connector: %v", err)
	}
	// M3-C: Mock ERP Connector(erp_purchase_request Saga:preview/request/cancel)
	if err := connectorRuntime.RegisterConnector(context.Background(), tool.NewMockERPConnector()); err != nil {
		log.Fatalf("Failed to register mock erp connector: %v", err)
	}
	connectorRuntimeHandler := tool.NewConnectorRuntimeHandler(connectorRegistry, connectorRuntime)
	// M3-C: Outbox 仓储(Saga 写操作入队 + Dispatcher 投递)
	outboxRepo := tool.NewOutboxEntryRepository(pool)
	toolSvc := tool.NewService(toolRepo, toolCallRepo, auditRepo,
		tool.WithApprovalRepository(toolApprovalRepo),
		tool.WithDomainPolicy(policyRepo),
		tool.WithCircuitBreaker(toolCircuit),
		tool.WithReliabilityPolicy(cfg.ToolCallDefaultTimeout, cfg.ToolCallMaxRetry),
		tool.WithBindingValidator(credentialSvc),
		tool.WithConnectorRuntime(connectorRuntime), // M3-A:Connector 驱动执行
		tool.WithOutboxRepository(outboxRepo),       // M3-C:Saga 写操作经 Outbox
		tool.WithTraceSink(traceRecorder),           // M5-A:L4 Tool Call Trace
	)
	toolCallHandler := tool.NewToolCallHandler(toolSvc, policyRepo, toolRepo)
	// M2-C.8: executing 超时 → indeterminate 扫描器(Verify/Reconcile 对账入口)
	toolTimeoutScanner := tool.NewToolCallTimeoutScanner(toolCallRepo, auditRepo, cfg.ToolCallTimeoutScanEvery)
	toolTimeoutScanner.SetTraceSink(traceRecorder) // M5-A:L4 timed_out
	go toolTimeoutScanner.Start(context.Background())
	// M2-C:审批 UI 决策 → Tool Call 生命周期绑定(approved→重检→executing / rejected→cancelled)
	agentHandler.SetToolCallDecisionBinder(toolSvc)

	// M3-B: Webhook Inbox(去重落库 + 签名认证接收 + 周期消费)
	webhookRepo := tool.NewWebhookEventRepository(pool)
	webhookConsumer := tool.NewWebhookConsumer(webhookRepo, cfg.ToolWebhookScanEvery, 50)
	webhookConsumer.RegisterProcessor("ticket_connector", tool.NewTicketWebhookProcessor(ticketConnector))
	go webhookConsumer.Start(context.Background())

	// M3-C: Connector Outbox Dispatcher(Saga 投递/退避重试/stale Verify 收敛/补偿)
	outboxDispatcher := tool.NewOutboxDispatcher(outboxRepo, connectorRuntime, toolCallRepo, toolSvc,
		cfg.ToolOutboxScanEvery, 50, cfg.ToolOutboxMaxAttempts,
		cfg.ToolOutboxBackoffBase, cfg.ToolOutboxConfirmWait)
	go outboxDispatcher.Start(context.Background())
	outboxHandler := tool.NewOutboxHandler(outboxRepo)

	// ── M6: 工作台查询端点(protected,JWT 租户隔离) ──
	// M6-A: Run 查询(Run 时间线页数据源)
	runQueryHandler := agent.NewRunQueryHandler(agentRepo)
	// M6-B: 可靠性运维(ToolCall 探索器 / DLQ / Outbox 监控)
	toolOpsHandler := tool.NewOpsHandler(toolCallRepo)
	outboxOpsHandler := tool.NewOutboxOpsHandler(outboxRepo)
	// M6-C: 连接器授权范围可视化(注册表 + Binding)
	connectorOpsHandler := tool.NewConnectorOpsHandler(connectorRegistry, credentialSvc)

	businessRepo := business.NewRepository(pool)
	businessHandler := business.NewHandler(businessRepo)
	policyHandler := policy.NewHandler(policyRepo)
	governanceRepo := governance.NewRepository(pool)
	governanceHandler := governance.NewHandler(governanceRepo, auditRepo)
	observabilityRepo := observability.NewRepository(pool)
	observabilityHandler := observability.NewHandler(observabilityRepo)

	// M4-A: Memory 分层记忆(仓储/服务/端点,供 Runtime 与 Context Builder 消费)
	memoryRepo := memory.NewRepository(pool)
	memorySvc := memory.NewService(memoryRepo)
	memoryHandler := memory.NewHandler(memorySvc)
	// M4-B: Skill 生命周期(draft -> review -> published -> deprecated)
	skillRepo := skill.NewRepository(pool)
	skillSvc := skill.NewService(skillRepo)
	skillHandler := skill.NewHandler(skillSvc)
	// M8-B: Skill Marketplace(市场 + 安装/卸载 + 使用追踪)
	skillMarketplaceRepo := skill.NewMarketplaceRepository(pool)
	skillMarketplaceSvc := skill.NewMarketplaceService(skillRepo, skillMarketplaceRepo)
	skillMarketplaceHandler := skill.NewMarketplaceHandler(skillMarketplaceSvc, auditRepo)
	// M4-C: Context Builder(聚合 Memory + ACL 过滤 + Token 预算裁剪)
	contextBuilder := contextbuilder.NewBuilder(memorySvc)
	contextHandler := contextbuilder.NewHandler(contextBuilder)

	fileRepo := platformfile.NewRepository(pool)
	fileHandler := platformfile.NewHandler(fileRepo, auditRepo, cfg.MinIOBucket, cfg.FileStorageDir)

	// ── M7-A: Conversation Engine(对话引擎:SSE + 多轮会话 + 澄清追问) ──
	conversationRepo := conversation.NewRepository(pool)
	conversationSSEWriter := conversation.NewSSEWriter()
	conversationSvc := conversation.NewService(conversationRepo, conversationSSEWriter)
	conversationHandler := conversation.NewHandler(conversationSvc, auditRepo, conversationSSEWriter)

	// ── M7-B: Agent Gallery(Agent 画廊 — 发现与选择层) ──
	galleryRepo := agent_gallery.NewRepository(pool)
	gallerySvc := agent_gallery.NewService(galleryRepo)
	galleryHandler := agent_gallery.NewHandler(gallerySvc, auditRepo)

	// ── M8-A: Agent Package Dynamic Loading(版本管理 + 安装/卸载 + 注册协议) ──
	packageHandler := agent_gallery.NewPackageHandler(gallerySvc, auditRepo)

	// ── 第 9 步：关键连线 — Gateway 注入 Workflow Worker ──
	// agent_graph 节点执行时，Worker 通过 Gateway 调用 Python Agent Service
	workflowWorker.SetGateway(agentGateway, agentRepo)

	// ── M8-C: Connector Protocol Sidecar(sidecar 注册 + HTTP Connector) ──
	sidecarRepo := tool.NewSidecarRepository(pool)
	sidecarService := tool.NewSidecarService(sidecarRepo, connectorRuntime)
	sidecarHandler := tool.NewSidecarHandler(sidecarService, auditRepo)
	_ = sidecarService.LoadExistingSidecars(context.Background(), "")

	// ── M8-D: Knowledge Base v1 — 文档上传→pgvector→检索→引用溯源 ──
	knowledgeRepo := knowledge.NewRepository(pool)
	knowledgeSvc := knowledge.NewService(knowledgeRepo, cfg.AgentServiceURL, cfg.FileStorageDir)
	knowledgeHandler := knowledge.NewHandler(knowledgeSvc, auditRepo)

	// ── 第 9.5 步：启动 Durable Run 失联收敛与终态补偿扫描(M1-C)──
	convergenceScanner := workflow.NewRunConvergenceScanner(
		agentRepo, workflowRepo, workflowWorker, workflowSvc, workflowRepo, cfg.AgentRunStaleAfter,
	)
	go convergenceScanner.Start(context.Background())

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

	router.Use(middleware.NewRateLimiter(rate.Limit(100), 200))
	router.Use(platform.TraceMiddleware(), gin.Logger(), gin.Recovery())
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
	internalV2 := router.Group("/internal/v2")
	internalV2.Use(agent.RequireInternalServiceToken(cfg.InternalServiceToken))
	internalV2.POST("/runtime-events", runtimeHandler.ConsumeEvent)

	// M2-B/M2-C: Tool Execution API(受信任执行边界,仅服务身份可访问)
	internalTool := router.Group("/internal/v1/tool-calls")
	internalTool.Use(agent.RequireInternalServiceToken(cfg.InternalServiceToken))
	{
		internalTool.POST("", toolCallHandler.CreateToolCall)
		internalTool.GET("/:tool_call_id", toolCallHandler.GetToolCall)
		internalTool.GET("/dead-letters", toolCallHandler.ListDeadLetterToolCalls)
		internalTool.GET("/by-trace/:trace_id", toolCallHandler.ListByTrace)
		internalTool.POST("/:tool_call_id/request-approval", toolCallHandler.RequestApprovalToolCall)
		internalTool.POST("/:tool_call_id/confirm", toolCallHandler.ConfirmToolCall)
		internalTool.POST("/:tool_call_id/verify", toolCallHandler.VerifyToolCall)
		internalTool.POST("/:tool_call_id/reconcile", toolCallHandler.ReconcileToolCall)
		internalTool.POST("/:tool_call_id/retry", toolCallHandler.RetryToolCall)
	}

	// M2-D: Connector Binding + 凭证解析(internal-only,执行器调用)
	connectorHandler := tool.NewConnectorCredentialHandler(credentialSvc)
	internalConnector := router.Group("/internal/v1/connector-bindings")
	internalConnector.Use(agent.RequireInternalServiceToken(cfg.InternalServiceToken))
	{
		internalConnector.POST("", connectorHandler.CreateBinding)
		internalConnector.GET("", connectorHandler.ListBindings)
		internalConnector.GET("/:id", connectorHandler.GetBinding)
		internalConnector.POST("/:id/resolve-credential", connectorHandler.ResolveCredential)
	}

	// M3-A: Connector Registry + Runtime 健康(治理视图,只读)
	internalConnectorRegistry := router.Group("/internal/v1/connectors")
	internalConnectorRegistry.Use(agent.RequireInternalServiceToken(cfg.InternalServiceToken))
	{
		internalConnectorRegistry.GET("", connectorRuntimeHandler.ListRegistry)
		internalConnectorRegistry.GET("/:code/health", connectorRuntimeHandler.HealthCheck)
	}

	// M3-C: Outbox 治理(查询/人工补偿,内部服务)
	internalOutbox := router.Group("/internal/v1/outbox")
	internalOutbox.Use(agent.RequireInternalServiceToken(cfg.InternalServiceToken))
	{
		internalOutbox.GET("", outboxHandler.List)
		internalOutbox.GET("/:id", outboxHandler.Get)
		internalOutbox.POST("/:id/compensate", outboxHandler.Compensate)
	}

	// M4-A: Memory 分层记忆(internal,Runtime 服务身份读写;X-Tenant-ID 租户校验)
	internalMemory := router.Group("/internal/v1/memory")
	internalMemory.Use(agent.RequireInternalServiceToken(cfg.InternalServiceToken))
	{
		internalMemory.POST("", memoryHandler.Write)
		internalMemory.GET("", memoryHandler.Query)
		internalMemory.DELETE("/:id", memoryHandler.Delete)
	}

	// M4-C: Context Builder(供 Python Agent Service 组装上下文)
	internalContext := router.Group("/internal/v1/context")
	internalContext.Use(agent.RequireInternalServiceToken(cfg.InternalServiceToken))
	{
		internalContext.POST("/build", contextHandler.Build)
	}

	// M5-A: Trace 事件批量追加(Python L3 上报 + Go 模块直投的统一入口)
	traceHandler := trace.NewHandler(traceRepo, traceRecorder)
	internalTrace := router.Group("/internal/v1/trace")
	internalTrace.Use(agent.RequireInternalServiceToken(cfg.InternalServiceToken))
	{
		internalTrace.POST("/events", traceHandler.Append)
	}

	// M3-B: Webhook Inbox 接收端点。外部系统推送,不走 InternalServiceToken,
	// 认证靠 HMAC-SHA256 签名(TOOL_WEBHOOK_SECRET);签名失败的事件落库但不处理。
	webhookHandler := tool.NewWebhookHandler(webhookRepo, map[string]string{
		"ticket_connector": cfg.ToolWebhookSecret,
	})
	router.POST("/webhooks/:code", webhookHandler.Ingest)

	v1 := router.Group("/api/v1")

	// ── 公开 API ──
	{
		v1.POST("/auth/login", authHandler.Login)
	}

	// ── 受保护 API ──
	protected := v1.Group("")
	protected.Use(authMiddleware)
	{
		require := func(permission string) gin.HandlerFunc {
			return auth.RequirePermissionWithAudit(authSvc, permission, auditRepo)
		}
		// Auth
		protected.GET("/auth/me", authHandler.Me)
		protected.GET("/business-apps", require("business_app:read"), authHandler.GetBusinessApps)
		protected.GET("/business-apps/registry", require("business_app:read"), businessHandler.ListApps)
		protected.GET("/domain-policies", require("business_app:read"), policyHandler.ListDomainPolicies)
		protected.GET("/configuration-versions", require("configuration:manage"), governanceHandler.List)
		protected.POST("/configuration-versions", require("configuration:manage"), governanceHandler.Create)
		protected.POST("/configuration-versions/:id/submit", require("configuration:manage"), governanceHandler.Submit)
		protected.POST("/configuration-versions/:id/approve", require("configuration:approve"), governanceHandler.Approve)
		protected.POST("/configuration-versions/:id/deprecate", require("configuration:manage"), governanceHandler.Deprecate)
		protected.GET("/platform-observability/summary", require("observability:read"), observabilityHandler.Summary)
		protected.GET("/observability/avr", require("agent:read"), observabilityHandler.GetAVR)
		protected.GET("/rbac/permission-matrix", require("role:manage"), authHandler.ListPermissionMatrix)
		protected.GET("/rbac/user-roles", require("user:manage"), authHandler.ListUserRoles)

		// Workflow
		protected.GET("/workflow-templates", require("workflow_template:read"), workflowHandler.ListTemplates)
		protected.GET("/business-apps/:code/workflow-templates", require("workflow_template:read"), workflowHandler.GetTemplates)
		protected.POST("/workflow-instances", require("workflow:create"), workflowHandler.CreateInstance)
		protected.GET("/workflow-instances", require("workflow:read"), workflowHandler.ListInstances)
		protected.GET("/workflow-instances/:id", require("workflow:read"), workflowHandler.GetInstance)
		protected.POST("/workflow-instances/:id/start", require("workflow:start"), workflowHandler.StartInstance)
		protected.POST("/workflow-instances/:id/cancel", require("workflow:cancel"), workflowHandler.CancelInstance)
		protected.POST("/workflow-instances/:id/retry", require("workflow:retry"), workflowHandler.RetryNode)
		protected.GET("/workflow-instances/:id/nodes", require("workflow:read"), workflowHandler.GetNodes)

		// Agent Registry
		protected.GET("/agents", require("agent:manage"), agentHandler.ListAgents)
		protected.POST("/agents", require("agent:manage"), agentHandler.CreateAgent)

		// Agent Run Logs
		protected.GET("/agent-run-logs", require("workflow:read"), agentHandler.ListRunLogs)

		// Tool Registry
		protected.GET("/tools", require("tool:manage"), toolHandler.ListTools)

		// M4-B: Skill Registry 生命周期管理
		protected.GET("/skills", require("skill:manage"), skillHandler.List)
		protected.POST("/skills", require("skill:manage"), skillHandler.Create)
		protected.GET("/skills/:id", require("skill:manage"), skillHandler.Get)
		protected.PUT("/skills/:id/config", require("skill:manage"), skillHandler.UpdateConfig)
		protected.POST("/skills/:id/submit", require("skill:manage"), skillHandler.Submit)
		protected.POST("/skills/:id/publish", require("skill:manage"), skillHandler.Publish)
		protected.POST("/skills/:id/deprecate", require("skill:manage"), skillHandler.Deprecate)

		// M8-B: Skill Marketplace(市场 + 安装/卸载 + 使用追踪)
		protected.GET("/skill-marketplace", require("skill:read"), skillMarketplaceHandler.ListMarketplace)
		protected.POST("/skill-marketplace/install", require("skill:read"), skillMarketplaceHandler.InstallSkill)
		protected.POST("/skill-marketplace/:code/uninstall", require("skill:read"), skillMarketplaceHandler.UninstallSkill)
		protected.PATCH("/skill-marketplace/:code", require("skill:read"), skillMarketplaceHandler.UpdateInstallation)
		protected.GET("/skill-marketplace/installed", require("skill:read"), skillMarketplaceHandler.ListInstalled)
		protected.PUT("/skills/:id/metadata", require("skill:manage"), skillMarketplaceHandler.UpdateMetadata)
		protected.POST("/skills/:code/versions/:version/publish", require("skill:manage"), skillMarketplaceHandler.PublishNewVersion)

		// M8-C: Connector Protocol Sidecar(sidecar 注册 + HTTP Connector)
		protected.POST("/sidecars", require("tool:manage"), sidecarHandler.Register)
		protected.GET("/sidecars", require("tool:manage"), sidecarHandler.List)
		protected.GET("/sidecars/:id", require("tool:manage"), sidecarHandler.Get)
		protected.DELETE("/sidecars/:id", require("tool:manage"), sidecarHandler.Deregister)
		protected.POST("/sidecars/:id/health", require("tool:manage"), sidecarHandler.HealthCheck)
		protected.POST("/sidecars/:id/validate", require("tool:manage"), sidecarHandler.Validate)

		// M8-D: Knowledge Base v1 — 文档上传→pgvector→检索
		protected.POST("/knowledge/collections", require("tool:manage"), knowledgeHandler.CreateCollection)
		protected.GET("/knowledge/collections", require("tool:read"), knowledgeHandler.ListCollections)
		protected.GET("/knowledge/collections/:id", require("tool:read"), knowledgeHandler.GetCollection)
		protected.DELETE("/knowledge/collections/:id", require("tool:manage"), knowledgeHandler.DeleteCollection)
		protected.POST("/knowledge/collections/:collection_id/documents", require("tool:manage"), knowledgeHandler.UploadDocument)
		protected.GET("/knowledge/collections/:collection_id/documents", require("tool:read"), knowledgeHandler.ListDocuments)
		protected.DELETE("/knowledge/documents/:id", require("tool:manage"), knowledgeHandler.DeleteDocument)
		protected.POST("/knowledge/collections/:collection_id/search", require("tool:read"), knowledgeHandler.Search)

		// Files
		protected.POST("/files", require("file:upload"), fileHandler.Upload)
		protected.GET("/files/:id", require("file:read"), fileHandler.Get)
		protected.GET("/files/:id/content", require("file:read"), fileHandler.Download)

		// Approval Tasks
		protected.GET("/approval-tasks", require("approval:read"), agentHandler.ListApprovalTasks)
		protected.GET("/approval-tasks/:id", require("approval:read"), agentHandler.GetApprovalTask)
		protected.POST("/approval-tasks/:id/approve", require("approval:decide"), agentHandler.ApproveTask)
		protected.POST("/approval-tasks/:id/reject", require("approval:decide"), agentHandler.RejectTask)

		// Audit Logs
		protected.GET("/audit-logs", require("audit:read"), auditHandler.ListAuditLogs)
		protected.GET("/audit-logs/stats", require("audit:read"), auditHandler.Stats)
		protected.GET("/audit-logs/export", require("audit:read"), auditHandler.ExportCSV)

		// M5-A: Trace 查询(六层事件时间线)
		protected.GET("/traces/:trace_id", require("trace:read"), traceHandler.GetTrace)

		// M5-B: Eval 评估报告(成本/效率/质量/稳定性指标 + 环比)
		protected.POST("/eval/reports", require("eval:read"), evalHandler.GenerateReport)

		// M5-C: Replay / Shadow / Canary 实验管理(权限统一 experiment:manage; Spec §4.5)
		protected.POST("/replays", require("experiment:manage"), experimentHandler.CreateReplay)
		protected.GET("/replays/:id", require("experiment:manage"), experimentHandler.GetReplay)
		protected.POST("/shadow-rules", require("experiment:manage"), experimentHandler.CreateShadowRule)
		protected.GET("/shadow-rules", require("experiment:manage"), experimentHandler.ListShadowRules)
		protected.POST("/shadow-rules/:id/stop", require("experiment:manage"), experimentHandler.StopShadowRule)
		protected.GET("/shadow-executions", require("experiment:manage"), experimentHandler.ListShadowExecutions)
		protected.POST("/canary-releases", require("experiment:manage"), experimentHandler.CreateCanaryRelease)
		protected.GET("/canary-releases", require("experiment:manage"), experimentHandler.ListCanaryReleases)
		protected.GET("/canary-releases/:id", require("experiment:manage"), experimentHandler.GetCanaryRelease)
		protected.POST("/canary-releases/:id/advance", require("experiment:manage"), experimentHandler.AdvanceCanary)
		protected.POST("/canary-releases/:id/promote", require("experiment:manage"), experimentHandler.PromoteCanary)
		protected.POST("/canary-releases/:id/rollback", require("experiment:manage"), experimentHandler.RollbackCanary)
		protected.POST("/canary-releases/:id/check", require("experiment:manage"), experimentHandler.CheckCanary)

		// M6-A: Run 查询(Run 时间线详情页数据源; Spec WORKBENCH_DESIGN §6.1)
		protected.GET("/runs", require("workflow:read"), runQueryHandler.ListRuns)
		protected.GET("/runs/:id", require("workflow:read"), runQueryHandler.GetRun)

		// M6-B: 可靠性运维(ToolCall 探索器 / DLQ / Outbox; Spec §6.2)
		protected.GET("/ops/tool-calls", require("tool:read"), toolOpsHandler.ListToolCalls)
		protected.GET("/ops/tool-calls/dead-letters", require("tool:read"), toolOpsHandler.ListDeadLetters)
		protected.GET("/ops/tool-calls/:id", require("tool:read"), toolOpsHandler.GetToolCall)
		protected.GET("/ops/outbox", require("outbox:read"), outboxOpsHandler.ListOutbox)
		protected.GET("/ops/outbox/:id", require("outbox:read"), outboxOpsHandler.GetOutbox)
		protected.POST("/ops/outbox/:id/compensate", require("outbox:read"), outboxOpsHandler.Compensate)

		// M6-C: 连接器授权范围可视化(Spec §6.3)
		protected.GET("/connector-registry", require("tool:manage"), connectorOpsHandler.ListRegistry)
		protected.GET("/connector-bindings", require("tool:manage"), connectorOpsHandler.ListBindings)

		// M7-A: Conversation Engine(对话引擎:会话管理 + SSE 流式 + 澄清追问)
		protected.POST("/conversations", require("conversation:write"), conversationHandler.CreateConversation)
		protected.GET("/conversations", require("conversation:read"), conversationHandler.ListConversations)
		protected.GET("/conversations/:id", require("conversation:read"), conversationHandler.GetConversation)
		protected.PATCH("/conversations/:id", require("conversation:write"), conversationHandler.UpdateConversation)
		protected.POST("/conversations/:id/messages", require("conversation:write"), conversationHandler.SendMessage)
		protected.POST("/conversations/:id/answers", require("conversation:write"), conversationHandler.AnswerClarification)
		protected.POST("/conversations/:id/cancel", require("conversation:write"), conversationHandler.CancelRun)
		protected.GET("/conversations/:id/stream", require("conversation:read"), conversationHandler.Stream)

		// M7-B: Agent Gallery(Agent 画廊 — 发现与选择层)
		protected.GET("/agent-gallery", require("business_app:read"), galleryHandler.ListGallery)
		protected.GET("/agent-gallery/:code", require("business_app:read"), galleryHandler.GetPackage)
		protected.POST("/agent-packages", require("agent:manage"), galleryHandler.CreatePackage)
		protected.PATCH("/agent-packages/:code", require("agent:manage"), galleryHandler.UpdatePackage)

		// M8-A: Agent Package Version Management
		protected.POST("/agent-packages/:code/versions", require("agent:manage"), packageHandler.CreateVersion)
		protected.GET("/agent-packages/:code/versions", require("agent:manage"), packageHandler.ListVersions)
		protected.POST("/agent-packages/:code/versions/:version/publish", require("agent:manage"), packageHandler.PublishVersion)

		// M8-A: Agent Package Installation Management
		protected.GET("/agent-package-installations", require("agent:manage"), packageHandler.ListInstalled)
		protected.POST("/agent-package-installations", require("agent:manage"), packageHandler.InstallPackage)
		protected.POST("/agent-package-installations/:code/uninstall", require("agent:manage"), packageHandler.UninstallPackage)
		protected.PATCH("/agent-package-installations/:code", require("agent:manage"), packageHandler.UpdateInstallationStatus)

		// M8-A: Agent Package Registration Protocol
		protected.POST("/agent-package-registrations", require("agent:manage"), packageHandler.RegisterPackage)
		protected.GET("/agent-package-registrations", require("agent:manage"), packageHandler.ListRegistrations)
		protected.GET("/agent-package-registrations/:code", require("agent:manage"), packageHandler.GetRegistration)
		protected.POST("/agent-package-registrations/:code/verify", require("agent:manage"), packageHandler.VerifyRegistration)
		protected.POST("/agent-package-registrations/:code/reject", require("agent:manage"), packageHandler.RejectRegistration)
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
	traceRecorder.Close() // M5-A:排空缓冲中的 trace 事件
	log.Println("server stopped")
}
