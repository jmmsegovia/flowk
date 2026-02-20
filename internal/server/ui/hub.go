package ui

import (
	"strings"
	"sync"

	"flowk/internal/app"
)

type EventHub struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan app.FlowEvent
	history     []app.FlowEvent
	nextID      uint64
}

func NewEventHub() *EventHub {
	return &EventHub{
		subscribers: make(map[uint64]chan app.FlowEvent),
	}
}

func (h *EventHub) Publish(event app.FlowEvent) {
	h.mu.Lock()
	h.history = append(h.history, event)
	subscribers := make([]chan app.FlowEvent, 0, len(h.subscribers))
	for _, ch := range h.subscribers {
		subscribers = append(subscribers, ch)
	}
	h.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			go func(c chan app.FlowEvent, evt app.FlowEvent) {
				c <- evt
			}(ch, event)
		}
	}
}

func (h *EventHub) Subscribe() (<-chan app.FlowEvent, func()) {
	ch := make(chan app.FlowEvent, 32)

	h.mu.Lock()
	id := h.nextID
	h.nextID++
	history := append([]app.FlowEvent(nil), h.history...)
	h.subscribers[id] = ch
	h.mu.Unlock()

	go func(entries []app.FlowEvent) {
		for _, evt := range entries {
			ch <- evt
		}
	}(history)

	cancel := func() {
		h.mu.Lock()
		if existing, ok := h.subscribers[id]; ok {
			delete(h.subscribers, id)
			close(existing)
		}
		h.mu.Unlock()
	}

	return ch, cancel
}

func (h *EventHub) ClearHistory(flowID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.history == nil {
		return
	}

	trimmed := strings.TrimSpace(flowID)
	if trimmed == "" {
		h.history = nil
		return
	}

	filtered := h.history[:0]
	for _, evt := range h.history {
		if evt.FlowID != trimmed {
			filtered = append(filtered, evt)
		}
	}
	h.history = append([]app.FlowEvent(nil), filtered...)
}

func (h *EventHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for id, ch := range h.subscribers {
		delete(h.subscribers, id)
		close(ch)
	}
	h.subscribers = nil
}
