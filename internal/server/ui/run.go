package ui

import (
	"context"
	"net"
	"net/http"
	"time"
)

type Runner struct {
	server   *http.Server
	listener net.Listener
}

func NewRunner(address string, handler http.Handler) *Runner {
	return &Runner{server: &http.Server{Addr: address, Handler: handler}}
}

func (r *Runner) Bind() error {
	if r == nil || r.server == nil {
		return nil
	}
	if r.listener != nil {
		return nil
	}
	ln, err := net.Listen("tcp", r.server.Addr)
	if err != nil {
		return err
	}
	r.listener = ln
	return nil
}

func (r *Runner) Start() error {
	if r == nil || r.server == nil {
		return nil
	}
	if err := r.Bind(); err != nil {
		return err
	}
	if err := r.server.Serve(r.listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (r *Runner) Shutdown(ctx context.Context) error {
	if r == nil || r.server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.server.Shutdown(shutdownCtx)
}
