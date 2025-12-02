package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/infodiode/recipient/config"
	"github.com/infodiode/recipient/internal/broker"
	"github.com/infodiode/recipient/internal/processor"
	"github.com/infodiode/recipient/internal/tcp"
	"github.com/infodiode/shared/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Version информация о версии (устанавливается при сборке)
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Парсинг флагов командной строки
	var (
		configPath  = flag.String("config", "config.yaml", "путь к файлу конфигурации")
		showVersion = flag.Bool("version", false, "показать версию и выйти")
	)
	flag.Parse()

	// Показываем версию если запрошено
	if *showVersion {
		fmt.Printf("Recipient Service\nVersion: %s\nBuild time: %s\n", Version, BuildTime)
		os.Exit(0)
	}

	// Загружаем конфигурацию
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Ошибка загрузки конфигурации: %v\n", err)
		os.Exit(1)
	}

	// Инициализируем логгер
	logger, err := initLogger(cfg)
	if err != nil {
		fmt.Printf("Ошибка инициализации логгера: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Логируем информацию о запуске
	logger.Info("Запуск Recipient сервиса",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("config", *configPath))

	// Создаем обработчик сообщений
	msgProcessor := processor.NewMessageProcessor(logger)

	// Создаем обработчик для MQTT consumer
	messageHandler := func(msg *models.Message) error {
		return msgProcessor.ProcessMessage(msg)
	}

	// Создаем MQTT consumer
	consumer, err := broker.NewMQTTConsumer(&cfg.MQTT, logger, messageHandler)
	if err != nil {
		logger.Fatal("Ошибка создания MQTT consumer", zap.Error(err))
	}
	defer consumer.Close()

	// Запускаем consumer
	if err := consumer.Start(); err != nil {
		logger.Fatal("Ошибка запуска consumer", zap.Error(err))
	}

	// Создаем и запускаем TCP сервер (если включен)
	var tcpServer *tcp.TCPServer
	if cfg.TCP.Enabled {
		tcpConfig := &tcp.Config{
			Address:         cfg.TCP.Address,
			MaxConnections:  cfg.TCP.MaxConnections,
			ReadTimeout:     cfg.TCP.ReadTimeout,
			WriteTimeout:    cfg.TCP.WriteTimeout,
			KeepAlive:       cfg.TCP.KeepAlive,
			KeepAlivePeriod: cfg.TCP.KeepAlivePeriod,
		}

		tcpServer, err = tcp.NewTCPServer(tcpConfig, logger, msgProcessor)
		if err != nil {
			logger.Error("Ошибка создания TCP сервера", zap.Error(err))
		} else {
			if err := tcpServer.Start(); err != nil {
				logger.Error("Ошибка запуска TCP сервера", zap.Error(err))
			} else {
				logger.Info("TCP сервер запущен", zap.String("address", cfg.TCP.Address))
			}

			defer func() {
				if err := tcpServer.Stop(); err != nil {
					logger.Error("Ошибка остановки TCP сервера", zap.Error(err))
				}
			}()
		}
	}

	// Запускаем HTTP сервер для метрик и health checks
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := models.HealthStatus{
			Status:    "healthy",
			Service:   "recipient",
			Version:   Version,
			Timestamp: time.Now(),
			Checks:    []models.Check{},
		}

		// Проверка подключения к MQTT
		mqttCheck := models.Check{
			Component: "mqtt",
			Status:    "healthy",
		}

		if !consumer.IsConnected() {
			mqttCheck.Status = "unhealthy"
			mqttCheck.Message = "MQTT broker disconnected"
			status.Status = "unhealthy"
		}

		status.Checks = append(status.Checks, mqttCheck)

		// Проверка обработчика
		stats := msgProcessor.GetStats()
		processorCheck := models.Check{
			Component: "processor",
			Status:    "healthy",
			Message: fmt.Sprintf("Processed: %d, Valid: %d, Invalid: %d",
				stats.MessagesProcessed, stats.MessagesValid, stats.MessagesInvalid),
		}
		status.Checks = append(status.Checks, processorCheck)

		w.Header().Set("Content-Type", "application/json")
		if status.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		// Простой JSON вывод без внешних зависимостей
		fmt.Fprintf(w, `{"status":"%s","service":"%s","version":"%s"}`,
			status.Status, status.Service, status.Version)
	})

	// Ready check endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if consumer.IsConnected() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"ready"}`)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"not ready"}`)
		}
	})

	// Metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := msgProcessor.GetStats()
		consumerStats := consumer.GetStats()

		w.Header().Set("Content-Type", "text/plain")

		// Выводим метрики в формате Prometheus
		fmt.Fprintf(w, "# HELP messages_received_total Total number of messages received\n")
		fmt.Fprintf(w, "# TYPE messages_received_total counter\n")
		fmt.Fprintf(w, "messages_received_total %d\n", stats.MessagesReceived)

		fmt.Fprintf(w, "\n# HELP messages_processed_total Total number of messages processed\n")
		fmt.Fprintf(w, "# TYPE messages_processed_total counter\n")
		fmt.Fprintf(w, "messages_processed_total %d\n", stats.MessagesProcessed)

		fmt.Fprintf(w, "\n# HELP messages_valid_total Total number of valid messages\n")
		fmt.Fprintf(w, "# TYPE messages_valid_total counter\n")
		fmt.Fprintf(w, "messages_valid_total %d\n", stats.MessagesValid)

		fmt.Fprintf(w, "\n# HELP checksum_errors_total Total number of checksum errors\n")
		fmt.Fprintf(w, "# TYPE checksum_errors_total counter\n")
		fmt.Fprintf(w, "checksum_errors_total %d\n", stats.ChecksumErrors)

		fmt.Fprintf(w, "\n# HELP message_latency_ms Message processing latency in milliseconds\n")
		fmt.Fprintf(w, "# TYPE message_latency_ms summary\n")
		fmt.Fprintf(w, "message_latency_ms{quantile=\"0.5\"} %.2f\n", stats.AvgLatency)
		fmt.Fprintf(w, "message_latency_ms{quantile=\"0.95\"} %.2f\n", stats.MaxLatency)
		fmt.Fprintf(w, "message_latency_ms_sum %.2f\n", stats.AvgLatency*float64(stats.MessagesProcessed))
		fmt.Fprintf(w, "message_latency_ms_count %d\n", stats.MessagesProcessed)

		fmt.Fprintf(w, "\n# HELP throughput_messages_per_sec Current message throughput\n")
		fmt.Fprintf(w, "# TYPE throughput_messages_per_sec gauge\n")
		fmt.Fprintf(w, "throughput_messages_per_sec %.2f\n", stats.Throughput)

		fmt.Fprintf(w, "\n# HELP mqtt_connected MQTT connection status\n")
		fmt.Fprintf(w, "# TYPE mqtt_connected gauge\n")
		if consumerStats.Connected {
			fmt.Fprintf(w, "mqtt_connected 1\n")
		} else {
			fmt.Fprintf(w, "mqtt_connected 0\n")
		}
	})

	// Stats endpoint (JSON формат статистики)
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := msgProcessor.GetStats()
		consumerStats := consumer.GetStats()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"processor": {
				"messages_received": %d,
				"messages_processed": %d,
				"messages_valid": %d,
				"messages_invalid": %d,
				"checksum_errors": %d,
				"processing_errors": %d,
				"total_bytes_received": %d,
				"avg_message_size": %d,
				"min_latency_ms": %.2f,
				"max_latency_ms": %.2f,
				"avg_latency_ms": %.2f,
				"throughput_msg_per_sec": %.2f
			},
			"consumer": {
				"messages_received": %d,
				"bytes_received": %d,
				"errors": %d,
				"reconnect_count": %d,
				"connected": %t,
				"uptime_seconds": %.0f
			}
		}`,
			stats.MessagesReceived,
			stats.MessagesProcessed,
			stats.MessagesValid,
			stats.MessagesInvalid,
			stats.ChecksumErrors,
			stats.ProcessingErrors,
			stats.TotalBytesReceived,
			stats.AvgMessageSize,
			stats.MinLatency,
			stats.MaxLatency,
			stats.AvgLatency,
			stats.Throughput,
			consumerStats.MessagesReceived,
			consumerStats.BytesReceived,
			consumerStats.Errors,
			consumerStats.ReconnectCount,
			consumerStats.Connected,
			consumerStats.Uptime.Seconds())
	})

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Metrics.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Канал для graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Канал для ошибок
	errChan := make(chan error, 1)

	// Запускаем HTTP сервер
	go func() {
		logger.Info("Запуск HTTP сервера для метрик",
			zap.Int("port", cfg.Metrics.Port))

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("ошибка HTTP сервера: %w", err)
		}
	}()

	// Периодический вывод статистики в лог
	statsTicker := time.NewTicker(30 * time.Second)
	go func() {
		for range statsTicker.C {
			stats := msgProcessor.GetStats()
			logger.Info("Статистика обработки",
				zap.Int64("received", stats.MessagesReceived),
				zap.Int64("processed", stats.MessagesProcessed),
				zap.Int64("valid", stats.MessagesValid),
				zap.Int64("invalid", stats.MessagesInvalid),
				zap.Float64("throughput", stats.Throughput))
		}
	}()

	// Ожидаем сигнал завершения или ошибку
	select {
	case sig := <-shutdown:
		logger.Info("Получен сигнал завершения", zap.String("signal", sig.String()))
	case err := <-errChan:
		logger.Error("Критическая ошибка", zap.Error(err))
	}

	// Graceful shutdown
	logger.Info("Начало graceful shutdown...")
	statsTicker.Stop()

	// Создаем контекст с таймаутом для shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Останавливаем HTTP сервер
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("Ошибка остановки HTTP сервера", zap.Error(err))
	}

	// Останавливаем обработчик сообщений
	if err := msgProcessor.Stop(); err != nil {
		logger.Error("Ошибка остановки обработчика", zap.Error(err))
	}

	// Закрываем MQTT соединение
	if err := consumer.Close(); err != nil {
		logger.Error("Ошибка закрытия MQTT consumer", zap.Error(err))
	}

	// Выводим финальную статистику
	finalStats := msgProcessor.GetStats()
	logger.Info("Финальная статистика",
		zap.Int64("всего_получено", finalStats.MessagesReceived),
		zap.Int64("всего_обработано", finalStats.MessagesProcessed),
		zap.Int64("валидных", finalStats.MessagesValid),
		zap.Int64("невалидных", finalStats.MessagesInvalid),
		zap.Int64("ошибок_контрольной_суммы", finalStats.ChecksumErrors),
		zap.Float64("средняя_задержка_ms", finalStats.AvgLatency))

	logger.Info("Recipient сервис остановлен")
}

// initLogger инициализирует логгер
func initLogger(cfg *config.Config) (*zap.Logger, error) {
	// Парсим уровень логирования
	level, err := zapcore.ParseLevel(cfg.Logger.Level)
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
	if cfg.Logger.FilePath != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.Logger.FilePath,
			MaxSize:    cfg.Logger.MaxSize, // megabytes
			MaxBackups: cfg.Logger.MaxBackups,
			MaxAge:     cfg.Logger.MaxAge, // days
			Compress:   cfg.Logger.Compress,
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
	if cfg.Logger.Console {
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
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return logger, nil
}
