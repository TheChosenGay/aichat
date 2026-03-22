package service

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
	"github.com/google/uuid"
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
	messageStore        store.MessageStore
	roomService         RoomService
	userService         UserService
	conversationService ConversationService
	router              MessageRouter
	pender              MessagePender
}

func NewMessageService(messageStore store.MessageStore, roomService RoomService, conversationService ConversationService, router MessageRouter, userService UserService) *MessageService {
	m := &MessageService{
		messageStore:        messageStore,
		roomService:         roomService,
		conversationService: conversationService,
		router:              router,
		userService:         userService,
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
			if err := m.messageStore.Update(msg); err != nil {
				return err
			}
			// 通知发送方：接收方已收到
			ackMsg := &types.Message{
				MsgId:  msg.MsgId,  // 原消息 id，发送方用来匹配
				FromId: msg.ToId,   // 接收方
				ToId:   msg.FromId, // 发给发送方
				Type:   types.MessageTypeAck,
				SendAt: time.Now().Unix(),
			}
			return m.router.Route(ackMsg)
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
	if message.Type != types.MessageTypeFailed && message.Type != types.MessageTypeAck && message.Type != types.MessageTypeConversationUpdate && message.Type != types.MessageTypeSent {
		// 保存消息
		message.ChannelId = s.getChannelId(message)
		if err := s.messageStore.Save(message); err != nil {
			slog.Error("save message failed", "error", err)
			return err
		}

		// 立即回 Sent 确认给发送方，告知服务端已收到并保存
		sentMsg := &types.Message{
			MsgId:       message.MsgId,
			ClientMsgId: message.ClientMsgId,
			FromId:      message.ToId,
			ToId:        message.FromId,
			Type:        types.MessageTypeSent,
			SendAt:      message.SendAt,
		}
		s.router.Route(sentMsg)

		if message.RoomId != "" {
			// 群消息暂时不支持重试
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

	if online, err := s.userService.GetOnlineStatus(message.ToId); err != nil || !online {
		// 用户不在线，那就不发了
		return nil
	}

	// 普通消息
	// 存入pender 待确认
	if err := s.pender.Pend(message); err != nil {
		slog.Error("failed to pend message", "error", err)
	}

	go func() {
		s.updateConversation(message.FromId, message.ToId, message.RoomId != "", message, 0)
		s.updateConversation(message.ToId, message.FromId, message.RoomId != "", message, 1)
	}()

	// 转发给对应用户
	return s.router.Route(message)
}

func (s *MessageService) sendGroupMessage(message *types.Message) error {
	members, err := s.roomService.GetMembers(message.RoomId)
	if err != nil {
		return err
	}

	var memberIds []string
	for _, member := range members {
		memberIds = append(memberIds, member.Id)
		go func() {
			var unreadCnt int = 1
			if member.Id == message.FromId {
				unreadCnt = 0
			}
			s.updateConversation(member.Id, message.RoomId, true, message, unreadCnt)
		}()
	}
	return s.router.RouteGroup(message, memberIds)
}

func (s *MessageService) FetchChannelHistoryMessages(channelId string, limit int, currentTime int64) ([]*types.Message, error) {
	return s.messageStore.FetchChannelHistoryMessages(channelId, currentTime, limit)
}

func (s *MessageService) updateConversation(userId, peerId string, isGroup bool, message *types.Message, unreadCnt int) error {
	var conversation *types.Conversation
	var err error
	if !isGroup {
		conversation, err = s.conversationService.GetConversationByUserIdAndPeerId(userId, peerId)
	} else {
		conversation, err = s.conversationService.GetConversationByUserIdAndRoomId(userId, peerId)
	}
	if err != nil {
		return err
	}
	if conversation == nil {
		// 会话不存在则自动创建（如直接发消息未提前 open）
		var convPeerId, convRoomId string
		if isGroup {
			convRoomId = peerId // updateConversation 里群聊时 peerId 参数传的是 roomId
		} else {
			convPeerId = peerId
		}
		conversation, err = s.conversationService.CreateConversation(userId, convPeerId, convRoomId)
		if err != nil {
			return err
		}
	}
	if conversation.LastMsgId == message.MsgId {
		// 防重入
		return nil
	}
	user, err := s.userService.GetById(message.FromId)
	if err != nil {
		return err
	}

	conversation.LastMsgId = message.MsgId
	conversation.LastMsgTime = message.SendAt
	conversation.LastMsgContent = message.Content
	conversation.LastSenderName = user.Name
	conversation.UnreadCount = unreadCnt
	conversation.ChannelId = message.ChannelId
	if err := s.conversationService.UpdateConversation(conversation); err != nil {
		return err
	}

	msg := &types.Message{
		MsgId:     uuid.New().String(),
		FromId:    peerId,
		ChannelId: conversation.ChannelId,
		ToId:      userId,
		Content:   "已读",
		Type:      types.MessageTypeConversationUpdate,
		SendAt:    time.Now().Unix(),
	}

	// 发送会话更新消息
	s.router.Route(msg)
	return nil
}

func (s *MessageService) getChannelId(message *types.Message) string {
	if message.RoomId != "" {
		return types.CalcRoomChannelId(message.RoomId)
	}
	return types.CalcChannelId(message.FromId, message.ToId)
}
