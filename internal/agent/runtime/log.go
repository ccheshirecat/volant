package runtime

import (
	"sync"
	"time"
)

// LogEvent represents a single log line emitted by the agent or underlying
// browser process.
type LogEvent struct {
	Stream    string    `json:"stream"`
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
}

// logEmitter fan-outs log events to subscribers.
type logEmitter struct {
	mu     sync.RWMutex
	subs   map[int64]chan LogEvent
	next   int64
	closed bool
}

// newLogEmitter initializes a log emitter.
func newLogEmitter() *logEmitter {
	return &logEmitter{
		subs: make(map[int64]chan LogEvent),
	}
}

// Publish sends the event to all active subscribers. Publishing on a closed
// emitter is a no-op.
func (e *logEmitter) Publish(event LogEvent) {
	e.mu.RLock()
	closed := e.closed
	e.mu.RUnlock()
	if closed {
		return
	}

	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, ch := range e.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

// Subscribe registers a new subscriber channel and returns an unsubscribe
// callback that must be invoked to release resources.
func (e *logEmitter) Subscribe(buffer int) (<-chan LogEvent, func()) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		ch := make(chan LogEvent)
		close(ch)
		return ch, func() {}
	}

	if buffer <= 0 {
		buffer = 32
	}

	id := e.next
	e.next++

	ch := make(chan LogEvent, buffer)
	e.subs[id] = ch

	unsubscribe := func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if sub, ok := e.subs[id]; ok {
			delete(e.subs, id)
			close(sub)
		}
	}

	return ch, unsubscribe
}

// Close closes the emitter and all subscriber channels.
func (e *logEmitter) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	e.closed = true
	for id, ch := range e.subs {
		close(ch)
		delete(e.subs, id)
	}
}
