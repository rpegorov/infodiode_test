package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// CalculateChecksum вычисляет SHA256 контрольную сумму для данных
func CalculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// VerifyChecksum проверяет соответствие контрольной суммы данным
func VerifyChecksum(data []byte, checksum string) bool {
	calculated := CalculateChecksum(data)
	return calculated == checksum
}

// CalculateChecksumString вычисляет SHA256 контрольную сумму для строки
func CalculateChecksumString(data string) string {
	return CalculateChecksum([]byte(data))
}

// VerifyChecksumString проверяет соответствие контрольной суммы строке
func VerifyChecksumString(data string, checksum string) bool {
	return VerifyChecksum([]byte(data), checksum)
}
