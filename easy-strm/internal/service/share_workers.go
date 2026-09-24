package service

import (
	"context"
	"easy-strm/internal/domain"
	"fmt"
	"sync"
)

type shareIdentifyOutcome struct {
	media domain.ShareMedia
	ok    bool
	err   error
}

// runShareIdentifyWorkers 限制在途文件数；单文件异常隔离，汇总回调始终串行执行。
func runShareIdentifyWorkers(ctx context.Context, items []domain.ShareMedia, workers int, identify func(context.Context, domain.ShareMedia) (bool, error), collect func(domain.ShareMedia, bool, error)) {
	if workers < 1 {
		workers = defaultShareWorkerCount
	}
	workers = min(min(workers, 32), len(items))
	jobs := make(chan domain.ShareMedia)
	results := make(chan shareIdentifyOutcome, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				if ctx.Err() != nil {
					return
				}
				result := func() (out shareIdentifyOutcome) {
					out.media = m
					defer func() {
						if recover() != nil {
							out.ok = false
							out.err = fmt.Errorf("单文件识别异常")
						}
					}()
					out.ok, out.err = identify(ctx, m)
					return
				}()
				results <- result
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, m := range items {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case jobs <- m:
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	for result := range results {
		collect(result.media, result.ok, result.err)
	}
}
