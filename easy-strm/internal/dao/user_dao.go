package dao

import (
"database/sql"
"fmt"

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

func (u *UserDAO) GetByID(id int) (*domain.User, error) {
user := &domain.User{}
var userID int64
err := db.QueryRow(
"SELECT id, name, password, create_time, update_time FROM t_user WHERE id = $1",
id,
).Scan(&userID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
if err != nil {
if err == sql.ErrNoRows {
return nil, nil
}
logger.Errorf("UserDAO[GetByID] 查询用户失败: %v", err)
return nil, err
}
user.ID = int(userID)
return user, nil
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

func (u *UserDAO) Create(name, password string) (*domain.User, error) {
user := &domain.User{}
var userID int64
err := db.QueryRow(
"INSERT INTO t_user (name, password) VALUES ($1, $2) RETURNING id, name, password, create_time, update_time",
name, password,
).Scan(&userID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
if err != nil {
logger.Errorf("UserDAO[Create] 创建用户失败: %v", err)
return nil, err
}
user.ID = int(userID)
return user, nil
}

func (u *UserDAO) UpdatePassword(name, newPassword string) error {
result, err := db.Exec("UPDATE t_user SET password = $1, update_time = NOW() WHERE name = $2", newPassword, name)
if err != nil {
logger.Errorf("UserDAO[UpdatePassword] 更新密码失败: %v", err)
return err
}
rows, _ := result.RowsAffected()
if rows == 0 {
return fmt.Errorf("用户不存在")
}
return nil
}