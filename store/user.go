package store

import "github.com/TheChosenGay/aichat/types"

type UserStore interface {
	Save(*types.User) error
	GetById(id string) (*types.User, error)
	GetByEmail(email string) (*types.User, error)
	List(limit int) ([]*types.User, error)
}
