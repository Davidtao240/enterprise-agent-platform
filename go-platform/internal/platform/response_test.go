package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTraceMiddlewareUsesIncomingTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceMiddleware())
	router.GET("/ok", func(c *gin.Context) {
		Success(c, gin.H{"trace_id": c.GetString("trace_id")})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set(TraceIDHeader, "trace-from-client")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get(TraceIDHeader); got != "trace-from-client" {
		t.Fatalf("expected response trace header %q, got %q", "trace-from-client", got)
	}
	if !strings.Contains(rec.Body.String(), `"trace_id":"trace-from-client"`) {
		t.Fatalf("expected response body to include incoming trace id, got %s", rec.Body.String())
	}
}

func TestTraceMiddlewareGeneratesMissingTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceMiddleware())
	router.GET("/ok", func(c *gin.Context) {
		Success(c, gin.H{"request_trace_id": c.GetHeader(TraceIDHeader)})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	traceID := rec.Header().Get(TraceIDHeader)
	if traceID == "" {
		t.Fatal("expected generated response trace header")
	}
	if !strings.Contains(rec.Body.String(), `"trace_id":"`+traceID+`"`) {
		t.Fatalf("expected response body trace_id %q, got %s", traceID, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"request_trace_id":"`+traceID+`"`) {
		t.Fatalf("expected generated trace id to be available through request header, got %s", rec.Body.String())
	}
}
