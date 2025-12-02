package broker

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/infodiode/recipient/config"
	"github.com/infodiode/shared/models"
	"go.uber.org/zap"
)

// MQTTConsumer структура для приема сообщений из MQTT
type MQTTConsumer struct {
	client          mqtt.Client
	config          *config.MQTTConfig
	logger          *zap.Logger
	connected       atomic.Bool
	messageCounter  atomic.Int64
	errorCounter    atomic.Int64
	bytesCounter    atomic.Int64
	reconnectCount  atomic.Int32
	lastConnectTime time.Time
	messageHandler  MessageHandler
	mu              sync.RWMutex
	stopChan        chan struct{}
	wg              sync.WaitGroup
}

// MessageHandler обработчик входящих сообщений
type MessageHandler func(*models.Message) error

// NewMQTTConsumer создает новый экземпляр MQTT consumer
func NewMQTTConsumer(cfg *config.MQTTConfig, logger *zap.Logger, handler MessageHandler) (*MQTTConsumer, error) {
	if handler == nil {
		return nil, fmt.Errorf("обработчик сообщений не может быть nil")
	}

	c := &MQTTConsumer{
		config:         cfg,
		logger:         logger,
		messageHandler: handler,
		stopChan:       make(chan struct{}),
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

	// Настройка хранилища для сохранения состояния
	if cfg.StoreDirectory != "" {
		store := mqtt.NewFileStore(cfg.StoreDirectory)
		opts.SetStore(store)
	}

	// Обработчики событий подключения
	opts.SetOnConnectHandler(c.onConnect)
	opts.SetConnectionLostHandler(c.onConnectionLost)
	opts.SetReconnectingHandler(c.onReconnecting)

	// Настройка обработчика сообщений по умолчанию
	opts.SetDefaultPublishHandler(c.onMessageReceived)

	// Создание клиента
	c.client = mqtt.NewClient(opts)

	// Подключение к брокеру
	if err := c.connect(); err != nil {
		return nil, fmt.Errorf("не удалось подключиться к MQTT брокеру: %w", err)
	}

	return c, nil
}

// connect выполняет подключение к брокеру
func (c *MQTTConsumer) connect() error {
	c.logger.Info("Подключение к MQTT брокеру",
		zap.String("broker", c.config.Broker),
		zap.String("client_id", c.config.ClientID),
		zap.String("topic", c.config.Topic))

	token := c.client.Connect()
	if !token.WaitTimeout(c.config.ConnectTimeout) {
		return fmt.Errorf("таймаут подключения к брокеру")
	}

	if err := token.Error(); err != nil {
		return fmt.Errorf("ошибка подключения: %w", err)
	}

	return nil
}

// onConnect вызывается при успешном подключении
func (c *MQTTConsumer) onConnect(client mqtt.Client) {
	c.mu.Lock()
	c.lastConnectTime = time.Now()
	c.mu.Unlock()

	c.connected.Store(true)
	reconnects := c.reconnectCount.Load()

	if reconnects > 0 {
		c.logger.Info("Переподключение к MQTT брокеру выполнено успешно",
			zap.Int32("попытка", reconnects),
			zap.String("broker", c.config.Broker))
	} else {
		c.logger.Info("Подключение к MQTT брокеру установлено",
			zap.String("broker", c.config.Broker),
			zap.String("client_id", c.config.ClientID))
	}

	// Подписка на топик
	if err := c.subscribe(); err != nil {
		c.logger.Error("Ошибка подписки на топик", zap.Error(err))
	}
}

// subscribe подписывается на топик
func (c *MQTTConsumer) subscribe() error {
	token := c.client.Subscribe(c.config.Topic, c.config.QoS, nil)

	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("таймаут подписки на топик %s", c.config.Topic)
	}

	if err := token.Error(); err != nil {
		return fmt.Errorf("ошибка подписки на топик %s: %w", c.config.Topic, err)
	}

	c.logger.Info("Подписка на топик выполнена",
		zap.String("topic", c.config.Topic),
		zap.Uint8("qos", c.config.QoS))

	return nil
}

// onConnectionLost вызывается при потере соединения
func (c *MQTTConsumer) onConnectionLost(client mqtt.Client, err error) {
	c.connected.Store(false)
	c.errorCounter.Add(1)

	c.logger.Error("Потеря соединения с MQTT брокером",
		zap.Error(err),
		zap.String("broker", c.config.Broker))
}

// onReconnecting вызывается при попытке переподключения
func (c *MQTTConsumer) onReconnecting(client mqtt.Client, opts *mqtt.ClientOptions) {
	attempts := c.reconnectCount.Add(1)
	c.logger.Warn("Попытка переподключения к MQTT брокеру",
		zap.Int32("попытка", attempts),
		zap.String("broker", c.config.Broker))
}

// onMessageReceived обработчик входящих сообщений
func (c *MQTTConsumer) onMessageReceived(client mqtt.Client, msg mqtt.Message) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.processMessage(msg)
	}()
}

// processMessage обрабатывает полученное сообщение
func (c *MQTTConsumer) processMessage(msg mqtt.Message) {
	startTime := time.Now()
	payload := msg.Payload()

	// Обновление счетчиков
	c.messageCounter.Add(1)
	c.bytesCounter.Add(int64(len(payload)))

	// Десериализация сообщения
	var message models.Message
	if err := json.Unmarshal(payload, &message); err != nil {
		c.errorCounter.Add(1)
		c.logger.Error("Ошибка десериализации сообщения",
			zap.Error(err),
			zap.String("topic", msg.Topic()),
			zap.Int("size", len(payload)))
		return
	}

	// Логирование полученного сообщения
	c.logger.Debug("Сообщение получено",
		zap.Int("message_id", message.MessageID),
		zap.String("topic", msg.Topic()),
		zap.Int("size", len(payload)),
		zap.Uint8("qos", msg.Qos()),
		zap.Bool("retained", msg.Retained()),
		zap.Bool("duplicate", msg.Duplicate()))

	// Вызов обработчика сообщения
	if err := c.messageHandler(&message); err != nil {
		c.errorCounter.Add(1)
		c.logger.Error("Ошибка обработки сообщения",
			zap.Error(err),
			zap.Int("message_id", message.MessageID))
		return
	}

	// Логирование времени обработки
	processingTime := time.Since(startTime)
	if processingTime > time.Second {
		c.logger.Warn("Долгая обработка сообщения",
			zap.Int("message_id", message.MessageID),
			zap.Duration("время_обработки", processingTime))
	}
}

// Start начинает прием сообщений (подписка уже выполнена в onConnect)
func (c *MQTTConsumer) Start() error {
	if !c.IsConnected() {
		return fmt.Errorf("нет соединения с MQTT брокером")
	}

	c.logger.Info("Consumer запущен и готов к приему сообщений",
		zap.String("topic", c.config.Topic))

	return nil
}

// Stop останавливает прием сообщений
func (c *MQTTConsumer) Stop() error {
	c.logger.Info("Остановка consumer")

	// Отписка от топика
	if c.client.IsConnected() {
		token := c.client.Unsubscribe(c.config.Topic)
		if token.WaitTimeout(5 * time.Second) {
			if err := token.Error(); err != nil {
				c.logger.Warn("Ошибка при отписке от топика",
					zap.Error(err),
					zap.String("topic", c.config.Topic))
			} else {
				c.logger.Info("Отписка от топика выполнена",
					zap.String("topic", c.config.Topic))
			}
		}
	}

	return nil
}

// IsConnected проверяет состояние подключения
func (c *MQTTConsumer) IsConnected() bool {
	return c.client.IsConnected() && c.connected.Load()
}

// GetStats возвращает статистику consumer
func (c *MQTTConsumer) GetStats() ConsumerStats {
	c.mu.RLock()
	lastConnect := c.lastConnectTime
	c.mu.RUnlock()

	messagesReceived := c.messageCounter.Load()
	bytesReceived := c.bytesCounter.Load()

	var avgMessageSize int64
	if messagesReceived > 0 {
		avgMessageSize = bytesReceived / messagesReceived
	}

	return ConsumerStats{
		MessagesReceived: messagesReceived,
		BytesReceived:    bytesReceived,
		Errors:           c.errorCounter.Load(),
		ReconnectCount:   c.reconnectCount.Load(),
		Connected:        c.IsConnected(),
		LastConnectTime:  lastConnect,
		Uptime:           time.Since(lastConnect),
		AvgMessageSize:   avgMessageSize,
	}
}

// ResetStats сбрасывает счетчики статистики
func (c *MQTTConsumer) ResetStats() {
	c.messageCounter.Store(0)
	c.bytesCounter.Store(0)
	c.errorCounter.Store(0)
	// reconnectCount не сбрасываем, так как это общий счетчик
}

// SetMessageHandler устанавливает новый обработчик сообщений
func (c *MQTTConsumer) SetMessageHandler(handler MessageHandler) error {
	if handler == nil {
		return fmt.Errorf("обработчик не может быть nil")
	}

	c.mu.Lock()
	c.messageHandler = handler
	c.mu.Unlock()

	return nil
}

// Flush ожидает завершения обработки всех сообщений
func (c *MQTTConsumer) Flush(timeout time.Duration) error {
	done := make(chan struct{})

	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("таймаут ожидания завершения обработки сообщений")
	}
}

// Close закрывает соединение с брокером
func (c *MQTTConsumer) Close() error {
	c.logger.Info("Закрытие соединения с MQTT брокером")

	// Сигнал остановки
	close(c.stopChan)

	// Остановка приема сообщений
	if err := c.Stop(); err != nil {
		c.logger.Warn("Ошибка при остановке consumer", zap.Error(err))
	}

	// Ожидание завершения обработки сообщений
	if err := c.Flush(10 * time.Second); err != nil {
		c.logger.Warn("Таймаут при ожидании завершения обработки", zap.Error(err))
	}

	// Отключение от брокера
	if c.client.IsConnected() {
		c.client.Disconnect(5000) // 5 секунд на graceful disconnect
	}

	c.connected.Store(false)

	// Логирование финальной статистики
	stats := c.GetStats()
	c.logger.Info("MQTT consumer закрыт",
		zap.Int64("сообщений_получено", stats.MessagesReceived),
		zap.Int64("байт_получено", stats.BytesReceived),
		zap.Int64("ошибок", stats.Errors),
		zap.Int64("средний_размер_сообщения", stats.AvgMessageSize),
		zap.Duration("время_работы", stats.Uptime))

	return nil
}

// ConsumerStats статистика consumer
type ConsumerStats struct {
	MessagesReceived int64
	BytesReceived    int64
	Errors           int64
	ReconnectCount   int32
	Connected        bool
	LastConnectTime  time.Time
	Uptime           time.Duration
	AvgMessageSize   int64
}
