package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/TheChosenGay/aichat/types"
)

const InsertUserSql = `
INSERT INTO users (id, email, name, password, is_valid, create_at, birth_at, update_at, sex, avatar_url)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

const GetUserByIdSql = `
SELECT id, email, name, password, is_valid, create_at, birth_at, update_at, sex, avatar_url
FROM users
WHERE id = ?
`

const GetUserByEmailSql = `
SELECT id, email, name, password, is_valid, create_at, birth_at, update_at, sex, avatar_url
FROM users
WHERE email = ?
`

const ListUserSql = `
SELECT id, email, name, is_valid, create_at, birth_at, update_at, sex, avatar_url
FROM users
ORDER BY create_at DESC
LIMIT ?
`

const UpdateAvatarUrlSql = `
UPDATE users SET avatar_url = ? WHERE id = ?
`

const GetAvatarUrlSql = `
SELECT avatar_url FROM users WHERE id = ?
`

type UserDbStore struct {
	db *sql.DB
}

func NewUserDbStore(db *sql.DB) *UserDbStore {
	return &UserDbStore{
		db: db,
	}
}

func (s *UserDbStore) Save(user *types.User) error {
	result, err := s.db.Exec(InsertUserSql, user.Id, user.Email, user.Name, user.Password, user.IsValid, user.CreateAt, user.BirthAt, user.UpdateAt, user.Sex, user.AvatarUrl)
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
	err := row.Scan(&user.Id, &user.Email, &user.Name, &user.Password, &user.IsValid, &user.CreateAt, &user.BirthAt, &user.UpdateAt, &user.Sex, &user.AvatarUrl)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserDbStore) GetByEmail(email string) (*types.User, error) {
	row := s.db.QueryRowContext(context.Background(), GetUserByEmailSql, email)
	if err := row.Err(); err != nil {
		return nil, err
	}
	var user types.User
	err := row.Scan(&user.Id, &user.Email, &user.Name, &user.Password, &user.IsValid, &user.CreateAt, &user.BirthAt, &user.UpdateAt, &user.Sex, &user.AvatarUrl)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserDbStore) List(limit int) ([]*types.User, error) {
	rows, err := s.db.QueryContext(context.Background(), ListUserSql, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*types.User
	for rows.Next() {
		var user types.User
		err := rows.Scan(&user.Id, &user.Email, &user.Name, &user.IsValid, &user.CreateAt, &user.BirthAt, &user.UpdateAt, &user.Sex, &user.AvatarUrl)
		if err != nil {
			return nil, err
		}
		users = append(users, &user)
	}
	return users, nil
}

func (s *UserDbStore) GetUsersByIds(ids []string) ([]*types.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// 动态构建 IN (?, ?, ...) 占位符
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1] // 去掉末尾逗号
	sql := fmt.Sprintf(`
SELECT id, email, name, password, is_valid, create_at, birth_at, update_at, sex, avatar_url
FROM users WHERE id IN (%s)`, placeholders)

	// 把 []string 展开为 []any
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*types.User
	for rows.Next() {
		var user types.User
		err := rows.Scan(&user.Id, &user.Email, &user.Name, &user.Password, &user.IsValid, &user.CreateAt, &user.BirthAt, &user.UpdateAt, &user.Sex, &user.AvatarUrl)
		if err != nil {
			return nil, err
		}
		users = append(users, &user)
	}
	return users, nil
}

func (s *UserDbStore) UpdateAvatarUrl(userId string, avatarUrl string) error {
	_, err := s.db.Exec(UpdateAvatarUrlSql, avatarUrl, userId)
	if err != nil {
		return err
	}
	return nil
}

func (s *UserDbStore) GetAvatarUrl(userId string) (string, error) {
	row := s.db.QueryRowContext(context.Background(), GetAvatarUrlSql, userId)
	if err := row.Err(); err != nil {
		return "", err
	}
	var avatarUrl string
	err := row.Scan(&avatarUrl)
	if err != nil {
		return "", err
	}
	return avatarUrl, nil
}
