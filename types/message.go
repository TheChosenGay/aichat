package types

type MessageType int

const (
	MessageTypeText MessageType = iota
	MessageTypeImage
	MessageTypeAudio
	MessageTypeSystem
)

type Message struct {
	MsgId       string      `validate:"required,uuid"`
	FromId      string      `validate:"required,uuid"`
	ToId        string      `validate:"required,uuid"`
	Content     string      `validate:"required,min=1,max=1000"`
	Type        MessageType `validate:"required"`
	SendAt      int64       `validate:"required,int64,gt=0"`
	IsDelivered bool        `validate:"required, boolean"`
}
