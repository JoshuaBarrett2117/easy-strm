package dao

import (
	"database/sql"

	"github.com/go-redis/redis/v8"
)

var DB *sql.DB
var RedisClient *redis.Client

// Init 初始化DAO层数据库连接
func Init(database *sql.DB, redis *redis.Client) {
	DB = database
	RedisClient = redis
}
