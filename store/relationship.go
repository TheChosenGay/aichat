package store

import "github.com/TheChosenGay/aichat/types"

type RelationshipStore interface {
	CreateRelationship(userId string, friendId string, nickName string) error
	GetRelationshipsByUserId(userId string, createAt int64, limit int) ([]*types.Relationship, error)
	UpdateRelationshipNickName(userId string, friendId string, nickName string) error
	DeleteRelationship(userId string, friendId string) error
	IsFriend(userId string, friendId string) (bool, error)
}
