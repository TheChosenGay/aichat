package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/TheChosenGay/aichat/middleware"
	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type CreateRoomRequest struct {
	Name string `json:"name" validate:"required,min=3,max=32"`
}

type RoomServer struct {
	roomService service.RoomService
}

func NewRoomServer(roomService service.RoomService) *RoomServer {
	return &RoomServer{
		roomService: roomService,
	}
}

func (s *RoomServer) RegisterHandler(mx *mux.Router) {
	mx.HandleFunc("/room/create", middleware.JwtMiddleware(s.createRoomHandler)).Methods("POST")
	mx.HandleFunc("/room/get/{room_id}", middleware.JwtMiddleware(s.getRoomHandler)).Methods("GET")
	mx.HandleFunc("/room/add-member/{room_id}/{member_id}", middleware.JwtMiddleware(s.addMemberToRoomHandler)).Methods("POST")
	mx.HandleFunc("/room/remove-member/{room_id}/{member_id}", middleware.JwtMiddleware(s.removeMemberFromRoomHandler)).Methods("POST")
	mx.HandleFunc("/room/get-members/{room_id}", middleware.JwtMiddleware(s.getMembersHandler)).Methods("GET")
}

func (s *RoomServer) createRoomHandler(w http.ResponseWriter, r *http.Request) {
	req := &CreateRoomRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		WriteToJson(w, map[string]any{
			"code":  1,
			"error": err.Error(),
		})
		return
	}
	room := &types.Room{
		RoomId:   uuid.New().String(),
		OwnerId:  r.Context().Value(middleware.UserIdKey).(string),
		Name:     req.Name,
		CreateAt: time.Now().UnixNano(),
	}

	if err := s.roomService.CreateRoom(room); err != nil {
		WriteToJson(w, map[string]any{
			"code":  1,
			"error": err.Error(),
		})
		return
	}

	if err := WriteToJson(w, map[string]any{
		"code": 0,
		"room": room,
	}); err != nil {
		slog.Error("Failed to write to json", "error", err.Error())
		return
	}
}

func (s *RoomServer) getRoomHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomId := vars["room_id"]
	room, err := s.roomService.GetRoomById(roomId)
	if err != nil {
		WriteToJson(w, map[string]any{
			"code":  1,
			"error": err.Error(),
		})
		return
	}
	if err := WriteToJson(w, map[string]any{
		"code": 0,
		"room": room,
	}); err != nil {
		slog.Error("Failed to write to json", "error", err.Error())
		return
	}
}

func (s *RoomServer) addMemberToRoomHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomId := vars["room_id"]
	memberId := vars["member_id"]
	err := s.roomService.AddMemberToRoom(roomId, memberId)
	if err != nil {
		WriteToJson(w, map[string]any{
			"code":  1,
			"error": err.Error(),
		})
		return
	}
	if err := WriteToJson(w, map[string]any{
		"code":    0,
		"message": "success",
	}); err != nil {
		slog.Error("Failed to write to json", "error", err.Error())
		return
	}
}

func (s *RoomServer) removeMemberFromRoomHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomId := vars["room_id"]
	memberId := vars["member_id"]
	err := s.roomService.RemoveMemberFromRoom(roomId, memberId)
	if err != nil {
		WriteToJson(w, map[string]any{
			"code":  1,
			"error": err.Error(),
		})
		return
	}
	if err := WriteToJson(w, map[string]any{
		"code":    0,
		"message": "success",
	}); err != nil {
		slog.Error("Failed to write to json", "error", err.Error())
		return
	}
}

func (s *RoomServer) getMembersHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomId := vars["room_id"]
	members, err := s.roomService.GetMembers(roomId)
	if err != nil {
		WriteToJson(w, map[string]any{
			"code":  1,
			"error": err.Error(),
		})
		return
	}
	if err := WriteToJson(w, map[string]any{
		"code":    0,
		"members": members,
	}); err != nil {
		slog.Error("Failed to write to json", "error", err.Error())
		return
	}
}
