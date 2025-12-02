# Sender Service

Сервис генерации и отправки тестовых данных через MQTT для тестирования производительности передачи данных через инфодиод.

## Описание

Sender - это компонент системы тестирования, который:
- Генерирует тестовые данные различного размера
- Отправляет данные в MQTT брокер с заданной интенсивностью
- Предоставляет HTTP API для управления тестами
- Собирает статистику отправки

## API Endpoints

### Health Check

#### `GET /health`
Проверка состояния сервиса и его компонентов.

**Ответ:**
```json
{
  "status": "healthy",
  "service": "sender",
  "version": "1.0.0",
  "timestamp": "2024-01-20T15:30:45Z",
  "checks": [
    {
      "component": "mqtt",
      "status": "healthy"
    },
    {
      "component": "test_manager",
      "status": "healthy",
      "message": "Test running: stream"
    }
  ]
}
```

#### `GET /ready`
Проверка готовности сервиса к работе.

**Ответ:**
```json
{
  "status": "ready"
}
```

### Генерация данных для тестов

### `POST /generate`
**Параметры запроса:**
```json
{
  "type": "all"    // выбор типа генерации данных all small medium large
}
```
Генерирует данные для тестов.

**Ответ:**
```json
{
  "status": "generation started"
}
```

### Управление тестами

#### `POST /test/stream` - Потоковый тест

Запускает тест с постоянной скоростью отправки сообщений. Идеально подходит для тестирования стабильной нагрузки и измерения пропускной способности при равномерном потоке данных.

**Параметры запроса:**
```json
{
  "messages_per_sec": 1000,    // Количество сообщений в секунду (1-100000)
  "packet_size": 1024,          // Размер пакета в байтах (минимум 100)
  "duration": 60                // Длительность теста в секундах (минимум 1)
}
```

**Описание параметров:**
- `messages_per_sec` - целевая скорость отправки сообщений. Sender будет стараться поддерживать эту скорость на протяжении всего теста
- `packet_size` - размер полезной нагрузки каждого сообщения
- `duration` - общее время выполнения теста

**Пример запроса:**
```bash
curl -X POST http://localhost:8080/test/stream \
  -H "Content-Type: application/json" \
  -d '{
    "messages_per_sec": 500,
    "packet_size": 2048,
    "duration": 120
  }'
```

**Ответ:**
```json
{
  "status": "started",
  "test_id": 1705764645,
  "config": {
    "type": "stream",
    "messages_per_sec": 500,
    "packet_size": 2048,
    "duration": 120
  }
}
```

**Особенности потокового теста:**
- Равномерная нагрузка на канал передачи
- Предсказуемое использование пропускной способности
- Подходит для тестирования стабильности при длительной работе
- Позволяет точно измерить максимальную устойчивую пропускную способность

#### `POST /test/batch` - Пакетный тест

Запускает тест с параллельной отправкой сообщений в несколько потоков.

**Параметры запроса:**
```json
{
  "thread_count": 50,           // Количество параллельных потоков (1-1000)
  "packet_size": 1000,          // Размер пакета в байтах (минимум 100)
  "total_messages": 10000,      // Общее количество сообщений (минимум 1)
  "duration": 60                // Максимальная длительность теста в секундах
}
```

**Особенности:**
- Максимальная нагрузка на систему
- Тестирование предельной производительности
- Параллельная обработка

#### `POST /test/large` - Тест больших пакетов

Запускает тест с отправкой больших пакетов данных.

**Параметры запроса:**
```json
{
  "thread_count": 10,           // Количество потоков (1-100)
  "packet_size_mb": 50,         // Размер пакета в мегабайтах (1-1000)
  "duration": 60                // Длительность теста в секундах
}
```

**Особенности:**
- Тестирование передачи больших объемов данных
- Проверка буферизации и фрагментации
- Оценка пропускной способности для больших файлов

#### `POST /test/stop` - Остановка теста

Останавливает текущий выполняющийся тест.

**Ответ:**
```json
{
  "status": "stopped"
}
```

### Статистика

#### `GET /stats`

Получение текущей статистики отправки сообщений.

**Ответ:**
```json
{
  "producer": {
    "messages_sent": 5000,
    "bytes_sent": 5120000,
    "errors": 0,
    "connection_status": "connected",
    "last_send_time": "2024-01-20T15:30:45Z"
  },
  "test": {
    "type": "stream",
    "status": "running",
    "start_time": "2024-01-20T15:29:45Z",
    "messages_sent": 5000,
    "target_messages": 10000,
    "progress_percent": 50,
    "current_rate": 498.5,
    "average_rate": 500.2
  },
  "active": true,
  "current_test": "stream"
}
```

### Генерация данных

#### `POST /generate`

Генерирует тестовые данные для последующего использования.

**Параметры запроса:**
```json
{
  "type": "all"  // Тип данных: "all", "small", "medium", "large"
}
```

**Типы данных:**
- `small` - маленькие пакеты (~100KB)
- `medium` - средние пакеты (~1MB) 
- `large` - большие пакеты (5-100MB)
- `all` - генерация всех типов

### Метрики

#### `GET /metrics`

Возвращает метрики в формате Prometheus для мониторинга.

## Примеры использования

### Сценарий 1: Тестирование стабильной нагрузки

```bash
# Запуск потокового теста на 5 минут с умеренной нагрузкой
curl -X POST http://localhost:8080/test/stream \
  -H "Content-Type: application/json" \
  -d '{
    "messages_per_sec": 100,
    "packet_size": 4096,
    "duration": 300
  }'

# Мониторинг статистики каждые 10 секунд
while true; do
  curl -s http://localhost:8080/stats | jq '.test.current_rate'
  sleep 10
done
```

### Сценарий 2: Стресс-тестирование

```bash
# Запуск пакетного теста с максимальной нагрузкой
curl -X POST http://localhost:8080/test/batch \
  -H "Content-Type: application/json" \
  -d '{
    "thread_count": 100,
    "packet_size": 10240,
    "total_messages": 100000,
    "duration": 600
  }'
```

### Сценарий 3: Тестирование больших файлов

```bash
# Генерация больших тестовых данных
curl -X POST http://localhost:8080/generate \
  -H "Content-Type: application/json" \
  -d '{"type": "large"}'

# Запуск теста с большими пакетами
curl -X POST http://localhost:8080/test/large \
  -H "Content-Type: application/json" \
  -d '{
    "thread_count": 5,
    "packet_size_mb": 100,
    "duration": 120
  }'
```

## Конфигурация

Основные параметры конфигурации в `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

mqtt:
  broker: "tcp://localhost:1883"
  client_id: "sender-001"
  topic: "test/data"
  qos: 1
  keep_alive: 60
  connect_timeout: 30s
  publish_timeout: 10s

generator:
  data_dir: "./data"
  batch_sizes:
    small: 102400    # 100KB
    medium: 1048576  # 1MB
    large: 10485760  # 10MB

logger:
  level: "info"
  output_path: "./logs/sender.log"
  max_size_mb: 100
  max_backups: 3
  max_age_days: 7
```

## Мониторинг и отладка

### Логи

Логи сохраняются в `./logs/sender.log` и включают:
- Информацию о старте/остановке тестов
- Статистику отправки
- Ошибки подключения к MQTT
- Метрики производительности

### Метрики производительности

При анализе результатов обратите внимание на:
- `current_rate` vs `messages_per_sec` - отклонение от целевой скорости
- `errors` - количество ошибок отправки
- `connection_status` - статус подключения к брокеру

## Требования к ресурсам

### Минимальные требования
- CPU: 2 ядра
- RAM: 512MB
- Сеть: 100 Mbps

### Рекомендуемые для нагрузочного тестирования
- CPU: 4+ ядра
- RAM: 2GB+
- Сеть: 1 Gbps
- SSD для логов и тестовых данных

## Устранение неполадок

### Проблема: Низкая скорость отправки

1. Проверьте нагрузку на CPU: `top` или `htop`
2. Проверьте сетевую пропускную способность: `iftop` или `nethogs`
3. Увеличьте количество потоков в пакетном тесте
4. Проверьте настройки MQTT брокера (max_inflight_messages, max_queued_messages)

### Проблема: Ошибки подключения к MQTT

1. Проверьте доступность брокера: `telnet <broker_host> 1883`
2. Проверьте логи MQTT брокера
3. Увеличьте таймауты подключения в конфигурации

### Проблема: Высокое использование памяти

1. Уменьшите размер пакетов
2. Уменьшите количество потоков
3. Проверьте настройки буферизации MQTT

## Docker

### Сборка образа

```bash
docker build -t infodiode-sender:latest -f docker/Dockerfile.sender .
```

### Запуск контейнера

```bash
docker run -d \
  --name infodiode-sender \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/logs:/app/logs \
  -v $(pwd)/data:/app/data \
  --network infodiode-net \
  infodiode-sender:latest
```

## Связанные компоненты

- [Recipient Service](../recipient/README.md) - сервис приема и валидации данных
- [Shared Models](../shared/README.md) - общие структуры данных
- [Test Data Generator](./internal/generator/README.md) - генератор тестовых данных
