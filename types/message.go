package types

type MessageType int

const (
	MessageTypeText MessageType = iota
	MessageTypeImage
	MessageTypeAudio
	MessageTypeSystem
	MessageTypeAck    // ACK 消息类型
	MessageTypeFailed // 消息发送失败类型
)

type Message struct {
	MsgId       string      `validate:"required,uuid"`
	FromId      string      `validate:"required,uuid"`
	ToId        string      `validate:"omitempty,uuid"`
	RoomId      string      `validate:"omitempty,uuid"`
	Content     string      `validate:"required,min=1,max=1000"`
	Type        MessageType `validate:""`
	SendAt      int64       `validate:"gt=0"`
	IsDelivered bool
}
