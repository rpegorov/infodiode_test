package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger обертка для zap логгера
type Logger struct {
	*zap.Logger
	sugar *zap.SugaredLogger
}

// Config конфигурация логгера
type Config struct {
	Level      string
	FilePath   string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
	Console    bool
}

// New создает новый экземпляр логгера
func New(cfg Config) (*Logger, error) {
	// Парсим уровень логирования
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("неверный уровень логирования: %w", err)
	}

	// Создаем encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Создаем JSON encoder для файла
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	// Создаем cores
	var cores []zapcore.Core

	// Файловый core с ротацией
	if cfg.FilePath != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    cfg.MaxSize, // megabytes
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge, // days
			Compress:   cfg.Compress,
			LocalTime:  true,
		}

		fileCore := zapcore.NewCore(
			jsonEncoder,
			zapcore.AddSync(fileWriter),
			level,
		)
		cores = append(cores, fileCore)
	}

	// Консольный core
	if cfg.Console {
		consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
		consoleCore := zapcore.NewCore(
			consoleEncoder,
			zapcore.AddSync(os.Stdout),
			level,
		)
		cores = append(cores, consoleCore)
	}

	// Создаем tee core
	core := zapcore.NewTee(cores...)

	// Создаем логгер
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	logger := &Logger{
		Logger: zapLogger,
		sugar:  zapLogger.Sugar(),
	}

	return logger, nil
}

// parseLevel парсит уровень логирования из строки
func parseLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("неизвестный уровень: %s", level)
	}
}

// Sugar возвращает SugaredLogger для удобного использования
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.sugar
}

// WithFields добавляет поля к логгеру
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	newLogger := l.Logger.With(zapFields...)
	return &Logger{
		Logger: newLogger,
		sugar:  newLogger.Sugar(),
	}
}

// LogMessage логирует сообщение брокера
func (l *Logger) LogMessage(messageID int, sendTime string, checksum string, size int, threadCount int) {
	l.Info("Сообщение отправлено",
		zap.Int("message_id", messageID),
		zap.String("send_time", sendTime),
		zap.String("checksum", checksum),
		zap.Int("message_size", size),
		zap.Int("thread_count", threadCount),
	)
}

// LogError логирует ошибку с дополнительным контекстом
func (l *Logger) LogError(msg string, err error, fields ...zap.Field) {
	allFields := append([]zap.Field{zap.Error(err)}, fields...)
	l.Error(msg, allFields...)
}

// Close закрывает логгер
func (l *Logger) Close() error {
	return l.Logger.Sync()
}
