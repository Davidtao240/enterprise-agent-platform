package trace

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestEventValidate(t *testing.T) {
	cases := []struct {
		name    string
		event   Event
		wantErr bool
	}{
		{"valid L2", Event{TraceID: "t1", Layer: LayerRun, EventType: "start", TenantID: "tenant-1"}, false},
		{"missing trace_id", Event{Layer: LayerRun, EventType: "start", TenantID: "tenant-1"}, true},
		{"invalid layer", Event{TraceID: "t1", Layer: "L9", EventType: "start", TenantID: "tenant-1"}, true},
		{"missing event_type", Event{TraceID: "t1", Layer: LayerRun, TenantID: "tenant-1"}, true},
		{"missing tenant", Event{TraceID: "t1", Layer: LayerRun, EventType: "start"}, true},
		{"zero timestamp backfilled", Event{TraceID: "t1", Layer: LayerModelTurn, EventType: "end", TenantID: "tenant-1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.event.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && tc.event.Timestamp.IsZero() {
				t.Fatal("expected zero timestamp to be backfilled")
			}
		})
	}
}

type fakeWriter struct {
	mu     sync.Mutex
	events []*Event
}

func (f *fakeWriter) InsertBatch(_ context.Context, events []*Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, events...)
	return nil
}

func (f *fakeWriter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

func TestRecorderFlushesOnClose(t *testing.T) {
	writer := &fakeWriter{}
	rec := NewRecorder(writer, 128, time.Hour) // 长 interval,依赖 Close 排空
	rec.Record(context.Background(),
		&Event{TraceID: "t1", Layer: LayerRun, EventType: "start", TenantID: "tenant-1"},
		&Event{TraceID: "t1", Layer: LayerToolCall, EventType: "end", TenantID: "tenant-1"},
	)
	rec.Close()
	if got := writer.count(); got != 2 {
		t.Fatalf("expected 2 flushed events, got %d", got)
	}
}

// blockingWriter 阻塞在 InsertBatch,使后台 loop 停在 flush 中无法排空通道,
// 从而确定性验证缓冲满时的丢弃行为。
type blockingWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingWriter) InsertBatch(_ context.Context, _ []*Event) error {
	select {
	case <-b.entered:
	default:
		close(b.entered)
	}
	<-b.release
	return nil
}

func TestRecorderDropsInvalidAndOverflow(t *testing.T) {
	writer := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	rec := NewRecorder(writer, 2, 5*time.Millisecond)
	defer func() {
		close(writer.release)
		rec.Close()
	}()

	// 无效事件 (nil / 缺 trace_id) 直接跳过,不进通道。
	rec.Record(context.Background(), nil, &Event{TraceID: "", Layer: "L1", EventType: "start", TenantID: "x"})

	// 先投 1 条,等 loop 取走并进入 flush(阻塞在 InsertBatch)。
	rec.Record(context.Background(), &Event{TraceID: "t0", Layer: LayerWorkflow, EventType: "start", TenantID: "tenant-1"})
	select {
	case <-writer.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("flush did not start in time")
	}

	// loop 已阻塞,通道容量 2:写入 5 条合法事件,后 3 条应被丢弃而非阻塞。
	for i := 0; i < 5; i++ {
		rec.Record(context.Background(), &Event{
			TraceID: "t", Layer: LayerWorkflow, EventType: "start", TenantID: "tenant-1",
		})
	}
	if got := rec.dropped.Load(); got != 3 {
		t.Fatalf("expected exactly 3 dropped, got %d", got)
	}
}

func TestHandlerAppendTenantEnforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := NewRecorder(&fakeWriter{}, 16, time.Hour)
	defer rec.Close()
	h := NewHandler(nil, rec)

	t.Run("missing tenant header", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/trace/events", nil)
		h.Append(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("tenant mismatch rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := `{"events":[{"trace_id":"t1","layer":"L3","event_type":"end","tenant_id":"other-tenant"}]}`
		c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/trace/events", http.NoBody)
		c.Request.Header.Set("X-Tenant-ID", "tenant-1")
		c.Request.Body = http.NoBody
		c.Request.ContentLength = int64(len(body))
		c.Request.Body = io.NopCloser(strings.NewReader(body))
		h.Append(c)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	})

	t.Run("tenant filled from header and accepted", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := `{"events":[{"trace_id":"t1","layer":"L3","event_type":"end"}]}`
		c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/trace/events", http.NoBody)
		c.Request.Header.Set("X-Tenant-ID", "tenant-1")
		c.Request.Body = io.NopCloser(strings.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		h.Append(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
		}
	})
}
