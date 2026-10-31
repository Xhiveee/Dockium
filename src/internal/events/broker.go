package events

import "sync"

type Broker[T any] struct {
	mu          sync.RWMutex
	subscribers map[int]chan T
	next        int
}

func NewBroker[T any]() *Broker[T] {
	return &Broker[T]{subscribers: make(map[int]chan T)}
}

func (b *Broker[T]) Subscribe(buffer int) (<-chan T, func()) {
	if buffer < 1 {
		buffer = 1
	}
	channel := make(chan T, buffer)
	b.mu.Lock()
	id := b.next
	b.next++
	b.subscribers[id] = channel
	b.mu.Unlock()
	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, id)
			b.mu.Unlock()
			close(channel)
		})
	}
	return channel, unsubscribe
}

func (b *Broker[T]) Publish(value T) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, channel := range b.subscribers {
		select {
		case channel <- value:
		default:
		}
	}
}
