package trace

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// eventWriter flush 目标(由 Repository 实现,便于测试替身)。
type eventWriter interface {
	InsertBatch(ctx context.Context, events []*Event) error
}

// Recorder 实现 Sink:异步批量写入 trace_events。
//
// 风险缓解 (TRACE_AND_EVAL.md §2.3):
//   - Record 永不阻塞:缓冲满时丢弃并计数告警(Trace 是 best-effort 观测面,允许有损)。
//   - 后台 goroutine 定时/凑批 flush,避免高频单条 insert。
type Recorder struct {
	writer   eventWriter
	ch       chan *Event
	interval time.Duration
	done     chan struct{}
	stopped  chan struct{}

	dropped atomic.Uint64
}

// NewRecorder 创建并启动异步 Recorder。
// bufferSize: 缓冲事件数; interval: 最大 flush 间隔。
func NewRecorder(writer eventWriter, bufferSize int, interval time.Duration) *Recorder {
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	r := &Recorder{
		writer:   writer,
		ch:       make(chan *Event, bufferSize),
		interval: interval,
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go r.loop()
	return r
}

// Record 非阻塞投递事件 (实现 Sink)。无效事件直接丢弃。
func (r *Recorder) Record(ctx context.Context, events ...*Event) {
	for _, e := range events {
		if e == nil || e.Validate() != nil {
			continue
		}
		select {
		case r.ch <- e:
		default:
			n := r.dropped.Add(1)
			if n == 1 || n%1000 == 0 {
				log.Printf("[trace] recorder buffer full, dropped=%d events", n)
			}
		}
	}
}

// Close 停止后台 loop 并排空缓冲。
func (r *Recorder) Close() {
	close(r.done)
	<-r.stopped
}

func (r *Recorder) loop() {
	defer close(r.stopped)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	buffer := make([]*Event, 0, 256)
	flush := func() {
		if len(buffer) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := r.writer.InsertBatch(ctx, buffer); err != nil {
			log.Printf("[trace] flush %d events failed: %v", len(buffer), err)
		}
		cancel()
		buffer = buffer[:0]
	}
	for {
		select {
		case e := <-r.ch:
			buffer = append(buffer, e)
			if len(buffer) >= 256 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-r.done:
			for {
				select {
				case e := <-r.ch:
					buffer = append(buffer, e)
				default:
					flush()
					return
				}
			}
		}
	}
}
