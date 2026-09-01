package main

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestCrossAccountCopyReal 使用真实账号验证主号到小号的公共秒传组件。
// 默认跳过；设置 EASY_STRM_REAL_115_COPY_TEST=1 后执行。
func TestCrossAccountCopyReal(t *testing.T) {
	if os.Getenv("EASY_STRM_REAL_115_COPY_TEST") != "1" {
		t.Skip("未启用真实115跨账号复制测试")
	}

	config := LoadConfig()
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable client_encoding=UTF8",
		config.PostgreSQL.Host, config.PostgreSQL.Port, config.PostgreSQL.User,
		config.PostgreSQL.Password, config.PostgreSQL.Database)
	database, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer database.Close()
	if err := database.Ping(); err != nil {
		t.Fatalf("连接数据库失败: %v", err)
	}
	db = database

	source, err := GetCloud115ByName("115主号")
	if err != nil {
		t.Fatalf("读取源账号失败: %v", err)
	}
	target, err := GetCloud115ByName("115小号")
	if err != nil {
		t.Fatalf("读取目标账号失败: %v", err)
	}
	client := NewClient(config)
	const fileName = "21世纪大君夫人 - S01E01 - 我说过要嫁入王室吗？.mkv"

	sourceCID := mustResolve115CID(t, client, "/EmbyCache", source)
	sourceFile := mustFind115File(t, client, sourceCID, fileName, source)
	targetCID := mustResolve115CID(t, client, "/影视资源-未整理", target)

	newPickCode, err := client.RapidTransferFileByMetadata(
		sourceFile.FileID, sourceFile.PickCode, sourceFile.Sha1, int64(sourceFile.Size),
		source.ID, source.Cookie, strconv.Itoa(targetCID), target.ID, target.Cookie, sourceFile.Name,
	)
	if err != nil {
		t.Fatalf("真实跨账号秒传失败: %v", err)
	}
	if newPickCode == "" {
		t.Fatal("真实跨账号秒传未返回pickcode")
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		files, listErr := client.GetFileList(targetCID, 1, 0, 1000, target.ID, target.Cookie)
		if listErr == nil {
			for _, file := range files.Files {
				if file.Name == fileName && file.Sha1 == sourceFile.Sha1 {
					t.Logf("跨账号秒传验证成功: source_file_id=%s target_file_id=%s", sourceFile.FileID, file.FileID)
					return
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("秒传接口返回成功，但目标目录未找到文件: last_error=%v", listErr)
		}
		time.Sleep(time.Second)
	}
}

func mustResolve115CID(t *testing.T, client *Client, path string, account *Cloud115) int {
	t.Helper()
	cid, err := client.GetCIDByPath(path, account.ID, account.Cookie)
	if err != nil {
		t.Fatalf("解析115目录%s失败: %v", path, err)
	}
	parsed, err := strconv.Atoi(cid)
	if err != nil {
		t.Fatalf("115目录CID无效: path=%s cid=%s", path, cid)
	}
	return parsed
}

func mustFind115File(t *testing.T, client *Client, cid int, name string, account *Cloud115) *struct {
	FileID   string
	PickCode string
	Sha1     string
	Size     int64
	Name     string
} {
	t.Helper()
	files, err := client.GetFileList(cid, 1, 0, 1000, account.ID, account.Cookie)
	if err != nil {
		t.Fatalf("读取115源目录失败: %v", err)
	}
	for _, file := range files.Files {
		if file.Name == name {
			return &struct {
				FileID   string
				PickCode string
				Sha1     string
				Size     int64
				Name     string
			}{file.FileID, file.PickCode, file.Sha1, int64(file.Size), file.Name}
		}
	}
	t.Fatalf("源目录未找到文件: %s", name)
	return nil
}
