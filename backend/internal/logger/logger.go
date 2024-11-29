// internal/pkg/logger/logger.go

package logger

import (
	"io"
	"log"
	"os"
	"sync"

	"github.com/fatih/color"
)

type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	OffLevel
)

var (
	defaultLogger *Logger
	once          sync.Once

	// 预定义带颜色的打印函数
	debugPrintf = color.New(color.FgCyan).SprintfFunc()
	infoPrintf  = color.New(color.FgGreen).SprintfFunc()
	warnPrintf  = color.New(color.FgYellow).SprintfFunc()
	errorPrintf = color.New(color.FgRed).SprintfFunc()
)

type Logger struct {
	logger *log.Logger
	level  Level
	mu     sync.Mutex
}

func init() {
	color.NoColor = false
	defaultLogger = &Logger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
		level:  OffLevel,
	}
}

func SetLevel(level Level) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.level = level
}

func SetOutput(w io.Writer) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.logger = log.New(w, "", log.LstdFlags)

	// 如果输出不是终端，禁用颜色
	if f, ok := w.(*os.File); !ok || (f != os.Stdout && f != os.Stderr) {
		color.NoColor = true
	}
}

func Debug(format string, v ...interface{}) {
	if defaultLogger.level <= DebugLevel {
		msg := debugPrintf("[DEBUG] "+format, v...)
		defaultLogger.logger.Print(msg)
	}
}

func Info(format string, v ...interface{}) {
	if defaultLogger.level <= InfoLevel {
		msg := infoPrintf("[INFO] "+format, v...)
		defaultLogger.logger.Print(msg)
	}
}

func Warn(format string, v ...interface{}) {
	if defaultLogger.level <= WarnLevel {
		msg := warnPrintf("[WARN] "+format, v...)
		defaultLogger.logger.Print(msg)
	}
}

func Error(format string, v ...interface{}) {
	if defaultLogger.level <= ErrorLevel {
		msg := errorPrintf("[ERROR] "+format, v...)
		defaultLogger.logger.Print(msg)
	}
}
