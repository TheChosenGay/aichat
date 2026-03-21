package service

import (
	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
	"github.com/google/uuid"
)

type ConversationService interface {
	CreateConversation(userId, peerId, roomId string) (*types.Conversation, error)
	GetConversations(userId string, last_time int64, limit int) ([]*types.Conversation, error)
	UpdateConversation(conversation *types.Conversation) error
	GetConversationByUserIdAndRoomId(userId, roomId string) (*types.Conversation, error)
	GetConversationByUserIdAndPeerId(userId, peerId string) (*types.Conversation, error)
}

type defaultConversationService struct {
	conversationDbStore *store.ConversationDbStore
	userService         UserService
}

func NewConversationService(conversationDbStore *store.ConversationDbStore, userService UserService) ConversationService {
	return &defaultConversationService{
		conversationDbStore: conversationDbStore,
		userService:         userService,
	}
}

func (s *defaultConversationService) CreateConversation(userId, peerId, roomId string) (*types.Conversation, error) {
	conversation := &types.Conversation{
		CId:    uuid.New().String(),
		UserId: userId,
		PeerId: peerId,
		RoomId: roomId,
	}
	if err := s.conversationDbStore.InsertOrUpdate(conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

func (s *defaultConversationService) GetConversations(userId string, last_time int64, limit int) ([]*types.Conversation, error) {
	return s.conversationDbStore.GetByUserId(userId, last_time, limit)
}

func (s *defaultConversationService) UpdateConversation(conversation *types.Conversation) error {
	return s.conversationDbStore.InsertOrUpdate(conversation)
}

func (s *defaultConversationService) GetConversationByUserIdAndPeerId(userId string, peerId string) (*types.Conversation, error) {
	return s.conversationDbStore.GetConversationByUserIdAndPeerId(userId, peerId)
}

func (s *defaultConversationService) GetConversationByUserIdAndRoomId(userId, roomId string) (*types.Conversation, error) {
	return s.conversationDbStore.GetConversationByUserIdAndRoomId(userId, roomId)
}
