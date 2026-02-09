package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/TheChosenGay/aichat/types"
)

const InsertUserSql = `
INSERT INTO users (id, email, name, password, is_valid, create_at, birth_at, update_at, sex)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`

const GetUserByIdSql = `
SELECT id, email, name, password, is_valid, create_at, birth_at, update_at, sex
FROM users
WHERE id = ?
`

type UserDbStore struct {
	db *sql.DB
}

func (s *UserDbStore) Save(user *types.User) error {
	result, err := s.db.Exec(InsertUserSql, user.Id, user.Email, user.Name, user.Password, user.IsValid, user.CreateAt, user.BirthAt, user.UpdateAt, user.Sex)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

func (s *UserDbStore) GetById(id string) (*types.User, error) {
	row := s.db.QueryRowContext(context.Background(), GetUserByIdSql, id)
	if err := row.Err(); err != nil {
		return nil, err
	}
	var user types.User
	err := row.Scan(&user.Id, &user.Email, &user.Name, &user.Password, &user.IsValid, &user.CreateAt, &user.BirthAt, &user.UpdateAt, &user.Sex)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
