package runtime

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type Runtime struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewRuntime() *Runtime {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runtime{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (r *Runtime) Context() context.Context {
	return r.ctx
}

func (r *Runtime) Stop() {
	r.cancel()
	r.wg.Wait()
}

func (r *Runtime) Go(fn func(ctx context.Context)) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		fn(r.ctx)
	}()
}

func (r *Runtime) SetupSignalHandlers() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		r.Stop()
	}()
}
