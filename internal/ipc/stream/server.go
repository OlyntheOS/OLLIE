package stream

import (
	"context"
	"net"
)

// Server accepts framed stream connections.
type Server struct {
	listener net.Listener
	handler  *Handler
}

func NewServer(listener net.Listener, handler *Handler) *Server {
	return &Server{listener: listener, handler: handler}
}

func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.listener.Close()
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return err
			}
		}
		go s.handler.HandleConn(ctx, conn)
	}
}

func (s *Server) Close() error {
	return s.listener.Close()
}
