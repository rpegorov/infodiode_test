# Recipient Service

Сервис приема и валидации тестовых данных, переданных через MQTT для тестирования производительности передачи данных через инфодиод.

## Описание

Recipient - это компонент системы тестирования, который:
- Принимает сообщения из MQTT брокера
- Валидирует целостность данных через контрольные суммы
- Измеряет задержки передачи
- Собирает статистику приема и обработки
- Выявляет потери и повреждения данных

## API Endpoints

### Health Check

#### `GET /health`
Проверка состояния сервиса и его компонентов.

**Ответ:**
```json
{
  "status": "healthy",
  "service": "recipient",
  "version": "1.0.0",
  "timestamp": "2024-01-20T15:30:45Z",
  "checks": [
    {
      "component": "mqtt",
      "status": "healthy"
    },
    {
      "component": "processor",
      "status": "healthy"
    }
  ]
}
```

#### `GET /ready`
Проверка готовности сервиса к приему данных.

**Ответ:**
```json
{
  "status": "ready"
}
```

### Статистика и метрики

#### `GET /stats`
Получение подробной статистики обработки сообщений.

**Ответ:**
```json
{
  "messages": {
    "received": 10000,
    "processed": 9998,
    "valid": 9950,
    "invalid": 48,
    "checksum_errors": 48,
    "processing_errors": 2
  },
  "performance": {
    "total_bytes_received": 10240000,
    "avg_message_size": 1024,
    "throughput": 523.4,
    "min_latency_ms": 12.3,
    "max_latency_ms": 145.7,
    "avg_latency_ms": 23.5
  },
  "timing": {
    "first_message_time": "2024-01-20T15:25:00Z",
    "last_message_time": "2024-01-20T15:30:00Z",
    "duration_seconds": 300
  },
  "errors": {
    "checksum_mismatches": [
      {
        "message_id": 1234,
        "timestamp": "2024-01-20T15:26:12Z",
        "expected": "a1b2c3...",
        "actual": "d4e5f6..."
      }
    ],
    "processing_failures": [
      {
        "message_id": 5678,
        "timestamp": "2024-01-20T15:27:45Z",
        "error": "JSON parsing failed"
      }
    ]
  }
}
```

#### `GET /metrics`
Возвращает метрики в формате Prometheus для мониторинга.

**Формат метрик:**
```
# HELP messages_received_total Total number of messages received
# TYPE messages_received_total counter
messages_received_total 10000

# HELP messages_valid_total Total number of valid messages
# TYPE messages_valid_total counter
messages_valid_total 9950

# HELP messages_invalid_total Total number of invalid messages
# TYPE messages_invalid_total counter
messages_invalid_total 48

# HELP message_latency_ms Message delivery latency in milliseconds
# TYPE message_latency_ms histogram
message_latency_ms_bucket{le="10"} 1000
message_latency_ms_bucket{le="25"} 7500
message_latency_ms_bucket{le="50"} 9500
message_latency_ms_bucket{le="100"} 9900
message_latency_ms_bucket{le="+Inf"} 10000
message_latency_ms_sum 235000
message_latency_ms_count 10000

# HELP throughput_messages_per_second Current message processing throughput
# TYPE throughput_messages_per_second gauge
throughput_messages_per_second 523.4
```

## Интерпретация результатов

### Основные метрики

#### 1. Целостность данных (Data Integrity)

**Показатели:**
- `messages.valid` - количество сообщений с корректной контрольной суммой
- `messages.invalid` - количество поврежденных сообщений
- `messages.checksum_errors` - ошибки контрольной суммы

**Интерпретация:**
- **Отличный результат**: 100% valid (0 invalid)
  - Данные передаются без искажений
  - Инфодиод работает корректно
  
- **Хороший результат**: >99.9% valid (<0.1% invalid)
  - Минимальные потери, приемлемо для большинства задач
  - Возможны редкие сбои в канале передачи
  
- **Удовлетворительный результат**: >99% valid (<1% invalid)
  - Требуется дополнительная проверка оборудования
  - Возможны проблемы с буферизацией или пропускной способностью
  
- **Неудовлетворительный результат**: <99% valid (>1% invalid)
  - Серьезные проблемы с каналом передачи
  - Требуется диагностика инфодиода и сетевого оборудования

#### 2. Пропускная способность (Throughput)

**Показатели:**
- `performance.throughput` - сообщений в секунду
- `performance.total_bytes_received` - общий объем данных
- `performance.avg_message_size` - средний размер сообщения

**Интерпретация:**
- **Высокая пропускная способность**: >1000 msg/sec
  - Канал справляется с высокой нагрузкой
  - Инфодиод не является узким местом
  
- **Средняя пропускная способность**: 100-1000 msg/sec
  - Подходит для большинства задач мониторинга
  - Может потребоваться оптимизация для пиковых нагрузок
  
- **Низкая пропускная способность**: <100 msg/sec
  - Проверьте настройки буферизации
  - Возможны ограничения инфодиода или сети

#### 3. Задержка передачи (Latency)

**Показатели:**
- `performance.min_latency_ms` - минимальная задержка
- `performance.max_latency_ms` - максимальная задержка
- `performance.avg_latency_ms` - средняя задержка

**Интерпретация:**
- **Отличная задержка**: <50ms avg
  - Подходит для систем реального времени
  - Минимальная буферизация в канале
  
- **Хорошая задержка**: 50-200ms avg
  - Приемлемо для большинства применений
  - Нормальная работа с буферизацией
  
- **Высокая задержка**: >200ms avg
  - Может быть проблемой для критичных систем
  - Проверьте размеры буферов и нагрузку

**Важно**: Большая разница между min и max задержкой указывает на:
- Неравномерную нагрузку
- Проблемы с буферизацией
- Сетевые задержки

#### 4. Потери сообщений

**Расчет:**
```
Потери = (Отправлено - Получено) / Отправлено * 100%
```

**Интерпретация:**
- **0% потерь**: Идеальная передача
- **<0.01% потерь**: Отличный результат
- **<0.1% потерь**: Приемлемо для некритичных данных
- **>0.1% потерь**: Требуется анализ причин

### Типичные проблемы и их признаки

#### Проблема 1: Переполнение буфера инфодиода

**Признаки:**
- Резкий рост потерь при увеличении нагрузки
- Большая разница между min и max задержкой
- Периодические всплески checksum_errors

**Решение:**
- Уменьшите скорость отправки
- Увеличьте буферы инфодиода
- Используйте пакетную отправку вместо потоковой

#### Проблема 2: Искажение данных при передаче

**Признаки:**
- Постоянный процент checksum_errors
- Ошибки не зависят от нагрузки
- Повреждения в случайных местах пакетов

**Решение:**
- Проверьте физическое подключение
- Проверьте настройки инфодиода
- Используйте меньшие пакеты

#### Проблема 3: Проблемы с MQTT брокером

**Признаки:**
- processing_errors без checksum_errors
- Задержки кратны таймаутам MQTT
- Периодические разрывы соединения

**Решение:**
- Проверьте настройки QoS
- Увеличьте keep_alive интервал
- Проверьте лимиты брокера

## Конфигурация

Основные параметры в `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8081
  read_timeout: 30s
  write_timeout: 30s

mqtt:
  broker: "tcp://localhost:1883"
  client_id: "recipient-001"
  topic: "test/data"
  qos: 1
  keep_alive: 60
  connect_timeout: 30s
  max_reconnect_interval: 1m

processor:
  buffer_size: 1000
  workers: 4
  batch_timeout: 100ms

validator:
  checksum_algorithm: "sha256"
  strict_mode: true

logger:
  level: "info"
  output_path: "./logs/recipient.log"
  max_size_mb: 100
  max_backups: 5
  max_age_days: 7
```

## Мониторинг производительности

### Ключевые метрики для мониторинга

1. **Throughput** - должна соответствовать скорости отправки
2. **Valid message ratio** - должно быть >99.9%
3. **Average latency** - должна быть стабильной
4. **Memory usage** - не должна постоянно расти

### Grafana Dashboard

Рекомендуемые панели:
- График throughput во времени
- Гистограмма распределения задержек
- Счетчик валидных/невалидных сообщений
- График использования памяти и CPU

### Алерты

Настройте оповещения для:
- Valid ratio < 99%
- Throughput < 80% от целевой
- Average latency > 500ms
- Отсутствие новых сообщений > 30 секунд

## Логирование

### Уровни логов

- `DEBUG` - детальная информация о каждом сообщении
- `INFO` - основные события и статистика
- `WARN` - проблемы с контрольными суммами
- `ERROR` - критические ошибки обработки

### Анализ логов

```bash
# Подсчет ошибок контрольной суммы
grep "Checksum mismatch" recipient.log | wc -l

# Средняя задержка за последний час
grep "latency" recipient.log | tail -n 1000 | \
  awk '{print $NF}' | awk '{sum+=$1} END {print sum/NR}'

# Топ-10 message_id с ошибками
grep "error" recipient.log | \
  grep -o "message_id=[0-9]*" | \
  sort | uniq -c | sort -rn | head -10
```

## Требования к ресурсам

### Минимальные
- CPU: 1 ядро
- RAM: 256MB
- Диск: 1GB для логов

### Рекомендуемые для высокой нагрузки
- CPU: 2+ ядра
- RAM: 1GB+
- Диск: 10GB+ для логов
- SSD для быстрой записи логов

## Docker

### Сборка
```bash
docker build -t infodiode-recipient:latest -f docker/Dockerfile.recipient .
```

### Запуск
```bash
docker run -d \
  --name infodiode-recipient \
  -p 8081:8081 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/logs:/app/logs \
  --network infodiode-net \
  infodiode-recipient:latest
```

## Связанные компоненты

- [Sender Service](../sender/README.md) - сервис отправки тестовых данных
- [Shared Models](../shared/README.md) - общие структуры данных
- [Checksum Validator](./internal/validator/README.md) - модуль валидации