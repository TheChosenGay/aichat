package service

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
)

type MessagePendOpts struct {
	TTL           time.Duration                  // 消息超时时间
	MaxRetry      int                            // 最大重试次数
	RetryInterval time.Duration                  // 超时重试时间
	OnMsgFailed   func(msg *types.Message) error // 消息发送失败回调
	OnMsgAcked    func(msg *types.Message) error // 消息发送成功回调
	OnMsgRetry    func(msg *types.Message) error // 消息重试回调
}

type PendMessage struct {
	retryCount  int
	lastRetryAt int64
	msg         *types.Message
}

type MessagePender interface {
	Pend(msg *types.Message) error
	UnPend(msgId string) error
}

type defaultMessagePender struct {
	opt     MessagePendOpts
	mx      sync.Mutex
	msgList map[string]*PendMessage
}

func NewMessagePender(opt MessagePendOpts) MessagePender {
	p := &defaultMessagePender{
		opt:     opt,
		mx:      sync.Mutex{},
		msgList: make(map[string]*PendMessage),
	}

	go func() {
		p.clearUp()
	}()
	return p
}

func (p *defaultMessagePender) clearUp() {
	c := time.NewTicker(p.opt.RetryInterval)
	defer c.Stop()
	for {
		select {
		case <-c.C:
			p.mx.Lock()
			// 先复制需要处理的消息，避免在持有锁时调用回调
			var toRetry []*PendMessage
			var toFail []*PendMessage
			for _, msg := range p.msgList {
				expiredAt := msg.msg.SendAt + int64(p.opt.TTL.Seconds())
				if time.Now().Unix() >= expiredAt || msg.retryCount >= p.opt.MaxRetry {
					toFail = append(toFail, msg)
					continue
				}
				lastExpiredAt := msg.lastRetryAt + int64(p.opt.RetryInterval.Seconds())
				if time.Now().Unix() >= lastExpiredAt {
					toRetry = append(toRetry, msg)
					continue
				}
			}
			// 删除所有已经失败的消息
			for _, msg := range toFail {
				delete(p.msgList, msg.msg.MsgId)
			}
			p.mx.Unlock()

			// 在锁外调用回调
			for _, msg := range toFail {
				p.opt.OnMsgFailed(msg.msg)
			}
			for _, msg := range toRetry {
				msg.lastRetryAt = time.Now().Unix()
				msg.retryCount++
				p.opt.OnMsgRetry(msg.msg)
			}
		}
	}
}

func (p *defaultMessagePender) Pend(msg *types.Message) error {
	p.mx.Lock()
	defer p.mx.Unlock()
	if _, ok := p.msgList[msg.MsgId]; ok {
		return errors.New("message already pend")
	}
	pMsg := &PendMessage{
		retryCount:  0,
		msg:         msg,
		lastRetryAt: time.Now().Unix(),
	}
	p.msgList[msg.MsgId] = pMsg
	return nil
}

func (p *defaultMessagePender) UnPend(msgId string) error {
	p.mx.Lock()
	if _, ok := p.msgList[msgId]; !ok {
		return errors.New("message not found")
	}
	msg := p.msgList[msgId].msg
	delete(p.msgList, msgId)
	p.mx.Unlock()

	p.opt.OnMsgAcked(msg)
	return nil
}

// MARK -- MessageService

type MessageService struct {
	messageStore store.MessageStore
	roomStore    store.RoomStore
	router       MessageRouter
	pender       MessagePender
}

func NewMessageService(messageStore store.MessageStore, roomStore store.RoomStore, router MessageRouter) *MessageService {
	m := &MessageService{
		messageStore: messageStore,
		roomStore:    roomStore,
		router:       router,
	}

	m.pender = NewMessagePender(MessagePendOpts{
		TTL:           30 * time.Second,
		MaxRetry:      3,
		RetryInterval: 5 * time.Second,
		OnMsgFailed: func(msg *types.Message) error {
			msg.Type = types.MessageTypeFailed
			failedMsg := &types.Message{
				MsgId:   msg.MsgId,
				FromId:  msg.ToId,
				ToId:    msg.FromId,
				RoomId:  msg.RoomId,
				Content: "",
				Type:    types.MessageTypeFailed,
				SendAt:  time.Now().Unix(),
			}
			m.SendMessage(failedMsg)
			return nil
		},

		OnMsgAcked: func(msg *types.Message) error {
			msg.IsDelivered = true
			slog.Info("message acked", "message", msg)
			return m.messageStore.Update(msg)
		},

		OnMsgRetry: func(msg *types.Message) error {
			m.retryMessage(msg)
			slog.Info("message retry", "message", msg)
			return nil
		},
	})
	return m
}

func (s *MessageService) retryMessage(message *types.Message) error {
	return s.router.Route(message)
}

func (s *MessageService) SendMessage(message *types.Message) error {
	if message.ToId == "" && message.RoomId == "" {
		return errors.New("to_id or room_id is required")
	}
	if message.Type != types.MessageTypeFailed && message.Type != types.MessageTypeAck {
		// 保存消息
		if err := s.messageStore.Save(message); err != nil {
			slog.Error("save message failed", "error", err)
			return err
		}
		if message.RoomId != "" {
			// 群消息暂时支持重试
			return s.sendGroupMessage(message)
		}
	}

	// 如果是ACK消息，则直接从pender中删除
	if message.Type == types.MessageTypeAck {
		return s.pender.UnPend(message.MsgId)
	}

	if message.Type == types.MessageTypeFailed {
		// 失败通知消息不需要pending
		return s.router.Route(message)
	}

	// 普通消息
	// 存入pender 待确认
	if err := s.pender.Pend(message); err != nil {
		slog.Error("failed to pend message", "error", err)
	}
	// 转发给对应用户
	return s.router.Route(message)
}

func (s *MessageService) sendGroupMessage(message *types.Message) error {
	memberIds, err := s.roomStore.GetMembers(message.RoomId)
	if err != nil {
		return err
	}
	return s.router.RouteGroup(message, memberIds)
}
