package service

import (
	"context"
	"sync"
)

type Completion[T any] struct {
	Sequence uint64
	Value    T
	Err      error
}

type work[T any] struct {
	sequence uint64
	value    T
}

type Latest[T, R any] struct {
	mu      sync.Mutex
	next    uint64
	pending *work[T]
	result  Completion[R]
	wake    chan struct{}
	done    chan struct{}
	cancel  context.CancelFunc
	closed  bool
}

func NewLatest[T, R any](run func(context.Context, T) (R, error)) *Latest[T, R] {
	ctx, cancel := context.WithCancel(context.Background())
	w := &Latest[T, R]{wake: make(chan struct{}, 1), done: make(chan struct{}), cancel: cancel}
	go w.loop(ctx, run)
	return w
}

func (w *Latest[T, R]) Submit(value T) uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0
	}
	w.next++
	w.pending = &work[T]{sequence: w.next, value: value}
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return w.next
}

func (w *Latest[T, R]) Result() Completion[R] {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.result
}

func (w *Latest[T, R]) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	w.pending = nil
	w.cancel()
}

func (w *Latest[T, R]) Done() <-chan struct{} { return w.done }

func (w *Latest[T, R]) loop(ctx context.Context, run func(context.Context, T) (R, error)) {
	defer close(w.done)
	for {
		w.mu.Lock()
		item := w.pending
		w.pending = nil
		w.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if item == nil {
			select {
			case <-ctx.Done():
				return
			case <-w.wake:
				continue
			}
		}
		value, err := run(ctx, item.value)
		w.mu.Lock()
		w.result = Completion[R]{Sequence: item.sequence, Value: value, Err: err}
		w.mu.Unlock()
	}
}
