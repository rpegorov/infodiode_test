package broker

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/infodiode/sender/config"
	"github.com/infodiode/shared/models"
	"go.uber.org/zap"
)

// MQTTProducer структура для отправки сообщений в MQTT
type MQTTProducer struct {
	client          mqtt.Client
	config          *config.MQTTConfig
	logger          *zap.Logger
	connected       atomic.Bool
	messageCounter  atomic.Int64
	errorCounter    atomic.Int64
	bytesCounter    atomic.Int64
	reconnectCount  atomic.Int32
	lastConnectTime time.Time
	mu              sync.RWMutex
	stopChan        chan struct{}
	wg              sync.WaitGroup
}

// NewMQTTProducer создает новый экземпляр MQTT producer
func NewMQTTProducer(cfg *config.MQTTConfig, logger *zap.Logger) (*MQTTProducer, error) {
	p := &MQTTProducer{
		config:   cfg,
		logger:   logger,
		stopChan: make(chan struct{}),
	}

	// Настройка опций клиента MQTT
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}

	opts.SetCleanSession(cfg.CleanSession)
	opts.SetKeepAlive(cfg.KeepAlive)
	opts.SetConnectTimeout(cfg.ConnectTimeout)
	opts.SetAutoReconnect(cfg.AutoReconnect)
	opts.SetMaxReconnectInterval(cfg.MaxReconnectInt)
	opts.SetOrderMatters(cfg.OrderMatters)

	// Настройка хранилища для буферизации сообщений
	if cfg.StoreDirectory != "" {
		store := mqtt.NewFileStore(cfg.StoreDirectory)
		opts.SetStore(store)
	}

	// Обработчики событий подключения
	opts.SetOnConnectHandler(p.onConnect)
	opts.SetConnectionLostHandler(p.onConnectionLost)
	opts.SetReconnectingHandler(p.onReconnecting)

	// Создание клиента
	p.client = mqtt.NewClient(opts)

	// Подключение к брокеру
	if err := p.connect(); err != nil {
		return nil, fmt.Errorf("не удалось подключиться к MQTT брокеру: %w", err)
	}

	return p, nil
}

// connect выполняет подключение к брокеру
func (p *MQTTProducer) connect() error {
	p.logger.Info("Подключение к MQTT брокеру",
		zap.String("broker", p.config.Broker),
		zap.String("client_id", p.config.ClientID),
		zap.String("topic", p.config.Topic))

	token := p.client.Connect()
	if !token.WaitTimeout(p.config.ConnectTimeout) {
		return fmt.Errorf("таймаут подключения к брокеру")
	}

	if err := token.Error(); err != nil {
		return fmt.Errorf("ошибка подключения: %w", err)
	}

	return nil
}

// onConnect вызывается при успешном подключении
func (p *MQTTProducer) onConnect(client mqtt.Client) {
	p.mu.Lock()
	p.lastConnectTime = time.Now()
	p.mu.Unlock()

	p.connected.Store(true)
	reconnects := p.reconnectCount.Load()

	if reconnects > 0 {
		p.logger.Info("Переподключение к MQTT брокеру выполнено успешно",
			zap.Int32("попытка", reconnects),
			zap.String("broker", p.config.Broker))
	} else {
		p.logger.Info("Подключение к MQTT брокеру установлено",
			zap.String("broker", p.config.Broker),
			zap.String("client_id", p.config.ClientID))
	}
}

// onConnectionLost вызывается при потере соединения
func (p *MQTTProducer) onConnectionLost(client mqtt.Client, err error) {
	p.connected.Store(false)
	p.errorCounter.Add(1)

	p.logger.Error("Потеря соединения с MQTT брокером",
		zap.Error(err),
		zap.String("broker", p.config.Broker))
}

// onReconnecting вызывается при попытке переподключения
func (p *MQTTProducer) onReconnecting(client mqtt.Client, opts *mqtt.ClientOptions) {
	attempts := p.reconnectCount.Add(1)
	p.logger.Warn("Попытка переподключения к MQTT брокеру",
		zap.Int32("попытка", attempts),
		zap.String("broker", p.config.Broker))
}

// Publish отправляет сообщение в MQTT
func (p *MQTTProducer) Publish(message *models.Message) error {
	if !p.IsConnected() {
		return fmt.Errorf("нет соединения с MQTT брокером")
	}

	// Сериализация сообщения в JSON
	data, err := json.Marshal(message)
	if err != nil {
		p.errorCounter.Add(1)
		return fmt.Errorf("ошибка сериализации сообщения: %w", err)
	}

	// Публикация сообщения
	token := p.client.Publish(
		p.config.Topic,
		p.config.QoS,
		p.config.Retained,
		data,
	)

	// Ожидание подтверждения отправки (для QoS > 0)
	if p.config.QoS > 0 {
		if !token.WaitTimeout(5 * time.Second) {
			p.errorCounter.Add(1)
			return fmt.Errorf("таймаут при отправке сообщения")
		}

		if err := token.Error(); err != nil {
			p.errorCounter.Add(1)
			return fmt.Errorf("ошибка при отправке сообщения: %w", err)
		}
	}

	// Обновление счетчиков
	p.messageCounter.Add(1)
	p.bytesCounter.Add(int64(len(data)))

	p.logger.Debug("Сообщение отправлено",
		zap.Int("message_id", message.MessageID),
		zap.String("topic", p.config.Topic),
		zap.Int("size", len(data)))

	return nil
}

// PublishBatch отправляет пакет сообщений
func (p *MQTTProducer) PublishBatch(messages []*models.Message) error {
	if !p.IsConnected() {
		return fmt.Errorf("нет соединения с MQTT брокером")
	}

	var errs []error
	successCount := 0

	for _, msg := range messages {
		if err := p.Publish(msg); err != nil {
			errs = append(errs, fmt.Errorf("сообщение %d: %w", msg.MessageID, err))
		} else {
			successCount++
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("отправлено %d из %d сообщений, ошибки: %v",
			successCount, len(messages), errs)
	}

	return nil
}

// PublishAsync отправляет сообщение асинхронно
func (p *MQTTProducer) PublishAsync(message *models.Message, callback func(error)) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		err := p.Publish(message)
		if callback != nil {
			callback(err)
		}
	}()
}

// PublishWithRetry отправляет сообщение с повторными попытками
func (p *MQTTProducer) PublishWithRetry(message *models.Message, maxRetries int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Экспоненциальная задержка между попытками
			delay := time.Duration(attempt) * time.Second
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}

			p.logger.Debug("Повторная попытка отправки сообщения",
				zap.Int("message_id", message.MessageID),
				zap.Int("попытка", attempt),
				zap.Duration("задержка", delay))

			time.Sleep(delay)
		}

		if err := p.Publish(message); err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("не удалось отправить сообщение после %d попыток: %w", maxRetries, lastErr)
}

// IsConnected проверяет состояние подключения
func (p *MQTTProducer) IsConnected() bool {
	return p.client.IsConnected() && p.connected.Load()
}

// GetStats возвращает статистику producer
func (p *MQTTProducer) GetStats() ProducerStats {
	p.mu.RLock()
	lastConnect := p.lastConnectTime
	p.mu.RUnlock()

	return ProducerStats{
		MessagesPublished: p.messageCounter.Load(),
		BytesSent:         p.bytesCounter.Load(),
		Errors:            p.errorCounter.Load(),
		ReconnectCount:    p.reconnectCount.Load(),
		Connected:         p.IsConnected(),
		LastConnectTime:   lastConnect,
		Uptime:            time.Since(lastConnect),
	}
}

// ResetStats сбрасывает счетчики статистики
func (p *MQTTProducer) ResetStats() {
	p.messageCounter.Store(0)
	p.bytesCounter.Store(0)
	p.errorCounter.Store(0)
	// reconnectCount не сбрасываем, так как это общий счетчик
}

// Flush ожидает завершения всех асинхронных операций
func (p *MQTTProducer) Flush(timeout time.Duration) error {
	done := make(chan struct{})

	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("таймаут ожидания завершения операций")
	}
}

// Close закрывает соединение с брокером
func (p *MQTTProducer) Close() error {
	p.logger.Info("Закрытие соединения с MQTT брокером")

	// Сигнал остановки
	close(p.stopChan)

	// Ожидание завершения асинхронных операций
	if err := p.Flush(10 * time.Second); err != nil {
		p.logger.Warn("Таймаут при ожидании завершения операций", zap.Error(err))
	}

	// Отключение от брокера
	if p.client.IsConnected() {
		p.client.Disconnect(5000) // 5 секунд на graceful disconnect
	}

	p.connected.Store(false)

	// Логирование финальной статистики
	stats := p.GetStats()
	p.logger.Info("MQTT producer закрыт",
		zap.Int64("сообщений_отправлено", stats.MessagesPublished),
		zap.Int64("байт_отправлено", stats.BytesSent),
		zap.Int64("ошибок", stats.Errors),
		zap.Duration("время_работы", stats.Uptime))

	return nil
}

// ProducerStats статистика producer
type ProducerStats struct {
	MessagesPublished int64
	BytesSent         int64
	Errors            int64
	ReconnectCount    int32
	Connected         bool
	LastConnectTime   time.Time
	Uptime            time.Duration
}
