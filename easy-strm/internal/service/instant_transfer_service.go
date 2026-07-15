package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"github.com/go-redis/redis/v8"
)

const (
	sha1CachePrefix   = "easy_strm:sha1:cache:"
	sha1CacheTTL      = 7 * 24 * time.Hour
	transferBlockTime = 200 * time.Millisecond
)

type InstantTransferService struct {
	hashService     *HashService
	bloomFilter     *BloomFilter
	redisClient     *redis.Client
	transferLimiter *TransferLimiter
}

func NewInstantTransferService(hashService *HashService, bloomFilter *BloomFilter, redisClient *redis.Client) *InstantTransferService {
	return &InstantTransferService{
		hashService:     hashService,
		bloomFilter:     bloomFilter,
		redisClient:     redisClient,
		transferLimiter: NewTransferLimiter(redisClient),
	}
}

type TransferResult struct {
	Success   bool
	Skip      bool
	SHA1      string
	FileName  string
	FileSize  int64
	Message   string
	Error     error
	NeedRetry bool
}

func (s *InstantTransferService) Transfer(sourceFile *domain.FileInfo, sourceAccount *domain.Cloud115, targetAccount *domain.Cloud115, targetDir string) *TransferResult {
	sha1 := strings.ToLower(sourceFile.Sha1)
	if sha1 == "" {
		return &TransferResult{
			Success: false,
			Message: "source file has no SHA1",
			Error:   fmt.Errorf("source file SHA1 is empty"),
		}
	}

	mayExist, err := s.bloomFilter.ContainsSHA1(sha1)
	if err != nil {
		logger.Warnf("InstantTransferService[Transfer] BloomFilter check failed: %v", err)
	}

	if !mayExist {
		logger.Infof("InstantTransferService[Transfer] SHA1 %s not in BloomFilter, proceeding with transfer", sha1)
	}

	cachedCID, err := s.GetCachedSHA1(sha1)
	if err == nil && cachedCID != "" {
		logger.Infof("InstantTransferService[Transfer] SHA1 %s found in cache as CID %s", sha1, cachedCID)
		return &TransferResult{
			Success: true,
			Skip:    true,
			SHA1:    sha1,
			Message: fmt.Sprintf("file already transferred (cached): %s", cachedCID),
		}
	}

	if err := s.transferLimiter.Wait(sourceAccount.ID); err != nil {
		logger.Warnf("InstantTransferService[Transfer] Rate limit wait failed: %v", err)
		return &TransferResult{
			Success:   false,
			SHA1:      sha1,
			Message:   "rate limit exceeded",
			Error:     err,
			NeedRetry: true,
		}
	}

	return &TransferResult{
		Success: true,
		SHA1:    sha1,
		Message: "transfer initiated",
	}
}

func (s *InstantTransferService) CheckSHA1Exists(sha1 string) (bool, error) {
	mayExist, err := s.bloomFilter.ContainsSHA1(sha1)
	if err != nil {
		return false, err
	}

	if !mayExist {
		return false, nil
	}

	cachedCID, err := s.GetCachedSHA1(sha1)
	if err == nil && cachedCID != "" {
		return true, nil
	}

	return true, nil
}

func (s *InstantTransferService) GetCachedSHA1(sha1 string) (string, error) {
	if s.redisClient == nil {
		return "", fmt.Errorf("redis client not initialized")
	}

	ctx := context.Background()
	key := sha1CachePrefix + sha1
	cid, err := s.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get cached SHA1 failed: %w", err)
	}
	return cid, nil
}

func (s *InstantTransferService) SetCachedSHA1(sha1, cid string) error {
	if s.redisClient == nil {
		return fmt.Errorf("redis client not initialized")
	}

	ctx := context.Background()
	key := sha1CachePrefix + sha1
	err := s.redisClient.Set(ctx, key, cid, sha1CacheTTL).Err()
	if err != nil {
		return fmt.Errorf("set cached SHA1 failed: %w", err)
	}

	if err := s.bloomFilter.AddSHA1(sha1); err != nil {
		logger.Warnf("InstantTransferService[SetCachedSHA1] Failed to add SHA1 to BloomFilter: %v", err)
	}

	logger.Debugf("InstantTransferService[SetCachedSHA1] Cached SHA1 %s -> CID %s", sha1, cid)
	return nil
}

func (s *InstantTransferService) InvalidateSHA1Cache(sha1 string) error {
	if s.redisClient == nil {
		return fmt.Errorf("redis client not initialized")
	}

	ctx := context.Background()
	key := sha1CachePrefix + sha1
	err := s.redisClient.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("invalidate SHA1 cache failed: %w", err)
	}

	logger.Debugf("InstantTransferService[InvalidateSHA1Cache] Invalidated SHA1 cache: %s", sha1)
	return nil
}

func (s *InstantTransferService) GetLocalSHA1(filePath string) (string, error) {
	return s.hashService.GetSHA1(filePath)
}

func (s *InstantTransferService) GetBloomFilterStats() (uint64, uint8, int64) {
	return s.bloomFilter.GetBitSize(), s.bloomFilter.GetHashCount(), s.bloomFilter.EstimateCount()
}
