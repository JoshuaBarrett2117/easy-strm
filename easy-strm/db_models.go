package main

import (
	"easy-strm/internal/domain"
	"time"
)

type User struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Password   string    `json:"password"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// Cloud115 115云账号信息。
type Cloud115 struct {
	ID                int        `json:"id"`
	Name              string     `json:"name"`
	Cookie            string     `json:"cookie"`
	CookieSource      string     `json:"cookie_source"`
	RefreshToken      string     `json:"refresh_token"`
	AccessToken       string     `json:"access_token"`
	ExpiresIn         int        `json:"expires_in"`
	TransferAccountID int        `json:"transfer_account_id"`
	TransferDirectory string     `json:"transfer_directory"`
	AccountType       string     `json:"account_type"`
	QuotaUsed         int64      `json:"quota_used"`
	Priority          int        `json:"priority"`
	Status            string     `json:"status"`
	CoolingStartTime  *time.Time `json:"cooling_start_time"`
	TransferMethod    string     `json:"transfer_method"`
	AlistUrl          string     `json:"alist_url"`
	AlistToken        string     `json:"alist_token"`
	CreateTime        time.Time  `json:"create_time"`
	UpdateTime        time.Time  `json:"update_time"`
}

// StrmConfig STRM文件配置信息。
type StrmConfig = domain.StrmConfig

// SystemConfig 系统配置信息。
type SystemConfig struct {
	ID         int       `json:"id"`
	ConfigKey  string    `json:"config_key"` // 配置键
	ConfigVal  string    `json:"config_val"` // 配置值
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// StrmFile 已生成的STRM文件记录。
type StrmFile struct {
	ID            int       `json:"id"`
	StrmConfigID  int       `json:"strm_config_id"`  // 关联的STRM配置ID
	FileName      string    `json:"file_name"`       // 文件名
	FilePath      string    `json:"file_path"`       // 相对路径
	PickCode      string    `json:"pick_code"`       // 115文件pickcode
	Sha1          string    `json:"sha1"`            // 文件SHA1
	FileSize      int64     `json:"file_size"`       // 文件大小
	LocalStrmPath string    `json:"local_strm_path"` // 本地STRM文件路径
	CreateTime    time.Time `json:"create_time"`
	UpdateTime    time.Time `json:"update_time"`
}

// CronTask 定时任务配置。
type CronTask = domain.CronTask
