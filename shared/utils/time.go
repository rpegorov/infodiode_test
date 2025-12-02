package utils

import (
	"time"
)

const (
	// TimeFormat стандартный формат времени для проекта (RFC3339)
	TimeFormat = time.RFC3339Nano
)

// GetCurrentTime возвращает текущее время в формате RFC3339Nano
func GetCurrentTime() string {
	return time.Now().Format(TimeFormat)
}

// ParseTime парсит строку времени в формате RFC3339Nano
func ParseTime(timeStr string) (time.Time, error) {
	return time.Parse(TimeFormat, timeStr)
}

// CalculateLatency вычисляет задержку между двумя временными метками в миллисекундах
func CalculateLatency(sendTime, receiveTime string) (float64, error) {
	sent, err := ParseTime(sendTime)
	if err != nil {
		return 0, err
	}

	received, err := ParseTime(receiveTime)
	if err != nil {
		return 0, err
	}

	return float64(received.Sub(sent).Microseconds()) / 1000.0, nil
}

// FormatDuration форматирует продолжительность в читаемый вид
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Millisecond).String()
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return (time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second).String()
	}
	return d.Round(time.Second).String()
}

// GetTimestamp возвращает текущее время в формате Unix timestamp
func GetTimestamp() int64 {
	return time.Now().Unix()
}

// GetTimestampMillis возвращает текущее время в формате Unix timestamp (миллисекунды)
func GetTimestampMillis() int64 {
	return time.Now().UnixMilli()
}
