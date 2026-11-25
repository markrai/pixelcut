package service

import (
	"context"
	"errors"
	"sync"
)

var ErrBusy = errors.New("background job already running")

type JobResult[T any] struct {
	ID    uint64
	Epoch uint64
	Value T
}

type Jobs[T any] struct {
	mu     sync.Mutex
	next   uint64
	active uint64
	cancel context.CancelFunc
	result *JobResult[T]
}

func (j *Jobs[T]) Start(epoch uint64, run func(context.Context) T, notify func()) error {
	j.mu.Lock()
	if j.active != 0 {
		j.mu.Unlock()
		return ErrBusy
	}
	j.next++
	id := j.next
	ctx, cancel := context.WithCancel(context.Background())
	j.active, j.cancel = id, cancel
	j.mu.Unlock()
	go func() {
		defer cancel()
		value := run(ctx)
		j.mu.Lock()
		publish := j.active == id && ctx.Err() == nil
		if publish {
			j.result = &JobResult[T]{ID: id, Epoch: epoch, Value: value}
		}
		j.mu.Unlock()
		if publish && notify != nil {
			notify()
		}
	}()
	return nil
}

func (j *Jobs[T]) Take(epoch uint64) (T, bool, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var zero T
	if j.result == nil {
		return zero, false, false
	}
	r := *j.result
	j.result = nil
	j.active, j.cancel = 0, nil
	return r.Value, true, r.Epoch == epoch
}

func (j *Jobs[T]) Cancel() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cancel != nil {
		j.cancel()
	}
	j.active, j.cancel, j.result = 0, nil, nil
}
