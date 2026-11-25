package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/session"
)

func waitFor(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-deadline.C:
			t.Fatal("worker did not complete")
		case <-ticker.C:
		}
	}
}

func TestLatestCoalescesAndSerializes(t *testing.T) {
	started := make(chan int, 3)
	release := make(chan struct{})
	w := NewLatest(func(ctx context.Context, n int) (int, error) {
		started <- n
		if n == 1 {
			select {
			case <-release:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
		return n, nil
	})
	defer w.Close()
	first := w.Submit(1)
	select {
	case n := <-started:
		if n != 1 {
			t.Fatal(n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first write did not start")
	}
	w.Submit(2)
	last := w.Submit(3)
	if w.Result().Sequence != 0 || last <= first {
		t.Fatal("submission reported as durable")
	}
	close(release)
	waitFor(t, func() bool { return w.Result().Sequence == last })
	if r := w.Result(); r.Value != 3 || r.Err != nil {
		t.Fatalf("latest completion: %+v", r)
	}
	if n := <-started; n != 3 {
		t.Fatalf("superseded value executed: %d", n)
	}
	w.Close()
	waitFor(t, func() bool {
		select {
		case <-w.Done():
			return true
		default:
			return false
		}
	})
	if w.Submit(4) != 0 {
		t.Fatal("closed worker accepted submission")
	}
}

func TestLatestFailureAndRetry(t *testing.T) {
	failed := errors.New("disk full")
	w := NewLatest(func(_ context.Context, n int) (int, error) {
		if n == 1 {
			return 0, failed
		}
		return n, nil
	})
	defer w.Close()
	id := w.Submit(1)
	waitFor(t, func() bool { return w.Result().Sequence == id })
	if !errors.Is(w.Result().Err, failed) {
		t.Fatal("write failure lost")
	}
	id = w.Submit(2)
	waitFor(t, func() bool { return w.Result().Sequence == id })
	if w.Result().Err != nil || w.Result().Value != 2 {
		t.Fatal("retry failed")
	}
}

func TestJobsRejectStaleAndCanceledResults(t *testing.T) {
	var j Jobs[int]
	gate := make(chan struct{})
	started := make(chan struct{})
	exited := make(chan struct{})
	if err := j.Start(1, func(ctx context.Context) int {
		close(started)
		<-gate
		close(exited)
		return 10
	}, nil); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := j.Start(1, func(context.Context) int { return 0 }, nil); !errors.Is(err, ErrBusy) {
		t.Fatal("overlapping job accepted")
	}
	j.Cancel()
	notify := make(chan struct{}, 1)
	if err := j.Start(2, func(context.Context) int { return 20 }, func() { notify <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	close(gate)
	<-exited
	select {
	case <-notify:
	case <-time.After(2 * time.Second):
		t.Fatal("completion missing")
	}
	if v, ready, current := j.Take(3); v != 20 || !ready || current {
		t.Fatal("wrong epoch accepted")
	}
	if _, ready, _ := j.Take(3); ready {
		t.Fatal("completion replayed")
	}
	if err := j.Start(3, func(context.Context) int { return 30 }, func() { notify <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-notify:
	case <-time.After(2 * time.Second):
		t.Fatal("completion missing")
	}
	if v, ready, current := j.Take(3); v != 30 || !ready || !current {
		t.Fatal("current result rejected")
	}
}

func TestSourceScanAndWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := path + ".missing"
	r, err := ScanSources(context.Background(), SourceScan{Epoch: 7, Paths: []string{path, missing, path}})
	if err != nil || r.Epoch != 7 || len(r.Checked) != 2 || len(r.Present) != 1 || !r.Checked[missing] {
		t.Fatalf("scan: %+v %v", r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ScanSources(ctx, SourceScan{Paths: []string{path}}); !errors.Is(err, context.Canceled) {
		t.Fatal("scan ignored cancellation")
	}
	if err := WaitAll(ctx, []<-chan struct{}{make(chan struct{})}); !errors.Is(err, context.Canceled) {
		t.Fatal("worker wait ignored cancellation")
	}
	done := make(chan struct{})
	close(done)
	if err := WaitAll(context.Background(), []<-chan struct{}{done}); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryAcknowledgementPreservesDurableSlot(t *testing.T) {
	dir := t.TempDir()
	store, _, err := session.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := NewLatest(func(_ context.Context, s session.State) (struct{}, error) {
		return struct{}{}, store.Save(s)
	})
	defer w.Close()
	state := session.State{Project: project.New(), Asset: -1, Clip: -1, Playhead: 10}
	id := w.Submit(state)
	waitFor(t, func() bool { return w.Result().Sequence == id })
	if w.Result().Err != nil {
		t.Fatal(w.Result().Err)
	}
	invalid := state
	invalid.Asset = 1
	id = w.Submit(invalid)
	waitFor(t, func() bool { return w.Result().Sequence == id })
	if w.Result().Err == nil {
		t.Fatal("invalid recovery acknowledged as saved")
	}
	_, restored, err := session.Open(dir)
	if err != nil || restored == nil || restored.Playhead != 10 {
		t.Fatalf("previous durable state lost: %+v %v", restored, err)
	}
	state.Playhead = 20
	id = w.Submit(state)
	waitFor(t, func() bool { return w.Result().Sequence == id })
	if w.Result().Err != nil {
		t.Fatal(w.Result().Err)
	}
	w.Close()
	waitFor(t, func() bool {
		select {
		case <-w.Done():
			return true
		default:
			return false
		}
	})
	_, restored, err = session.Open(dir)
	if err != nil || restored == nil || restored.Playhead != 20 {
		t.Fatalf("latest durable state lost: %+v %v", restored, err)
	}
}
