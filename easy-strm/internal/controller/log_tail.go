package controller

import (
	"bytes"
	"io"
	"os"
	"strings"
)

const (
	// maxLogViewerLines 单次最多返回的匹配行数。
	maxLogViewerLines = 5000
	// maxLogScanBytes 带筛选时最多向前扫描的字节数，防止大文件拖慢接口。
	maxLogScanBytes = 64 << 20
	logTailChunk    = 64 << 10
)

var logLevelRank = map[string]int{"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3}

// logLineFilter 日志查看筛选条件；零值表示不过滤。
type logLineFilter struct {
	minLevel int
	terms    []string
}

func newLogLineFilter(level, query string) logLineFilter {
	filter := logLineFilter{}
	if rank, ok := logLevelRank[strings.ToUpper(strings.TrimSpace(level))]; ok {
		filter.minLevel = rank
	}
	for _, term := range strings.Fields(strings.ToLower(query)) {
		filter.terms = append(filter.terms, term)
	}
	return filter
}

func (filter logLineFilter) active() bool {
	return filter.minLevel > 0 || len(filter.terms) > 0
}

// lineLevel 识别人类可读格式 "][LEVEL][" 与 JSON 格式 "level":"LEVEL"；无法识别返回 -1。
func lineLevel(line string) int {
	for name, rank := range logLevelRank {
		if strings.Contains(line, "]["+name+"][") || strings.Contains(line, `"level":"`+name+`"`) {
			return rank
		}
	}
	return -1
}

func (filter logLineFilter) match(line string) bool {
	if filter.minLevel > 0 {
		if level := lineLevel(line); level >= 0 && level < filter.minLevel {
			return false
		}
	}
	if len(filter.terms) > 0 {
		lower := strings.ToLower(line)
		for _, term := range filter.terms {
			if !strings.Contains(lower, term) {
				return false
			}
		}
	}
	return true
}

type tailResult struct {
	lines        []string
	truncated    bool
	scannedBytes int64
}

// readTailLines 从文件尾部按块倒序扫描，返回最多 n 条匹配行（最新在前）。
func readTailLines(filePath string, n int, filter logLineFilter) (tailResult, error) {
	result := tailResult{}
	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return result, err
	}
	offset := stat.Size()
	var carry []byte
	emit := func(line []byte) bool {
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			return false
		}
		text := string(line)
		if filter.match(text) {
			result.lines = append(result.lines, text)
		}
		return len(result.lines) >= n
	}
	buffer := make([]byte, logTailChunk)
	for offset > 0 {
		if filter.active() && result.scannedBytes >= maxLogScanBytes {
			result.truncated = true
			return result, nil
		}
		size := int64(logTailChunk)
		if offset < size {
			size = offset
		}
		offset -= size
		chunk := buffer[:size]
		if _, err := file.ReadAt(chunk, offset); err != nil && err != io.EOF {
			return result, err
		}
		result.scannedBytes += size
		data := append(append([]byte{}, chunk...), carry...)
		for {
			index := bytes.LastIndexByte(data, '\n')
			if index < 0 {
				break
			}
			if emit(data[index+1:]) {
				return result, nil
			}
			data = data[:index]
		}
		carry = data
	}
	emit(carry)
	return result, nil
}
