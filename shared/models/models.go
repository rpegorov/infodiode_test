package models

import (
	"time"
)

// Message представляет структуру сообщения в брокере
type Message struct {
	SendTime  string `json:"send_time"`  // Время отправки сообщения
	MessageID int    `json:"message_id"` // Уникальный идентификатор сообщения
	Timestamp string `json:"timestamp"`  // Временная метка создания данных
	Payload   string `json:"payload"`    // Полезная нагрузка в виде JSON строки
	Checksum  string `json:"checksum"`   // Контрольная сумма payload (SHA256 hex)
}

// Data представляет структуру генерируемых данных
type Data struct {
	ID             int    `json:"id"`              // Уникальный идентификатор записи
	Timestamp      string `json:"timestamp"`       // Временная метка создания
	IndicatorID    int    `json:"indicator_id"`    // Идентификатор индикатора
	IndicatorValue string `json:"indicator_value"` // Значение индикатора (15 символов)
	EquipmentID    int    `json:"equipment_id"`    // Идентификатор оборудования
}

// LogEntry представляет структуру записи в лог файле
type LogEntry struct {
	Timestamp     time.Time `json:"timestamp"`                // Время события
	MessageID     int       `json:"message_id"`               // Идентификатор сообщения
	SendTime      string    `json:"send_time"`                // Время отправки
	ReceiveTime   string    `json:"receive_time,omitempty"`   // Время получения (только для recipient)
	Checksum      string    `json:"checksum"`                 // Контрольная сумма
	ChecksumValid *bool     `json:"checksum_valid,omitempty"` // Результат проверки суммы (только для recipient)
	MessageSize   int       `json:"message_size"`             // Размер сообщения в байтах
	ThreadCount   int       `json:"thread_count,omitempty"`   // Количество потоков (только для sender)
	Error         string    `json:"error,omitempty"`          // Ошибка, если есть
}

// TestConfig представляет конфигурацию теста
type TestConfig struct {
	Type           TestType     `json:"type"`             // Тип теста
	Protocol       TestProtocol `json:"protocol"`         // Протокол передачи (MQTT или TCP)
	ThreadCount    int          `json:"thread_count"`     // Количество потоков
	PacketSize     int          `json:"packet_size"`      // Размер пакета в байтах
	MessagesPerSec int          `json:"messages_per_sec"` // Сообщений в секунду
	Duration       int          `json:"duration"`         // Продолжительность теста в секундах
	TotalMessages  int          `json:"total_messages"`   // Общее количество сообщений
}

// TestType определяет тип теста
type TestType string

const (
	TestTypeBatch  TestType = "batch"  // Пакетная отправка
	TestTypeStream TestType = "stream" // Потоковая отправка
	TestTypeLarge  TestType = "large"  // Большие пакеты
	TestTypeBulk   TestType = "bulk"   // Большие пакеты в несколько потоков
)

// TestProtocol определяет протокол передачи данных
type TestProtocol string

const (
	ProtocolMQTT TestProtocol = "mqtt" // Передача через MQTT брокер
	ProtocolTCP  TestProtocol = "tcp"  // Передача через TCP соединение
)

// TestStats представляет статистику теста
type TestStats struct {
	StartTime        time.Time     `json:"start_time"`         // Время начала теста
	EndTime          *time.Time    `json:"end_time,omitempty"` // Время окончания теста
	Duration         time.Duration `json:"duration"`           // Продолжительность
	MessagesSent     int64         `json:"messages_sent"`      // Отправлено сообщений
	MessagesReceived int64         `json:"messages_received"`  // Получено сообщений
	BytesSent        int64         `json:"bytes_sent"`         // Отправлено байт
	BytesReceived    int64         `json:"bytes_received"`     // Получено байт
	Errors           int64         `json:"errors"`             // Количество ошибок
	AvgThroughput    float64       `json:"avg_throughput"`     // Средняя пропускная способность (msg/sec)
	AvgLatency       float64       `json:"avg_latency_ms"`     // Средняя задержка (ms)
	MinLatency       float64       `json:"min_latency_ms"`     // Минимальная задержка (ms)
	MaxLatency       float64       `json:"max_latency_ms"`     // Максимальная задержка (ms)
	P50Latency       float64       `json:"p50_latency_ms"`     // 50-й перцентиль задержки
	P95Latency       float64       `json:"p95_latency_ms"`     // 95-й перцентиль задержки
	P99Latency       float64       `json:"p99_latency_ms"`     // 99-й перцентиль задержки
}

// MessageBatch представляет пакет сообщений для отправки
type MessageBatch struct {
	Messages  []*Message `json:"messages"`  // Массив сообщений
	Timestamp string     `json:"timestamp"` // Временная метка пакета
	Count     int        `json:"count"`     // Количество сообщений в пакете
}

// HealthStatus представляет статус здоровья сервиса
type HealthStatus struct {
	Status    string    `json:"status"`    // "healthy" или "unhealthy"
	Service   string    `json:"service"`   // Имя сервиса
	Version   string    `json:"version"`   // Версия сервиса
	Timestamp time.Time `json:"timestamp"` // Время проверки
	Checks    []Check   `json:"checks"`    // Детальные проверки
}

// Check представляет результат проверки компонента
type Check struct {
	Component string `json:"component"`         // Название компонента
	Status    string `json:"status"`            // Статус проверки
	Message   string `json:"message,omitempty"` // Дополнительное сообщение
}
