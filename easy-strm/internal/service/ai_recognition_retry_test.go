package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func retryAIService(t *testing.T, handler http.HandlerFunc, timeout int) *AIRecognitionService {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	service := NewAIRecognitionService(&aiMemoryStore{}, server.Client())
	cfg := defaultAIConfig()
	cfg.Enabled, cfg.BaseURL, cfg.Model, cfg.TimeoutSeconds = true, server.URL, "fixture", timeout
	cfg.Scenes = []string{"no_match"}
	if err := service.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return service
}

func writeRetryAIHint(w http.ResponseWriter) {
	fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"Show\",\"year\":2009,\"media_type\":\"tv\"}"}}]}`)
}

func TestAIAssistTransientRetries(t *testing.T) {
	for _, test := range []struct {
		name     string
		statuses []int
		want     int32
		success  bool
	}{
		{"429_503_success", []int{429, 503, 200}, 3, true},
		{"exhausted", []int{500, 502, 503}, 3, false},
		{"400", []int{400}, 1, false},
		{"401", []int{401}, 1, false},
		{"malformed", []int{200}, 1, false},
		{"invalid_json", []int{200}, 1, false},
		{"oversized", []int{200}, 1, false},
		{"invalid_hint", []int{200}, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			var timestamps []time.Time
			var mu sync.Mutex
			service := retryAIService(t, func(w http.ResponseWriter, r *http.Request) {
				index := int(calls.Add(1)) - 1
				mu.Lock()
				timestamps = append(timestamps, time.Now())
				mu.Unlock()
				if index >= len(test.statuses) {
					t.Error("unexpected extra attempt")
					index = len(test.statuses) - 1
				}
				w.WriteHeader(test.statuses[index])
				switch test.name {
				case "malformed":
					fmt.Fprint(w, `{"choices":[{"message":{"content":"bad"}}]}`)
				case "invalid_json":
					fmt.Fprint(w, `not json`)
				case "oversized":
					fmt.Fprint(w, strings.Repeat("x", 1024*1024+1))
				case "invalid_hint":
					fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"Show\",\"year\":9999,\"media_type\":\"tv\"}"}}]}`)
				default:
					writeRetryAIHint(w)
				}
			}, 10)
			hint, called, err := service.Assist(WithRecognitionRound(context.Background()), "Show.S01E01.mkv", "no_match")
			if !called || calls.Load() != test.want || (err == nil) != test.success || (hint != nil) != test.success {
				t.Fatalf("calls=%d called=%v hint=%+v err=%v", calls.Load(), called, hint, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(timestamps) == 3 && (timestamps[1].Sub(timestamps[0]) < time.Second || timestamps[2].Sub(timestamps[1]) < 2*time.Second) {
				t.Fatalf("expected production backoff [1s,2s]: %v", timestamps)
			}
		})
	}
}

type retryAITransport func(*http.Request) (*http.Response, error)

func (transport retryAITransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type retryAIReadBody struct{ closed *atomic.Int32 }

func (body *retryAIReadBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (body *retryAIReadBody) Close() error             { body.closed.Add(1); return nil }

func TestAIAssistTransportReadAndConcurrentDedupe(t *testing.T) {
	for _, readFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(readFailure), func(t *testing.T) {
			var calls, closed atomic.Int32
			service := retryAIService(t, func(w http.ResponseWriter, r *http.Request) { t.Error("mock transport bypassed") }, 10)
			service.client = &http.Client{Transport: retryAITransport(func(request *http.Request) (*http.Response, error) {
				if calls.Add(1) == 1 {
					if !readFailure {
						return nil, errors.New("local transport fixture")
					}
					return &http.Response{StatusCode: 200, Body: &retryAIReadBody{closed: &closed}, Header: http.Header{}}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"title\":\"Show\",\"year\":2009,\"media_type\":\"tv\"}"}}]}`)), Header: http.Header{}}, nil
			})}
			ctx := WithRecognitionRound(context.Background())
			var workers sync.WaitGroup
			for index := 0; index < 8; index++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					hint, called, err := service.Assist(ctx, "same-file", "no_match")
					if err != nil || !called || hint == nil {
						t.Errorf("called=%v hint=%+v err=%v", called, hint, err)
					}
				}()
			}
			workers.Wait()
			if calls.Load() != 2 || readFailure && closed.Load() != 1 {
				t.Fatalf("calls=%d closed=%d", calls.Load(), closed.Load())
			}
		})
	}
}

func TestAIAssistExhaustedShareFailure(t *testing.T) {
	var calls atomic.Int32
	ai := retryAIService(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }, 10)
	s, _ := shareYearFixture(t, "Other Show", 1, "2005-01-01", "2020-01-01", 1, "2005-01-01")
	s.SetAIRecognitionService(ai)
	result, err := s.IdentifyShareFile(context.Background(), "Unknown.Show.S01E01.2009.mkv", "tmdb", "tv")
	if err != nil || result.Success || !result.AIUsed || !strings.Contains(result.FailureReason, "ai_request_failed") || calls.Load() != 3 {
		t.Fatalf("calls=%d result=%+v err=%v", calls.Load(), result, err)
	}
}

func TestAIAssistRetryCancellationAndBudget(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelParent), func(t *testing.T) {
			var calls atomic.Int32
			started := make(chan struct{}, 1)
			service := retryAIService(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
				w.WriteHeader(503)
			}, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelParent {
				go func() { <-started; time.Sleep(50 * time.Millisecond); cancel() }()
			}
			start := time.Now()
			_, called, err := service.Assist(ctx, "file", "no_match")
			elapsed := time.Since(start)
			if err == nil || !called || calls.Load() != 1 || elapsed > 1500*time.Millisecond || !cancelParent && elapsed < 900*time.Millisecond {
				t.Fatalf("calls=%d called=%v elapsed=%s err=%v", calls.Load(), called, elapsed, err)
			}
		})
	}
}

func TestAITransientCauseDoesNotExposeCredentials(t *testing.T) {
	cause := errors.New("transport failure at https://user:password@example.invalid/?token=secret")
	service := retryAIService(t, func(w http.ResponseWriter, r *http.Request) { t.Error("mock transport bypassed") }, 10)
	service.client = &http.Client{Transport: retryAITransport(func(request *http.Request) (*http.Response, error) {
		return nil, cause
	})}
	err := service.request(context.Background(), defaultAIConfig(), http.MethodGet, "/models", nil, &struct{}{})
	var transient *aiTransientError
	if !errors.As(err, &transient) || !errors.Is(err, cause) {
		t.Fatalf("transient cause lost: %v", err)
	}
	for _, secret := range []string{"password", "token", "secret", "example.invalid"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("endpoint credentials exposed: %v", err)
		}
	}
}
