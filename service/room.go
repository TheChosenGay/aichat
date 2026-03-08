package service

import (
	"errors"

	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
)

const ROOM_MAX_LIMIT = 20 // 每个人最多只允许创作20个群组

type RoomService interface {
	CreateRoom(*types.Room) error
	GetRoomById(id string) (*types.Room, error)
	AddMemberToRoom(roomId string, memberId string) error
	RemoveMemberFromRoom(roomId string, memberId string) error
	GetMembers(roomId string) ([]*types.User, error)
	GetMemberPaged(roomId string, afterUserId string, limit int) ([]*types.User, error)
}

type defaultRoomService struct {
	roomStore store.RoomStore
	userStore store.UserStore
}

func NewRoomService(roomStore store.RoomStore, userStore store.UserStore) RoomService {
	return &defaultRoomService{
		roomStore: roomStore,
		userStore: userStore,
	}
}

func (s *defaultRoomService) CreateRoom(room *types.Room) error {
	count, err := s.roomStore.GetRoomCountByUserId(room.OwnerId)
	if err != nil {
		return err
	}
	if count >= ROOM_MAX_LIMIT {
		return errors.New("user room count limit reached")
	}

	return s.roomStore.CreateRoom(room)
}

func (s *defaultRoomService) GetRoomById(id string) (*types.Room, error) {
	room, err := s.roomStore.GetRoomById(id)
	if err != nil {
		return nil, err
	}
	return room, nil
}

func (s *defaultRoomService) AddMemberToRoom(roomId string, memberId string) error {
	return s.roomStore.AddMember(roomId, memberId)
}

func (s *defaultRoomService) RemoveMemberFromRoom(roomId string, memberId string) error {
	return s.roomStore.RemoveMember(roomId, memberId)
}

func (s *defaultRoomService) GetMembers(roomId string) ([]*types.User, error) {
	userIds, err := s.roomStore.GetMembers(roomId)
	if err != nil {
		return nil, err
	}
	users, err := s.userStore.GetUsersByIds(userIds)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (s *defaultRoomService) GetMemberPaged(roomId string, afterUserId string, limit int) ([]*types.User, error) {
	userIds, err := s.roomStore.GetMembersPaged(roomId, afterUserId, limit)
	if err != nil {
		return nil, err
	}
	users, err := s.userStore.GetUsersByIds(userIds)
	if err != nil {
		return nil, err
	}
	return users, nil
}
