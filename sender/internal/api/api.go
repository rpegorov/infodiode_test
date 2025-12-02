package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/infodiode/sender/internal/broker"
	"github.com/infodiode/sender/internal/generator"
	"github.com/infodiode/sender/internal/tcp"
	"github.com/infodiode/sender/internal/test"
	"github.com/infodiode/shared/models"
	"go.uber.org/zap"
)

// API структура HTTP API сервера
type API struct {
	router       *gin.Engine
	logger       *zap.Logger
	producer     *broker.MQTTProducer
	generator    *generator.DataGenerator
	testManager  *test.Manager
	server       *http.Server
	mu           sync.RWMutex
	currentTest  *models.TestConfig
	isTestActive bool
}

// Config конфигурация API
type Config struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// NewAPI создает новый API сервер
func NewAPI(
	cfg *Config,
	logger *zap.Logger,
	producer *broker.MQTTProducer,
	generator *generator.DataGenerator,
	tcpClient *tcp.TCPClient,
) *API {
	api := &API{
		logger:      logger,
		producer:    producer,
		generator:   generator,
		testManager: test.NewManager(logger, producer, tcpClient, generator),
	}

	api.setupRouter()

	api.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      api.router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return api
}

// setupRouter настраивает маршруты
func (api *API) setupRouter() {
	api.router = gin.New()

	// Middleware
	api.router.Use(gin.Recovery())
	api.router.Use(api.loggingMiddleware())
	api.router.Use(api.corsMiddleware())

	// Health checks
	api.router.GET("/health", api.healthCheck)
	api.router.GET("/ready", api.readyCheck)

	// Metrics
	api.router.GET("/metrics", api.prometheusMetrics)

	// Test management
	testGroup := api.router.Group("/test")
	{
		testGroup.POST("/batch", api.startBatchTest)
		testGroup.POST("/stream", api.startStreamTest)
		testGroup.POST("/large", api.startLargeTest)
		testGroup.POST("/stop", api.stopTest)
	}

	// Statistics
	api.router.GET("/stats", api.getStats)

	// Generator
	api.router.POST("/generate", api.generateData)
}

// loggingMiddleware middleware для логирования запросов
func (api *API) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Обработка запроса
		c.Next()

		// Логирование после обработки
		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		api.logger.Info("HTTP запрос",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", statusCode),
			zap.String("client_ip", clientIP),
			zap.Duration("latency", latency),
		)
	}
}

// corsMiddleware middleware для CORS
func (api *API) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// healthCheck проверка состояния сервиса
func (api *API) healthCheck(c *gin.Context) {
	status := models.HealthStatus{
		Status:    "healthy",
		Service:   "sender",
		Version:   "1.0.0",
		Timestamp: time.Now(),
		Checks:    []models.Check{},
	}

	// Проверка подключения к MQTT
	mqttCheck := models.Check{
		Component: "mqtt",
		Status:    "healthy",
	}

	if !api.producer.IsConnected() {
		mqttCheck.Status = "unhealthy"
		mqttCheck.Message = "MQTT broker disconnected"
		status.Status = "unhealthy"
	}

	status.Checks = append(status.Checks, mqttCheck)

	// Проверка тестового менеджера
	testCheck := models.Check{
		Component: "test_manager",
		Status:    "healthy",
	}

	if api.isTestActive {
		testCheck.Message = fmt.Sprintf("Test running: %s", api.currentTest.Type)
	}

	status.Checks = append(status.Checks, testCheck)

	if status.Status == "healthy" {
		c.JSON(http.StatusOK, status)
	} else {
		c.JSON(http.StatusServiceUnavailable, status)
	}
}

// readyCheck проверка готовности сервиса
func (api *API) readyCheck(c *gin.Context) {
	if api.producer.IsConnected() {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
	}
}

// startBatchTest запуск пакетного теста
func (api *API) startBatchTest(c *gin.Context) {
	var req BatchTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Проверка, что нет активного теста
	api.mu.RLock()
	if api.isTestActive {
		api.mu.RUnlock()
		c.JSON(http.StatusConflict, gin.H{"error": "тест уже запущен"})
		return
	}
	api.mu.RUnlock()

	// Создание конфигурации теста
	config := &models.TestConfig{
		Type:          models.TestTypeBatch,
		Protocol:      req.Protocol,
		ThreadCount:   req.ThreadCount,
		PacketSize:    req.PacketSize,
		TotalMessages: req.TotalMessages,
		Duration:      req.Duration,
	}

	// Установка протокола по умолчанию, если не указан
	if config.Protocol == "" {
		config.Protocol = models.ProtocolMQTT
	}

	// Запуск теста
	api.mu.Lock()
	api.currentTest = config
	api.isTestActive = true
	api.mu.Unlock()

	go func() {
		defer func() {
			api.mu.Lock()
			api.isTestActive = false
			api.mu.Unlock()
		}()

		if err := api.testManager.RunBatchTest(config); err != nil {
			api.logger.Error("Ошибка выполнения batch теста", zap.Error(err))
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "started",
		"test_id": time.Now().Unix(),
		"config":  config,
	})
}

// startStreamTest запуск потокового теста
func (api *API) startStreamTest(c *gin.Context) {
	var req StreamTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Проверка, что нет активного теста
	api.mu.RLock()
	if api.isTestActive {
		api.mu.RUnlock()
		c.JSON(http.StatusConflict, gin.H{"error": "тест уже запущен"})
		return
	}
	api.mu.RUnlock()

	// Создание конфигурации теста
	config := &models.TestConfig{
		Type:           models.TestTypeStream,
		Protocol:       req.Protocol,
		MessagesPerSec: req.MessagesPerSec,
		PacketSize:     req.PacketSize,
		Duration:       req.Duration,
		ThreadCount:    1, // Потоковый тест использует один поток
	}

	// Установка протокола по умолчанию, если не указан
	if config.Protocol == "" {
		config.Protocol = models.ProtocolMQTT
	}

	// Запуск теста
	api.mu.Lock()
	api.currentTest = config
	api.isTestActive = true
	api.mu.Unlock()

	go func() {
		defer func() {
			api.mu.Lock()
			api.isTestActive = false
			api.mu.Unlock()
		}()

		if err := api.testManager.RunStreamTest(config); err != nil {
			api.logger.Error("Ошибка выполнения stream теста", zap.Error(err))
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "started",
		"test_id": time.Now().Unix(),
		"config":  config,
	})
}

// startLargeTest запуск теста с большими пакетами
func (api *API) startLargeTest(c *gin.Context) {
	var req LargeTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Проверка, что нет активного теста
	api.mu.RLock()
	if api.isTestActive {
		api.mu.RUnlock()
		c.JSON(http.StatusConflict, gin.H{"error": "тест уже запущен"})
		return
	}
	api.mu.RUnlock()

	// Создание конфигурации теста
	config := &models.TestConfig{
		Type:        models.TestTypeLarge,
		Protocol:    req.Protocol,
		ThreadCount: req.ThreadCount,
		PacketSize:  req.PacketSizeMB * 1024 * 1024, // Конвертация MB в байты
		Duration:    req.Duration,
	}

	// Установка протокола по умолчанию, если не указан
	if config.Protocol == "" {
		config.Protocol = models.ProtocolMQTT
	}

	// Запуск теста
	api.mu.Lock()
	api.currentTest = config
	api.isTestActive = true
	api.mu.Unlock()

	go func() {
		defer func() {
			api.mu.Lock()
			api.isTestActive = false
			api.mu.Unlock()
		}()

		if err := api.testManager.RunLargeTest(config); err != nil {
			api.logger.Error("Ошибка выполнения large теста", zap.Error(err))
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "started",
		"test_id": time.Now().Unix(),
		"config":  config,
	})
}

// stopTest остановка текущего теста
func (api *API) stopTest(c *gin.Context) {
	api.mu.RLock()
	if !api.isTestActive {
		api.mu.RUnlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "нет активного теста"})
		return
	}
	api.mu.RUnlock()

	if err := api.testManager.StopCurrentTest(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	api.mu.Lock()
	api.isTestActive = false
	api.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

// getStats получение статистики
func (api *API) getStats(c *gin.Context) {
	producerStats := api.producer.GetStats()
	testStats := api.testManager.GetStats()

	api.mu.RLock()
	isActive := api.isTestActive
	var currentTestType string
	if api.currentTest != nil {
		currentTestType = string(api.currentTest.Type)
	}
	api.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"producer":     producerStats,
		"test":         testStats,
		"active":       isActive,
		"current_test": currentTestType,
	})
}

// generateData генерация тестовых данных
func (api *API) generateData(c *gin.Context) {
	var req GenerateDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	go func() {
		switch req.Type {
		case "all":
			if err := api.generator.GenerateAllTestData(); err != nil {
				api.logger.Error("Ошибка генерации всех данных", zap.Error(err))
			}
		case "small":
			if err := api.generator.GenerateSmallBatches(); err != nil {
				api.logger.Error("Ошибка генерации маленьких пакетов", zap.Error(err))
			}
		case "medium":
			if err := api.generator.GenerateMediumBatches(); err != nil {
				api.logger.Error("Ошибка генерации средних пакетов", zap.Error(err))
			}
		case "large":
			if err := api.generator.GenerateLargeBatches(); err != nil {
				api.logger.Error("Ошибка генерации больших пакетов", zap.Error(err))
			}
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"status": "generation started"})
}

// prometheusMetrics возвращает метрики в формате Prometheus
func (api *API) prometheusMetrics(c *gin.Context) {
	// TODO: Реализовать экспорт метрик в формате Prometheus
	c.String(http.StatusOK, "# HELP mqtt_messages_sent_total Total number of messages sent\n")
}

// Start запускает HTTP сервер
func (api *API) Start() error {
	api.logger.Info("Запуск HTTP API сервера", zap.String("addr", api.server.Addr))
	return api.server.ListenAndServe()
}

// Shutdown корректно останавливает HTTP сервер
func (api *API) Shutdown(ctx context.Context) error {
	api.logger.Info("Остановка HTTP API сервера")
	return api.server.Shutdown(ctx)
}

// Request structures

// BatchTestRequest запрос на запуск пакетного теста
type BatchTestRequest struct {
	Protocol      models.TestProtocol `json:"protocol" binding:"omitempty,oneof=mqtt tcp"`
	ThreadCount   int                 `json:"thread_count" binding:"required,min=1,max=1000"`
	PacketSize    int                 `json:"packet_size" binding:"required,min=100"`
	TotalMessages int                 `json:"total_messages" binding:"required,min=1"`
	Duration      int                 `json:"duration" binding:"required,min=1"`
}

// StreamTestRequest запрос на запуск потокового теста
type StreamTestRequest struct {
	Protocol       models.TestProtocol `json:"protocol" binding:"omitempty,oneof=mqtt tcp"`
	MessagesPerSec int                 `json:"messages_per_sec" binding:"required,min=1,max=100000"`
	PacketSize     int                 `json:"packet_size" binding:"required,min=100"`
	Duration       int                 `json:"duration" binding:"required,min=1"`
}

// LargeTestRequest запрос на запуск теста с большими пакетами
type LargeTestRequest struct {
	Protocol     models.TestProtocol `json:"protocol" binding:"omitempty,oneof=mqtt tcp"`
	ThreadCount  int                 `json:"thread_count" binding:"required,min=1,max=100"`
	PacketSizeMB int                 `json:"packet_size_mb" binding:"required,min=1,max=1000"`
	Duration     int                 `json:"duration" binding:"required,min=1"`
}

// GenerateDataRequest запрос на генерацию данных
type GenerateDataRequest struct {
	Type string `json:"type" binding:"required,oneof=all small medium large"`
}
