package server

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/TDD-all-the-things/shutdown-app-gracefully/app"
)

var _ app.Server = &Server{}

type Server struct {
	name string
	addr string
	srv  *http.Server
	mux  *serveMux
}

func New(name string, addr string) *Server {
	mux := &serveMux{reject: atomic.Bool{}, ServeMux: http.NewServeMux()}
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return &Server{name: name, addr: addr, mux: mux, srv: srv}
}

func (s *Server) Name() string {
	return s.name
}
func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	s.mux.reject.Store(true)
	return s.srv.Shutdown(ctx)
}

type serveMux struct {
	reject atomic.Bool
	*http.ServeMux
}

func (m *serveMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.reject.Load() {
		log.Println("拒绝请求......")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(http.StatusText(http.StatusServiceUnavailable)))
		return
	}
	m.ServeMux.ServeHTTP(w, r)
}
