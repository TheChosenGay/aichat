package router

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"github.com/TheChosenGay/aichat/service"
	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

type pubsubContext struct {
	pubsub *redis.PubSub
	cancel context.CancelFunc
}

type RedisMsgRouter struct {
	redisStore     *store.UserRedisStore
	mtx            sync.Mutex
	userChannlsMap map[string]*pubsubContext
	localRouter    service.MessageRouter
}

func NewRedisMsgRouter(msgRouter service.MessageRouter, redisStore *store.UserRedisStore) *RedisMsgRouter {
	return &RedisMsgRouter{
		redisStore:     redisStore,
		localRouter:    msgRouter,
		mtx:            sync.Mutex{},
		userChannlsMap: map[string]*pubsubContext{},
	}
}

func (s *RedisMsgRouter) Subscribe(userId string) error {
	ctx, cancel := context.WithCancel(context.Background())
	pubsub := s.redisStore.SubUser(userId)
	s.mtx.Lock()
	s.userChannlsMap[userId] = &pubsubContext{
		pubsub: pubsub,
		cancel: cancel,
	}
	s.mtx.Unlock()
	go func() {
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			var message types.Message
			if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
				slog.Error("Failed to unmarshal message", "error", err.Error())
				continue
			}
			s.localRouter.Route(&message)
		}
	}()
	return nil
}

func (s *RedisMsgRouter) Unsubscribe(userId string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	pubsub, ok := s.userChannlsMap[userId]
	if !ok {
		return errors.New("user not found")
	}
	if err := s.redisStore.Unsubscribe(pubsub.pubsub); err != nil {
		slog.Error("Failed to unsubscribe from redis", "error", err.Error())
		return err
	}

	pubsub.cancel()
	delete(s.userChannlsMap, userId)
	return nil
}

func (s *RedisMsgRouter) Route(message *types.Message) error {
	return s.redisStore.Publish(message.ToId, message)
}

func (s *RedisMsgRouter) RouteGroup(message *types.Message, memberIds []string) error {
	eg := errgroup.Group{}
	eg.SetLimit(40)
	for _, memberId := range memberIds {
		eg.Go(func() error {
			return s.redisStore.Publish(memberId, message)
		})
	}
	return eg.Wait()
}
