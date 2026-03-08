package service

import (
	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
)

type MessageService struct {
	messageStore store.MessageStore
	roomStore    store.RoomStore
	router       MessageRouter
}

func NewMessageService(messageStore store.MessageStore, roomStore store.RoomStore, router MessageRouter) *MessageService {
	return &MessageService{
		messageStore: messageStore,
		roomStore:    roomStore,
		router:       router,
	}
}

func (s *MessageService) SendMessage(message *types.Message) error {
	// 保存消息
	if err := s.messageStore.Save(message); err != nil {
		return err
	}

	if message.RoomId != "" {
		return s.sendGroupMessage(message)
	}

	// 转发给对应用户
	return s.router.Route(message)
}

func (s *MessageService) sendGroupMessage(message *types.Message) error {
	memberIds, err := s.roomStore.GetMembers(message.RoomId)
	if err != nil {
		return err
	}
	return s.router.RouteGroup(message, memberIds)
}
