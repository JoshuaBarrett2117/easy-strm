package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

func TestMetaTubeSearchHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	defer server.Close()
	defer close(released)
	svc := NewTmdbService("key", nil)
	svc.metatubeURL, svc.httpClient = server.URL, server.Client()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := svc.searchMetaTubeContext(ctx, "Example", 0)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled request unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("MetaTube did not release cancelled request")
	}
}

func TestRecognitionRoundReusesAttemptAndSeparatesRounds(t *testing.T) {
	ctx := WithRecognitionRound(context.Background())
	round := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hint, called, err := round.infer(ctx, "Example.mkv", func() (*domain.AIRecognitionHint, error) {
				calls.Add(1)
				return &domain.AIRecognitionHint{Title: "Example"}, nil
			})
			if err != nil || !called || hint == nil {
				t.Errorf("unexpected attempt: %v %v %v", hint, called, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("got %d calls", calls.Load())
	}
	if WithRecognitionRound(ctx) != ctx {
		t.Fatal("wrapping must preserve round")
	}
	fresh := WithRecognitionRound(context.Background()).Value(recognitionRoundKey{}).(*recognitionRound)
	if fresh == round {
		t.Fatal("different tasks must have independent rounds")
	}
}

func TestRecognitionRoundDoesNotRepeatFailure(t *testing.T) {
	ctx := WithRecognitionRound(context.Background())
	round := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	calls := 0
	for i := 0; i < 2; i++ {
		_, called, err := round.infer(ctx, "Example.mkv", func() (*domain.AIRecognitionHint, error) {
			calls++
			return nil, errors.New("AI unavailable")
		})
		if !called || err == nil {
			t.Fatal("failure must be retained")
		}
	}
	if calls != 1 {
		t.Fatalf("got %d calls", calls)
	}
}

func TestOrganizeCancelledBeforePreviewDoesNotScan(t *testing.T) {
	svc := &OrganizeService{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.OrganizeDirectoryForSource(ctx, &domain.MediaSource{}, "", "", "movie", "", "skip", "copy", nil, false, nil, nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestOrganizeStopBeforePreviewDoesNotScan(t *testing.T) {
	svc := &OrganizeService{}
	_, err := svc.OrganizeDirectoryForSource(context.Background(), &domain.MediaSource{}, "", "", "movie", "", "skip", "copy", nil, false, nil, nil, nil, func() bool { return true })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
