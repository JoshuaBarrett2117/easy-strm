package controller

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"easy-strm/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

func TestLoginControllerLogsExcludeAccountAndCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger.SetOutputs(&output, &output, &output, &output)
	logger.SetLevel(logger.DEBUG)
	t.Cleanup(func() { logger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr); logger.SetLevel(logger.INFO) })
	for _, branch := range []string{"missing", "mismatch", "success"} {
		output.Reset()
		controller := NewAuthController(nil)
		controller.SetGetUserByName(func(string) (*UserInfo, error) {
			if branch == "missing" {
				return nil, errors.New("synthetic-private-account")
			}
			return &UserInfo{ID: 1, Name: "synthetic-private-account", Password: "synthetic-private-password"}, nil
		})
		controller.SetVerifyPassword(func(string, string) error {
			if branch == "mismatch" {
				return errors.New("synthetic-private-password")
			}
			return nil
		})
		controller.SetGenerateToken(func(int, string) (string, error) { return "synthetic-private-token", nil })
		controller.SetSetToken(func(int, string) error { return nil })
		router := gin.New()
		router.POST("/login", controller.Login)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", "/login", strings.NewReader(`{"name":"synthetic-private-account","password":"synthetic-private-password"}`)))
		expectedStatus := 401
		if branch == "success" {
			expectedStatus = 200
		}
		if response.Code != expectedStatus {
			t.Fatal("登录测试未经过预期分支")
		}
		if strings.Contains(output.String(), "synthetic-private") {
			t.Fatal("登录控制器不得输出账号或凭据")
		}
	}
}
