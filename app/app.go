package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

const (
	// 选择不常用的值方便调试
	InterruptExitCode = 13
	TimeoutExitCode   = 18
)

var (
	ErrAtLeastOneServer = errors.New("at least one server is required")
)

type Server interface {
	Name() string
	Addr() string
	Handle(pattern string, handler http.Handler)
	Start() error
	Stop(ctx context.Context) error
}

type ShutdownCallback func(ctx context.Context)

type app struct {
	servers   []Server
	callbacks []ShutdownCallback
	sigs      chan os.Signal
	opts      *options
}

type options struct {
	// 优雅退出整个超时时间，默认30秒
	shutdownTimeout time.Duration
	// 优雅退出时候等待处理已有请求时间，默认10秒钟
	waitTime time.Duration
	// 自定义回调超时时间，默认三秒钟
	callbackTimeout time.Duration
}

type option func(*app)

func WithShutdownCallbacks(s ...ShutdownCallback) option {
	return func(a *app) {
		a.callbacks = s
	}
}

func WithShutdownTimeout(timeout time.Duration) option {
	return func(a *app) {
		a.opts.shutdownTimeout = timeout
	}
}

func WithWaitTimeout(timeout time.Duration) option {
	return func(a *app) {
		a.opts.waitTime = timeout
	}
}

func WithCallbackTimeout(timeout time.Duration) option {
	return func(a *app) {
		a.opts.callbackTimeout = timeout
	}
}

func New(servers []Server, opts ...option) (*app, error) {
	if len(servers) < 2 {
		return nil, ErrAtLeastOneServer
	}
	app := &app{
		servers: servers,
		opts: &options{
			shutdownTimeout: 30 * time.Second,
			waitTime:        10 * time.Second,
			callbackTimeout: 3 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(app)
	}
	return app, nil
}

func (a *app) StartAndServe() {
	for _, s := range a.servers {
		s := s
		// todo: handle error and panic
		go func() {
			if err := s.Start(); err != nil {
				log.Println(err)
			}
		}()
	}
	a.monitorSignals()
	a.shutdown()
}

func (a *app) monitorSignals() {
	a.sigs = make(chan os.Signal, 2)
	switch runtime.GOOS {
	case "windows":
		signal.Notify(a.sigs, os.Interrupt, os.Kill, syscall.SIGKILL, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL, syscall.SIGABRT, syscall.SIGTERM)
	case "linux":
		signal.Notify(a.sigs, os.Interrupt, os.Kill, syscall.SIGKILL, syscall.SIGSTOP, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL, syscall.SIGABRT, syscall.SIGFPE, syscall.SIGSEGV, syscall.SIGTERM)
	case "darwin":
		signal.Notify(a.sigs, os.Interrupt, os.Kill, syscall.SIGKILL, syscall.SIGSTOP, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL, syscall.SIGABRT, syscall.SIGSYS, syscall.SIGTERM)
	}
}

func (a *app) shutdown() {
	select {
	case <-a.sigs:
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go a.forceShutdown(ctx)
		a.stopAllServers()
		a.executeAllCallbacks()
		return
	}
}

func (a *app) forceShutdown(ctx context.Context) {
	select {
	case <-a.sigs:
		fmt.Println(time.Now().Format("2006/01/02 15:04:05")+ " "+ "强制退出")
		os.Exit(InterruptExitCode)
	case <-time.After(a.opts.shutdownTimeout):
		fmt.Println(time.Now().Format("2006/01/02 15:04:05")+ " "+ "超时退出")
		os.Exit(TimeoutExitCode)
	case <-ctx.Done():
		fmt.Println(time.Now().Format("2006/01/02 15:04:05")+ " "+ "正常退出")
		return
	}
}

func (a *app) stopAllServers() {
	var wg sync.WaitGroup
	for _, s := range a.servers {
		s := s
		wg.Add(1)
		// todo: handle error and panic
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), a.opts.waitTime)
			defer cancel()

			s.Stop(ctx)
		}()
	}
	wg.Wait()
}

func (a *app) executeAllCallbacks() {
	var wg sync.WaitGroup

	for _, c := range a.callbacks {
		callback := c
		// todo: how to handle this goroutine panic?
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), a.opts.callbackTimeout)
			defer cancel()

			// todo: how to handle error/panic of each callback
			callback(ctx)
		}()
	}

	wg.Wait()
}
