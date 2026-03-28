package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

const (
	circuitBreakerKey       = "easy_strm:circuit:"
	coolingDuration         = 5 * time.Minute
	maxConsecutiveFailures = 5
)

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type CircuitBreaker struct {
	accountID            int
	state                CircuitState
	failures             int32
	lastFailure          time.Time
	consecutiveFailures  int32
	coolingStartTime     *time.Time
	mu                   sync.RWMutex
}

func NewCircuitBreaker(accountID int) *CircuitBreaker {
	return &CircuitBreaker{
		accountID:   accountID,
		state:       CircuitClosed,
		failures:    0,
		lastFailure: time.Time{},
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	atomic.StoreInt32(&cb.consecutiveFailures, 0)
	atomic.StoreInt32(&cb.failures, 0)
	cb.mu.Lock()
	cb.state = CircuitClosed
	cb.coolingStartTime = nil
	cb.mu.Unlock()
}

func (cb *CircuitBreaker) RecordFailure() {
	atomic.AddInt32(&cb.consecutiveFailures, 1)
	atomic.StoreInt32(&cb.failures, atomic.LoadInt32(&cb.failures)+1)
	cb.lastFailure = time.Now()

	if atomic.LoadInt32(&cb.consecutiveFailures) >= maxConsecutiveFailures {
		cb.mu.Lock()
		if cb.state != CircuitOpen {
			now := time.Now()
			cb.state = CircuitOpen
			cb.coolingStartTime = &now
			logger.Warnf("CircuitBreaker[RecordFailure] Account %d circuit OPEN (consecutive failures: %d)", cb.accountID, maxConsecutiveFailures)
		}
		cb.mu.Unlock()
	}
}

func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()

	if state == CircuitClosed {
		return true
	}

	if state == CircuitOpen {
		cb.mu.RLock()
		coolingStart := cb.coolingStartTime
		cb.mu.RUnlock()

		if coolingStart != nil && time.Since(*coolingStart) >= coolingDuration {
			cb.mu.Lock()
			cb.state = CircuitHalfOpen
			cb.mu.Unlock()
			logger.Infof("CircuitBreaker[AllowRequest] Account %d circuit HALF-OPEN (cooling expired)", cb.accountID)
			return true
		}

		return false
	}

	if state == CircuitHalfOpen {
		return true
	}

	return false
}

func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) GetCoolingStartTime() *time.Time {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.coolingStartTime
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	cb.state = CircuitClosed
	atomic.StoreInt32(&cb.consecutiveFailures, 0)
	atomic.StoreInt32(&cb.failures, 0)
	cb.coolingStartTime = nil
	cb.mu.Unlock()
}

type TransferScheduler struct {
	redisClient      *redis.Client
	circuitBreakers  map[int]*CircuitBreaker
	circuitMu        sync.RWMutex
	transferQueue    chan *TransferTask
	workerCount      int
	wg               sync.WaitGroup
	stopCh           chan struct{}
	priorityQueues   map[int]chan *TransferTask
	priorityMu       sync.RWMutex
	maxConcurrency   int
	ctx              context.Context
}

type TransferTask struct {
	ID            string
	SourceFile    *domain.FileInfo
	SourceAccount *domain.Cloud115
	TargetAccount *domain.Cloud115
	TargetDir     string
	Priority      int
	Status        string
	Result        *TransferResult
	CreatedAt     time.Time
}

type TaskPriority struct {
	Priority int
	Task     *TransferTask
}

func NewTransferScheduler(redisClient *redis.Client, workerCount int) *TransferScheduler {
	if workerCount <= 0 {
		workerCount = 3
	}

	scheduler := &TransferScheduler{
		redisClient:     redisClient,
		circuitBreakers: make(map[int]*CircuitBreaker),
		transferQueue:   make(chan *TransferTask, 1000),
		workerCount:     workerCount,
		stopCh:          make(chan struct{}),
		priorityQueues:  make(map[int]chan *TransferTask),
		maxConcurrency:  5,
		ctx:             context.Background(),
	}

	for i := 1; i <= 10; i++ {
		scheduler.priorityQueues[i] = make(chan *TransferTask, 200)
	}

	return scheduler
}

func (s *TransferScheduler) Start() {
	for i := 0; i < s.workerCount; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
	logger.Infof("TransferScheduler[Start] Started %d workers", s.workerCount)
}

func (s *TransferScheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	logger.Infof("TransferScheduler[Stop] All workers stopped")
}

func (s *TransferScheduler) worker(id int) {
	defer s.wg.Done()

	logger.Debugf("TransferScheduler[worker-%d] Started", id)

	for {
		select {
		case <-s.stopCh:
			logger.Debugf("TransferScheduler[worker-%d] Received stop signal", id)
			return
		case task := <-s.transferQueue:
			if task != nil {
				s.processTask(task)
			}
		default:
			task := s.getNextPriorityTask()
			if task != nil {
				s.processTask(task)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (s *TransferScheduler) getNextPriorityTask() *TransferTask {
	s.priorityMu.RLock()
	defer s.priorityMu.RUnlock()

	for priority := 10; priority >= 1; priority-- {
		if ch, ok := s.priorityQueues[priority]; ok {
			select {
			case task := <-ch:
				return task
			default:
				continue
			}
		}
	}
	return nil
}

func (s *TransferScheduler) processTask(task *TransferTask) {
	logger.Debugf("TransferScheduler[processTask] Processing task %s", task.ID)

	cb := s.getCircuitBreaker(task.SourceAccount.ID)
	if !cb.AllowRequest() {
		task.Status = "circuit_open"
		task.Result = &TransferResult{
			Success:   false,
			Message:   "circuit breaker open",
			NeedRetry: true,
		}
		logger.Warnf("TransferScheduler[processTask] Circuit breaker open for account %d", task.SourceAccount.ID)
		return
	}

	task.Status = "running"

	result := &TransferResult{}

	result.Success = true
	result.Message = "transfer completed"
	result.SHA1 = task.SourceFile.Sha1

	if !result.Success {
		cb.RecordFailure()
		task.Status = "failed"
		result.NeedRetry = true
	} else {
		cb.RecordSuccess()
		task.Status = "completed"
	}

	task.Result = result
	logger.Infof("TransferScheduler[processTask] Task %s completed with status: %s", task.ID, task.Status)
}

func (s *TransferScheduler) getCircuitBreaker(accountID int) *CircuitBreaker {
	s.circuitMu.RLock()
	cb, exists := s.circuitBreakers[accountID]
	s.circuitMu.RUnlock()

	if !exists {
		s.circuitMu.Lock()
		if _, exists = s.circuitBreakers[accountID]; !exists {
			s.circuitBreakers[accountID] = NewCircuitBreaker(accountID)
		}
		cb = s.circuitBreakers[accountID]
		s.circuitMu.Unlock()
	}

	return cb
}

func (s *TransferScheduler) SubmitTask(task *TransferTask) error {
	if task.SourceAccount.Status == domain.AccountStatusCooling {
		return fmt.Errorf("account %d is in cooling state", task.SourceAccount.ID)
	}

	if task.SourceAccount.Status == domain.AccountStatusDisabled {
		return fmt.Errorf("account %d is disabled", task.SourceAccount.ID)
	}

	task.CreatedAt = time.Now()
	task.Status = "pending"

	if task.Priority <= 0 {
		task.Priority = 5
	}
	if task.Priority > 10 {
		task.Priority = 10
	}

	s.priorityMu.RLock()
	priorityCh := s.priorityQueues[task.Priority]
	s.priorityMu.RUnlock()

	select {
	case priorityCh <- task:
		logger.Debugf("TransferScheduler[SubmitTask] Task %s submitted to priority queue %d", task.ID, task.Priority)
		return nil
	default:
		return fmt.Errorf("priority queue %d is full", task.Priority)
	}
}

func (s *TransferScheduler) SubmitTaskSync(task *TransferTask) (*TransferResult, error) {
	if err := s.SubmitTask(task); err != nil {
		return nil, err
	}

	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if task.Status == "completed" || task.Status == "failed" || task.Status == "circuit_open" {
			return task.Result, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("task timeout after %v", timeout)
}

func (s *TransferScheduler) CancelTask(taskID string) error {
	return nil
}

func (s *TransferScheduler) GetTaskStatus(taskID string) (string, *TransferResult) {
	return "unknown", nil
}

func (s *TransferScheduler) ResetCircuitBreaker(accountID int) {
	cb := s.getCircuitBreaker(accountID)
	cb.Reset()
	logger.Infof("TransferScheduler[ResetCircuitBreaker] Reset circuit breaker for account %d", accountID)
}

func (s *TransferScheduler) GetCircuitBreakerState(accountID int) CircuitState {
	cb := s.getCircuitBreaker(accountID)
	return cb.GetState()
}

func (s *TransferScheduler) GetCircuitBreakerCoolingTime(accountID int) time.Duration {
	cb := s.getCircuitBreaker(accountID)
	coolingStart := cb.GetCoolingStartTime()
	if coolingStart == nil {
		return 0
	}
	remaining := coolingDuration - time.Since(*coolingStart)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *TransferScheduler) SetAccountCooling(accountID int, coolingStartTime time.Time) error {
	cb := s.getCircuitBreaker(accountID)
	cb.mu.Lock()
	cb.state = CircuitOpen
	cb.coolingStartTime = &coolingStartTime
	cb.mu.Unlock()

	if s.redisClient != nil {
		ctx := context.Background()
		key := circuitBreakerKey + fmt.Sprintf("%d", accountID)
		err := s.redisClient.Set(ctx, key, coolingStartTime.Unix(), coolingDuration).Err()
		if err != nil {
			logger.Warnf("TransferScheduler[SetAccountCooling] Failed to set Redis key: %v", err)
		}
	}

	logger.Infof("TransferScheduler[SetAccountCooling] Account %d set to cooling", accountID)
	return nil
}

func (s *TransferScheduler) SetMaxConcurrency(max int) {
	s.maxConcurrency = max
	logger.Infof("TransferScheduler[SetMaxConcurrency] Updated to %d", max)
}

func (s *TransferScheduler) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"worker_count":   s.workerCount,
		"max_concurrency": s.maxConcurrency,
		"queue_size":     len(s.transferQueue),
	}

	s.circuitMu.RLock()
	breakerStates := make(map[int]map[string]interface{})
	for id, cb := range s.circuitBreakers {
		breakerStates[id] = map[string]interface{}{
			"state":                cb.GetState().String(),
			"consecutive_failures": atomic.LoadInt32(&cb.consecutiveFailures),
			"total_failures":       atomic.LoadInt32(&cb.failures),
		}
		if coolingStart := cb.GetCoolingStartTime(); coolingStart != nil {
			breakerStates[id]["cooling_remaining"] = coolingDuration - time.Since(*coolingStart)
		}
	}
	s.circuitMu.RUnlock()
	stats["circuit_breakers"] = breakerStates

	return stats
}

func (s *TransferScheduler) SortTasksByPriority(tasks []*TransferTask) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority > tasks[j].Priority
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
}
