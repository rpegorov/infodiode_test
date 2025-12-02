package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/infodiode/sender/config"
	"github.com/infodiode/sender/internal/api"
	"github.com/infodiode/sender/internal/broker"
	"github.com/infodiode/sender/internal/generator"
	"github.com/infodiode/sender/internal/logger"
	"github.com/infodiode/sender/internal/tcp"
	"go.uber.org/zap"
)

var (
	// Version информация о версии (устанавливается при сборке)
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Парсинг флагов командной строки
	var (
		configPath   = flag.String("config", "config.yaml", "путь к файлу конфигурации")
		showVersion  = flag.Bool("version", false, "показать версию и выйти")
		generateOnly = flag.Bool("generate", false, "только сгенерировать тестовые данные и выйти")
	)
	flag.Parse()

	// Показываем версию если запрошено
	if *showVersion {
		fmt.Printf("Sender Service\nVersion: %s\nBuild time: %s\n", Version, BuildTime)
		os.Exit(0)
	}

	// Загружаем конфигурацию
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Ошибка загрузки конфигурации: %v\n", err)
		os.Exit(1)
	}

	// Инициализируем логгер
	log, err := logger.New(logger.Config{
		Level:      cfg.Logger.Level,
		FilePath:   cfg.Logger.FilePath,
		MaxSize:    cfg.Logger.MaxSize,
		MaxBackups: cfg.Logger.MaxBackups,
		MaxAge:     cfg.Logger.MaxAge,
		Compress:   cfg.Logger.Compress,
		Console:    cfg.Logger.Console,
	})
	if err != nil {
		fmt.Printf("Ошибка инициализации логгера: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	// Логируем информацию о запуске
	log.Info("Запуск Sender сервиса",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("config", *configPath))

	// Создаем генератор данных
	genConfig := &generator.Config{
		DataPath:         cfg.Data.DataPath,
		Seed:             cfg.Data.GeneratorSeed,
		IndicatorIDRange: cfg.Data.IndicatorIDRange,
		EquipmentIDRange: cfg.Data.EquipmentIDRange,
		NullPercent:      cfg.Data.NullPercent,
		BoolPercent:      cfg.Data.BoolPercent,
		FloatPercent:     cfg.Data.FloatPercent,
		StringPercent:    cfg.Data.StringPercent,
		SmallBatchSize:   cfg.Data.SmallBatchSize,
		MediumBatchSize:  cfg.Data.MediumBatchSize,
		LargeBatchSizes:  cfg.Data.LargeBatchSizes,
	}
	dataGenerator := generator.NewDataGenerator(genConfig, log.Logger)

	// Если указан флаг generate, генерируем данные и выходим
	if *generateOnly {
		log.Info("Режим генерации данных")
		if err := dataGenerator.GenerateAllTestData(); err != nil {
			log.Error("Ошибка генерации данных", zap.Error(err))
			os.Exit(1)
		}
		log.Info("Генерация данных завершена успешно")
		os.Exit(0)
	}

	// Проверяем наличие тестовых данных
	stats, err := dataGenerator.GetStatistics()
	if err != nil {
		log.Error("Ошибка получения статистики данных", zap.Error(err))
	} else {
		log.Info("Статистика тестовых данных",
			zap.Int("small_batches", stats.SmallBatches),
			zap.Int("medium_batches", stats.MediumBatches),
			zap.Int("large_batches", stats.LargeBatches),
			zap.Int64("total_size", stats.TotalSize))

		// Если данных нет, генерируем
		if stats.SmallBatches == 0 && stats.MediumBatches == 0 && stats.LargeBatches == 0 {
			log.Info("Тестовые данные отсутствуют, запуск генерации...")
			if err := dataGenerator.GenerateAllTestData(); err != nil {
				log.Error("Ошибка генерации данных", zap.Error(err))
				// Продолжаем работу, так как можно генерировать данные на лету
			}
		}
	}

	// Создаем MQTT producer
	producer, err := broker.NewMQTTProducer(&cfg.MQTT, log.Logger)
	if err != nil {
		log.Fatal("Ошибка создания MQTT producer", zap.Error(err))
	}
	defer producer.Close()

	// Создаем TCP client (если включен)
	var tcpClient *tcp.TCPClient
	if cfg.TCP.Enabled {
		tcpConfig := &tcp.Config{
			Address:         cfg.TCP.Address,
			ReconnectInt:    cfg.TCP.ReconnectInt,
			MaxRetries:      cfg.TCP.MaxRetries,
			Timeout:         cfg.TCP.Timeout,
			KeepAlive:       cfg.TCP.KeepAlive,
			KeepAlivePeriod: cfg.TCP.KeepAlivePeriod,
		}
		tcpClient, err = tcp.NewTCPClient(tcpConfig, log.Logger)
		if err != nil {
			log.Error("Ошибка создания TCP клиента", zap.Error(err))
			// Не завершаем работу, продолжаем без TCP
		} else {
			// Пытаемся подключиться
			if err := tcpClient.Connect(); err != nil {
				log.Warn("Не удалось подключиться к TCP серверу при старте", zap.Error(err))
			} else {
				log.Info("TCP клиент подключен", zap.String("address", cfg.TCP.Address))
			}
			defer func() {
				if err := tcpClient.Disconnect(); err != nil {
					log.Error("Ошибка отключения TCP клиента", zap.Error(err))
				}
			}()
		}
	}

	// Создаем HTTP API сервер
	apiConfig := &api.Config{
		Host:            cfg.HTTP.Host,
		Port:            cfg.HTTP.Port,
		ReadTimeout:     cfg.HTTP.ReadTimeout,
		WriteTimeout:    cfg.HTTP.WriteTimeout,
		ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
	}

	apiServer := api.NewAPI(apiConfig, log.Logger, producer, dataGenerator, tcpClient)

	// Канал для graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Канал для ошибок
	errChan := make(chan error, 1)

	// Запускаем HTTP сервер
	go func() {
		log.Info("Запуск HTTP API сервера",
			zap.String("host", cfg.HTTP.Host),
			zap.Int("port", cfg.HTTP.Port))

		if err := apiServer.Start(); err != nil {
			errChan <- fmt.Errorf("ошибка HTTP сервера: %w", err)
		}
	}()

	// Ожидаем сигнал завершения или ошибку
	select {
	case sig := <-shutdown:
		log.Info("Получен сигнал завершения", zap.String("signal", sig.String()))
	case err := <-errChan:
		log.Error("Критическая ошибка", zap.Error(err))
	}

	// Graceful shutdown
	log.Info("Начало graceful shutdown...")

	// Создаем контекст с таймаутом для shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	// Останавливаем HTTP сервер
	if err := apiServer.Shutdown(ctx); err != nil {
		log.Error("Ошибка остановки HTTP сервера", zap.Error(err))
	}

	// Закрываем MQTT соединение
	if err := producer.Close(); err != nil {
		log.Error("Ошибка закрытия MQTT producer", zap.Error(err))
	}

	// Очищаем кеш генератора
	dataGenerator.ClearCache()

	log.Info("Sender сервис остановлен")
}
