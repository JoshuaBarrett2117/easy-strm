package controller

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"easy-strm/internal/domain"
	"easy-strm/internal/service"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/gin-gonic/gin"
)

type directLinkPlaybackStore struct {
	service.ShareStrmStore
	entry domain.ShareStrmEntry
}

func (store *directLinkPlaybackStore) GetStrmEntry(ctx context.Context, id string) (domain.ShareStrmEntry, error) {
	if err := ctx.Err(); err != nil {
		return domain.ShareStrmEntry{}, err
	}
	if id != store.entry.ID {
		return domain.ShareStrmEntry{}, sql.ErrNoRows
	}
	return store.entry, nil
}

func (store *directLinkPlaybackStore) LockStrmPlayback(context.Context, string) (func(), error) {
	return func() {}, nil
}

type directLinkPlaybackSettings struct{ raw string }

func (settings *directLinkPlaybackSettings) GetByKey(string) (*domain.SystemConfig, error) {
	return &domain.SystemConfig{ConfigVal: settings.raw}, nil
}
func (settings *directLinkPlaybackSettings) Upsert(string, string) error { return nil }

type directLinkPlaybackClient struct{ service.Cloud115Client }

func (client *directLinkPlaybackClient) MkdirAll115(string, int, string) (string, error) {
	return "123", nil
}
func (client *directLinkPlaybackClient) GetFileList(int, int, int, int, int, string) (*driver.FileListResp, error) {
	return &driver.FileListResp{Files: []driver.FileInfo{{Name: "fixture.mkv", FileID: "fixture", PickCode: "fixture"}}}, nil
}

func TestShareStrmDirectLinkConcurrentHandlerRedirects(t *testing.T) {
	const mapping = "56c94081-7619-4824-9e6f-ea13141599e1"
	raw, err := json.Marshal(domain.ShareStrmSettings{OutputPath: t.TempDir(), BaseURL: "http://media.example", Cloud115ID: 7, TransferPath: "/fixture"})
	if err != nil {
		t.Fatal(err)
	}
	playback := service.NewShareStrmService(&directLinkPlaybackStore{entry: domain.ShareStrmEntry{ID: mapping, ShareCode: "fixture", FileID: "fixture", FileName: "fixture.mkv"}}, &directLinkPlaybackSettings{raw: string(raw)}, &directLinkPlaybackClient{}, nil, nil, nil, nil, func(int) (*domain.Cloud115, error) { return &domain.Cloud115{ID: 7, Cookie: "fixture"}, nil }, nil)
	var applications atomic.Int32
	entered, release := make(chan struct{}, 1), make(chan struct{})
	playback.SetPlaybackDirectLink(func(context.Context, string, int, string, string) (string, error) {
		applications.Add(1)
		entered <- struct{}{}
		<-release
		return "https://cdn.example/video?t=4102444800", nil
	})
	controller := NewShareStrmController(playback)
	router := gin.New()
	router.GET("/share-strm/:id", controller.Playback)
	router.HEAD("/share-strm/:id", controller.Playback)
	const requests = 12
	start := make(chan struct{})
	var wait sync.WaitGroup
	for request := 0; request < requests; request++ {
		wait.Add(1)
		go func(request int) {
			defer wait.Done()
			<-start
			method := http.MethodGet
			if request%2 == 0 {
				method = http.MethodHead
			}
			req := httptest.NewRequest(method, "/share-strm/"+mapping, nil)
			req.Header.Set("User-Agent", "Mozilla/fixture")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != 302 || response.Header().Get("Location") != "https://cdn.example/video?t=4102444800" || response.Header().Get("Cache-Control") != "no-store" {
				t.Error("concurrent handler redirect contract failed")
			}
		}(request)
	}
	close(start)
	<-entered
	close(release)
	wait.Wait()
	if applications.Load() != 1 {
		t.Fatalf("applications=%d, want 1", applications.Load())
	}
}
