package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LogLevel represents the severity of a log entry
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// OutputFormat represents the format of log output
type OutputFormat int

const (
	TextFormat OutputFormat = iota
	JSONFormat
)

// ANSI color codes for different log levels
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m" // Error
	ColorYellow = "\033[33m" // Warn
	ColorBlue   = "\033[34m" // Info
	ColorCyan   = "\033[36m" // Debug
)

// Config holds the logger configuration
type Config struct {
	Level      LogLevel
	Format     OutputFormat
	Output     io.Writer
	FileOutput string
	UseColors  bool
}

// Logger wraps slog.Logger with additional functionality
type Logger struct {
	logger     *slog.Logger
	config     Config
	fileWriter io.WriteCloser
	mutex      sync.RWMutex
}

// coloredHandler implements slog.Handler with color support
type coloredHandler struct {
	writer    io.Writer
	useColors bool
	isJSON    bool
}

func (h *coloredHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *coloredHandler) Handle(ctx context.Context, record slog.Record) error {
	if h.isJSON {
		// Use default JSON formatting
		jsonHandler := slog.NewJSONHandler(h.writer, &slog.HandlerOptions{
			Level: record.Level,
		})
		return jsonHandler.Handle(ctx, record)
	}

	// Custom text formatting with colors
	var color string
	if h.useColors {
		switch record.Level {
		case slog.LevelDebug:
			color = ColorCyan
		case slog.LevelInfo:
			color = ColorBlue
		case slog.LevelWarn:
			color = ColorYellow
		case slog.LevelError:
			color = ColorRed
		}
	}

	timestamp := record.Time.Format("2006-01-02 15:04:05")
	level := strings.ToUpper(record.Level.String())

	var message strings.Builder

	if h.useColors && color != "" {
		message.WriteString(color)
	}

	message.WriteString(fmt.Sprintf("[%s] %s: %s", timestamp, level, record.Message))

	// Add attributes if any
	record.Attrs(func(attr slog.Attr) bool {
		message.WriteString(fmt.Sprintf(" %s=%v", attr.Key, attr.Value))
		return true
	})

	if h.useColors && color != "" {
		message.WriteString(ColorReset)
	}

	message.WriteString("\n")

	_, err := h.writer.Write([]byte(message.String()))
	return err
}

func (h *coloredHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// For simplicity, return the same handler
	return h
}

func (h *coloredHandler) WithGroup(name string) slog.Handler {
	// For simplicity, return the same handler
	return h
}

// New creates a new logger with the given configuration
func New(config Config) *Logger {
	var writer io.Writer
	var fileWriter io.WriteCloser

	// Set up writers based on configuration
	if config.FileOutput != "" {
		// Create directory if it doesn't exist
		dir := filepath.Dir(config.FileOutput)
		if err := os.MkdirAll(dir, 0755); err != nil {
			panic(fmt.Sprintf("failed to create log directory: %v", err))
		}

		file, err := os.OpenFile(config.FileOutput, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			panic(fmt.Sprintf("failed to open log file: %v", err))
		}
		fileWriter = file

		if config.Output != nil {
			// Write to both file and specified output
			writer = io.MultiWriter(config.Output, file)
		} else {
			writer = file
		}
	} else if config.Output != nil {
		writer = config.Output
	} else {
		writer = os.Stdout
	}

	// Create handler
	var handler slog.Handler
	if config.Format == JSONFormat {
		handler = &coloredHandler{
			writer:    writer,
			useColors: false, // JSON doesn't use colors
			isJSON:    true,
		}
	} else {
		handler = &coloredHandler{
			writer:    writer,
			useColors: config.UseColors,
			isJSON:    false,
		}
	}

	// Create slog logger
	slogLogger := slog.New(handler)

	return &Logger{
		logger:     slogLogger,
		config:     config,
		fileWriter: fileWriter,
	}
}

// NewConsole creates a logger that outputs to console with colors
func NewConsole(level LogLevel, format OutputFormat) *Logger {
	return New(Config{
		Level:     level,
		Format:    format,
		Output:    os.Stdout,
		UseColors: true,
	})
}

// NewFile creates a logger that outputs to a file
func NewFile(level LogLevel, format OutputFormat, filePath string) *Logger {
	return New(Config{
		Level:      level,
		Format:     format,
		FileOutput: filePath,
		UseColors:  false,
	})
}

// NewBoth creates a logger that outputs to both console and file
func NewBoth(level LogLevel, format OutputFormat, filePath string) *Logger {
	return New(Config{
		Level:      level,
		Format:     format,
		Output:     os.Stdout,
		FileOutput: filePath,
		UseColors:  true, // Colors for console, plain text for file
	})
}

// Close closes any open file handles
func (l *Logger) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.fileWriter != nil {
		return l.fileWriter.Close()
	}
	return nil
}

// Printf-style logging methods

// Infof logs an info message with printf-style formatting
func (l *Logger) Infof(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

// Debugf logs a debug message with printf-style formatting
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}

// Warnf logs a warning message with printf-style formatting
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs an error message with printf-style formatting
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
}

// Regular logging methods

// Info logs an info message
func (l *Logger) Info(msg string, args ...interface{}) {
	if len(args) > 0 {
		l.logger.Info(fmt.Sprintf(msg, args...))
	} else {
		l.logger.Info(msg)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...interface{}) {
	if len(args) > 0 {
		l.logger.Debug(fmt.Sprintf(msg, args...))
	} else {
		l.logger.Debug(msg)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...interface{}) {
	if len(args) > 0 {
		l.logger.Warn(fmt.Sprintf(msg, args...))
	} else {
		l.logger.Warn(msg)
	}
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...interface{}) {
	if len(args) > 0 {
		l.logger.Error(fmt.Sprintf(msg, args...))
	} else {
		l.logger.Error(msg)
	}
}

// WithFields returns a new logger with the specified fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}

	newLogger := l.logger.With(args...)
	return &Logger{
		logger:     newLogger,
		config:     l.config,
		fileWriter: l.fileWriter,
	}
}

// Default logger instance
var defaultLogger *Logger

// SetDefault sets the default logger
func SetDefault(logger *Logger) {
	defaultLogger = logger
}

// GetDefault returns the default logger, creating one if it doesn't exist
func GetDefault() *Logger {
	if defaultLogger == nil {
		defaultLogger = NewConsole(InfoLevel, TextFormat)
	}
	return defaultLogger
}

// Package-level convenience functions using the default logger

// Infof logs an info message using the default logger
func Infof(format string, args ...interface{}) {
	GetDefault().Infof(format, args...)
}

// Debugf logs a debug message using the default logger
func Debugf(format string, args ...interface{}) {
	GetDefault().Debugf(format, args...)
}

// Warnf logs a warning message using the default logger
func Warnf(format string, args ...interface{}) {
	GetDefault().Warnf(format, args...)
}

// Errorf logs an error message using the default logger
func Errorf(format string, args ...interface{}) {
	GetDefault().Errorf(format, args...)
}

// Info logs an info message using the default logger
func Info(msg string, args ...interface{}) {
	GetDefault().Info(msg, args...)
}

// Debug logs a debug message using the default logger
func Debug(msg string, args ...interface{}) {
	GetDefault().Debug(msg, args...)
}

// Warn logs a warning message using the default logger
func Warn(msg string, args ...interface{}) {
	GetDefault().Warn(msg, args...)
}

// Error logs an error message using the default logger
func Error(msg string, args ...interface{}) {
	GetDefault().Error(msg, args...)
}
