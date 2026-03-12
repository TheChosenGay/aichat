package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/TheChosenGay/aichat/gateway"
	"github.com/TheChosenGay/aichat/middleware"
	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/types"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/websocket"
)

type WsServer struct {
	opt            *gateway.ServerOpt
	upgrader       *websocket.Upgrader
	ConnManager    *gateway.ConnManager
	messageService *service.MessageService
	userService    service.UserService
	validate       *validator.Validate
}

func NewWsServer(
	opt *gateway.ServerOpt,
	connManager *gateway.ConnManager,
	messageService *service.MessageService,
	userService service.UserService) *WsServer {
	return &WsServer{
		opt:            opt,
		messageService: messageService,
		userService:    userService,
		ConnManager:    connManager,
		validate:       validator.New(),
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
			slog.Info("user connect", "id", id)
			// 主动拉取历史消息
			if err := s.messageService.FetchHistoryMessages(id, 20, time.Now().Unix()); err != nil {
				slog.Error("Failed to fetch history messages", "error", err.Error())
				return
			}
		},
		func(id string) {
			s.ConnManager.RemoveConn(id)
			s.userService.SetOnlineStatus(id, false)
		},
		func(data []byte) {
			//gorilla/websocket会处理分片，返回的data []byte已经是完整的了
			slog.Info("receive message", "data", string(data))
			var message types.Message
			if err := json.Unmarshal(data, &message); err != nil {
				slog.Error("Failed to unmarshal message", "error", err.Error())
				return
			}

			message.FromId = id
			message.SendAt = time.Now().Unix()

			if err := s.validate.Struct(message); err != nil {
				slog.Error("Failed to validate message", "error", err.Error())
				return
			}
			if err := s.messageService.SendMessage(&message); err != nil {
				slog.Error("Failed to send message", "error", err.Error())
				return
			}
		},
	)

	if err := s.ConnManager.AddConn(conn); err != nil {
		slog.Error("Failed to add conn", "error", err.Error())
		conn.Close()
		return
	}
	s.userService.SetOnlineStatus(id, true)
	// 启动连接读写
	conn.Start()

}
