package service

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"easy-strm/internal/pkg/logger"
	"github.com/go-redis/redis/v8"
)

const (
	bloomFilterKey           = "easy_strm:bloom:sha1"
	bloomFilterRedisKey      = "easy_strm:bloom:keys"
	defaultFalsePositiveRate = 0.01
	defaultBitSize           = 10000000
)

type BloomFilter struct {
	client      *redis.Client
	ctx         context.Context
	bitSize     uint64
	hashCount   uint8
	mu          sync.RWMutex
	localFilter *SafeBitSet
	useLocal    bool
}

type SafeBitSet struct {
	bits []uint64
	size uint64
}

func NewSafeBitSet(size uint64) *SafeBitSet {
	return &SafeBitSet{
		bits: make([]uint64, (size+63)/64),
		size: size,
	}
}

func (bs *SafeBitSet) Add(pos uint64) {
	if pos >= bs.size {
		return
	}
	bs.bits[pos/64] |= 1 << (pos % 64)
}

func (bs *SafeBitSet) Contains(pos uint64) bool {
	if pos >= bs.size {
		return false
	}
	return (bs.bits[pos/64] & (1 << (pos % 64))) != 0
}

func (bs *SafeBitSet) Clear() {
	for i := range bs.bits {
		bs.bits[i] = 0
	}
}

func NewBloomFilter(client *redis.Client, bitSize uint64) *BloomFilter {
	if bitSize == 0 {
		bitSize = defaultBitSize
	}
	hashCount := optimalHashCount(bitSize, defaultFalsePositiveRate)

	bf := &BloomFilter{
		client:      client,
		ctx:         context.Background(),
		bitSize:     bitSize,
		hashCount:   hashCount,
		localFilter: NewSafeBitSet(bitSize),
		useLocal:    client == nil,
	}

	if client != nil {
		bf.loadFromRedis()
	}

	logger.Infof("BloomFilter[New] Initialized with bitSize=%d, hashCount=%d, useLocal=%v", bitSize, hashCount, bf.useLocal)
	return bf
}

func optimalHashCount(bitSize uint64, falsePositiveRate float64) uint8 {
	return uint8(math.Ceil(math.Log2(1 / falsePositiveRate)))
}

func (bf *BloomFilter) AddSHA1(sha1Hash string) error {
	if bf.useLocal {
		bf.mu.Lock()
		positions := bf.getPositions(sha1Hash)
		for _, pos := range positions {
			bf.localFilter.Add(pos)
		}
		bf.mu.Unlock()
		logger.Debugf("BloomFilter[AddSHA1] Added to local filter: %s", sha1Hash)
		return nil
	}

	positions := bf.getPositions(sha1Hash)
	for _, pos := range positions {
		if err := bf.client.SetBit(bf.ctx, bloomFilterKey, int64(pos), 1).Err(); err != nil {
			logger.Errorf("BloomFilter[AddSHA1] Failed to set bit: %v", err)
			return fmt.Errorf("set bit failed: %w", err)
		}
	}

	pipe := bf.client.Pipeline()
	pipe.SAdd(bf.ctx, bloomFilterRedisKey, sha1Hash)
	pipe.Expire(bf.ctx, bloomFilterRedisKey, 0)
	if _, err := pipe.Exec(bf.ctx); err != nil {
		logger.Warnf("BloomFilter[AddSHA1] Failed to update Redis key set: %v", err)
	}

	logger.Debugf("BloomFilter[AddSHA1] Added SHA1 to Redis: %s", sha1Hash)
	return nil
}

func (bf *BloomFilter) ContainsSHA1(sha1Hash string) (bool, error) {
	if bf.useLocal {
		bf.mu.RLock()
		positions := bf.getPositions(sha1Hash)
		for _, pos := range positions {
			if !bf.localFilter.Contains(pos) {
				bf.mu.RUnlock()
				bf.mu.RLock()
				return false, nil
			}
		}
		bf.mu.RUnlock()
		bf.mu.RLock()
		return true, nil
	}

	positions := bf.getPositions(sha1Hash)
	for _, pos := range positions {
		if bf.client.GetBit(bf.ctx, bloomFilterKey, int64(pos)).Val() == 0 {
			return false, nil
		}
	}
	return true, nil
}

func (bf *BloomFilter) MayContain(sha1Hash string) bool {
	exists, _ := bf.ContainsSHA1(sha1Hash)
	return exists
}

func (bf *BloomFilter) getPositions(sha1Hash string) []uint64 {
	positions := make([]uint64, bf.hashCount)
	var h1, h2 uint64 = bf.hash(sha1Hash), bf.hash(sha1Hash + "salt")
	for i := uint8(0); i < bf.hashCount; i++ {
		positions[i] = (h1 + uint64(i)*h2) % bf.bitSize
	}
	return positions
}

func (bf *BloomFilter) hash(s string) uint64 {
	var h uint64
	for i := 0; i < len(s); i++ {
		h = h*31 + uint64(s[i])
	}
	return h
}

func (bf *BloomFilter) loadFromRedis() {
	if bf.client == nil {
		return
	}

	exists, err := bf.client.Exists(bf.ctx, bloomFilterKey).Result()
	if err != nil || exists == 0 {
		logger.Debugf("BloomFilter[loadFromRedis] No existing filter in Redis")
		return
	}

	allSHA1, err := bf.client.SMembers(bf.ctx, bloomFilterRedisKey).Result()
	if err != nil {
		logger.Warnf("BloomFilter[loadFromRedis] Failed to load SHA1 set: %v", err)
		return
	}

	for _, sha1Hash := range allSHA1 {
		positions := bf.getPositions(sha1Hash)
		for _, pos := range positions {
			bf.localFilter.Add(pos)
		}
	}

	logger.Infof("BloomFilter[loadFromRedis] Loaded %d SHA1 entries from Redis", len(allSHA1))
}

func (bf *BloomFilter) Clear() error {
	if bf.useLocal {
		bf.mu.Lock()
		bf.localFilter.Clear()
		bf.mu.Unlock()
		logger.Infof("BloomFilter[Clear] Local filter cleared")
		return nil
	}

	pipe := bf.client.Pipeline()
	pipe.Del(bf.ctx, bloomFilterKey)
	pipe.Del(bf.ctx, bloomFilterRedisKey)
	_, err := pipe.Exec(bf.ctx)
	if err != nil {
		logger.Errorf("BloomFilter[Clear] Failed to clear Redis: %v", err)
		return fmt.Errorf("clear failed: %w", err)
	}

	bf.mu.Lock()
	bf.localFilter.Clear()
	bf.mu.Unlock()

	logger.Infof("BloomFilter[Clear] Redis filter cleared")
	return nil
}

func (bf *BloomFilter) GetBitSize() uint64 {
	return bf.bitSize
}

func (bf *BloomFilter) GetHashCount() uint8 {
	return bf.hashCount
}

func (bf *BloomFilter) SetRedisClient(client *redis.Client) {
	bf.client = client
	bf.useLocal = (client == nil)
	if client != nil {
		bf.loadFromRedis()
	}
	logger.Infof("BloomFilter[SetRedisClient] Updated Redis client, useLocal=%v", bf.useLocal)
}

func (bf *BloomFilter) EstimateCount() int64 {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	setBits := int64(0)
	for _, word := range bf.localFilter.bits {
		setBits += int64(popcount(word))
	}

	n := float64(bf.localFilter.size)
	k := float64(bf.hashCount)
	m := float64(setBits)

	if m == 0 {
		return 0
	}

	count := -n / k * math.Log(1-m/n)
	return int64(math.Round(count))
}

func popcount(x uint64) int {
	count := 0
	for x != 0 {
		count++
		x &= x - 1
	}
	return count
}

func (bf *BloomFilter) MarshalBinary() ([]byte, error) {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	data := make([]byte, 8+len(bf.localFilter.bits)*8)
	binary.LittleEndian.PutUint64(data[:8], bf.bitSize)
	for i, bits := range bf.localFilter.bits {
		binary.LittleEndian.PutUint64(data[8+i*8:8+(i+1)*8], bits)
	}
	return data, nil
}

func (bf *BloomFilter) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("data too short")
	}

	bf.mu.Lock()
	defer bf.mu.Unlock()

	bf.bitSize = binary.LittleEndian.Uint64(data[:8])
	numWords := (len(data) - 8) / 8
	bf.localFilter.bits = make([]uint64, numWords)
	bf.localFilter.size = bf.bitSize

	for i := 0; i < numWords; i++ {
		bf.localFilter.bits[i] = binary.LittleEndian.Uint64(data[8+i*8 : 8+(i+1)*8])
	}

	bf.hashCount = optimalHashCount(bf.bitSize, defaultFalsePositiveRate)
	return nil
}
