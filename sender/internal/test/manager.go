package test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/infodiode/sender/internal/broker"
	"github.com/infodiode/sender/internal/generator"
	"github.com/infodiode/sender/internal/tcp"
	"github.com/infodiode/shared/models"
	"github.com/infodiode/shared/utils"
	"go.uber.org/zap"
)

// Manager управляет выполнением тестов
type Manager struct {
	logger       *zap.Logger
	producer     *broker.MQTTProducer
	tcpClient    *tcp.TCPClient
	generator    *generator.DataGenerator
	currentTest  *TestContext
	mu           sync.RWMutex
	stopChan     chan struct{}
	messageIDGen atomic.Int64
}

// TestContext контекст выполнения теста
type TestContext struct {
	Config    *models.TestConfig
	Stats     *models.TestStats
	StartTime time.Time
	Cancel    context.CancelFunc
	ctx       context.Context
	wg        sync.WaitGroup
}

// NewManager создает новый менеджер тестов
func NewManager(logger *zap.Logger, producer *broker.MQTTProducer, tcpClient *tcp.TCPClient, generator *generator.DataGenerator) *Manager {
	return &Manager{
		logger:    logger,
		producer:  producer,
		tcpClient: tcpClient,
		generator: generator,
	}
}

// RunBatchTest запускает пакетный тест
func (m *Manager) RunBatchTest(config *models.TestConfig) error {
	m.logger.Info("Запуск пакетного теста",
		zap.String("protocol", string(config.Protocol)),
		zap.Int("threads", config.ThreadCount),
		zap.Int("packet_size", config.PacketSize),
		zap.Int("total_messages", config.TotalMessages))

	// Проверяем протокол и подключение
	if config.Protocol == models.ProtocolTCP {
		if m.tcpClient == nil {
			return fmt.Errorf("TCP клиент не инициализирован")
		}
		if !m.tcpClient.IsConnected() {
			if err := m.tcpClient.Connect(); err != nil {
				return fmt.Errorf("ошибка подключения к TCP серверу: %w", err)
			}
		}
	}

	// Создаем контекст теста
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Duration)*time.Second)
	defer cancel()

	testCtx := &TestContext{
		Config:    config,
		Stats:     &models.TestStats{StartTime: time.Now()},
		StartTime: time.Now(),
		Cancel:    cancel,
		ctx:       ctx,
	}

	m.mu.Lock()
	m.currentTest = testCtx
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	// Загружаем тестовые данные
	data, err := m.generator.GetDataForTest("medium", 1)
	if err != nil {
		return fmt.Errorf("ошибка загрузки данных для теста: %w", err)
	}

	// Запускаем потоки отправки
	messagesPerThread := config.TotalMessages / config.ThreadCount
	remainingMessages := config.TotalMessages % config.ThreadCount

	for i := 0; i < config.ThreadCount; i++ {
		messages := messagesPerThread
		if i == 0 {
			messages += remainingMessages
		}

		testCtx.wg.Add(1)
		go m.batchWorker(testCtx, i, messages, data)
	}

	// Ожидаем завершения
	testCtx.wg.Wait()

	// Финализируем статистику
	m.finalizeTestStats(testCtx)

	return nil
}

// batchWorker обработчик для пакетной отправки
func (m *Manager) batchWorker(testCtx *TestContext, workerID int, messageCount int, data []*models.Data) {
	defer testCtx.wg.Done()

	m.logger.Info("Запуск batch worker",
		zap.Int("worker_id", workerID),
		zap.Int("messages", messageCount))

	batchSize := 100 // Размер пакета для отправки
	if batchSize > messageCount {
		batchSize = messageCount
	}

	sent := 0
	dataIndex := 0

	for sent < messageCount {
		select {
		case <-testCtx.ctx.Done():
			m.logger.Info("Worker остановлен по таймауту",
				zap.Int("worker_id", workerID),
				zap.Int("sent", sent))
			return
		case <-m.stopChan:
			m.logger.Info("Worker остановлен пользователем",
				zap.Int("worker_id", workerID),
				zap.Int("sent", sent))
			return
		default:
		}

		// Формируем пакет сообщений
		currentBatch := batchSize
		if sent+currentBatch > messageCount {
			currentBatch = messageCount - sent
		}

		messages := make([]*models.Message, 0, currentBatch)
		for i := 0; i < currentBatch; i++ {
			// Берем данные циклически
			payload, _ := json.Marshal(data[dataIndex%len(data)])
			dataIndex++

			msg := &models.Message{
				MessageID: int(m.messageIDGen.Add(1)),
				SendTime:  utils.GetCurrentTime(),
				Timestamp: data[dataIndex%len(data)].Timestamp,
				Payload:   string(payload),
				Checksum:  utils.CalculateChecksumString(string(payload)),
			}
			messages = append(messages, msg)
		}

		// Отправляем пакет в зависимости от протокола
		startSend := time.Now()
		var err error

		if testCtx.Config.Protocol == models.ProtocolTCP {
			err = m.tcpClient.SendBatch(messages)
		} else {
			err = m.producer.PublishBatch(messages)
		}

		if err != nil {
			atomic.AddInt64(&testCtx.Stats.Errors, 1)
			m.logger.Error("Ошибка отправки пакета",
				zap.String("protocol", string(testCtx.Config.Protocol)),
				zap.Int("worker_id", workerID),
				zap.Error(err))
		} else {
			atomic.AddInt64(&testCtx.Stats.MessagesSent, int64(currentBatch))
			atomic.AddInt64(&testCtx.Stats.BytesSent, int64(len(messages[0].Payload)*currentBatch))

			// Обновляем статистику задержки
			latency := time.Since(startSend).Milliseconds()
			m.updateLatencyStats(testCtx, float64(latency))
		}

		sent += currentBatch

		// Логируем прогресс каждые 1000 сообщений
		if sent%1000 == 0 {
			m.logger.Info("Прогресс отправки",
				zap.Int("worker_id", workerID),
				zap.Int("sent", sent),
				zap.Int("total", messageCount))
		}
	}

	m.logger.Info("Worker завершен",
		zap.Int("worker_id", workerID),
		zap.Int("total_sent", sent))
}

// RunStreamTest запускает потоковый тест
func (m *Manager) RunStreamTest(config *models.TestConfig) error {
	m.logger.Info("Запуск потокового теста",
		zap.String("protocol", string(config.Protocol)),
		zap.Int("messages_per_sec", config.MessagesPerSec),
		zap.Int("duration", config.Duration))

	// Проверяем протокол и подключение
	if config.Protocol == models.ProtocolTCP {
		if m.tcpClient == nil {
			return fmt.Errorf("TCP клиент не инициализирован")
		}
		if !m.tcpClient.IsConnected() {
			if err := m.tcpClient.Connect(); err != nil {
				return fmt.Errorf("ошибка подключения к TCP серверу: %w", err)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Duration)*time.Second)
	defer cancel()

	testCtx := &TestContext{
		Config:    config,
		Stats:     &models.TestStats{StartTime: time.Now()},
		StartTime: time.Now(),
		Cancel:    cancel,
		ctx:       ctx,
	}

	m.mu.Lock()
	m.currentTest = testCtx
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	// Загружаем тестовые данные
	data, err := m.generator.GetDataForTest("small", 100)
	if err != nil {
		return fmt.Errorf("ошибка загрузки данных: %w", err)
	}

	// Рассчитываем интервал между сообщениями
	interval := time.Second / time.Duration(config.MessagesPerSec)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	dataIndex := 0
	for {
		select {
		case <-testCtx.ctx.Done():
			m.finalizeTestStats(testCtx)
			return nil
		case <-m.stopChan:
			m.finalizeTestStats(testCtx)
			return fmt.Errorf("тест остановлен пользователем")
		case <-ticker.C:
			// Отправляем одно сообщение
			payload, _ := json.Marshal(data[dataIndex%len(data)])
			dataIndex++

			msg := &models.Message{
				MessageID: int(m.messageIDGen.Add(1)),
				SendTime:  utils.GetCurrentTime(),
				Timestamp: data[dataIndex%len(data)].Timestamp,
				Payload:   string(payload),
				Checksum:  utils.CalculateChecksumString(string(payload)),
			}

			// Отправляем асинхронно чтобы не блокировать ticker
			go func(message *models.Message) {
				startSend := time.Now()
				var err error

				if testCtx.Config.Protocol == models.ProtocolTCP {
					err = m.tcpClient.Send(message)
				} else {
					err = m.producer.Publish(message)
				}

				if err != nil {
					atomic.AddInt64(&testCtx.Stats.Errors, 1)
				} else {
					atomic.AddInt64(&testCtx.Stats.MessagesSent, 1)
					atomic.AddInt64(&testCtx.Stats.BytesSent, int64(len(message.Payload)))

					latency := time.Since(startSend).Milliseconds()
					m.updateLatencyStats(testCtx, float64(latency))
				}
			}(msg)
		}
	}
}

// RunLargeTest запускает тест с большими пакетами
func (m *Manager) RunLargeTest(config *models.TestConfig) error {
	m.logger.Info("Запуск теста с большими пакетами",
		zap.String("protocol", string(config.Protocol)),
		zap.Int("threads", config.ThreadCount),
		zap.Int("packet_size", config.PacketSize))

	// Проверяем протокол и подключение
	if config.Protocol == models.ProtocolTCP {
		if m.tcpClient == nil {
			return fmt.Errorf("TCP клиент не инициализирован")
		}
		if !m.tcpClient.IsConnected() {
			if err := m.tcpClient.Connect(); err != nil {
				return fmt.Errorf("ошибка подключения к TCP серверу: %w", err)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Duration)*time.Second)
	defer cancel()

	testCtx := &TestContext{
		Config:    config,
		Stats:     &models.TestStats{StartTime: time.Now()},
		StartTime: time.Now(),
		Cancel:    cancel,
		ctx:       ctx,
	}

	m.mu.Lock()
	m.currentTest = testCtx
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	// Определяем размер файла в MB
	sizeMB := config.PacketSize / (1024 * 1024)
	if sizeMB < 5 {
		sizeMB = 5
	}

	// Загружаем большой файл данных
	data, err := m.generator.GetDataForTest("large", sizeMB)
	if err != nil {
		return fmt.Errorf("ошибка загрузки больших данных: %w", err)
	}

	// Запускаем потоки
	for i := 0; i < config.ThreadCount; i++ {
		testCtx.wg.Add(1)
		go m.largePacketWorker(testCtx, i, data)
	}

	testCtx.wg.Wait()
	m.finalizeTestStats(testCtx)

	return nil
}

// largePacketWorker обработчик для отправки больших пакетов
func (m *Manager) largePacketWorker(testCtx *TestContext, workerID int, data []*models.Data) {
	defer testCtx.wg.Done()

	m.logger.Info("Запуск large packet worker",
		zap.Int("worker_id", workerID),
		zap.Int("data_size", len(data)))

	sent := 0
	for {
		select {
		case <-testCtx.ctx.Done():
			m.logger.Info("Large worker остановлен по таймауту",
				zap.Int("worker_id", workerID),
				zap.Int("sent", sent))
			return
		case <-m.stopChan:
			m.logger.Info("Large worker остановлен пользователем",
				zap.Int("worker_id", workerID),
				zap.Int("sent", sent))
			return
		default:
		}

		// Создаем большое сообщение из всех данных
		payload, _ := json.Marshal(data)

		msg := &models.Message{
			MessageID: int(m.messageIDGen.Add(1)),
			SendTime:  utils.GetCurrentTime(),
			Timestamp: utils.GetCurrentTime(),
			Payload:   string(payload),
			Checksum:  utils.CalculateChecksumString(string(payload)),
		}

		startSend := time.Now()
		var err error

		if testCtx.Config.Protocol == models.ProtocolTCP {
			err = m.tcpClient.Send(msg)
		} else {
			err = m.producer.Publish(msg)
		}

		if err != nil {
			atomic.AddInt64(&testCtx.Stats.Errors, 1)
			m.logger.Error("Ошибка отправки большого пакета",
				zap.String("protocol", string(testCtx.Config.Protocol)),
				zap.Int("worker_id", workerID),
				zap.Int("size", len(payload)),
				zap.Error(err))
		} else {
			atomic.AddInt64(&testCtx.Stats.MessagesSent, 1)
			atomic.AddInt64(&testCtx.Stats.BytesSent, int64(len(payload)))

			latency := time.Since(startSend).Milliseconds()
			m.updateLatencyStats(testCtx, float64(latency))
			sent++
		}

		// Задержка между отправками больших пакетов
		time.Sleep(100 * time.Millisecond)
	}
}

// StopCurrentTest останавливает текущий тест
func (m *Manager) StopCurrentTest() error {
	m.mu.RLock()
	if m.currentTest == nil {
		m.mu.RUnlock()
		return fmt.Errorf("нет активного теста")
	}
	m.mu.RUnlock()

	close(m.stopChan)
	m.currentTest.Cancel()

	return nil
}

// GetStats возвращает статистику текущего или последнего теста
func (m *Manager) GetStats() *models.TestStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentTest == nil || m.currentTest.Stats == nil {
		return &models.TestStats{}
	}

	stats := *m.currentTest.Stats
	if stats.EndTime == nil && stats.StartTime.Unix() > 0 {
		stats.Duration = time.Since(stats.StartTime)
		if stats.MessagesSent > 0 {
			stats.AvgThroughput = float64(stats.MessagesSent) / stats.Duration.Seconds()
		}
	}

	return &stats
}

// updateLatencyStats обновляет статистику задержек
func (m *Manager) updateLatencyStats(testCtx *TestContext, latencyMs float64) {
	// Обновляем минимальную задержку
	for {
		old := testCtx.Stats.MinLatency
		if old == 0 || latencyMs < old {
			if atomic.CompareAndSwapUint64(
				(*uint64)(unsafe.Pointer(&testCtx.Stats.MinLatency)),
				*(*uint64)(unsafe.Pointer(&old)),
				*(*uint64)(unsafe.Pointer(&latencyMs))) {
				break
			}
		} else {
			break
		}
	}

	// Обновляем максимальную задержку
	for {
		old := testCtx.Stats.MaxLatency
		if latencyMs > old {
			if atomic.CompareAndSwapUint64(
				(*uint64)(unsafe.Pointer(&testCtx.Stats.MaxLatency)),
				*(*uint64)(unsafe.Pointer(&old)),
				*(*uint64)(unsafe.Pointer(&latencyMs))) {
				break
			}
		} else {
			break
		}
	}

	// Для средней задержки нужна более сложная логика
	// В реальной реализации лучше использовать mutex для этого
}

// finalizeTestStats финализирует статистику теста
func (m *Manager) finalizeTestStats(testCtx *TestContext) {
	now := time.Now()
	testCtx.Stats.EndTime = &now
	testCtx.Stats.Duration = now.Sub(testCtx.Stats.StartTime)

	if testCtx.Stats.MessagesSent > 0 {
		testCtx.Stats.AvgThroughput = float64(testCtx.Stats.MessagesSent) / testCtx.Stats.Duration.Seconds()
		// Здесь можно добавить расчет перцентилей задержек
	}

	m.logger.Info("Тест завершен",
		zap.String("type", string(testCtx.Config.Type)),
		zap.Int64("messages_sent", testCtx.Stats.MessagesSent),
		zap.Int64("bytes_sent", testCtx.Stats.BytesSent),
		zap.Int64("errors", testCtx.Stats.Errors),
		zap.Duration("duration", testCtx.Stats.Duration),
		zap.Float64("throughput", testCtx.Stats.AvgThroughput))
}
