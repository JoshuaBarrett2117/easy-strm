package service

import (
	"context"
	"strings"
	"sync"

	"easy-strm/internal/domain"
)

type recognitionRoundKey struct{}

type recognitionAttempt struct {
	done   chan struct{}
	hint   *domain.AIRecognitionHint
	err    error
	called bool
}

type recognitionRound struct {
	mu       sync.Mutex
	attempts map[string]*recognitionAttempt
	slots    chan struct{}
}

// WithRecognitionRound 为一轮任务建立独立AI请求账本，默认最多3个并发请求。
// 同一上下文重复包装不会重置账本；任务结束后账本随上下文释放。
func WithRecognitionRound(ctx context.Context) context.Context {
	if _, ok := ctx.Value(recognitionRoundKey{}).(*recognitionRound); ok {
		return ctx
	}
	return context.WithValue(ctx, recognitionRoundKey{}, &recognitionRound{
		attempts: make(map[string]*recognitionAttempt), slots: make(chan struct{}, 3),
	})
}

func (r *recognitionRound) infer(ctx context.Context, filename string, call func() (*domain.AIRecognitionHint, error)) (*domain.AIRecognitionHint, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key := strings.TrimSpace(strings.ReplaceAll(filename, "\\", "/"))
	r.mu.Lock()
	if previous := r.attempts[key]; previous != nil {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-previous.done:
			return cloneRecognitionHint(previous.hint), previous.called, previous.err
		}
	}
	attempt := &recognitionAttempt{done: make(chan struct{})}
	r.attempts[key] = attempt
	r.mu.Unlock()
	defer close(attempt.done)
	select {
	case <-ctx.Done():
		attempt.err = ctx.Err()
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
		if err := ctx.Err(); err != nil {
			attempt.err = err
			break
		}
		attempt.called = true
		attempt.hint, attempt.err = call()
	}
	return cloneRecognitionHint(attempt.hint), attempt.called, attempt.err
}

func cloneRecognitionHint(hint *domain.AIRecognitionHint) *domain.AIRecognitionHint {
	if hint == nil {
		return nil
	}
	copy := *hint
	return &copy
}
