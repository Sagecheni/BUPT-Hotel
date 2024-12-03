package events

import (
	"sync"
)

// EventBus 是事件总线的实现
type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
}

// NewEventBus 创建新的事件总线
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[EventType][]Handler),
	}
}

// Publish 发布事件
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if handlers, exists := eb.handlers[event.Type]; exists {
		for _, handler := range handlers {
			go handler(event) // 异步处理事件
		}
	}
}

// Subscribe 订阅事件
func (eb *EventBus) Subscribe(eventType EventType, handler Handler) Subscription {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
	return Subscription{
		EventType: eventType,
		Handler:   handler,
	}
}

// Unsubscribe 取消订阅
func (eb *EventBus) Unsubscribe(sub Subscription) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if handlers, exists := eb.handlers[sub.EventType]; exists {
		for i, h := range handlers {
			if &h == &sub.Handler {
				// 从切片中移除处理器
				eb.handlers[sub.EventType] = append(
					handlers[:i],
					handlers[i+1:]...,
				)
				break
			}
		}
	}
}
