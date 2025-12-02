package tcp

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/infodiode/recipient/internal/processor"
	"github.com/infodiode/shared/models"
	"go.uber.org/zap"
)

// TCPServer сервер для приема данных по TCP
type TCPServer struct {
	address   string
	listener  net.Listener
	logger    *zap.Logger
	processor *processor.MessageProcessor
	wg        sync.WaitGroup
	stopChan  chan struct{}
	isRunning bool
	mu        sync.RWMutex
	stats     *ServerStats
}

// ServerStats статистика работы сервера
type ServerStats struct {
	ConnectionsTotal  int64
	ConnectionsActive int64
	MessagesReceived  int64
	BatchesReceived   int64
	BytesReceived     int64
	Errors            int64
	LastMessageTime   time.Time
	mu                sync.RWMutex
}

// Config конфигурация TCP сервера
type Config struct {
	Address         string        `yaml:"address" json:"address"`
	MaxConnections  int           `yaml:"max_connections" json:"max_connections"`
	ReadTimeout     time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout" json:"write_timeout"`
	KeepAlive       bool          `yaml:"keep_alive" json:"keep_alive"`
	KeepAlivePeriod time.Duration `yaml:"keep_alive_period" json:"keep_alive_period"`
}

// NewTCPServer создает новый TCP сервер
func NewTCPServer(config *Config, logger *zap.Logger, processor *processor.MessageProcessor) (*TCPServer, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("TCP адрес не указан")
	}

	server := &TCPServer{
		address:   config.Address,
		logger:    logger,
		processor: processor,
		stopChan:  make(chan struct{}),
		stats:     &ServerStats{},
	}

	return server, nil
}

// Start запускает TCP сервер
func (s *TCPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("сервер уже запущен")
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("ошибка запуска TCP сервера: %w", err)
	}

	s.listener = listener
	s.isRunning = true

	s.logger.Info("TCP сервер запущен", zap.String("address", s.address))

	// Запускаем обработку подключений
	s.wg.Add(1)
	go s.acceptConnections()

	return nil
}

// Stop останавливает TCP сервер
func (s *TCPServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return nil
	}

	s.logger.Info("Остановка TCP сервера")

	close(s.stopChan)
	s.isRunning = false

	if s.listener != nil {
		s.listener.Close()
	}

	// Ждем завершения всех горутин
	s.wg.Wait()

	s.logger.Info("TCP сервер остановлен")
	return nil
}

// acceptConnections принимает входящие подключения
func (s *TCPServer) acceptConnections() {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopChan:
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				return
			default:
				s.logger.Error("Ошибка принятия подключения", zap.Error(err))
				s.incrementErrorCount()
				continue
			}
		}

		s.incrementConnectionCount()
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection обрабатывает подключение клиента
func (s *TCPServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()
	defer s.decrementConnectionCount()

	clientAddr := conn.RemoteAddr().String()
	s.logger.Info("Новое подключение", zap.String("client", clientAddr))

	// Устанавливаем keep-alive
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	reader := bufio.NewReader(conn)

	for {
		select {
		case <-s.stopChan:
			return
		default:
		}

		// Устанавливаем таймаут на чтение
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Читаем первый байт для определения типа сообщения
		firstByte, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				s.logger.Info("Клиент закрыл соединение", zap.String("client", clientAddr))
				return
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Таймаут - продолжаем ждать
				continue
			}
			if firstByte != 0x00 { // Игнорируем keep-alive пакеты
				s.logger.Error("Ошибка чтения данных", zap.String("client", clientAddr), zap.Error(err))
				s.incrementErrorCount()
			}
			return
		}

		// Обрабатываем в зависимости от типа
		if firstByte == 0x01 {
			// Пакетная отправка
			if err := s.handleBatch(reader, clientAddr); err != nil {
				s.logger.Error("Ошибка обработки пакета", zap.String("client", clientAddr), zap.Error(err))
				s.incrementErrorCount()
			}
		} else if firstByte == 0x00 {
			// Keep-alive пакет - игнорируем
			continue
		} else {
			// Обычное сообщение - возвращаем байт обратно
			reader.UnreadByte()
			if err := s.handleMessage(reader, clientAddr); err != nil {
				s.logger.Error("Ошибка обработки сообщения", zap.String("client", clientAddr), zap.Error(err))
				s.incrementErrorCount()
			}
		}
	}
}

// handleMessage обрабатывает одиночное сообщение
func (s *TCPServer) handleMessage(reader *bufio.Reader, clientAddr string) error {
	// Читаем длину сообщения (4 байта)
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, lengthBytes); err != nil {
		return fmt.Errorf("ошибка чтения длины сообщения: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthBytes)
	if length > 100*1024*1024 { // Максимум 100MB
		return fmt.Errorf("слишком большое сообщение: %d байт", length)
	}

	// Читаем само сообщение
	messageBytes := make([]byte, length)
	if _, err := io.ReadFull(reader, messageBytes); err != nil {
		return fmt.Errorf("ошибка чтения сообщения: %w", err)
	}

	// Десериализуем сообщение
	var message models.Message
	if err := json.Unmarshal(messageBytes, &message); err != nil {
		return fmt.Errorf("ошибка десериализации сообщения: %w", err)
	}

	// Обрабатываем сообщение
	if err := s.processor.ProcessMessage(&message); err != nil {
		return fmt.Errorf("ошибка обработки сообщения: %w", err)
	}

	// Обновляем статистику
	s.incrementMessageCount(int64(length))

	s.logger.Debug("Сообщение получено",
		zap.String("client", clientAddr),
		zap.Int("message_id", message.MessageID),
		zap.Int("size", int(length)))

	return nil
}

// handleBatch обрабатывает пакет сообщений
func (s *TCPServer) handleBatch(reader *bufio.Reader, clientAddr string) error {
	// Читаем длину пакета (4 байта)
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, lengthBytes); err != nil {
		return fmt.Errorf("ошибка чтения длины пакета: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthBytes)
	if length > 100*1024*1024 { // Максимум 100MB
		return fmt.Errorf("слишком большой пакет: %d байт", length)
	}

	// Читаем данные пакета
	batchBytes := make([]byte, length)
	if _, err := io.ReadFull(reader, batchBytes); err != nil {
		return fmt.Errorf("ошибка чтения пакета: %w", err)
	}

	// Десериализуем пакет
	var batch models.MessageBatch
	if err := json.Unmarshal(batchBytes, &batch); err != nil {
		return fmt.Errorf("ошибка десериализации пакета: %w", err)
	}

	// Обрабатываем каждое сообщение в пакете
	for _, message := range batch.Messages {
		if err := s.processor.ProcessMessage(message); err != nil {
			s.logger.Error("Ошибка обработки сообщения из пакета",
				zap.Int("message_id", message.MessageID),
				zap.Error(err))
			s.incrementErrorCount()
		}
	}

	// Обновляем статистику
	s.incrementBatchCount(int64(length), len(batch.Messages))

	s.logger.Info("Пакет сообщений получен",
		zap.String("client", clientAddr),
		zap.Int("count", batch.Count),
		zap.Int("size", int(length)))

	return nil
}

// incrementConnectionCount увеличивает счетчик подключений
func (s *TCPServer) incrementConnectionCount() {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.stats.ConnectionsTotal++
	s.stats.ConnectionsActive++
}

// decrementConnectionCount уменьшает счетчик активных подключений
func (s *TCPServer) decrementConnectionCount() {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.stats.ConnectionsActive--
}

// incrementMessageCount увеличивает счетчик сообщений
func (s *TCPServer) incrementMessageCount(bytes int64) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.stats.MessagesReceived++
	s.stats.BytesReceived += bytes
	s.stats.LastMessageTime = time.Now()
}

// incrementBatchCount увеличивает счетчик пакетов
func (s *TCPServer) incrementBatchCount(bytes int64, messages int) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.stats.BatchesReceived++
	s.stats.MessagesReceived += int64(messages)
	s.stats.BytesReceived += bytes
	s.stats.LastMessageTime = time.Now()
}

// incrementErrorCount увеличивает счетчик ошибок
func (s *TCPServer) incrementErrorCount() {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.stats.Errors++
}

// GetStats возвращает статистику сервера
func (s *TCPServer) GetStats() map[string]interface{} {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	return map[string]interface{}{
		"running":            s.isRunning,
		"address":            s.address,
		"connections_total":  s.stats.ConnectionsTotal,
		"connections_active": s.stats.ConnectionsActive,
		"messages_received":  s.stats.MessagesReceived,
		"batches_received":   s.stats.BatchesReceived,
		"bytes_received":     s.stats.BytesReceived,
		"errors":             s.stats.Errors,
		"last_message_time":  s.stats.LastMessageTime.Format(time.RFC3339),
	}
}

// IsRunning проверяет, работает ли сервер
func (s *TCPServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}
