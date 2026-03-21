package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/TheChosenGay/aichat/middleware"
	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/types"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

type ConversationCreateReq struct {
	PeerId string `json:"peer_id" validate:"omitempty,uuid"`
	RoomId string `json:"room_id" validate:"omitempty,uuid"`
}

type ConversationOpenReq struct {
	PeerId string `json:"peer_id" validate:"omitempty,uuid"`
	RoomId string `json:"room_id" validate:"omitempty,uuid"`
}

func (c *ConversationOpenReq) IsValid() bool {
	if c.PeerId == "" && c.RoomId == "" {
		return false
	}
	if c.PeerId != "" && c.RoomId != "" {
		return false
	}

	return true
}

type ConversationServer struct {
	validate            *validator.Validate
	conversationService service.ConversationService
}

func NewConversationServer(conversationService service.ConversationService) *ConversationServer {
	return &ConversationServer{
		conversationService: conversationService,
		validate:            validator.New(),
	}
}

func (s *ConversationServer) RegisterHandler(mx *mux.Router) {
	mx.HandleFunc("/conversation/create", middleware.JwtMiddleware(s.createConversationHandler)).Methods("POST")
	mx.HandleFunc("/conversation/list", middleware.JwtMiddleware(s.getConversationListHandler)).Methods("GET")
	mx.HandleFunc("/conversation/open", middleware.JwtMiddleware(s.openConversationHandler)).Methods("POST")
}

func (s *ConversationServer) createConversationHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	req := &ConversationCreateReq{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		BadRequest(w, err.Error())
		return
	}

	if err := s.validate.Struct(req); err != nil {
		BadRequest(w, err.Error())
		return
	}

	if req.PeerId != "" && req.RoomId != "" {
		BadRequest(w, "peer_id and room_id cannot be both set")
		return
	}

	if req.PeerId == "" && req.RoomId == "" {
		BadRequest(w, "peer_id or room_id is required")
		return
	}

	_, err := s.conversationService.CreateConversation(userId, req.PeerId, req.RoomId)
	if err != nil {
		InternalError(w, err.Error())
		return
	}
	OK(w, nil)
}

func (s *ConversationServer) getConversationListHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	cookie, limit, err := getCookieAndLimit(r, "cookie", "limit", 10)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}
	slog.Info("list conversation", "userId", userId, "cookie", cookie, "limit", limit)
	conversations, err := s.conversationService.GetConversations(userId, cookie, limit)
	slog.Info("list conversation", "conversations", conversations)
	if err != nil {
		InternalError(w, err.Error())
		return
	}
	OK(w, conversations)
}

func (s *ConversationServer) openConversationHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	req := &ConversationOpenReq{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		BadRequest(w, err.Error())
		return
	}
	slog.Info("open conversation", "userId", userId, "req", req)
	if err := s.validate.Struct(req); err != nil {
		BadRequest(w, err.Error())
		return
	}

	if !req.IsValid() {
		BadRequest(w, "peer_id or room_id is required")
		return
	}

	var conversation *types.Conversation
	var err error
	if req.PeerId != "" {
		conversation, err = s.conversationService.GetConversationByUserIdAndPeerId(userId, req.PeerId)
	} else {
		conversation, err = s.conversationService.GetConversationByUserIdAndRoomId(userId, req.RoomId)
	}

	if err != nil {
		InternalError(w, err.Error())
		slog.Error("open conversation", "error", err.Error())
		return
	}

	if conversation == nil {
		conversation, err = s.conversationService.CreateConversation(userId, req.PeerId, req.RoomId)
		if err != nil {
			InternalError(w, err.Error())
			return
		}
	}

	OK(w, conversation)
}
