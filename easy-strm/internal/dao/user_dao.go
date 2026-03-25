package dao

import (
	"database/sql"
	"fmt"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	_ "github.com/lib/pq"
)

var db *sql.DB

// InitDAO 初始化DAO层数据库连接
func InitDAO(database *sql.DB) {
	db = database
}

// UserDAO 用户数据访问层
type UserDAO struct{}

// NewUserDAO 创建用户DAO实例
func NewUserDAO() *UserDAO {
	return &UserDAO{}
}

// GetByID 根据ID获取用户
func (u *UserDAO) GetByID(id int) (*domain.User, error) {
	user := &domain.User{}
	err := db.QueryRow(
		"SELECT id, name, password, create_time, update_time FROM t_user WHERE id = $1",
		id,
	).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		logger.Errorf("UserDAO[GetByID] 查询用户失败: %v", err)
		return nil, err
	}
	return user, nil
}

// GetByName 根据用户名获取用户
func (u *UserDAO) GetByName(name string) (*domain.User, error) {
	user := &domain.User{}
	err := db.QueryRow(
		"SELECT id, name, password, create_time, update_time FROM t_user WHERE name = $1",
		name,
	).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		Error("UserDAO[GetByName] 查询用户失败: %v", err)
		return nil, err
	}
	return user, nil
}

// Create 创建新用户
func (u *UserDAO) Create(name, password string) (*domain.User, error) {
	user := &domain.User{}
	err := db.QueryRow(
		"INSERT INTO t_user (name, password) VALUES ($1, $2) RETURNING id, name, password, create_time, update_time",
		name, password,
	).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		logger.Errorf("UserDAO[Create] 创建用户失败: %v", err)
		return nil, err
	}
	logger.Infof("UserDAO[Create] 创建用户成功: %s (ID: %d)", name, user.ID)
	return user, nil
}
