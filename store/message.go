package store

import "github.com/TheChosenGay/aichat/types"

type MessageStore interface {
	Save(*types.Message) error
	ListByToId(toId string, before int64, limit int) ([]*types.Message, error)
}
