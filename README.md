# InfoDiode Test Project

Проект для тестирования производительности передачи данных через однонаправленный инфодиод с использованием MQTT и TCP протоколов.

## Архитектура

Система состоит из двух независимых сервисов:
- **Sender**: генерирует и отправляет тестовые данные через MQTT брокер или TCP соединение
- **Recipient**: получает данные через MQTT брокер или TCP сервер и проверяет их целостность

Между машинами установлен однонаправленный инфодиод, обеспечивающий безопасную передачу данных только в одном направлении.

### Режим MQTT
```
[Машина-источник]     [Инфодиод]     [Машина-приемник]
     Sender         →  односторонний →    Recipient
        ↓                                      ↑
   MQTT Publisher                      MQTT Subscriber
        ↓                                      ↑
   Mosquitto #1     →    MQTT      →    Mosquitto #2
```

### Режим TCP
```
[Машина-источник]     [Инфодиод]     [Машина-приемник]
     Sender         →  односторонний →    Recipient
        ↓                                      ↑
   TCP Client                           TCP Server
        ↓                                      ↑
   Port: random     →     TCP      →    Port: 9999
```

## Требования

- Go 1.21+
- Docker и Docker Compose
- Make

## Быстрый старт

### 1. Клонирование репозитория
```bash
git clone <repository-url>
cd infodiode_test
```

### 2. Установка зависимостей
```bash
make deps
```

### 3. Генерация тестовых данных
```bash
make generate-data
```

### 4. Запуск локального тестирования
```bash
# Запуск MQTT брокера
make run-mqtt

# В отдельном терминале - запуск recipient
make run-recipient

# В отдельном терминале - запуск sender
make run-sender
```

## Развертывание на разных машинах

### На машине-источнике (Sender)
```bash
# Сборка и развертывание sender
make deploy-sender
docker-compose -f docker-compose.sender.yml up -d
```

### На машине-приемнике (Recipient)
```bash
# Сборка и развертывание recipient
make deploy-recipient
docker-compose -f docker-compose.recipient.yml up -d
```

## Конфигурация

### Sender (sender/config.yaml)
- MQTT брокер и параметры подключения
- Настройки генератора данных
- HTTP API порт (8080)
- Параметры логирования

### Recipient (recipient/config.yaml)
- MQTT брокер и параметры подключения
- Настройки обработки сообщений
- Порт метрик (8081)
- Параметры логирования

## HTTP API

### Sender API (порт 8080)

#### Управление тестами
- `POST /test/batch` - запуск пакетного теста
- `POST /test/stream` - запуск потокового теста с постоянной скоростью отправки
- `POST /test/large` - запуск теста с большими пакетами
- `POST /test/stop` - остановка текущего теста

#### Мониторинг
- `GET /health` - проверка здоровья сервиса
- `GET /ready` - проверка готовности
- `GET /stats` - статистика текущего теста
- `GET /metrics` - метрики Prometheus

### Recipient API (порт 8081)
- `GET /health` - проверка здоровья сервиса
- `GET /ready` - проверка готовности
- `GET /stats` - подробная статистика обработки с метриками целостности данных
- `GET /metrics` - метрики Prometheus

Подробная документация по интерпретации результатов доступна в [Recipient README](recipient/README.md).

## Примеры запуска тестов

### Выбор протокола передачи

Все тесты поддерживают выбор протокола через параметр `protocol`:
- `"mqtt"` - передача через MQTT брокер (по умолчанию)
- `"tcp"` - прямая передача через TCP соединение

### Пакетная отправка через MQTT
```bash
curl -X POST http://localhost:8080/test/batch \
  -H "Content-Type: application/json" \
  -d '{
    "protocol": "mqtt",
    "thread_count": 50,
    "packet_size": 1048576,
    "total_messages": 10000,
    "duration": 60
  }'
```

### Пакетная отправка через TCP
```bash
curl -X POST http://localhost:8080/test/batch \
  -H "Content-Type: application/json" \
  -d '{
    "protocol": "tcp",
    "thread_count": 50,
    "packet_size": 1048576,
    "total_messages": 10000,
    "duration": 60
  }'
```

### Потоковый тест через TCP
```bash
curl -X POST http://localhost:8080/test/stream \
  -H "Content-Type: application/json" \
  -d '{
    "protocol": "tcp",
    "messages_per_sec": 1000,
    "packet_size": 102400,
    "duration": 60
  }'
```

### Интерактивный скрипт для тестирования
```bash
# Запуск интерактивного меню для выбора протокола и параметров теста
./run_test.sh
```

### Потоковая отправка
Тест с постоянной скоростью отправки сообщений. Идеально подходит для тестирования стабильной нагрузки и измерения пропускной способности при равномерном потоке данных.

```bash
curl -X POST http://localhost:8080/test/stream \
  -H "Content-Type: application/json" \
  -d '{
    "messages_per_sec": 1000,  # Целевая скорость (1-100000 msg/sec)
    "packet_size": 100,         # Размер пакета в байтах (мин. 100)
    "duration": 60              # Длительность теста в секундах
  }'
```

**Особенности:**
- Равномерная нагрузка на канал передачи
- Предсказуемое использование пропускной способности
- Подходит для длительного тестирования стабильности
- Точное измерение максимальной устойчивой пропускной способности

### Большие пакеты
```bash
curl -X POST http://localhost:8080/test/large \
  -H "Content-Type: application/json" \
  -d '{
    "thread_count": 10,
    "packet_size_mb": 50,
    "duration": 60
  }'
```

## Мониторинг

### Просмотр статистики
```bash
# Статистика sender
curl http://localhost:8080/stats | jq

# Статистика recipient
curl http://localhost:8081/stats | jq
```

### Логи
```bash
# Просмотр логов
tail -f logs/sender.log
tail -f logs/recipient.log

# Логи Docker контейнеров
docker-compose logs -f
```

### Метрики Prometheus
Метрики доступны по адресам:
- Sender: http://localhost:8080/metrics
- Recipient: http://localhost:8081/metrics

## Структура проекта

```
infodiode_test/
├── sender/                 # Сервис отправки
│   ├── cmd/               # Точка входа
│   ├── internal/          # Внутренние пакеты
│   │   ├── api/          # HTTP API
│   │   ├── broker/       # MQTT клиент
│   │   ├── generator/    # Генератор данных
│   │   ├── logger/       # Логирование
│   │   ├── metrics/      # Метрики
│   │   ├── storage/      # Хранение данных
│   │   └── test/         # Менеджер тестов
│   ├── config/           # Конфигурация
│   └── config.yaml       # Файл конфигурации
├── recipient/              # Сервис приема
│   ├── cmd/              # Точка входа
│   ├── internal/         # Внутренние пакеты
│   │   ├── broker/       # MQTT клиент
│   │   ├── processor/    # Обработчик сообщений
│   │   ├── validator/    # Валидатор контрольной суммы
│   │   └── logger/       # Логирование
│   ├── config/           # Конфигурация
│   └── config.yaml       # Файл конфигурации
├── shared/                 # Общие компоненты
│   ├── models/           # Модели данных
│   └── utils/            # Утилиты
├── data/                   # Тестовые данные
│   ├── small/            # Маленькие пакеты (~100KB)
│   ├── medium/           # Средние пакеты (~1MB)
│   └── large/            # Большие пакеты (5-100MB)
├── mosquitto/              # Конфигурация MQTT брокера
├── docker-compose.yml      # Основной Docker Compose
├── docker-compose.sender.yml    # Docker Compose для sender
├── docker-compose.recipient.yml # Docker Compose для recipient
├── Makefile               # Автоматизация сборки
└── README.md              # Этот файл
```

## Формат сообщений

### Сообщение в MQTT
```json
{
    "send_time": "2024-01-20T15:30:45.123Z",
    "message_id": 12345,
    "timestamp": "2024-01-20T15:30:45.123Z",
    "payload": "{\"id\":1,\"timestamp\":\"...\",\"indicator_id\":100,\"indicator_value\":\"...\",\"equipment_id\":10}",
    "checksum": "sha256_hex_string"
}
```

### Данные в payload
```json
{
    "id": 1,
    "timestamp": "2024-01-20T15:30:45.123Z",
    "indicator_id": 100,
    "indicator_value": "value_15_chars",
    "equipment_id": 10
}
```

## Полезные команды

```bash
# Сборка проекта
make build

# Запуск тестов
make test

# Очистка
make clean

# Полная очистка включая данные
make clean-all

# Проверка кода
make lint

# Форматирование кода
make fmt

# Бенчмарки
make benchmark

# Проверка здоровья сервисов
make health-check

# Показать версию
make version
```

## Переменные окружения

См. файл `.env` для полного списка переменных окружения.

Основные переменные:
- `SENDER_MQTT_BROKER` - адрес MQTT брокера для sender
- `RECIPIENT_MQTT_BROKER` - адрес MQTT брокера для recipient
- `SENDER_MQTT_TOPIC` - топик для публикации
- `RECIPIENT_MQTT_TOPIC` - топик для подписки
- `LOG_LEVEL` - уровень логирования (debug, info, warn, error)

## Лицензия

MIT

## Поддержка

При возникновении проблем создайте issue в репозитории проекта.