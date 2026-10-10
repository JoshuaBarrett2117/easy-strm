package controller

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type selectionControllerFake struct {
	err    error
	called bool
}

func (fake *selectionControllerFake) List(context.Context, domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error) {
	return []domain.ShareSelection{}, 0, fake.err
}
func (fake *selectionControllerFake) Detail(context.Context, string) (domain.ShareSelectionDetail, error) {
	return domain.ShareSelectionDetail{}, fake.err
}
func (fake *selectionControllerFake) Change(context.Context, domain.ShareSelectionChange) error {
	fake.called = true
	return fake.err
}
func (fake *selectionControllerFake) Refresh(context.Context) error { return fake.err }

func TestSelectionControllerCASResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		body   string
		err    error
		code   int
		called bool
	}{
		{`{"media_item_key":"w:0:0","candidate_id":2,"expected_revision":1}`, nil, 200, true},
		{`{"media_item_key":"w:0:0","candidate_id":2,"expected_revision":1}`, dao.ErrShareSelectionConflict, 409, true},
		{`{"media_item_key":"w:0:0","candidate_id":2}`, nil, 400, false},
		{`{"media_item_key":"w:0:0","candidate_id":-1,"expected_revision":1}`, nil, 400, false},
	} {
		fake := &selectionControllerFake{err: test.err}
		router := gin.New()
		controller := &ShareSelectionController{service: fake}
		router.PUT("/select", controller.Change)
		response := httptest.NewRecorder()
		request := httptest.NewRequest("PUT", "/select", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != test.code || fake.called != test.called {
			t.Fatalf("code=%d body=%s called=%v", response.Code, response.Body.String(), fake.called)
		}
	}
}
