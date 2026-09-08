package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	_ "github.com/lib/pq"
	"gopkg.in/yaml.v2"
)

// TestEmbyUserAccessLiveRepair 仅在明确指定目标用户时，将其既有数字媒体库选择转换为真实 Guid。
// 默认跳过；不新增权限范围，保持原有开关和非媒体库设置。
func TestEmbyUserAccessLiveRepair(t *testing.T) {
	userID := os.Getenv("EMBY_REPAIR_USER_ID")
	if userID == "" {
		t.Skip("未指定真实 Emby 修复目标")
	}
	var cfg struct {
		PostgreSQL struct {
			Host     string
			Port     int
			User     string
			Password string
			Database string
		} `yaml:"postgresql"`
	}
	b, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	p := cfg.PostgreSQL
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", p.Host, p.Port, p.User, p.Password, p.Database))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var server domain.EmbyServer
	err = db.QueryRow("SELECT id,name,base_url,api_key FROM t_emby_server WHERE is_default=true AND enabled=true").Scan(&server.ID, &server.Name, &server.BaseURL, &server.APIKey)
	if err != nil {
		t.Fatal(err)
	}
	manager, mock, _, cleanup := newEmbyManagementTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()
	manager.httpClient = &http.Client{Timeout: 20 * time.Second}
	var user domain.EmbyUser
	if err = manager.requestJSON(&server, http.MethodGet, "/emby/Users/"+userID, nil, nil, &user); err != nil {
		t.Fatal(err)
	}
	if user.Policy.EnableAllFolders {
		t.Fatal("目标是全部媒体库，无需修复")
	}
	normalized, err := manager.normalizeUserPolicy(&server, user.Policy)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(user.Policy.EnabledFolders)
	after, _ := json.Marshal(normalized.EnabledFolders)
	t.Logf("目标用户 %s；原选择=%s；映射结果=%s", user.Name, before, after)
	mock.ExpectQuery(regexp.QuoteMeta(testEmbyServerSelect)).WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}).
			AddRow(server.ID, server.Name, server.BaseURL, server.APIKey, true, true, time.Now(), time.Now()))
	if _, err = manager.UpdateUser(1, userID, "", &user.Policy); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	t.Log("真实 Emby 保存及回读验证通过")
}
