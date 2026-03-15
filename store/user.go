package store

import "github.com/TheChosenGay/aichat/types"

type UserStore interface {
	Save(*types.User) error
	GetById(id string) (*types.User, error)
	GetUsersByIds(ids []string) ([]*types.User, error)
	GetByEmail(email string) (*types.User, error)
	List(limit int) ([]*types.User, error)

	// Avatar
	UpdateAvatarUrl(userId string, avatarUrl string) error
	GetAvatarUrl(userId string) (string, error)
}
