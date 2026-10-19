package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
)

// Logger struct with different log levels
type Logger struct {
	logger *log.Logger
}

var (
	instance *Logger
	once     sync.Once
)

// GetLogger returns a singleton logger instance
func GetLogger() *Logger {
	once.Do(func() {
		instance = &Logger{
			logger: log.New(stderr{}, "", log.Ldate|log.Ltime|log.Lshortfile),
		}
	})
	return instance
}

// Info logs an informational message
func (l *Logger) Info(format string, v ...interface{}) {
	l.logger.SetPrefix("[INFO] ")
	_ = l.logger.Output(2, fmt.Sprintf(format, v...))
}

// Warn logs a warning message
func (l *Logger) Warn(format string, v ...interface{}) {
	l.logger.SetPrefix("[WARN] ")
	_ = l.logger.Output(2, fmt.Sprintf(format, v...))
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	l.logger.SetPrefix("[ERROR] ")
	_ = l.logger.Output(2, fmt.Sprintf(format, v...))
}

// Fatal logs a fatal error message and exits
func (l *Logger) Fatal(format string, v ...interface{}) {
	l.logger.SetPrefix("[FATAL] ")
	_ = l.logger.Output(2, fmt.Sprintf(format, v...))
	os.Exit(1)
}

// stderr writes to whatever os.Stderr is when a line is logged, keeping debug
// output out of a report on stdout.
type stderr struct{}

func (stderr) Write(p []byte) (int, error) { return os.Stderr.Write(p) }
