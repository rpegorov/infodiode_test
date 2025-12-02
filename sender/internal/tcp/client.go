package tcp

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/infodiode/shared/models"
	"go.uber.org/zap"
)

// TCPClient клиент для отправки данных по TCP
type TCPClient struct {
	address      string
	conn         net.Conn
	logger       *zap.Logger
	mu           sync.Mutex
	isConnected  bool
	reconnectInt time.Duration
	maxRetries   int
	timeout      time.Duration
	stopChan     chan struct{}
}

// Config конфигурация TCP клиента
type Config struct {
	Address         string        `yaml:"address" json:"address"`
	ReconnectInt    time.Duration `yaml:"reconnect_interval" json:"reconnect_interval"`
	MaxRetries      int           `yaml:"max_retries" json:"max_retries"`
	Timeout         time.Duration `yaml:"timeout" json:"timeout"`
	KeepAlive       bool          `yaml:"keep_alive" json:"keep_alive"`
	KeepAlivePeriod time.Duration `yaml:"keep_alive_period" json:"keep_alive_period"`
}

// NewTCPClient создает новый TCP клиент
func NewTCPClient(config *Config, logger *zap.Logger) (*TCPClient, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("TCP адрес не указан")
	}

	client := &TCPClient{
		address:      config.Address,
		logger:       logger,
		reconnectInt: config.ReconnectInt,
		maxRetries:   config.MaxRetries,
		timeout:      config.Timeout,
		stopChan:     make(chan struct{}),
	}

	// Устанавливаем значения по умолчанию
	if client.reconnectInt == 0 {
		client.reconnectInt = 5 * time.Second
	}
	if client.maxRetries == 0 {
		client.maxRetries = 3
	}
	if client.timeout == 0 {
		client.timeout = 10 * time.Second
	}

	return client, nil
}

// Connect устанавливает соединение с TCP сервером
func (c *TCPClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected {
		return nil
	}

	c.logger.Info("Подключение к TCP серверу", zap.String("address", c.address))

	conn, err := net.Dial("tcp", c.address)
	if err != nil {
		return fmt.Errorf("ошибка подключения к TCP серверу: %w", err)
	}

	// Устанавливаем keep-alive для поддержания соединения
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	c.conn = conn
	c.isConnected = true

	c.logger.Info("Успешное подключение к TCP серверу", zap.String("address", c.address))

	// Запускаем горутину для проверки соединения
	go c.monitorConnection()

	return nil
}

// Disconnect закрывает соединение с TCP сервером
func (c *TCPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected || c.conn == nil {
		return nil
	}

	close(c.stopChan)

	err := c.conn.Close()
	c.isConnected = false
	c.conn = nil

	c.logger.Info("Отключение от TCP сервера", zap.String("address", c.address))

	return err
}

// Send отправляет сообщение через TCP
func (c *TCPClient) Send(message *models.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected || c.conn == nil {
		// Пытаемся переподключиться
		c.mu.Unlock()
		if err := c.reconnect(); err != nil {
			return fmt.Errorf("не удалось переподключиться: %w", err)
		}
		c.mu.Lock()
	}

	// Сериализуем сообщение в JSON
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("ошибка сериализации сообщения: %w", err)
	}

	// Добавляем длину сообщения в начало (4 байта)
	// Это позволит получателю корректно читать сообщения
	length := uint32(len(data))
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, length)

	// Устанавливаем таймаут на запись
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))

	// Отправляем длину сообщения
	if _, err := c.conn.Write(lengthBytes); err != nil {
		c.isConnected = false
		return fmt.Errorf("ошибка отправки длины сообщения: %w", err)
	}

	// Отправляем само сообщение
	if _, err := c.conn.Write(data); err != nil {
		c.isConnected = false
		return fmt.Errorf("ошибка отправки сообщения: %w", err)
	}

	return nil
}

// SendBatch отправляет пакет сообщений через TCP
func (c *TCPClient) SendBatch(messages []*models.Message) error {
	// Для оптимизации можно отправлять все сообщения в одном пакете
	batch := &models.MessageBatch{
		Messages:  messages,
		Timestamp: time.Now().Format(time.RFC3339),
		Count:     len(messages),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected || c.conn == nil {
		c.mu.Unlock()
		if err := c.reconnect(); err != nil {
			return fmt.Errorf("не удалось переподключиться: %w", err)
		}
		c.mu.Lock()
	}

	// Сериализуем пакет в JSON
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("ошибка сериализации пакета: %w", err)
	}

	// Добавляем длину и маркер пакета
	length := uint32(len(data))
	header := make([]byte, 5)
	header[0] = 0x01 // Маркер пакетной отправки
	binary.BigEndian.PutUint32(header[1:], length)

	// Устанавливаем таймаут на запись
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout * 2)) // Увеличенный таймаут для пакета

	// Отправляем заголовок
	if _, err := c.conn.Write(header); err != nil {
		c.isConnected = false
		return fmt.Errorf("ошибка отправки заголовка пакета: %w", err)
	}

	// Отправляем данные
	if _, err := c.conn.Write(data); err != nil {
		c.isConnected = false
		return fmt.Errorf("ошибка отправки пакета: %w", err)
	}

	return nil
}

// reconnect пытается переподключиться к серверу
func (c *TCPClient) reconnect() error {
	retries := 0
	for retries < c.maxRetries {
		c.logger.Info("Попытка переподключения",
			zap.Int("attempt", retries+1),
			zap.Int("max_retries", c.maxRetries))

		if err := c.Connect(); err != nil {
			retries++
			if retries >= c.maxRetries {
				return fmt.Errorf("превышено количество попыток переподключения: %w", err)
			}
			time.Sleep(c.reconnectInt)
			continue
		}
		return nil
	}
	return fmt.Errorf("не удалось переподключиться после %d попыток", c.maxRetries)
}

// monitorConnection мониторит состояние соединения
func (c *TCPClient) monitorConnection() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.isConnected && c.conn != nil {
				// Проверяем соединение отправкой пустого пакета
				c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if _, err := c.conn.Write([]byte{0x00}); err != nil {
					c.logger.Warn("Потеря соединения с TCP сервером", zap.Error(err))
					c.isConnected = false
				}
			}
			c.mu.Unlock()
		}
	}
}

// IsConnected проверяет состояние соединения
func (c *TCPClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isConnected
}

// GetStats возвращает статистику TCP клиента
func (c *TCPClient) GetStats() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	return map[string]interface{}{
		"connected": c.isConnected,
		"address":   c.address,
		"retries":   c.maxRetries,
	}
}
