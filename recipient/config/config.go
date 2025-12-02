package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config представляет полную конфигурацию сервиса recipient
type Config struct {
	Service ServiceConfig `mapstructure:"service"`
	MQTT    MQTTConfig    `mapstructure:"mqtt"`
	TCP     TCPConfig     `mapstructure:"tcp"`
	Logger  LoggerConfig  `mapstructure:"logger"`
	Metrics MetricsConfig `mapstructure:"metrics"`
}

// ServiceConfig конфигурация сервиса
type ServiceConfig struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
}

// MQTTConfig конфигурация MQTT брокера
type MQTTConfig struct {
	Broker          string        `mapstructure:"broker"`                 // Адрес брокера (tcp://host:port)
	ClientID        string        `mapstructure:"client_id"`              // Уникальный идентификатор клиента
	Username        string        `mapstructure:"username"`               // Имя пользователя для аутентификации
	Password        string        `mapstructure:"password"`               // Пароль для аутентификации
	Topic           string        `mapstructure:"topic"`                  // Топик для подписки
	QoS             byte          `mapstructure:"qos"`                    // Quality of Service (0, 1, 2)
	CleanSession    bool          `mapstructure:"clean_session"`          // Очищать ли сессию при подключении
	KeepAlive       time.Duration `mapstructure:"keep_alive"`             // Интервал keep-alive
	ConnectTimeout  time.Duration `mapstructure:"connect_timeout"`        // Таймаут подключения
	MaxReconnectInt time.Duration `mapstructure:"max_reconnect_interval"` // Максимальный интервал переподключения
	AutoReconnect   bool          `mapstructure:"auto_reconnect"`         // Автоматическое переподключение
	OrderMatters    bool          `mapstructure:"order_matters"`          // Сохранять ли порядок сообщений
	StoreDirectory  string        `mapstructure:"store_directory"`        // Директория для хранения сообщений
	MaxInflight     int           `mapstructure:"max_inflight"`           // Максимум сообщений в обработке
}

// TCPConfig конфигурация TCP сервера
type TCPConfig struct {
	Address         string        `mapstructure:"address"`           // Адрес для прослушивания (host:port)
	MaxConnections  int           `mapstructure:"max_connections"`   // Максимальное количество подключений
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`      // Таймаут чтения
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`     // Таймаут записи
	KeepAlive       bool          `mapstructure:"keep_alive"`        // Использовать ли keep-alive
	KeepAlivePeriod time.Duration `mapstructure:"keep_alive_period"` // Период keep-alive
	Enabled         bool          `mapstructure:"enabled"`           // Включен ли TCP сервер
}

// LoggerConfig конфигурация логирования
type LoggerConfig struct {
	Level      string `mapstructure:"level"`
	FilePath   string `mapstructure:"file_path"`
	MaxSize    int    `mapstructure:"max_size"` // megabytes
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"` // days
	Compress   bool   `mapstructure:"compress"`
	Console    bool   `mapstructure:"console"`
}

// MetricsConfig конфигурация метрик
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
	Port    int    `mapstructure:"port"`
}

// Load загружает конфигурацию из файла и переменных окружения
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Устанавливаем значения по умолчанию
	setDefaults(v)

	// Настраиваем чтение из переменных окружения
	v.SetEnvPrefix("RECIPIENT")
	v.AutomaticEnv()

	// Если указан путь к конфигурации, читаем файл
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("ошибка чтения конфигурации: %w", err)
		}
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга конфигурации: %w", err)
	}

	// Валидация конфигурации
	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("ошибка валидации конфигурации: %w", err)
	}

	// Создаем директории если не существуют
	if err := ensureDirectories(&config); err != nil {
		return nil, fmt.Errorf("ошибка создания директорий: %w", err)
	}

	return &config, nil
}

// setDefaults устанавливает значения по умолчанию
func setDefaults(v *viper.Viper) {
	// Service
	v.SetDefault("service.name", "recipient")
	v.SetDefault("service.version", "1.0.0")

	// MQTT
	v.SetDefault("mqtt.broker", "tcp://localhost:1883")
	v.SetDefault("mqtt.client_id", "recipient-001")
	v.SetDefault("mqtt.username", "")
	v.SetDefault("mqtt.password", "")
	v.SetDefault("mqtt.topic", "test/messages")
	v.SetDefault("mqtt.qos", 1) // At least once delivery
	v.SetDefault("mqtt.clean_session", false)
	v.SetDefault("mqtt.keep_alive", "60s")
	v.SetDefault("mqtt.connect_timeout", "30s")
	v.SetDefault("mqtt.max_reconnect_interval", "10m")
	v.SetDefault("mqtt.auto_reconnect", true)
	v.SetDefault("mqtt.order_matters", true)
	v.SetDefault("mqtt.store_directory", "/tmp/mqtt-recipient-store")
	v.SetDefault("mqtt.max_inflight", 100)

	// Logger
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.file_path", "logs/recipient.log")
	v.SetDefault("logger.max_size", 100)
	v.SetDefault("logger.max_backups", 5)
	v.SetDefault("logger.max_age", 30)
	v.SetDefault("logger.compress", true)
	v.SetDefault("logger.console", true)

	// Metrics
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")
	v.SetDefault("metrics.port", 8081)
}

// validate проверяет корректность конфигурации
func validate(cfg *Config) error {
	if cfg.MQTT.Broker == "" {
		return fmt.Errorf("не указан адрес MQTT брокера")
	}

	if cfg.MQTT.ClientID == "" {
		return fmt.Errorf("не указан client_id для MQTT")
	}

	if cfg.MQTT.Topic == "" {
		return fmt.Errorf("не указан топик MQTT")
	}

	if cfg.MQTT.QoS > 2 {
		return fmt.Errorf("некорректный уровень QoS: %d (должен быть 0, 1 или 2)", cfg.MQTT.QoS)
	}

	if cfg.MQTT.MaxInflight <= 0 {
		return fmt.Errorf("max_inflight должно быть больше 0")
	}

	if cfg.Metrics.Port <= 0 || cfg.Metrics.Port > 65535 {
		return fmt.Errorf("некорректный порт для метрик: %d", cfg.Metrics.Port)
	}

	return nil
}

// ensureDirectories создает необходимые директории
func ensureDirectories(cfg *Config) error {
	// Создаем директорию для логов
	logDir := getDir(cfg.Logger.FilePath)
	if logDir != "" {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("не удалось создать директорию для логов: %w", err)
		}
	}

	// Создаем директорию для MQTT store
	if cfg.MQTT.StoreDirectory != "" {
		if err := os.MkdirAll(cfg.MQTT.StoreDirectory, 0755); err != nil {
			return fmt.Errorf("не удалось создать директорию для MQTT store: %w", err)
		}
	}

	return nil
}

// getDir возвращает директорию из пути к файлу
func getDir(filePath string) string {
	for i := len(filePath) - 1; i >= 0; i-- {
		if filePath[i] == '/' || filePath[i] == os.PathSeparator {
			return filePath[:i]
		}
	}
	return ""
}
