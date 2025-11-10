package kratos

import (
	"context"
	"errors"
	"sync"
)

type Option func(*App)

type AppServer interface {
	Start(context.Context) error
	Stop(context.Context) error
}

type App struct {
	name     string
	version  string
	logger   logLogger
	servers  []AppServer
	mu       sync.Mutex
	started  bool
	startErr error
}

type logLogger interface {
	Log(level string, keyvals ...interface{}) error
}

func New(opts ...Option) *App {
	app := &App{}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

func Name(name string) Option {
	return func(a *App) {
		a.name = name
	}
}

func Version(version string) Option {
	return func(a *App) {
		a.version = version
	}
}

func Logger(logger logLogger) Option {
	return func(a *App) {
		a.logger = logger
	}
}

func ServerOption(server AppServer) Option {
	return func(a *App) {
		a.servers = append(a.servers, server)
	}
}

func Server(server AppServer) Option {
	return ServerOption(server)
}

func (a *App) Run() error {
	if len(a.servers) == 0 {
		return errors.New("no servers configured")
	}

	ctx := context.Background()
	a.mu.Lock()
	a.started = true
	a.mu.Unlock()

	var wg sync.WaitGroup
	errCh := make(chan error, len(a.servers))
	for _, srv := range a.servers {
		wg.Add(1)
		go func(s AppServer) {
			defer wg.Done()
			if err := s.Start(ctx); err != nil {
				errCh <- err
			}
		}(srv)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *App) Stop(ctx context.Context) error {
	a.mu.Lock()
	started := a.started
	a.mu.Unlock()
	if !started {
		return nil
	}

	for _, srv := range a.servers {
		if err := srv.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}
