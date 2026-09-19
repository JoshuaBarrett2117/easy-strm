package service

import (
	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlaybackLinksSeriesIdentity(t *testing.T) {
	for _, mode := range []string{"match", "missing", "upstream"} {
		t.Run(mode, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "upstream" {
					w.WriteHeader(503)
					return
				}
				switch r.URL.Path {
				case "/emby/System/Info":
					w.Write([]byte(`{"Id":"server"}`))
				case "/emby/Items":
					if r.URL.Query().Get("IncludeItemTypes") != "Series" {
						t.Errorf("错误的剧集查询: %s", r.URL)
					}
					w.Write([]byte(`{"Items":[{"Id":"series","ProviderIds":{"Tmdb":"100074"}}]}`))
				case "/emby/Shows/series/Episodes":
					if r.URL.Query().Get("Season") != "2" {
						t.Error("季号错误")
					}
					if mode == "missing" {
						w.Write([]byte(`{"Items":[]}`))
						return
					}
					w.Write([]byte(`{"Items":[{"Id":"wrong","IndexNumber":1,"ParentIndexNumber":1},{"Id":"episode","Name":"醒来","IndexNumber":1,"ParentIndexNumber":2,"ProviderIds":{"Tmdb":"999"}}]}`))
				default:
					t.Errorf("意外路径 %s", r.URL.Path)
				}
			}))
			defer remote.Close()
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectQuery("SELECT id, name, base_url").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}).AddRow(1, "test", remote.URL, "secret", true, true, time.Now(), time.Now()))
			svc := NewEmbyManagementService(dao.NewEmbyServerDAO(db), nil, nil, remote.Client())
			links, err := svc.PlaybackLinks("生物黑客", 100074, 2, 1)
			if mode == "upstream" {
				if err == nil {
					t.Fatal("连接错误不能当作未找到")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "missing" {
				if len(links) != 1 || links[0]["fallback"] != "true" {
					t.Fatal(links)
				}
				return
			}
			if len(links) != 1 || links[0]["item_id"] != "episode" {
				t.Fatalf("错误链接: %#v", links)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
