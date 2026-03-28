package service

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"

	"easy-strm/internal/pkg/logger"
)

const (
	hashChunkSize = 128 * 1024
)

type HashService struct {
	sha1Cache map[string]string
	cacheMu   sync.RWMutex
}

func NewHashService() *HashService {
	return &HashService{
		sha1Cache: make(map[string]string),
	}
}

func (h *HashService) GetSHA1(filePath string) (string, error) {
	h.cacheMu.RLock()
	if sha1Hash, ok := h.sha1Cache[filePath]; ok {
		h.cacheMu.RUnlock()
		logger.Debugf("HashService[GetSHA1] Cache hit for: %s", filePath)
		return sha1Hash, nil
	}
	h.cacheMu.RUnlock()

	sha1Hash, err := h.calculateFileSHA1(filePath)
	if err != nil {
		return "", err
	}

	h.cacheMu.Lock()
	h.sha1Cache[filePath] = sha1Hash
	h.cacheMu.Unlock()

	logger.Infof("HashService[GetSHA1] Calculated SHA1 for %s: %s", filePath, sha1Hash)
	return sha1Hash, nil
}

func (h *HashService) calculateFileSHA1(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		logger.Errorf("HashService[calculateFileSHA1] Failed to open file %s: %v", filePath, err)
		return "", fmt.Errorf("open file failed: %w", err)
	}
	defer file.Close()

	hash := sha1.New()
	buffer := make([]byte, hashChunkSize)
	totalBytes := 0

	for {
		n, err := file.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			totalBytes += n
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Errorf("HashService[calculateFileSHA1] Failed to read file %s: %v", filePath, err)
			return "", fmt.Errorf("read file failed: %w", err)
		}
	}

	sha1Hash := hex.EncodeToString(hash.Sum(nil))
	logger.Debugf("HashService[calculateFileSHA1] File %s (%d bytes) -> SHA1: %s", filePath, totalBytes, sha1Hash)
	return sha1Hash, nil
}

func (h *HashService) GetSHA1FromBytes(data []byte) string {
	hash := sha1.Sum(data)
	sha1Hash := hex.EncodeToString(hash[:])
	return sha1Hash
}

func (h *HashService) InvalidateCache(filePath string) {
	h.cacheMu.Lock()
	delete(h.sha1Cache, filePath)
	h.cacheMu.Unlock()
	logger.Debugf("HashService[InvalidateCache] Cache invalidated for: %s", filePath)
}

func (h *HashService) ClearCache() {
	h.cacheMu.Lock()
	h.sha1Cache = make(map[string]string)
	h.cacheMu.Unlock()
	logger.Infof("HashService[ClearCache] All SHA1 cache cleared")
}

func (h *HashService) GetCachedSHA1(filePath string) (string, bool) {
	h.cacheMu.RLock()
	sha1Hash, ok := h.sha1Cache[filePath]
	h.cacheMu.RUnlock()
	return sha1Hash, ok
}
