package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type shareRequestAttempt struct {
	done   chan struct{}
	body   []byte
	status int
	header http.Header
	err    error
}

// doShareTMDBRequest 合并同轮相同请求；仅复用成功JSON响应，失败不阻塞后续重试。
func (s *TmdbService) doShareTMDBRequest(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	round, ok := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	if !ok {
		return s.performShareTMDBRequest(req)
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(req.Method+":"+req.URL.String())))
	round.mu.Lock()
	if round.requests == nil {
		round.requests = map[string]*shareRequestAttempt{}
	}
	attempt := round.requests[key]
	reused := attempt != nil
	if !reused {
		attempt = &shareRequestAttempt{done: make(chan struct{})}
		round.requests[key] = attempt
	}
	round.mu.Unlock()
	if reused {
		observeShareMetric(ctx, "tmdb_round_hit", 0)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-attempt.done:
		}
	} else {
		func() {
			defer close(attempt.done)
			defer func() {
				if attempt.err != nil || attempt.status != http.StatusOK || !json.Valid(attempt.body) {
					round.mu.Lock()
					delete(round.requests, key)
					round.mu.Unlock()
				}
			}()
			response, err := s.performShareTMDBRequest(req)
			attempt.err = err
			if err != nil {
				return
			}
			defer response.Body.Close()
			attempt.status = response.StatusCode
			attempt.header = response.Header.Clone()
			attempt.body, attempt.err = io.ReadAll(response.Body)
		}()
	}
	if attempt.err != nil {
		return nil, attempt.err
	}
	return &http.Response{StatusCode: attempt.status, Header: attempt.header.Clone(), Body: io.NopCloser(bytes.NewReader(attempt.body)), Request: req}, nil
}

func (s *TmdbService) shareAliases(ctx context.Context, kind string, id int) ([]string, error) {
	cacheKind := kind + "_aliases"
	if !bypassShareRecognitionCache(ctx) {
		if detail, ok := s.loadDetailCache(cacheKind, id, 0, 0); ok {
			observeShareMetric(ctx, "tmdb_alias_cache_hit", 0)
			return shareAliasTitles(detail), nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s/%d/alternative_titles?api_key=%s", s.baseURL, kind, id, s.apiKey), nil)
	if err != nil {
		return nil, err
	}
	response, err := s.doShareTMDBRequest(req)
	if err != nil {
		return nil, fmt.Errorf("获取TMDB别名失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取TMDB别名失败: HTTP %d", response.StatusCode)
	}
	var detail map[string]interface{}
	if err = json.NewDecoder(response.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("解析TMDB别名失败")
	}
	s.saveDetailCache(cacheKind, id, 0, 0, detail)
	return shareAliasTitles(detail), nil
}

func shareAliasTitles(detail map[string]interface{}) []string {
	titles := []string{}
	for _, field := range []string{"results", "titles"} {
		if list, ok := detail[field].([]interface{}); ok {
			for _, raw := range list {
				if item, ok := raw.(map[string]interface{}); ok {
					if title, ok := item["title"].(string); ok && title != "" {
						titles = append(titles, title)
					}
				}
			}
		}
	}
	return titles
}
