package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config представляет полную конфигурацию сервиса sender
type Config struct {
	Service ServiceConfig `mapstructure:"service"`
	MQTT    MQTTConfig    `mapstructure:"mqtt"`
	TCP     TCPConfig     `mapstructure:"tcp"`
	Logger  LoggerConfig  `mapstructure:"logger"`
	Data    DataConfig    `mapstructure:"data"`
	HTTP    HTTPConfig    `mapstructure:"http"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Tests   TestsConfig   `mapstructure:"tests"`
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
	Topic           string        `mapstructure:"topic"`                  // Топик для публикации
	QoS             byte          `mapstructure:"qos"`                    // Quality of Service (0, 1, 2)
	Retained        bool          `mapstructure:"retained"`               // Сохранять ли последнее сообщение
	CleanSession    bool          `mapstructure:"clean_session"`          // Очищать ли сессию при подключении
	KeepAlive       time.Duration `mapstructure:"keep_alive"`             // Интервал keep-alive
	ConnectTimeout  time.Duration `mapstructure:"connect_timeout"`        // Таймаут подключения
	MaxReconnectInt time.Duration `mapstructure:"max_reconnect_interval"` // Максимальный интервал переподключения
	AutoReconnect   bool          `mapstructure:"auto_reconnect"`         // Автоматическое переподключение
	OrderMatters    bool          `mapstructure:"order_matters"`          // Сохранять ли порядок сообщений
	StoreDirectory  string        `mapstructure:"store_directory"`        // Директория для хранения сообщений при отсутствии связи
	MaxBufferedMsgs int           `mapstructure:"max_buffered_messages"`  // Максимум буферизованных сообщений
}

// TCPConfig конфигурация TCP клиента
type TCPConfig struct {
	Address         string        `mapstructure:"address"`            // Адрес TCP сервера (host:port)
	ReconnectInt    time.Duration `mapstructure:"reconnect_interval"` // Интервал переподключения
	MaxRetries      int           `mapstructure:"max_retries"`        // Максимальное количество попыток
	Timeout         time.Duration `mapstructure:"timeout"`            // Таймаут операций
	KeepAlive       bool          `mapstructure:"keep_alive"`         // Использовать ли keep-alive
	KeepAlivePeriod time.Duration `mapstructure:"keep_alive_period"`  // Период keep-alive
	Enabled         bool          `mapstructure:"enabled"`            // Включен ли TCP транспорт
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

// DataConfig конфигурация генератора данных
type DataConfig struct {
	DataPath         string  `mapstructure:"data_path"`
	GeneratorSeed    int64   `mapstructure:"generator_seed"`
	IndicatorIDRange []int   `mapstructure:"indicator_id_range"`
	EquipmentIDRange []int   `mapstructure:"equipment_id_range"`
	NullPercent      float64 `mapstructure:"null_percent"`
	BoolPercent      float64 `mapstructure:"bool_percent"`
	FloatPercent     float64 `mapstructure:"float_percent"`
	StringPercent    float64 `mapstructure:"string_percent"`
	SmallBatchSize   int     `mapstructure:"small_batch_size"`
	MediumBatchSize  int     `mapstructure:"medium_batch_size"`
	LargeBatchSizes  []int   `mapstructure:"large_batch_sizes"`
}

// HTTPConfig конфигурация HTTP сервера
type HTTPConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// MetricsConfig конфигурация метрик
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// TestsConfig конфигурация тестов
type TestsConfig struct {
	BatchThreads    []int         `mapstructure:"batch_threads"`
	StreamRates     []int         `mapstructure:"stream_rates"`
	LargeSizes      []int         `mapstructure:"large_sizes"`
	DefaultDuration time.Duration `mapstructure:"default_duration"`
	MaxTestDuration time.Duration `mapstructure:"max_test_duration"`
}

// Load загружает конфигурацию из файла и переменных окружения
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Устанавливаем значения по умолчанию
	setDefaults(v)

	// Настраиваем чтение из переменных окружения
	v.SetEnvPrefix("SENDER")
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
	v.SetDefault("service.name", "sender")
	v.SetDefault("service.version", "1.0.0")

	// MQTT
	v.SetDefault("mqtt.broker", "tcp://localhost:1883")
	v.SetDefault("mqtt.client_id", "sender-001")
	v.SetDefault("mqtt.username", "")
	v.SetDefault("mqtt.password", "")
	v.SetDefault("mqtt.topic", "test/messages")
	v.SetDefault("mqtt.qos", 1) // At least once delivery
	v.SetDefault("mqtt.retained", false)
	v.SetDefault("mqtt.clean_session", false)
	v.SetDefault("mqtt.keep_alive", "60s")
	v.SetDefault("mqtt.connect_timeout", "30s")
	v.SetDefault("mqtt.max_reconnect_interval", "10m")
	v.SetDefault("mqtt.auto_reconnect", true)
	v.SetDefault("mqtt.order_matters", true)
	v.SetDefault("mqtt.store_directory", "/tmp/mqtt-sender-store")
	v.SetDefault("mqtt.max_buffered_messages", 10000)

	// Logger
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.file_path", "logs/sender.log")
	v.SetDefault("logger.max_size", 100)
	v.SetDefault("logger.max_backups", 5)
	v.SetDefault("logger.max_age", 30)
	v.SetDefault("logger.compress", true)
	v.SetDefault("logger.console", true)

	// Data
	v.SetDefault("data.data_path", "data")
	v.SetDefault("data.generator_seed", time.Now().UnixNano())
	v.SetDefault("data.indicator_id_range", []int{1, 1000})
	v.SetDefault("data.equipment_id_range", []int{1, 100})
	v.SetDefault("data.null_percent", 10.0)
	v.SetDefault("data.bool_percent", 20.0)
	v.SetDefault("data.float_percent", 40.0)
	v.SetDefault("data.string_percent", 30.0)
	v.SetDefault("data.small_batch_size", 1000)
	v.SetDefault("data.medium_batch_size", 10000)
	v.SetDefault("data.large_batch_sizes", []int{5, 10, 50, 100})

	// HTTP
	v.SetDefault("http.host", "0.0.0.0")
	v.SetDefault("http.port", 8080)
	v.SetDefault("http.read_timeout", "30s")
	v.SetDefault("http.write_timeout", "30s")
	v.SetDefault("http.shutdown_timeout", "10s")

	// Metrics
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")

	// Tests
	v.SetDefault("tests.batch_threads", []int{25, 50, 100})
	v.SetDefault("tests.stream_rates", []int{100, 1000, 5000, 10000})
	v.SetDefault("tests.large_sizes", []int{5, 10, 50, 100})
	v.SetDefault("tests.default_duration", "60s")
	v.SetDefault("tests.max_test_duration", "3600s")
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

	if cfg.HTTP.Port <= 0 || cfg.HTTP.Port > 65535 {
		return fmt.Errorf("некорректный порт HTTP: %d", cfg.HTTP.Port)
	}

	percentSum := cfg.Data.NullPercent + cfg.Data.BoolPercent +
		cfg.Data.FloatPercent + cfg.Data.StringPercent
	if percentSum != 100.0 {
		return fmt.Errorf("сумма процентов типов данных должна быть 100, получено: %.2f", percentSum)
	}

	if len(cfg.Data.IndicatorIDRange) != 2 || cfg.Data.IndicatorIDRange[0] >= cfg.Data.IndicatorIDRange[1] {
		return fmt.Errorf("некорректный диапазон indicator_id")
	}

	if len(cfg.Data.EquipmentIDRange) != 2 || cfg.Data.EquipmentIDRange[0] >= cfg.Data.EquipmentIDRange[1] {
		return fmt.Errorf("некорректный диапазон equipment_id")
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

	// Создаем директорию для данных
	if err := os.MkdirAll(cfg.Data.DataPath, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию для данных: %w", err)
	}

	// Создаем поддиректории для разных размеров данных
	dataDirs := []string{"small", "medium", "large"}
	for _, dir := range dataDirs {
		path := fmt.Sprintf("%s/%s", cfg.Data.DataPath, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("не удалось создать директорию %s: %w", path, err)
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
