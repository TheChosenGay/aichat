package service

import "github.com/TheChosenGay/aichat/types"

type MessageRouter interface {
	Route(message *types.Message) error
}
