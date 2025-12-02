package validator

import (
	"encoding/json"
	"fmt"

	"github.com/infodiode/shared/models"
	"github.com/infodiode/shared/utils"
	"go.uber.org/zap"
)

// ChecksumValidator проверяет контрольные суммы сообщений
type ChecksumValidator struct {
	logger *zap.Logger
}

// NewChecksumValidator создает новый валидатор
func NewChecksumValidator(logger *zap.Logger) *ChecksumValidator {
	return &ChecksumValidator{
		logger: logger,
	}
}

// ValidateMessage проверяет контрольную сумму сообщения
func (v *ChecksumValidator) ValidateMessage(message *models.Message) (bool, error) {
	if message == nil {
		return false, fmt.Errorf("сообщение не может быть nil")
	}

	// Проверяем наличие payload
	if message.Payload == "" {
		return false, fmt.Errorf("payload пустой")
	}

	// Проверяем наличие контрольной суммы
	if message.Checksum == "" {
		return false, fmt.Errorf("контрольная сумма отсутствует")
	}

	// Вычисляем контрольную сумму payload
	calculatedChecksum := utils.CalculateChecksumString(message.Payload)

	// Сравниваем контрольные суммы
	isValid := calculatedChecksum == message.Checksum

	if !isValid {
		v.logger.Debug("Несовпадение контрольной суммы",
			zap.Int("message_id", message.MessageID),
			zap.String("expected", message.Checksum),
			zap.String("calculated", calculatedChecksum),
			zap.Int("payload_length", len(message.Payload)))
	}

	return isValid, nil
}

// ValidatePayload проверяет корректность payload
func (v *ChecksumValidator) ValidatePayload(message *models.Message) (*models.Data, error) {
	if message.Payload == "" {
		return nil, fmt.Errorf("payload пустой")
	}

	// Пытаемся десериализовать payload
	var data models.Data
	if err := json.Unmarshal([]byte(message.Payload), &data); err != nil {
		return nil, fmt.Errorf("ошибка десериализации payload: %w", err)
	}

	// Проверяем обязательные поля
	if data.ID <= 0 {
		return nil, fmt.Errorf("некорректный ID: %d", data.ID)
	}

	if data.Timestamp == "" {
		return nil, fmt.Errorf("отсутствует timestamp")
	}

	if data.IndicatorID <= 0 {
		return nil, fmt.Errorf("некорректный indicator_id: %d", data.IndicatorID)
	}

	if data.EquipmentID <= 0 {
		return nil, fmt.Errorf("некорректный equipment_id: %d", data.EquipmentID)
	}

	// Проверяем длину indicator_value (должна быть 15 символов)
	if len(data.IndicatorValue) != 15 {
		return nil, fmt.Errorf("некорректная длина indicator_value: %d (должна быть 15)", len(data.IndicatorValue))
	}

	return &data, nil
}

// ValidateBatch проверяет пакет сообщений
func (v *ChecksumValidator) ValidateBatch(messages []*models.Message) ([]bool, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("пустой пакет сообщений")
	}

	results := make([]bool, len(messages))
	var hasErrors bool

	for i, msg := range messages {
		isValid, err := v.ValidateMessage(msg)
		if err != nil {
			v.logger.Error("Ошибка валидации сообщения в пакете",
				zap.Int("index", i),
				zap.Int("message_id", msg.MessageID),
				zap.Error(err))
			hasErrors = true
		}
		results[i] = isValid
	}

	if hasErrors {
		return results, fmt.Errorf("обнаружены ошибки при валидации пакета")
	}

	return results, nil
}

// GetStatistics возвращает статистику валидации (для тестирования)
func (v *ChecksumValidator) GetStatistics(messages []*models.Message) ValidationStats {
	stats := ValidationStats{
		Total: len(messages),
	}

	for _, msg := range messages {
		isValid, err := v.ValidateMessage(msg)
		if err != nil {
			stats.Errors++
		} else if isValid {
			stats.Valid++
		} else {
			stats.Invalid++
		}

		// Проверяем payload
		if _, err := v.ValidatePayload(msg); err != nil {
			stats.PayloadErrors++
		}
	}

	if stats.Total > 0 {
		stats.ValidPercent = float64(stats.Valid) / float64(stats.Total) * 100
		stats.InvalidPercent = float64(stats.Invalid) / float64(stats.Total) * 100
		stats.ErrorPercent = float64(stats.Errors) / float64(stats.Total) * 100
	}

	return stats
}

// ValidationStats статистика валидации
type ValidationStats struct {
	Total          int
	Valid          int
	Invalid        int
	Errors         int
	PayloadErrors  int
	ValidPercent   float64
	InvalidPercent float64
	ErrorPercent   float64
}

// ValidateDataIntegrity проверяет целостность данных в payload
func (v *ChecksumValidator) ValidateDataIntegrity(data *models.Data) error {
	// Проверяем, что все поля заполнены корректно
	if data.ID <= 0 {
		return fmt.Errorf("некорректный ID: %d", data.ID)
	}

	// Проверяем формат timestamp
	if _, err := utils.ParseTime(data.Timestamp); err != nil {
		return fmt.Errorf("некорректный формат timestamp: %w", err)
	}

	// Проверяем диапазоны ID
	if data.IndicatorID < 1 || data.IndicatorID > 1000 {
		return fmt.Errorf("indicator_id вне диапазона [1, 1000]: %d", data.IndicatorID)
	}

	if data.EquipmentID < 1 || data.EquipmentID > 100 {
		return fmt.Errorf("equipment_id вне диапазона [1, 100]: %d", data.EquipmentID)
	}

	// Проверяем indicator_value
	if err := v.validateIndicatorValue(data.IndicatorValue); err != nil {
		return fmt.Errorf("некорректный indicator_value: %w", err)
	}

	return nil
}

// validateIndicatorValue проверяет корректность значения индикатора
func (v *ChecksumValidator) validateIndicatorValue(value string) error {
	if len(value) != 15 {
		return fmt.Errorf("длина должна быть 15 символов, получено: %d", len(value))
	}

	// Удаляем trailing пробелы для проверки типа
	trimmed := trimRight(value, ' ')

	// Проверяем различные типы значений
	switch trimmed {
	case "null":
		return nil
	case "true", "false":
		return nil
	default:
		// Проверяем, является ли это числом или строкой
		// Для строки проверяем, что используются только допустимые символы
		for _, r := range value {
			if !isValidCharacter(r) {
				return fmt.Errorf("недопустимый символ: %c", r)
			}
		}
	}

	return nil
}

// trimRight удаляет символы справа
func trimRight(s string, char rune) string {
	i := len(s) - 1
	for i >= 0 && rune(s[i]) == char {
		i--
	}
	return s[:i+1]
}

// isValidCharacter проверяет, является ли символ допустимым
func isValidCharacter(r rune) bool {
	// Разрешаем буквы, цифры, пробелы, точку (для float), минус (для отрицательных чисел)
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == ' ' || r == '.' || r == '-'
}

// CompareChecksums сравнивает две контрольные суммы и возвращает детальную информацию
func (v *ChecksumValidator) CompareChecksums(expected, actual string) ChecksumComparison {
	isValid := expected == actual

	comparison := ChecksumComparison{
		Expected: expected,
		Actual:   actual,
		IsValid:  isValid,
	}

	if !isValid {
		// Находим позицию первого несовпадения
		minLen := len(expected)
		if len(actual) < minLen {
			minLen = len(actual)
		}

		for i := 0; i < minLen; i++ {
			if expected[i] != actual[i] {
				comparison.FirstMismatchPosition = i
				break
			}
		}

		comparison.LengthDifference = len(expected) - len(actual)
	}

	return comparison
}

// ChecksumComparison результат сравнения контрольных сумм
type ChecksumComparison struct {
	Expected              string
	Actual                string
	IsValid               bool
	FirstMismatchPosition int
	LengthDifference      int
}
