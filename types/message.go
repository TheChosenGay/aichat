package types

type MessageType int

const (
	MessageTypeText MessageType = iota
	MessageTypeImage
	MessageTypeAudio
	MessageTypeSystem
	MessageTypeAck                // 接收方已收到
	MessageTypeFailed             // 消息发送失败
	MessageTypeConversationUpdate // 会话更新
	MessageTypeSent               // 服务端已收到并保存（发给发送方的确认）
)

type Message struct {
	MsgId       string      `json:"msg_id" validate:"omitempty,uuid"`
	ClientMsgId string      `json:"client_msg_id" validate:"omitempty"`
	FromId      string      `json:"from_id" validate:"required"`
	ChannelId   string      `json:"channel_id" validate:"omitempty"`
	ToId        string      `json:"to_id" validate:"omitempty,uuid"`
	RoomId      string      `json:"room_id" validate:"omitempty,uuid"`
	Content     string      `json:"content" validate:"omitempty,max=1000"`
	Type        MessageType `json:"type" validate:""`
	SendAt      int64       `json:"send_at" validate:"omitempty"`
	IsDelivered bool        `json:"is_delivered"`
}
