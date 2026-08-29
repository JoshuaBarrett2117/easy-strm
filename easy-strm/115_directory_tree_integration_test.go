package main

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestDirectoryTreeDownloadSignatureReal 使用已配置账号验证115目录树签名下载链路。
// 默认跳过；设置 EASY_STRM_REAL_115_TEST=1 后执行，配置 ID 可通过 EASY_STRM_STRM_CONFIG_ID 指定。
func TestDirectoryTreeDownloadSignatureReal(t *testing.T) {
	if os.Getenv("EASY_STRM_REAL_115_TEST") != "1" {
		t.Skip("未启用真实115签名测试")
	}

	config := LoadConfig()
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable client_encoding=UTF8",
		config.PostgreSQL.Host,
		config.PostgreSQL.Port,
		config.PostgreSQL.User,
		config.PostgreSQL.Password,
		config.PostgreSQL.Database,
	)
	database, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer database.Close()
	if err := database.Ping(); err != nil {
		t.Fatalf("连接数据库失败: %v", err)
	}
	db = database

	configID := 2
	if rawConfigID := os.Getenv("EASY_STRM_STRM_CONFIG_ID"); rawConfigID != "" {
		parsedConfigID, parseErr := strconv.Atoi(rawConfigID)
		if parseErr != nil || parsedConfigID <= 0 {
			t.Fatalf("EASY_STRM_STRM_CONFIG_ID 无效: %q", rawConfigID)
		}
		configID = parsedConfigID
	}

	strmConfig, err := GetStrmConfigByID(configID)
	if err != nil {
		t.Fatalf("读取STRM配置失败: %v", err)
	}
	cloud115, err := GetCloud115ByID(strmConfig.Cloud115Id)
	if err != nil {
		t.Fatalf("读取115账号失败: %v", err)
	}

	client := NewClient(config)
	cid, err := client.GetCIDByPath(strmConfig.NetDiskPath, cloud115.ID, cloud115.Cookie)
	if err != nil {
		t.Fatalf("解析网盘目录CID失败: %v", err)
	}
	exportResponse, err := client.ExportDirectoryTree115(cid, "U_1_"+cid, cloud115.Cookie)
	if err != nil {
		t.Fatalf("导出目录树失败: %v", err)
	}
	if !exportResponse.State {
		t.Fatalf("115拒绝目录树导出: %s", exportResponse.Message)
	}

	exportID := exportResponse.Data.ExportID.String()
	var pickCode string
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		statusResponse, statusErr := client.GetExportDirectoryTreeStatus(exportID, cloud115.Cookie)
		if statusErr == nil {
			if data := statusResponse.GetFirstData(); data != nil {
				if data.Status == 2 && data.PickCode != "" {
					pickCode = data.PickCode
					break
				}
				if data.Status == 3 || data.Status == -1 {
					t.Fatalf("目录树导出失败，状态: %d", data.Status)
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	if pickCode == "" {
		t.Fatal("等待目录树导出超时")
	}

	content, err := client.DownloadDirectoryTreeFile(pickCode, cloud115.ID, cloud115.Cookie)
	if err != nil {
		t.Fatalf("真实115签名下载失败: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("真实115目录树内容为空")
	}
	if _, err := Parse115DirTreeFile(content); err != nil {
		t.Fatalf("真实115目录树解析失败: %v", err)
	}

	t.Logf("真实115签名下载通过，配置ID=%d，目录树大小=%d字节", configID, len(content))
}
