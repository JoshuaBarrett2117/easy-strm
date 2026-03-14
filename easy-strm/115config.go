package main

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	AppID       string `yaml:"app_id"`
	AppSecret   string `yaml:"app_secret"`
	RedirectURI string `yaml:"redirect_uri"`
	BaseURL     string `yaml:"base_url"`
	ServerURL   string `yaml:"server_url"` // 本服务的访问地址，用于生成STRM文件中的直链URL
	JWTSecret   string `yaml:"jwt_secret"`
	PostgreSQL  struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Database string `yaml:"database"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	} `yaml:"postgresql"`
	Redis struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Password string `yaml:"password"`
	} `yaml:"redis"`
	Log struct {
		OutputType string `yaml:"output_type"` // console or file
		LogLevel   string `yaml:"log_level"`   // 日志级别：DEBUG, INFO, WARN, ERROR
		LogDir     string `yaml:"log_dir"`     // 日志目录
		KeepDays   int    `yaml:"keep_days"`   // 日志保留天数
	} `yaml:"log"`
}

func LoadConfig() *Config {
	// 默认配置
	config := &Config{
		BaseURL:   "https://proapi.115.com",
		ServerURL: "http://localhost:8082", // 默认本服务地址
		JWTSecret: "default_jwt_secret",
		PostgreSQL: struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			Database string `yaml:"database"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
		}{},
		Redis: struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			Password string `yaml:"password"`
		}{},
		Log: struct {
			OutputType string `yaml:"output_type"`
			LogLevel   string `yaml:"log_level"`
			LogDir     string `yaml:"log_dir"`
			KeepDays   int    `yaml:"keep_days"`
		}{OutputType: "console", LogLevel: "INFO", LogDir: "logs", KeepDays: 7},
	}

	// 从config.yaml读取配置
	if _, err := os.Stat("config.yaml"); err == nil {
		data, err := os.ReadFile("config.yaml")
		if err == nil {
			err := yaml.Unmarshal(data, config)
			if err != nil {
				panic(err)
			}
		}
	}

	// 环境变量优先级高于配置文件
	if appID := os.Getenv("APP_ID"); appID != "" {
		config.AppID = appID
	}
	if appSecret := os.Getenv("APP_SECRET"); appSecret != "" {
		config.AppSecret = appSecret
	}
	if redirectURI := os.Getenv("REDIRECT_URI"); redirectURI != "" {
		config.RedirectURI = redirectURI
	}
	if baseURL := os.Getenv("BASE_URL"); baseURL != "" {
		config.BaseURL = baseURL
	}
	if serverURL := os.Getenv("SERVER_URL"); serverURL != "" {
		config.ServerURL = serverURL
	}
	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
		config.JWTSecret = jwtSecret
	}
	if pgHost := os.Getenv("PG_HOST"); pgHost != "" {
		config.PostgreSQL.Host = pgHost
	}
	if pgPort := os.Getenv("PG_PORT"); pgPort != "" {
		config.PostgreSQL.Port = parseInt(pgPort)
	}
	if pgDatabase := os.Getenv("PG_DATABASE"); pgDatabase != "" {
		config.PostgreSQL.Database = pgDatabase
	}
	if pgUser := os.Getenv("PG_USER"); pgUser != "" {
		config.PostgreSQL.User = pgUser
	}
	if pgPassword := os.Getenv("PG_PASSWORD"); pgPassword != "" {
		config.PostgreSQL.Password = pgPassword
	}
	if redisHost := os.Getenv("REDIS_HOST"); redisHost != "" {
		config.Redis.Host = redisHost
	}
	if redisPort := os.Getenv("REDIS_PORT"); redisPort != "" {
		config.Redis.Port = parseInt(redisPort)
	}
	if redisPassword := os.Getenv("REDIS_PASSWORD"); redisPassword != "" {
		config.Redis.Password = redisPassword
	}

	return config
}

func parseInt(s string) int {
	result := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}
