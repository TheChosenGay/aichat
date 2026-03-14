package service

import (
	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
)

type RelationshipService interface {
	// relationship
	CreateRelationship(user_id string, friend_id string, nick_name string) error
	UpdateRelationshipNickName(user_id string, friend_id string, nick_name string) error
	GetRelationship(user_id string, create_at int64, limit int) ([]*types.Relationship, error)
	DeleteRelationship(user_id string, friend_id string) error

	// friend request
	CreateFriendRequest(user_id string, req_user_id string) error
	AcceptFriendRequest(user_id string, req_user_id string) error
	RejectFriendRequest(user_id string, req_user_id string) error
	GetFriendRequests(user_id string, create_at int64, limit int) ([]*types.FriendRequest, error)
	GetPendingFriendRequests(user_id string, create_at int64, limit int) ([]*types.FriendRequest, error)
}

type defaultRelationshipService struct {
	relationshipDbStore  store.RelationshipStore
	friendRequestDbStore store.FriendRequestStore
}

func NewRelationshipService(relationshipDbStore *store.RelationshipDbStore) RelationshipService {
	return &defaultRelationshipService{
		relationshipDbStore:  relationshipDbStore,
		friendRequestDbStore: relationshipDbStore,
	}
}

func (s *defaultRelationshipService) CreateRelationship(user_id string, friend_id string, nick_name string) error {
	return s.relationshipDbStore.CreateRelationship(user_id, friend_id, nick_name)
}
func (s *defaultRelationshipService) UpdateRelationshipNickName(user_id string, friend_id string, nick_name string) error {
	return s.relationshipDbStore.UpdateRelationshipNickName(user_id, friend_id, nick_name)
}
func (s *defaultRelationshipService) GetRelationship(user_id string, create_at int64, limit int) ([]*types.Relationship, error) {
	return s.relationshipDbStore.GetRelationshipsByUserId(user_id, create_at, limit)
}
func (s *defaultRelationshipService) DeleteRelationship(user_id string, friend_id string) error {
	return s.relationshipDbStore.DeleteRelationship(user_id, friend_id)
}

func (s *defaultRelationshipService) CreateFriendRequest(user_id string, req_user_id string) error {
	return s.friendRequestDbStore.CreateRequest(user_id, req_user_id)
}
func (s *defaultRelationshipService) AcceptFriendRequest(user_id string, req_user_id string) error {
	return s.friendRequestDbStore.AcceptRequest(user_id, req_user_id)
}
func (s *defaultRelationshipService) RejectFriendRequest(user_id string, req_user_id string) error {
	return s.friendRequestDbStore.UpdateRequestStatus(user_id, req_user_id, types.RequestStatusRejected)
}
func (s *defaultRelationshipService) GetFriendRequests(user_id string, create_at int64, limit int) ([]*types.FriendRequest, error) {
	return s.friendRequestDbStore.GetRequestsByUserId(user_id, create_at, limit)
}
func (s *defaultRelationshipService) GetPendingFriendRequests(user_id string, create_at int64, limit int) ([]*types.FriendRequest, error) {
	return s.friendRequestDbStore.GetPendingRequestByUserId(user_id, create_at, limit)
}
