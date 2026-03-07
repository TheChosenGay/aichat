package ws

import (
	"log/slog"
	"net/http"

	"github.com/TheChosenGay/aichat/gateway"
	"github.com/TheChosenGay/aichat/middleware"
	"github.com/gorilla/websocket"
)

type WsServer struct {
	opt         *gateway.ServerOpt
	upgrader    *websocket.Upgrader
	connManager *gateway.ConnManager
}

func NewWsServer(opt *gateway.ServerOpt) *WsServer {
	return &WsServer{
		opt:         opt,
		connManager: gateway.NewConnManager(),
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:    1024,
			WriteBufferSize:   1024,
			EnableCompression: true,
		},
	}
}

func (s *WsServer) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", middleware.JwtMiddleware(s.handleWs))
	slog.Info("Start Ws Server", "list port: ", s.opt.ListenPort)
	return http.ListenAndServe(s.opt.ListenPort, mux)
}

func (s *WsServer) handleWs(w http.ResponseWriter, r *http.Request) {
	// 不要使用强制会直接panic，改为带检查的断言
	id, ok := r.Context().Value(middleware.UserIdKey).(string)
	if !ok || id == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade to websocket", "error", err.Error())
		return
	}

	conn := NewWsConn(
		id,
		c,
		func(id string) {
			s.connManager.RemoveConn(id)
		},
		func(data []byte) {
			slog.Info("receive message", "data", string(data))
		},
	)

	if err := s.connManager.AddConn(conn); err != nil {
		slog.Error("Failed to add conn", "error", err.Error())
		conn.Close()
		return
	}
	// 启动连接读写
	conn.Start()
}
