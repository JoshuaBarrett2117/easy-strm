package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// initWindowsConsole 初始化Windows控制台UTF-8编码
func initWindowsConsole() {
	stdout := windows.Handle(os.Stdout.Fd())
	windows.SetConsoleOutputCP(65001)
	windows.SetConsoleCP(65001)

	var mode uint32
	windows.GetConsoleMode(stdout, &mode)
	mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	windows.SetConsoleMode(stdout, mode)
}
