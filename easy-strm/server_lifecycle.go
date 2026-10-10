package main

import pkglogger "easy-strm/internal/pkg/logger"

func logServerLifecycle(event string, err error) {
	level := pkglogger.INFO
	if err != nil {
		level = pkglogger.ERROR
	}
	pkglogger.WithContext(nil, "lifecycle").Log(level, "HTTP 服务生命周期", pkglogger.Fields{"event": event}, err)
}
