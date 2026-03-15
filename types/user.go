package types

type User struct {
	Id        string `validate:"required,uuid" redis:"user:id"`
	Email     string `validate:"required,email" redis:"user:email"`
	Password  string `validate:"required,min=8,max=32" redis:"-" json:"-"`
	Name      string `validate:"required,min=3,max=32" redis:"user:name"`
	IsValid   bool   `validate:"required" redis:"user:is_valid"`  // true: valid, false: invalid
	CreateAt  int64  `validate:"required" redis:"user:create_at"` // unix timestamp
	AvatarUrl string `validate:"omitempty" redis:"user:avatar_url"`
	// optional fields
	BirthAt  int64 `validate:"omitempty" redis:"user:birth_at"`  // unix timestamp
	UpdateAt int64 `validate:"omitempty" redis:"user:update_at"` // unix timestamp
	Sex      bool  `validate:"omitempty" redis:"user:sex"`       // true: male, false: female
}

func NewUser(id, email, password, name string, sex bool, birthAt, CreateAt int64) *User {
	return &User{
		Id:       id,
		Email:    email,
		Password: password,
		Name:     name,
		IsValid:  true,
		CreateAt: CreateAt,
		Sex:      sex,
		BirthAt:  birthAt,
		UpdateAt: CreateAt,
	}
}
