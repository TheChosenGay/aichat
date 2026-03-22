package api

import (
	"encoding/json"
	"math"
	"net/http"

	"github.com/TheChosenGay/aichat/middleware"
	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/types"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

type FetchMessageRequest struct {
	ChannelId string `json:"channel_id" validate:"required"`
	Cookie    int64  `json:"cookie" validate:"omitempty"`
	Limit     int    `json:"limit" validate:"omitempty"`
}

type FetchMessageResponse struct {
	Messages []*types.Message `json:"messages"`
	Cookie   int64            `json:"cookie"`
	IsEnd    bool             `json:"is_end"`
}

type MessageServer struct {
	messageService service.MessageService
	validate       *validator.Validate
}

func NewMessageServer(messageService service.MessageService) *MessageServer {
	return &MessageServer{
		messageService: messageService,
		validate:       validator.New(),
	}
}

func (m *MessageServer) RegisterHandler(mx *mux.Router) {
	mx.HandleFunc("/message/fetch", middleware.JwtMiddleware(m.fetchMessageHandler)).Methods("POST")
}

func (m *MessageServer) fetchMessageHandler(w http.ResponseWriter, r *http.Request) {
	req := FetchMessageRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, err.Error())
		return
	}
	if err := m.validate.Struct(req); err != nil {
		BadRequest(w, err.Error())
		return
	}

	if req.Limit <= 0 {
		// 默认给个50
		req.Limit = 50
	}
	if req.Cookie <= 0 {
		req.Cookie = math.MaxInt64
	}

	messages, err := m.messageService.FetchChannelHistoryMessages(req.ChannelId, req.Limit, req.Cookie)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	var isEnd bool
	var cookie int64 = req.Cookie
	if len(messages) == 0 || len(messages) < req.Limit {
		isEnd = true
		if len(messages) > 0 {
			cookie = messages[len(messages)-1].SendAt
		}
	}

	OK(w, &FetchMessageResponse{
		Messages: messages,
		Cookie:   cookie,
		IsEnd:    isEnd,
	})
}
