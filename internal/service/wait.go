package service

import "context"

func WaitAll(ctx context.Context, workers []<-chan struct{}) error {
	for _, done := range workers {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}
