package main

import "fmt"

// GetUserByID 根据ID获取用户
func GetUserByID(id int) (*User, error) {
	Debug("Getting user by ID: %d", id)
	user := &User{}
	err := db.QueryRow("SELECT id, name, password, create_time, update_time FROM t_user WHERE id = $1", id).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		Error("Failed to get user by ID %d: %v", id, err)
		return nil, err
	}
	Debug("Found user by ID %d", id)
	return user, nil
}

// GetUserByName 根据用户名获取用户

func GetUserByName(name string) (*User, error) {
	Debug("Getting user by name")
	user := &User{}
	err := db.QueryRow("SELECT id, name, password, create_time, update_time FROM t_user WHERE name = $1", name).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		Debug("User not found by name")
		return nil, err
	}
	Debug("Found user: ID %d", user.ID)
	return user, nil
}

// CreateUser 创建新用户

func CreateUser(name, password string) (*User, error) {
	Debug("Creating new user")
	// 前端已经对密码进行了md5加密，直接存储
	user := &User{}
	err := db.QueryRow("INSERT INTO t_user (name, password) VALUES ($1, $2) RETURNING id, name, password, create_time, update_time", name, password).Scan(&user.ID, &user.Name, &user.Password, &user.CreateTime, &user.UpdateTime)
	if err != nil {
		Error("Failed to create user: error_type=%T", err)
		return nil, err
	}
	Info("Created new user (ID: %d)", user.ID)
	return user, nil
}

// VerifyPassword 验证密码（前端已发送MD5哈希值，直接比较）

func VerifyPassword(hashedPassword, password string) error {
	if hashedPassword != password {
		Debug("Password verification failed")
		return fmt.Errorf("invalid password")
	}
	return nil
}
