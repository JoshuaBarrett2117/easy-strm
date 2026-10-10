package main

import (
	pkglogger "easy-strm/internal/pkg/logger"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// 定义日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger 日志记录器
type Logger struct {
	outputType string
	logDir     string
	keepDays   atomic.Int64
	infoFile   *os.File
	debugFile  *os.File
}

var logger *Logger

// InitLogger 初始化日志系统
func InitLogger(config *Config) {
	currentLevel := INFO
	// 设置日志级别
	switch strings.ToUpper(config.Log.LogLevel) {
	case "DEBUG":
		currentLevel = DEBUG
	case "INFO":
		currentLevel = INFO
	case "WARN":
		currentLevel = WARN
	case "ERROR":
		currentLevel = ERROR
	default:
		currentLevel = INFO
	}

	var debugOutput, infoOutput, warnOutput, errorOutput io.Writer

	// 获取日志目录
	logDir := config.Log.LogDir
	if logDir == "" {
		logDir = "logs"
	}

	// 获取保留天数
	keepDays := config.Log.KeepDays
	if keepDays <= 0 {
		keepDays = 7
	}

	// 始终创建日志文件输出（拆分为INFO和DEBUG两个文件）
	infoFileOutput, infoFile, debugFileOutput, debugFile, err := setupFileOutput(logDir)
	if err != nil {
		pkglogger.WithContext(nil, "startup").Log(pkglogger.ERROR, "日志文件初始化失败，使用控制台输出", nil, err)
	}

	// 根据配置决定是否同时输出到控制台
	if config.Log.OutputType == "console" {
		// 只输出到控制台，但同时也写入文件（用于日志查看功能）
		if infoFileOutput != nil && debugFileOutput != nil {
			debugOutput = io.MultiWriter(os.Stdout, debugFileOutput)
			infoOutput = io.MultiWriter(os.Stdout, infoFileOutput)
			warnOutput = io.MultiWriter(os.Stdout, infoFileOutput)
			errorOutput = io.MultiWriter(os.Stderr, infoFileOutput)
		} else {
			debugOutput = os.Stdout
			infoOutput = os.Stdout
			warnOutput = os.Stdout
			errorOutput = os.Stderr
		}
	} else {
		// 输出到文件，同时也输出到控制台
		if infoFileOutput != nil && debugFileOutput != nil {
			debugOutput = io.MultiWriter(os.Stdout, debugFileOutput)
			infoOutput = io.MultiWriter(os.Stdout, infoFileOutput)
			warnOutput = io.MultiWriter(os.Stdout, infoFileOutput)
			errorOutput = io.MultiWriter(os.Stderr, infoFileOutput)
		} else {
			debugOutput = os.Stdout
			infoOutput = os.Stdout
			warnOutput = os.Stdout
			errorOutput = os.Stderr
		}
	}

	// 创建日志记录器
	logger = &Logger{
		outputType: config.Log.OutputType,
		logDir:     logDir,
		infoFile:   infoFile,
		debugFile:  debugFile,
	}
	logger.keepDays.Store(int64(keepDays))

	// 服务层与主程序共用输出文件和级别，避免系统日志页面遗漏识别等业务日志。
	pkglogger.SetOutputs(debugOutput, infoOutput, warnOutput, errorOutput)
	pkglogger.SetLevel(pkglogger.Level(currentLevel))
	log.SetFlags(0)
	log.SetOutput(pkglogger.Writer("stdlib"))

	// 清理旧日志
	go func() {
		if err := cleanOldLogs(logDir, keepDays); err != nil {
			pkglogger.WithContext(nil, "logger").Log(pkglogger.WARN, "旧日志清理未完全成功", nil, err)
		}
	}()

	// 启动定时清理任务（每小时检查一次）
	// 周期清理由统一调度管理。
}

// setupFileOutput 设置日志文件输出（拆分为INFO和DEBUG两个文件）
func setupFileOutput(logDir string) (io.Writer, *os.File, io.Writer, *os.File, error) {
	// 创建日志目录
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, nil, nil, nil, err
	}

	dateStr := time.Now().Format("2006-01-02")

	// INFO日志文件（包含INFO、WARN、ERROR级别）
	infoLogFileName := fmt.Sprintf("%s/info_%s.log", logDir, dateStr)
	infoFile, err := os.OpenFile(infoLogFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// DEBUG日志文件（仅包含DEBUG级别）
	debugLogFileName := fmt.Sprintf("%s/debug_%s.log", logDir, dateStr)
	debugFile, err := os.OpenFile(debugLogFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		infoFile.Close()
		return nil, nil, nil, nil, err
	}

	return infoFile, infoFile, debugFile, debugFile, nil
}

// cleanOldLogs 清理旧日志
func cleanOldLogs(logDir string, keepDays int) error {
	var firstErr error
	// 获取当前时间
	now := time.Now()

	// 计算过期时间
	expireTime := now.AddDate(0, 0, -keepDays)

	// 遍历日志目录，匹配info_*.log和debug_*.log
	patterns := []string{"info_*.log", "debug_*.log"}

	for _, pattern := range patterns {
		files, err := filepath.Glob(filepath.Join(logDir, pattern))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			pkglogger.WithContext(nil, "logger").Log(pkglogger.WARN, "日志匹配失败", pkglogger.Fields{"pattern": pattern}, err)
			continue
		}

		// 删除超过保留天数的日志文件
		for _, file := range files {
			// 获取文件信息
			info, err := os.Stat(file)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				pkglogger.WithContext(nil, "logger").Log(pkglogger.WARN, "日志属性读取失败", nil, err)
				continue
			}

			// 检查文件是否过期
			if info.ModTime().Before(expireTime) {
				if err := os.Remove(file); err != nil {
					if firstErr == nil {
						firstErr = err
					}
					pkglogger.WithContext(nil, "logger").Log(pkglogger.WARN, "过期日志删除失败", nil, err)
				} else {
					pkglogger.WithContext(nil, "logger").Log(pkglogger.INFO, "过期日志已删除", nil, nil)
				}
			}
		}
	}
	return firstErr
}

// Debug 调试日志
func Debug(format string, v ...interface{}) {
	pkglogger.Debug(format, v...)
}

// Info 信息日志
func Info(format string, v ...interface{}) {
	pkglogger.Info(format, v...)
}

// Warn 警告日志
func Warn(format string, v ...interface{}) {
	pkglogger.Warn(format, v...)
}

// Error 错误日志
func Error(format string, v ...interface{}) {
	pkglogger.Error(format, v...)
}
