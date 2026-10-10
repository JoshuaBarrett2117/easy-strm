package controller

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

type selectionRound2Store struct {
	mu       sync.Mutex
	revision int64
	err      error
}

func (store *selectionRound2Store) CheckSelectionSchema(context.Context) error { return store.err }
func (store *selectionRound2Store) ListSelections(context.Context, domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error) {
	return []domain.ShareSelection{}, 0, store.err
}
func (store *selectionRound2Store) SelectionDetail(context.Context, string) (domain.ShareSelectionDetail, error) {
	return domain.ShareSelectionDetail{Selection: domain.ShareSelection{ItemKey: "fixture:0:0", RelativePath: "fixed.strm"}, Candidates: []domain.ShareCandidate{}}, store.err
}
func (store *selectionRound2Store) SyncSelections(context.Context, string) error { return store.err }
func (store *selectionRound2Store) ChangeSelection(_ context.Context, change domain.ShareSelectionChange) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.err != nil {
		return store.err
	}
	if change.ExpectedRevision != store.revision {
		return dao.ErrShareSelectionConflict
	}
	store.revision++
	return nil
}

func selectionRound2Router(store *selectionRound2Store) *gin.Engine {
	gin.SetMode(gin.TestMode)
	auth := NewAuthController(nil)
	auth.SetVerifyTokenAndReturnUserID(func(token, secret string) (int, error) {
		if token != "fixture-token" {
			return 0, errors.New("invalid fixture token")
		}
		return 1, nil
	})
	auth.SetGetToken(func(int) (string, error) { return "fixture-token", nil })
	auth.SetGenerateToken(func(int, string) (string, error) { return "fixture-token", nil })
	router := gin.New()
	group := router.Group("/api")
	group.Use(auth.JWTMiddleware())
	controller := NewShareSelectionController(service.NewShareSelectionService(store))
	group.GET("/selections", controller.List)
	group.GET("/selections/detail", controller.Detail)
	group.PUT("/selections", controller.Change)
	group.POST("/selections/refresh", controller.Refresh)
	return router
}

func TestSelectionAuthenticatedControllerReadRefreshSuccessAndErrors(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path string
		err                error
		expected           int
	}{
		{"list", "GET", "/api/selections?page=1&page_size=20", nil, 200},
		{"invalid-page", "GET", "/api/selections?page=-1", nil, 400},
		{"invalid-type", "GET", "/api/selections?page=no", nil, 400},
		{"schema-error", "GET", "/api/selections", errors.New("未安装 v47"), 400},
		{"detail", "GET", "/api/selections/detail?media_item_key=fixture:0:0", nil, 200},
		{"empty-key", "GET", "/api/selections/detail", nil, 400},
		{"not-found", "GET", "/api/selections/detail?media_item_key=fixture:0:0", sql.ErrNoRows, 404},
		{"refresh", "POST", "/api/selections/refresh", nil, 200},
		{"refresh-error", "POST", "/api/selections/refresh", errors.New("刷新失败"), 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router := selectionRound2Router(&selectionRound2Store{err: scenario.err})
			for _, authenticated := range []bool{false, true} {
				request := httptest.NewRequest(scenario.method, scenario.path, nil)
				if authenticated {
					request.Header.Set("Authorization", "Bearer fixture-token")
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				expected := 401
				if authenticated {
					expected = scenario.expected
				}
				if response.Code != expected {
					t.Fatalf("code=%d body=%s", response.Code, response.Body.String())
				}
				var envelope map[string]interface{}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if expected == 200 && envelope["state"] != true {
					t.Fatal("成功契约错误")
				}
				if expected != 200 && envelope["error"] == nil {
					t.Fatal("错误契约错误")
				}
				if scenario.name == "list" && authenticated {
					data := envelope["data"].(map[string]interface{})
					if data["total"] != float64(0) || data["data"] == nil {
						t.Fatal("空列表 data/total 不稳定")
					}
				}
			}
		})
	}
}

func TestSelectionConcurrentHTTPCASOneWinner(t *testing.T) {
	store := &selectionRound2Store{revision: 7}
	router := selectionRound2Router(store)
	start := make(chan struct{})
	results := make(chan int, 2)
	var requests sync.WaitGroup
	for attempt := 0; attempt < 2; attempt++ {
		requests.Add(1)
		go func() {
			defer requests.Done()
			<-start
			request := httptest.NewRequest("PUT", "/api/selections", strings.NewReader(`{"media_item_key":"fixture:0:0","candidate_id":2,"expected_revision":7}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer fixture-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			results <- response.Code
		}()
	}
	close(start)
	requests.Wait()
	close(results)
	counts := map[int]int{}
	for status := range results {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 || store.revision != 8 {
		t.Fatalf("CAS 竞态未拒绝旧版本: %v revision=%d", counts, store.revision)
	}
	t.Log("authenticated HTTP + actual service validation; concurrent CAS store is an explicit in-memory fixture, not PostgreSQL")
}
