package service

import (
	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
)

type MessageService struct {
	messageStore store.MessageStore
	router       MessageRouter
}

func NewMessageService(messageStore store.MessageStore, router MessageRouter) *MessageService {
	return &MessageService{
		messageStore: messageStore,
		router:       router,
	}
}

func (s *MessageService) SendMessage(message *types.Message) error {
	// 保存消息
	if err := s.messageStore.Save(message); err != nil {
		return err
	}
	// 转发给对应用户
	return s.router.Route(message)
}
