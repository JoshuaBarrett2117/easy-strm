package logger

import (
	"fmt"
	"log"
	"runtime"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var levelNames = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
}

var currentLevel = INFO

func SetLevel(level Level) {
	currentLevel = level
}

func Debug(format string, v ...interface{}) {
	if currentLevel <= DEBUG {
		log.Printf("[%s] [DEBUG] %s %s", time.Now().Format("2006-01-02 15:04:05"), formatCaller(), fmt.Sprintf(format, v...))
	}
}

func Debugf(format string, v ...interface{}) {
	Debug(format, v...)
}

func Info(format string, v ...interface{}) {
	if currentLevel <= INFO {
		log.Printf("[%s] [INFO] %s", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, v...))
	}
}

func Infof(format string, v ...interface{}) {
	Info(format, v...)
}

func Warn(format string, v ...interface{}) {
	if currentLevel <= WARN {
		log.Printf("[%s] [WARN] %s", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, v...))
	}
}

func Warnf(format string, v ...interface{}) {
	Warn(format, v...)
}

func Error(format string, v ...interface{}) {
	if currentLevel <= ERROR {
		log.Printf("[%s] [ERROR] %s", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, v...))
	}
}

func Errorf(format string, v ...interface{}) {
	Error(format, v...)
}

func formatCaller() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown:0"
	}
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' || file[i] == '\\' {
			file = file[i+1:]
			break
		}
	}
	return fmt.Sprintf("%s:%d", file, line)
}
