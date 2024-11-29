// internal/pkg/logger/logger.go

package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	// 预定义带颜色的打印函数
	debugPrintf = color.New(color.FgCyan).SprintfFunc()
	infoPrintf  = color.New(color.FgGreen).SprintfFunc()
	warnPrintf  = color.New(color.FgYellow).SprintfFunc()
	errorPrintf = color.New(color.FgRed).SprintfFunc()
)

type Logger struct {
	logger *log.Logger
	file   *os.File
	level  Level
	mu     sync.Mutex
}

func init() {
	color.NoColor = false
	defaultLogger = NewLogger()
}

func NewLogger() *Logger {
	// 创建logs目录
	if err := os.MkdirAll("logs", 0755); err != nil {
		panic(fmt.Sprintf("无法创建日志目录: %v", err))
	}

	// 创建日志文件，使用当前日期作为文件名
	filename := filepath.Join("logs", fmt.Sprintf("%s.log", time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(fmt.Sprintf("无法创建日志文件: %v", err))
	}

	// 创建多重输出
	writers := []io.Writer{os.Stdout, file}
	multiWriter := io.MultiWriter(writers...)

	return &Logger{
		logger: log.New(multiWriter, "", log.LstdFlags),
		file:   file,
		level:  InfoLevel,
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

// 在程序退出时关闭日志文件
func Close() {
	if defaultLogger.file != nil {
		defaultLogger.file.Close()
	}
}
