package types

type Room struct {
	RoomId   string `validate:"required,uuid"`
	Name     string `validate:"required,min=3,max=32"`
	OwnerId  string `validate:"required,uuid"`
	CreateAt int64  `validate:"required"`
}
