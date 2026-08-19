package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const maxEventBufferSize = 1000

type eventBuffer struct {
	mu      sync.RWMutex
	events  []SSEEvent
	subs    map[chan SSEEvent]struct{}
	nextSeq int
}

type SSEWriter struct {
	mu      sync.RWMutex
	buffers map[string]*eventBuffer
}

func NewSSEWriter() *SSEWriter {
	return &SSEWriter{
		buffers: make(map[string]*eventBuffer),
	}
}

func (w *SSEWriter) getBuffer(conversationID string) *eventBuffer {
	w.mu.RLock()
	buf, ok := w.buffers[conversationID]
	w.mu.RUnlock()
	if ok {
		return buf
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if buf, ok = w.buffers[conversationID]; ok {
		return buf
	}
	buf = &eventBuffer{
		events: make([]SSEEvent, 0, maxEventBufferSize),
		subs:   make(map[chan SSEEvent]struct{}),
	}
	w.buffers[conversationID] = buf
	return buf
}

func (w *SSEWriter) PushEvent(conversationID string, event SSEEvent) {
	buf := w.getBuffer(conversationID)

	buf.mu.Lock()
	buf.nextSeq++
	event.Seq = buf.nextSeq
	event.ID = fmt.Sprintf("%d", buf.nextSeq)
	buf.events = append(buf.events, event)
	if len(buf.events) > maxEventBufferSize {
		buf.events = buf.events[len(buf.events)-maxEventBufferSize:]
	}
	subs := make([]chan SSEEvent, 0, len(buf.subs))
	for ch := range buf.subs {
		subs = append(subs, ch)
	}
	buf.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (w *SSEWriter) GetEventsSince(conversationID string, sinceSeq int) []SSEEvent {
	buf := w.getBuffer(conversationID)

	buf.mu.RLock()
	defer buf.mu.RUnlock()

	var result []SSEEvent
	for _, e := range buf.events {
		if e.Seq > sinceSeq {
			result = append(result, e)
		}
	}
	return result
}

func (w *SSEWriter) Subscribe(conversationID string) (<-chan SSEEvent, func()) {
	buf := w.getBuffer(conversationID)

	ch := make(chan SSEEvent, 64)

	buf.mu.Lock()
	buf.subs[ch] = struct{}{}
	buf.mu.Unlock()

	unsubscribe := func() {
		buf.mu.Lock()
		delete(buf.subs, ch)
		buf.mu.Unlock()
		close(ch)
	}

	return ch, unsubscribe
}

func (w *SSEWriter) Stream(ctx context.Context, writer http.ResponseWriter, flusher http.Flusher, conversationID string, lastEventID int) {
	if lastEventID > 0 {
		events := w.GetEventsSince(conversationID, lastEventID)
		for _, e := range events {
			WriteEvent(writer, flusher, e)
		}
		flusher.Flush()
	}

	ch, unsubscribe := w.Subscribe(conversationID)
	defer unsubscribe()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			WriteEvent(writer, flusher, event)
		case <-heartbeat.C:
			WriteHeartbeat(writer, flusher)
		}
	}
}

func WriteEvent(w http.ResponseWriter, flusher http.Flusher, event SSEEvent) {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "id: %s\n", event.ID)
	fmt.Fprintf(w, "event: %s\n", event.Event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func WriteHeartbeat(w http.ResponseWriter, flusher http.Flusher) {
	fmt.Fprintf(w, ":ping\n\n")
	flusher.Flush()
}