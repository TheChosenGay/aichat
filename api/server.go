package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
)

type ServerOpt struct {
	ListenPort string
}

type Server struct {
	opt              *ServerOpt
	registerHandlers []RegisterHandler
}

type RegisterHandler interface {
	RegisterHandler(mx *mux.Router)
}

func NewServer(opt *ServerOpt, registerHandlers ...RegisterHandler) *Server {
	return &Server{
		opt:              opt,
		registerHandlers: registerHandlers,
	}
}

func (s *Server) Run() error {
	mx := mux.NewRouter()
	for _, handler := range s.registerHandlers {
		handler.RegisterHandler(mx)
	}
	slog.Info("User server listening on port", "port", s.opt.ListenPort)
	return http.ListenAndServe(s.opt.ListenPort, mx)
}
