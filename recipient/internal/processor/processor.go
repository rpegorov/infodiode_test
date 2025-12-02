package processor

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/infodiode/recipient/internal/validator"
	"github.com/infodiode/shared/models"
	"github.com/infodiode/shared/utils"
	"go.uber.org/zap"
)

// MessageProcessor обрабатывает входящие сообщения
type MessageProcessor struct {
	logger     *zap.Logger
	validator  *validator.ChecksumValidator
	messageLog *MessageLogger
	stats      *ProcessorStats
	mu         sync.RWMutex
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// ProcessorStats статистика обработчика
type ProcessorStats struct {
	MessagesReceived   atomic.Int64
	MessagesProcessed  atomic.Int64
	MessagesValid      atomic.Int64
	MessagesInvalid    atomic.Int64
	ChecksumErrors     atomic.Int64
	ProcessingErrors   atomic.Int64
	TotalBytesReceived atomic.Int64
	LastMessageTime    atomic.Value // time.Time
	FirstMessageTime   atomic.Value // time.Time
	MinLatency         atomic.Int64 // microseconds
	MaxLatency         atomic.Int64 // microseconds
	TotalLatency       atomic.Int64 // microseconds
}

// MessageLogger логирует сообщения в файл
type MessageLogger struct {
	logger *zap.Logger
	mu     sync.Mutex
}

// NewMessageProcessor создает новый обработчик сообщений
func NewMessageProcessor(logger *zap.Logger) *MessageProcessor {
	return &MessageProcessor{
		logger:     logger,
		validator:  validator.NewChecksumValidator(logger),
		messageLog: &MessageLogger{logger: logger},
		stats:      &ProcessorStats{},
		stopChan:   make(chan struct{}),
	}
}

// ProcessMessage обрабатывает одно сообщение
func (p *MessageProcessor) ProcessMessage(message *models.Message) error {
	startTime := time.Now()
	receiveTime := utils.GetCurrentTime()

	// Обновляем счетчик полученных сообщений
	p.stats.MessagesReceived.Add(1)

	// Обновляем время первого сообщения
	if p.stats.MessagesReceived.Load() == 1 {
		p.stats.FirstMessageTime.Store(startTime)
	}
	p.stats.LastMessageTime.Store(startTime)

	// Размер сообщения
	messageBytes, err := json.Marshal(message)
	if err != nil {
		p.stats.ProcessingErrors.Add(1)
		return fmt.Errorf("ошибка сериализации сообщения: %w", err)
	}
	messageSize := len(messageBytes)
	p.stats.TotalBytesReceived.Add(int64(messageSize))

	// Валидация контрольной суммы
	isValid, err := p.validator.ValidateMessage(message)
	if err != nil {
		p.stats.ProcessingErrors.Add(1)
		p.logger.Error("Ошибка валидации сообщения",
			zap.Int("message_id", message.MessageID),
			zap.Error(err))
	}

	if !isValid {
		p.stats.MessagesInvalid.Add(1)
		p.stats.ChecksumErrors.Add(1)

		// Логируем сообщение с ошибкой контрольной суммы
		p.logMessage(message, receiveTime, messageSize, false)

		p.logger.Warn("Несовпадение контрольной суммы",
			zap.Int("message_id", message.MessageID),
			zap.String("expected", message.Checksum),
			zap.String("actual", utils.CalculateChecksumString(message.Payload)))
	} else {
		p.stats.MessagesValid.Add(1)

		// Логируем валидное сообщение
		p.logMessage(message, receiveTime, messageSize, true)
	}

	// Вычисляем задержку
	if message.SendTime != "" {
		latency, err := utils.CalculateLatency(message.SendTime, receiveTime)
		if err == nil {
			latencyMicros := int64(latency * 1000)
			p.stats.TotalLatency.Add(latencyMicros)
			p.updateMinMaxLatency(latencyMicros)
		}
	}

	// Обновляем счетчик обработанных сообщений
	p.stats.MessagesProcessed.Add(1)

	// Логируем время обработки если оно слишком большое
	processingTime := time.Since(startTime)
	if processingTime > 100*time.Millisecond {
		p.logger.Warn("Долгая обработка сообщения",
			zap.Int("message_id", message.MessageID),
			zap.Duration("processing_time", processingTime))
	}

	return nil
}

// logMessage логирует сообщение в файл
func (p *MessageProcessor) logMessage(message *models.Message, receiveTime string, size int, checksumValid bool) {
	p.messageLog.mu.Lock()
	defer p.messageLog.mu.Unlock()

	// Создаем запись лога
	logEntry := models.LogEntry{
		Timestamp:     time.Now(),
		MessageID:     message.MessageID,
		SendTime:      message.SendTime,
		ReceiveTime:   receiveTime,
		Checksum:      message.Checksum,
		ChecksumValid: &checksumValid,
		MessageSize:   size,
	}

	// Если контрольная сумма не совпадает, добавляем пометку об ошибке
	if !checksumValid {
		logEntry.Error = "Checksum mismatch"
	}

	// Логируем через zap logger (который настроен на запись в файл)
	fields := []zap.Field{
		zap.Int("message_id", logEntry.MessageID),
		zap.String("send_time", logEntry.SendTime),
		zap.String("receive_time", logEntry.ReceiveTime),
		zap.String("checksum", logEntry.Checksum),
		zap.Bool("checksum_valid", checksumValid),
		zap.Int("message_size", logEntry.MessageSize),
	}

	if logEntry.Error != "" {
		fields = append(fields, zap.String("error", logEntry.Error))
	}

	p.messageLog.logger.Info("Сообщение получено", fields...)
}

// updateMinMaxLatency обновляет минимальную и максимальную задержку
func (p *MessageProcessor) updateMinMaxLatency(latencyMicros int64) {
	// Обновляем минимальную задержку
	for {
		oldMin := p.stats.MinLatency.Load()
		if oldMin == 0 || latencyMicros < oldMin {
			if p.stats.MinLatency.CompareAndSwap(oldMin, latencyMicros) {
				break
			}
		} else {
			break
		}
	}

	// Обновляем максимальную задержку
	for {
		oldMax := p.stats.MaxLatency.Load()
		if latencyMicros > oldMax {
			if p.stats.MaxLatency.CompareAndSwap(oldMax, latencyMicros) {
				break
			}
		} else {
			break
		}
	}
}

// GetStats возвращает статистику обработчика
func (p *MessageProcessor) GetStats() ProcessorStatsSnapshot {
	received := p.stats.MessagesReceived.Load()
	processed := p.stats.MessagesProcessed.Load()
	valid := p.stats.MessagesValid.Load()
	invalid := p.stats.MessagesInvalid.Load()
	checksumErrors := p.stats.ChecksumErrors.Load()
	processingErrors := p.stats.ProcessingErrors.Load()
	totalBytes := p.stats.TotalBytesReceived.Load()
	totalLatency := p.stats.TotalLatency.Load()

	// Вычисляем средние значения
	var avgLatency float64
	var avgMessageSize int64
	var throughput float64

	if processed > 0 {
		avgLatency = float64(totalLatency) / float64(processed) / 1000.0 // в миллисекундах
		avgMessageSize = totalBytes / processed
	}

	// Вычисляем пропускную способность
	firstTime, _ := p.stats.FirstMessageTime.Load().(time.Time)
	lastTime, _ := p.stats.LastMessageTime.Load().(time.Time)
	if !firstTime.IsZero() && !lastTime.IsZero() {
		duration := lastTime.Sub(firstTime).Seconds()
		if duration > 0 {
			throughput = float64(processed) / duration
		}
	}

	return ProcessorStatsSnapshot{
		MessagesReceived:   received,
		MessagesProcessed:  processed,
		MessagesValid:      valid,
		MessagesInvalid:    invalid,
		ChecksumErrors:     checksumErrors,
		ProcessingErrors:   processingErrors,
		TotalBytesReceived: totalBytes,
		AvgMessageSize:     avgMessageSize,
		MinLatency:         float64(p.stats.MinLatency.Load()) / 1000.0, // ms
		MaxLatency:         float64(p.stats.MaxLatency.Load()) / 1000.0, // ms
		AvgLatency:         avgLatency,
		Throughput:         throughput,
		FirstMessageTime:   firstTime,
		LastMessageTime:    lastTime,
	}
}

// ProcessorStatsSnapshot снимок статистики
type ProcessorStatsSnapshot struct {
	MessagesReceived   int64
	MessagesProcessed  int64
	MessagesValid      int64
	MessagesInvalid    int64
	ChecksumErrors     int64
	ProcessingErrors   int64
	TotalBytesReceived int64
	AvgMessageSize     int64
	MinLatency         float64 // ms
	MaxLatency         float64 // ms
	AvgLatency         float64 // ms
	Throughput         float64 // msg/sec
	FirstMessageTime   time.Time
	LastMessageTime    time.Time
}

// ResetStats сбрасывает статистику
func (p *MessageProcessor) ResetStats() {
	p.stats = &ProcessorStats{}
	p.logger.Info("Статистика обработчика сброшена")
}

// Start запускает обработчик (для будущих расширений)
func (p *MessageProcessor) Start() error {
	p.logger.Info("Обработчик сообщений запущен")
	return nil
}

// Stop останавливает обработчик
func (p *MessageProcessor) Stop() error {
	close(p.stopChan)
	p.wg.Wait()

	// Выводим финальную статистику
	stats := p.GetStats()
	p.logger.Info("Обработчик сообщений остановлен",
		zap.Int64("всего_получено", stats.MessagesReceived),
		zap.Int64("обработано", stats.MessagesProcessed),
		zap.Int64("валидных", stats.MessagesValid),
		zap.Int64("невалидных", stats.MessagesInvalid),
		zap.Int64("ошибок_контрольной_суммы", stats.ChecksumErrors),
		zap.Float64("средняя_задержка_ms", stats.AvgLatency),
		zap.Float64("пропускная_способность_msg/sec", stats.Throughput))

	return nil
}

// ProcessBatch обрабатывает пакет сообщений
func (p *MessageProcessor) ProcessBatch(messages []*models.Message) error {
	var errs []error

	for _, msg := range messages {
		if err := p.ProcessMessage(msg); err != nil {
			errs = append(errs, fmt.Errorf("сообщение %d: %w", msg.MessageID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("ошибки при обработке пакета: %v", errs)
	}

	return nil
}

// ProcessAsync обрабатывает сообщение асинхронно
func (p *MessageProcessor) ProcessAsync(message *models.Message) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		if err := p.ProcessMessage(message); err != nil {
			p.logger.Error("Ошибка асинхронной обработки сообщения",
				zap.Int("message_id", message.MessageID),
				zap.Error(err))
		}
	}()
}
