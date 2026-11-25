package dao

import (
	"database/sql"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	_ "github.com/lib/pq"
)

var db *sql.DB

func InitDAO(database *sql.DB) {
	db = database
}

type UserDAO struct{}

func NewUserDAO() *UserDAO {
	return &UserDAO{}
}

func (u *UserDAO) GetByName(name string) (*domain.User, error) {
	user := &domain.User{}
	var userID int64
	err := db.QueryRow(
		"SELECT id, name, password, create_time, update_time FROM t_user WHERE name = $1",
		name,
	).Scan(&userID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		logger.Errorf("UserDAO[GetByName] 查询用户失败: %v", err)
		return nil, err
	}
	user.ID = int(userID)
	return user, nil
}
