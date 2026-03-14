package api

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/TheChosenGay/aichat/middleware"
	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/types"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type RelationServerOpt struct {
	ListenPort string
}

type CreateRelationRequest struct {
	FriendId string `json:"friend_id" validate:"required,uuid"`
	NickName string `json:"nick_name" validate:"omitempty,min=3,max=32"`
}

type RelationServer struct {
	opt             *RelationServerOpt
	relationService service.RelationshipService
	validator       *validator.Validate
}

// MARK - ListRelationRequest
type ListRelationResponse struct {
	Relations []*types.Relationship `json:"relations"`
	Cookie    int64                 `json:"cookie"`
	IsEnd     bool                  `json:"is_end"`
}

// MARK - UpdateRelationRequest
type UpdateRelationRequest struct {
	FriendId string `json:"friend_id" validate:"required,uuid"`
	NickName string `json:"nick_name" validate:"required,min=3,max=32"`
}

// MARK - DeleteRelationRequest
type DeleteRelationRequest struct {
	FriendId string `json:"friend_id" validate:"required,uuid"`
}

// MARK -- ListFriendRequestResponse
type ListFriendRequestResponse struct {
	FriendRequests []*types.FriendRequest `json:"friend_requests"`
	Cookie         int64                  `json:"cookie"`
	IsEnd          bool                   `json:"is_end"`
}

func NewRelationServer(opt RelationServerOpt, relationService service.RelationshipService) *RelationServer {
	return &RelationServer{
		opt:             &opt,
		relationService: relationService,
		validator:       validator.New(),
	}
}

func (r *RelationServer) RegisterHandler(mx *mux.Router) {
	// relationship
	mx.HandleFunc("/relation/create", middleware.JwtMiddleware(r.createRelationHandler)).Methods("POST")
	mx.HandleFunc("/relation/update", middleware.JwtMiddleware(r.updateRelationHandler)).Methods("POST")
	mx.HandleFunc("/relation/list", middleware.JwtMiddleware(r.listRelationHandler)).Methods("GET")
	mx.HandleFunc("/relation/delete", middleware.JwtMiddleware(r.deleteRelationHandler)).Methods("DELETE")

	// friend request
	mx.HandleFunc("/friend/request/{friendId}/create", middleware.JwtMiddleware(r.createFriendRequestHandler)).Methods("POST")
	mx.HandleFunc("/friend/request/{friendId}/reject", middleware.JwtMiddleware(r.rejectFriendRequestHandler)).Methods("POST")
	mx.HandleFunc("/friend/request/{friendId}/accept", middleware.JwtMiddleware(r.acceptFriendRequestHandler)).Methods("POST")
	mx.HandleFunc("/friend/request/list", middleware.JwtMiddleware(r.listFriendRequestHandler)).Methods("GET")
}

func (s *RelationServer) createRelationHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	req := CreateRelationRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		InternalError(w, err.Error())
		return
	}
	if err := s.validator.Struct(req); err != nil {
		BadRequest(w, err.Error())
		return
	}
	if err := s.relationService.CreateRelationship(userId, req.FriendId, req.NickName); err != nil {
		InternalError(w, err.Error())
		return
	}
	OK(w, nil)
}

func (s *RelationServer) listRelationHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	cookie, limit, err := getCookieAndLimit(r, "cookie", "limit", 20)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}

	relations, err := s.relationService.GetRelationship(userId, cookie, limit)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	if len(relations) > 0 {
		cookie = relations[len(relations)-1].CreateAt
	}

	response := ListRelationResponse{
		Relations: relations,
		Cookie:    cookie,
		IsEnd:     len(relations) < limit || len(relations) == 0,
	}

	OK(w, response)
}

func (s *RelationServer) updateRelationHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	req := UpdateRelationRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, err.Error())
		return
	}
	if err := s.validator.Struct(req); err != nil {
		BadRequest(w, err.Error())
		return
	}
	if err := s.relationService.UpdateRelationshipNickName(userId, req.FriendId, req.NickName); err != nil {
		InternalError(w, err.Error())
		return
	}
	OK(w, nil)
}

func (s *RelationServer) deleteRelationHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	req := DeleteRelationRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, err.Error())
		return
	}
	if err := s.validator.Struct(req); err != nil {
		BadRequest(w, err.Error())
		return
	}
	if err := s.relationService.DeleteRelationship(userId, req.FriendId); err != nil {
		InternalError(w, err.Error())
		return
	}
	OK(w, nil)
}

func (s *RelationServer) createFriendRequestHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	friendId := mux.Vars(r)["friendId"]
	if _, err := uuid.Parse(friendId); err != nil {
		BadRequest(w, "invalid friendId")
		return
	}
	if err := s.relationService.CreateFriendRequest(userId, friendId); err != nil {
		InternalError(w, err.Error())
		return
	}
	OK(w, nil)
}

func (s *RelationServer) rejectFriendRequestHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	friendId := mux.Vars(r)["friendId"]
	if _, err := uuid.Parse(friendId); err != nil {
		BadRequest(w, "invalid friendId")
		return
	}
	if err := s.relationService.RejectFriendRequest(userId, friendId); err != nil {
		InternalError(w, err.Error())
		return
	}
	OK(w, nil)
}

func (s *RelationServer) acceptFriendRequestHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	friendId := mux.Vars(r)["friendId"]
	if _, err := uuid.Parse(friendId); err != nil {
		BadRequest(w, "invalid friendId")
		return
	}
	if err := s.relationService.AcceptFriendRequest(userId, friendId); err != nil {
		InternalError(w, err.Error())
		return
	}
	OK(w, nil)
}

func (s *RelationServer) listFriendRequestHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.Context().Value(middleware.UserIdKey).(string)
	cookie, limit, err := getCookieAndLimit(r, "cookie", "limit", 20)
	slog.Info("listFriendRequestHandler", "cookie", cookie, "limit", limit)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}
	friendRequests, err := s.relationService.GetFriendRequests(userId, cookie, limit)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	if len(friendRequests) > 0 {
		cookie = friendRequests[len(friendRequests)-1].CreateAt
	}

	response := ListFriendRequestResponse{
		FriendRequests: friendRequests,
		Cookie:         cookie,
		IsEnd:          len(friendRequests) < limit || len(friendRequests) == 0,
	}
	slog.Info("listFriendRequestHandler", "response", response)
	OK(w, response)
}

func getCookieAndLimit(r *http.Request, cookieKey string, limitKey string, defaultLimit int) (int64, int, error) {
	cookie := int64(math.MaxInt64)
	if cookieStr := r.URL.Query().Get(cookieKey); cookieStr != "" {
		cookieVal, err := strconv.ParseInt(cookieStr, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		cookie = cookieVal
	}
	limit := defaultLimit
	if limitStr := r.URL.Query().Get(limitKey); limitStr != "" {
		limitVal, err := strconv.Atoi(limitStr)
		if err != nil {
			return 0, 0, err
		}
		limit = limitVal
	}
	return cookie, limit, nil
}
