package store

import "github.com/TheChosenGay/aichat/types"

type UserStore interface {
	Save(*types.User) error
	GetById(id string) (*types.User, error)
}
