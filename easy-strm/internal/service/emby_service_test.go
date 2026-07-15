package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easy-strm/internal/domain"
)

type mockSystemConfigDAO struct {
	configs map[string]string
}

func (m *mockSystemConfigDAO) GetByKey(key string) (*domain.SystemConfig, error) {
	val, ok := m.configs[key]
	if !ok {
		return nil, nil
	}
	return &domain.SystemConfig{ConfigKey: key, ConfigVal: val}, nil
}

func (m *mockSystemConfigDAO) GetAll() ([]*domain.SystemConfig, error) {
	var result []*domain.SystemConfig
	for k, v := range m.configs {
		result = append(result, &domain.SystemConfig{ConfigKey: k, ConfigVal: v})
	}
	return result, nil
}

func (m *mockSystemConfigDAO) Upsert(key, value string) error {
	m.configs[key] = value
	return nil
}

func (m *mockSystemConfigDAO) BatchUpsert(configs map[string]string) error {
	for k, v := range configs {
		m.configs[k] = v
	}
	return nil
}

func TestEmbyServiceGetConfig(t *testing.T) {
	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{
			"emby_url":     "http://localhost:8096",
			"emby_api_key": "test-api-key",
			"emby_enabled": "true",
		},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	enabled := embySvc.IsEnabled()
	if !enabled {
		t.Error("Expected Emby to be enabled")
	}
}

func TestEmbyServiceIsDisabled(t *testing.T) {
	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{
			"emby_url":     "http://localhost:8096",
			"emby_api_key": "test-api-key",
			"emby_enabled": "false",
		},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	enabled := embySvc.IsEnabled()
	if enabled {
		t.Error("Expected Emby to be disabled")
	}
}

func TestEmbyServiceNoConfig(t *testing.T) {
	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	enabled := embySvc.IsEnabled()
	if enabled {
		t.Error("Expected Emby to be disabled when no config")
	}
}

func TestEmbyServiceCheckConnectionSuccess(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/System/Info" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		info := EmbySystemInfo{
			ID:         "server-1",
			ServerName: "Test Emby",
			Version:    "4.7.0",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	}))
	defer mockServer.Close()

	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{
			"emby_url":     mockServer.URL,
			"emby_api_key": "test-api-key",
			"emby_enabled": "true",
		},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	connected, info, err := embySvc.CheckConnection()
	if err != nil {
		t.Fatalf("CheckConnection failed: %v", err)
	}
	if !connected {
		t.Error("Expected connection to succeed")
	}
	if info == nil {
		t.Fatal("Expected system info")
	}
	if info.ServerName != "Test Emby" {
		t.Errorf("Expected server name 'Test Emby', got '%s'", info.ServerName)
	}
}

func TestEmbyServiceCheckConnectionFailure(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer mockServer.Close()

	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{
			"emby_url":     mockServer.URL,
			"emby_api_key": "invalid-key",
			"emby_enabled": "true",
		},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	connected, _, err := embySvc.CheckConnection()
	if err != nil {
		return
	}
	if connected {
		t.Error("Expected connection to fail")
	}
}

func TestEmbyServiceListLibraries(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Library/VirtualFolders" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		libraries := []EmbyVirtualFolderInfo{
			{Name: "Movies", ItemID: "lib-1", CollectionType: "movies", Path: "/data/movies"},
			{Name: "TV Shows", ItemID: "lib-2", CollectionType: "tvshows", Path: "/data/tv"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(libraries)
	}))
	defer mockServer.Close()

	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{
			"emby_url":     mockServer.URL,
			"emby_api_key": "test-api-key",
			"emby_enabled": "true",
		},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	libraries, err := embySvc.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries failed: %v", err)
	}
	if len(libraries) != 2 {
		t.Errorf("Expected 2 libraries, got %d", len(libraries))
	}
	if libraries[0].Name != "Movies" {
		t.Errorf("Expected first library name 'Movies', got '%s'", libraries[0].Name)
	}
}

func TestEmbyServiceRefreshLibrary(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/emby/Library/VirtualFolders" {
			libraries := []EmbyVirtualFolderInfo{
				{Name: "Movies", ItemID: "lib-1"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(libraries)
		} else if r.URL.Path == "/emby/Items/lib-1/Refresh" {
			if r.Method != "POST" {
				t.Errorf("unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer mockServer.Close()

	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{
			"emby_url":     mockServer.URL,
			"emby_api_key": "test-api-key",
			"emby_enabled": "true",
		},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	result, err := embySvc.RefreshLibrary("lib-1")
	if err != nil {
		t.Fatalf("RefreshLibrary failed: %v", err)
	}
	if !result.Success {
		t.Error("Expected refresh to succeed")
	}
}

func TestEmbyServiceRefreshLibraryFailure(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{
			"emby_url":     mockServer.URL,
			"emby_api_key": "test-api-key",
			"emby_enabled": "true",
		},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	result, err := embySvc.RefreshLibrary("lib-1")
	if err != nil {
		t.Fatalf("RefreshLibrary should not return error: %v", err)
	}
	if result.Success {
		t.Error("Expected refresh to fail")
	}
}

func TestEmbyServiceRefreshAll(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/emby/Library/VirtualFolders" {
			libraries := []EmbyVirtualFolderInfo{
				{Name: "Movies", ItemID: "lib-1"},
				{Name: "TV Shows", ItemID: "lib-2"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(libraries)
		} else if r.URL.Path == "/emby/Library/Refresh" && r.Method == "POST" {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer mockServer.Close()

	mockDAO := &mockSystemConfigDAO{
		configs: map[string]string{
			"emby_url":     mockServer.URL,
			"emby_api_key": "test-api-key",
			"emby_enabled": "true",
		},
	}

	embySvc := NewEmbyService(mockDAO, nil)

	results, err := embySvc.RefreshAll()
	if err != nil {
		t.Fatalf("RefreshAll failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestEmbyServiceIsEnabledWithConfig(t *testing.T) {
	tests := []struct {
		name     string
		configs  map[string]string
		expected bool
	}{
		{
			name: "enabled with true",
			configs: map[string]string{
				"emby_enabled": "true",
			},
			expected: true,
		},
		{
			name: "enabled with 1",
			configs: map[string]string{
				"emby_enabled": "1",
			},
			expected: true,
		},
		{
			name: "disabled with false",
			configs: map[string]string{
				"emby_enabled": "false",
			},
			expected: false,
		},
		{
			name: "disabled with 0",
			configs: map[string]string{
				"emby_enabled": "0",
			},
			expected: false,
		},
		{
			name:     "no config",
			configs:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDAO := &mockSystemConfigDAO{
				configs: tt.configs,
			}

			embySvc := NewEmbyService(mockDAO, nil)
			result := embySvc.IsEnabled()

			if result != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", result, tt.expected)
			}
		})
	}
}
