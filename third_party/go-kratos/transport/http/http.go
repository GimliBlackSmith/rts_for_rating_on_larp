package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"sync"
)

type Context interface {
	Request() *stdhttp.Request
	Response() stdhttp.ResponseWriter
	JSON(code int, v interface{}) error
}

type Handler interface {
	ServeHTTP(Context) error
}

type HandlerFunc func(Context) error

func (f HandlerFunc) ServeHTTP(ctx Context) error {
	return f(ctx)
}

type ServerOption func(*Server)

type Server struct {
	addr   string
	mux    *stdhttp.ServeMux
	server *stdhttp.Server
	mu     sync.Mutex
}

func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		addr: ":0",
		mux:  stdhttp.NewServeMux(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func Address(addr string) ServerOption {
	return func(s *Server) {
		s.addr = addr
	}
}

func (s *Server) Handle(pattern string, handler Handler) {
	s.mux.HandleFunc(pattern, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		ctx := &httpContext{request: r, writer: w}
		if err := handler.ServeHTTP(ctx); err != nil {
			stdhttp.Error(w, err.Error(), stdhttp.StatusInternalServerError)
		}
	})
}

func (s *Server) HandleFunc(pattern string, handler HandlerFunc) {
	s.Handle(pattern, handler)
}

func (s *Server) Start(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.server = &stdhttp.Server{
		Addr:    s.addr,
		Handler: s.mux,
	}
	if err := s.server.ListenAndServe(); err != nil && err != stdhttp.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

type httpContext struct {
	request *stdhttp.Request
	writer  stdhttp.ResponseWriter
}

func (c *httpContext) Request() *stdhttp.Request {
	return c.request
}

func (c *httpContext) Response() stdhttp.ResponseWriter {
	return c.writer
}

func (c *httpContext) JSON(code int, v interface{}) error {
	c.writer.Header().Set("Content-Type", "application/json")
	c.writer.WriteHeader(code)
	return json.NewEncoder(c.writer).Encode(v)
}
