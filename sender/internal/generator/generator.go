package generator

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"

	"github.com/infodiode/shared/models"
	"github.com/infodiode/shared/utils"
	"go.uber.org/zap"
)

// DataGenerator генератор тестовых данных
type DataGenerator struct {
	config    *Config
	logger    *zap.Logger
	random    *rand.Rand
	idCounter int
	mu        sync.Mutex
	dataCache map[string][]*models.Data
	cacheMu   sync.RWMutex
}

// Config конфигурация генератора
type Config struct {
	DataPath         string
	Seed             int64
	IndicatorIDRange []int
	EquipmentIDRange []int
	NullPercent      float64
	BoolPercent      float64
	FloatPercent     float64
	StringPercent    float64
	SmallBatchSize   int
	MediumBatchSize  int
	LargeBatchSizes  []int
}

// NewDataGenerator создает новый генератор данных
func NewDataGenerator(config *Config, logger *zap.Logger) *DataGenerator {
	source := rand.NewSource(config.Seed)
	return &DataGenerator{
		config:    config,
		logger:    logger,
		random:    rand.New(source),
		idCounter: 1,
		dataCache: make(map[string][]*models.Data),
	}
}

// GenerateData генерирует одну запись данных
func (g *DataGenerator) GenerateData() *models.Data {
	g.mu.Lock()
	id := g.idCounter
	g.idCounter++
	g.mu.Unlock()

	indicatorID := g.randomInRange(g.config.IndicatorIDRange[0], g.config.IndicatorIDRange[1])
	equipmentID := g.randomInRange(g.config.EquipmentIDRange[0], g.config.EquipmentIDRange[1])

	return &models.Data{
		ID:             id,
		Timestamp:      utils.GetCurrentTime(),
		IndicatorID:    indicatorID,
		IndicatorValue: g.generateIndicatorValue(),
		EquipmentID:    equipmentID,
	}
}

// generateIndicatorValue генерирует значение индикатора согласно распределению
func (g *DataGenerator) generateIndicatorValue() string {
	// Определяем тип значения на основе процентного распределения
	roll := g.random.Float64() * 100

	if roll < g.config.NullPercent {
		return "null"
	} else if roll < g.config.NullPercent+g.config.BoolPercent {
		return g.generateBoolValue()
	} else if roll < g.config.NullPercent+g.config.BoolPercent+g.config.FloatPercent {
		return g.generateFloatValue()
	} else {
		return g.generateStringValue()
	}
}

// generateBoolValue генерирует булево значение (15 символов)
func (g *DataGenerator) generateBoolValue() string {
	if g.random.Intn(2) == 0 {
		return padToLength("true", 15)
	}
	return padToLength("false", 15)
}

// generateFloatValue генерирует число с плавающей точкой (15 символов)
func (g *DataGenerator) generateFloatValue() string {
	// Генерируем число от -9999.99 до 9999.99
	value := (g.random.Float64() * 20000) - 10000
	str := fmt.Sprintf("%.2f", value)
	return padToLength(str, 15)
}

// generateStringValue генерирует строку из букв и цифр (15 символов)
func (g *DataGenerator) generateStringValue() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 15)
	for i := range result {
		result[i] = charset[g.random.Intn(len(charset))]
	}
	return string(result)
}

// padToLength дополняет строку до нужной длины пробелами
func padToLength(s string, length int) string {
	if len(s) >= length {
		return s[:length]
	}
	return s + string(make([]byte, length-len(s)))
}

// randomInRange генерирует случайное число в диапазоне [min, max]
func (g *DataGenerator) randomInRange(min, max int) int {
	return min + g.random.Intn(max-min+1)
}

// GenerateBatch генерирует пакет данных заданного размера
func (g *DataGenerator) GenerateBatch(count int) []*models.Data {
	batch := make([]*models.Data, count)
	for i := 0; i < count; i++ {
		batch[i] = g.GenerateData()
	}
	return batch
}

// SaveToFile сохраняет данные в файл в формате JSON Lines
func (g *DataGenerator) SaveToFile(filename string, data []*models.Data) error {
	// Создаем директорию если не существует
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию %s: %w", dir, err)
	}

	// Открываем файл для записи
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("не удалось создать файл %s: %w", filename, err)
	}
	defer file.Close()

	// Записываем данные в формате JSON Lines
	encoder := json.NewEncoder(file)
	for _, item := range data {
		if err := encoder.Encode(item); err != nil {
			return fmt.Errorf("ошибка записи в файл: %w", err)
		}
	}

	// Получаем информацию о файле
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	g.logger.Info("Данные сохранены в файл",
		zap.String("файл", filename),
		zap.Int("записей", len(data)),
		zap.Int64("размер_байт", fileInfo.Size()))

	return nil
}

// LoadFromFile загружает данные из файла JSON Lines
func (g *DataGenerator) LoadFromFile(filename string) ([]*models.Data, error) {
	// Проверяем кеш
	g.cacheMu.RLock()
	if cached, ok := g.dataCache[filename]; ok {
		g.cacheMu.RUnlock()
		return cached, nil
	}
	g.cacheMu.RUnlock()

	// Открываем файл
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл %s: %w", filename, err)
	}
	defer file.Close()

	// Читаем данные
	var data []*models.Data
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var item models.Data
		if err := decoder.Decode(&item); err != nil {
			return nil, fmt.Errorf("ошибка чтения из файла: %w", err)
		}
		data = append(data, &item)
	}

	// Сохраняем в кеш
	g.cacheMu.Lock()
	g.dataCache[filename] = data
	g.cacheMu.Unlock()

	g.logger.Info("Данные загружены из файла",
		zap.String("файл", filename),
		zap.Int("записей", len(data)))

	return data, nil
}

// GenerateAllTestData генерирует все тестовые данные
func (g *DataGenerator) GenerateAllTestData() error {
	g.logger.Info("Начало генерации всех тестовых данных")

	// Генерируем маленькие пакеты
	if err := g.GenerateSmallBatches(); err != nil {
		return err
	}

	// Генерируем средние пакеты
	if err := g.GenerateMediumBatches(); err != nil {
		return err
	}

	// Генерируем большие пакеты
	if err := g.GenerateLargeBatches(); err != nil {
		return err
	}

	g.logger.Info("Генерация всех тестовых данных завершена")
	return nil
}

// GenerateSmallBatches генерирует маленькие пакеты данных (~100KB каждый)
func (g *DataGenerator) GenerateSmallBatches() error {
	g.logger.Info("Генерация маленьких пакетов данных")

	// Примерно 100 записей на файл для ~100KB
	recordsPerFile := 100
	numFiles := 10 // 10 файлов

	for i := 1; i <= numFiles; i++ {
		data := g.GenerateBatch(recordsPerFile)
		filename := fmt.Sprintf("%s/small/batch_%03d.jsonl", g.config.DataPath, i)

		if err := g.SaveToFile(filename, data); err != nil {
			return fmt.Errorf("ошибка генерации маленького пакета %d: %w", i, err)
		}
	}

	return nil
}

// GenerateMediumBatches генерирует средние пакеты данных (~1MB каждый)
func (g *DataGenerator) GenerateMediumBatches() error {
	g.logger.Info("Генерация средних пакетов данных")

	// Примерно 1000 записей на файл для ~1MB
	recordsPerFile := 1000
	numFiles := 5 // 5 файлов

	for i := 1; i <= numFiles; i++ {
		data := g.GenerateBatch(recordsPerFile)
		filename := fmt.Sprintf("%s/medium/batch_%03d.jsonl", g.config.DataPath, i)

		if err := g.SaveToFile(filename, data); err != nil {
			return fmt.Errorf("ошибка генерации среднего пакета %d: %w", i, err)
		}
	}

	return nil
}

// GenerateLargeBatches генерирует большие пакеты данных (5-100MB)
func (g *DataGenerator) GenerateLargeBatches() error {
	g.logger.Info("Генерация больших пакетов данных")

	// Размеры в MB и соответствующее количество записей
	sizeMap := map[int]int{
		5:   5000,   // ~5MB
		10:  10000,  // ~10MB
		50:  50000,  // ~50MB
		100: 100000, // ~100MB
	}

	for _, sizeMB := range g.config.LargeBatchSizes {
		recordsCount, ok := sizeMap[sizeMB]
		if !ok {
			// Примерная оценка: 1000 записей на MB
			recordsCount = sizeMB * 1000
		}

		data := g.GenerateBatch(recordsCount)
		filename := fmt.Sprintf("%s/large/batch_%dmb.jsonl", g.config.DataPath, sizeMB)

		if err := g.SaveToFile(filename, data); err != nil {
			return fmt.Errorf("ошибка генерации большого пакета %dMB: %w", sizeMB, err)
		}
	}

	return nil
}

// GetDataForTest возвращает данные для конкретного теста
func (g *DataGenerator) GetDataForTest(testType string, size int) ([]*models.Data, error) {
	var filename string

	switch testType {
	case "small":
		// Берем первый файл из маленьких пакетов
		filename = fmt.Sprintf("%s/small/batch_001.jsonl", g.config.DataPath)
	case "medium":
		// Берем первый файл из средних пакетов
		filename = fmt.Sprintf("%s/medium/batch_001.jsonl", g.config.DataPath)
	case "large":
		// Берем файл соответствующего размера
		filename = fmt.Sprintf("%s/large/batch_%dmb.jsonl", g.config.DataPath, size)
	default:
		return nil, fmt.Errorf("неизвестный тип теста: %s", testType)
	}

	return g.LoadFromFile(filename)
}

// StreamDataFromFile читает данные из файла построчно без загрузки в память
func (g *DataGenerator) StreamDataFromFile(filename string, handler func(*models.Data) error) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("не удалось открыть файл %s: %w", filename, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	lineNum := 0

	for decoder.More() {
		lineNum++
		var item models.Data
		if err := decoder.Decode(&item); err != nil {
			g.logger.Error("Ошибка декодирования строки",
				zap.String("файл", filename),
				zap.Int("строка", lineNum),
				zap.Error(err))
			continue
		}

		if err := handler(&item); err != nil {
			return fmt.Errorf("ошибка обработки данных на строке %d: %w", lineNum, err)
		}
	}

	return nil
}

// ClearCache очищает кеш загруженных данных
func (g *DataGenerator) ClearCache() {
	g.cacheMu.Lock()
	g.dataCache = make(map[string][]*models.Data)
	g.cacheMu.Unlock()
	g.logger.Info("Кеш данных очищен")
}

// GetStatistics возвращает статистику по сгенерированным данным
func (g *DataGenerator) GetStatistics() (*GeneratorStats, error) {
	stats := &GeneratorStats{
		SmallBatches:  0,
		MediumBatches: 0,
		LargeBatches:  0,
		TotalRecords:  0,
		TotalSize:     0,
	}

	// Подсчет маленьких пакетов
	smallPath := filepath.Join(g.config.DataPath, "small")
	if files, err := filepath.Glob(filepath.Join(smallPath, "*.jsonl")); err == nil {
		stats.SmallBatches = len(files)
		for _, file := range files {
			if info, err := os.Stat(file); err == nil {
				stats.TotalSize += info.Size()
			}
		}
	}

	// Подсчет средних пакетов
	mediumPath := filepath.Join(g.config.DataPath, "medium")
	if files, err := filepath.Glob(filepath.Join(mediumPath, "*.jsonl")); err == nil {
		stats.MediumBatches = len(files)
		for _, file := range files {
			if info, err := os.Stat(file); err == nil {
				stats.TotalSize += info.Size()
			}
		}
	}

	// Подсчет больших пакетов
	largePath := filepath.Join(g.config.DataPath, "large")
	if files, err := filepath.Glob(filepath.Join(largePath, "*.jsonl")); err == nil {
		stats.LargeBatches = len(files)
		for _, file := range files {
			if info, err := os.Stat(file); err == nil {
				stats.TotalSize += info.Size()
			}
		}
	}

	return stats, nil
}

// GeneratorStats статистика генератора
type GeneratorStats struct {
	SmallBatches  int
	MediumBatches int
	LargeBatches  int
	TotalRecords  int
	TotalSize     int64
}
