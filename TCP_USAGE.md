# Руководство по использованию TCP протокола

## Обзор

Система InfoDiode поддерживает два протокола передачи данных:
- **MQTT** - передача через брокер сообщений (по умолчанию)
- **TCP** - прямая передача через TCP соединение

TCP протокол обеспечивает более высокую производительность для больших объемов данных и меньшую задержку за счет отсутствия промежуточного брокера.

## Архитектура TCP режима

```
┌─────────────────┐         ┌────────────┐         ┌──────────────────┐
│     Sender      │         │  Инфодиод  │         │    Recipient     │
│                 │         │            │         │                  │
│   TCP Client    │────────►│  Односто-  │────────►│   TCP Server     │
│                 │         │  ронний    │         │                  │
│  (случайный     │         │  канал     │         │  (порт 9999)     │
│    порт)        │         │            │         │                  │
└─────────────────┘         └────────────┘         └──────────────────┘
```

## Конфигурация

### Настройка Sender (TCP клиент)

Отредактируйте файл `sender/config.yaml`:

```yaml
# Настройки TCP клиента
tcp:
  enabled: true                    # Включить поддержку TCP протокола
  address: localhost:9999          # Адрес TCP сервера recipient
  reconnect_interval: 5s           # Интервал между попытками переподключения
  max_retries: 3                   # Максимальное количество попыток
  timeout: 10s                     # Таймаут операций чтения/записи
  keep_alive: true                 # Использовать TCP keep-alive
  keep_alive_period: 30s           # Период отправки keep-alive пакетов
```

### Настройка Recipient (TCP сервер)

Отредактируйте файл `recipient/config.yaml`:

```yaml
# Настройки TCP сервера
tcp:
  enabled: true                    # Включить TCP сервер
  address: :9999                   # Адрес для прослушивания
  max_connections: 100             # Максимальное количество подключений
  read_timeout: 60s                # Таймаут чтения данных
  write_timeout: 60s               # Таймаут записи данных
  keep_alive: true                 # Использовать TCP keep-alive
  keep_alive_period: 30s           # Период keep-alive пакетов
```

## Запуск сервисов

### 1. Запустите Recipient (TCP сервер)
```bash
cd recipient
go run cmd/main.go
```

Убедитесь, что в логах появилось сообщение:
```
TCP сервер запущен address=:9999
```

### 2. Запустите Sender (TCP клиент)
```bash
cd sender
go run cmd/main.go
```

При успешном подключении в логах появится:
```
TCP клиент подключен address=localhost:9999
```

## API запросы для тестирования

### Пакетный тест через TCP
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

### Тест с большими пакетами через TCP
```bash
curl -X POST http://localhost:8080/test/large \
  -H "Content-Type: application/json" \
  -d '{
    "protocol": "tcp",
    "thread_count": 5,
    "packet_size_mb": 50,
    "duration": 120
  }'
```

## Интерактивное тестирование

Используйте интерактивный скрипт для удобного запуска тестов:

```bash
./run_test.sh
```

Скрипт предоставляет меню для:
- Выбора протокола (MQTT или TCP)
- Выбора типа теста (пакетный, потоковый, большие пакеты)
- Сравнительного тестирования MQTT vs TCP
- Настройки пользовательских параметров

## Сравнение производительности MQTT vs TCP

### Автоматическое сравнение
```bash
make test-mqtt-vs-tcp
```

### Ручное сравнение

1. Запустите тест через MQTT:
```bash
curl -X POST http://localhost:8080/test/stream \
  -H "Content-Type: application/json" \
  -d '{
    "protocol": "mqtt",
    "messages_per_sec": 100,
    "packet_size": 102400,
    "duration": 30
  }'
```

2. Дождитесь завершения и получите статистику:
```bash
curl http://localhost:8080/stats
```

3. Остановите тест:
```bash
curl -X POST http://localhost:8080/test/stop
```

4. Запустите тест через TCP с теми же параметрами:
```bash
curl -X POST http://localhost:8080/test/stream \
  -H "Content-Type: application/json" \
  -d '{
    "protocol": "tcp",
    "messages_per_sec": 100,
    "packet_size": 102400,
    "duration": 30
  }'
```

5. Сравните результаты

## Преимущества и недостатки

### TCP протокол

**Преимущества:**
- ✅ Меньшая задержка (отсутствие брокера)
- ✅ Выше пропускная способность для больших данных
- ✅ Прямое соединение точка-точка
- ✅ Меньше накладных расходов на протокол
- ✅ Контроль над буферизацией и потоками

**Недостатки:**
- ❌ Необходимость прямого сетевого доступа
- ❌ Отсутствие встроенной персистентности
- ❌ Нет автоматической маршрутизации
- ❌ Сложнее масштабирование

### MQTT протокол

**Преимущества:**
- ✅ Встроенная персистентность сообщений
- ✅ Гибкая маршрутизация через топики
- ✅ Поддержка QoS уровней
- ✅ Легкое масштабирование подписчиков
- ✅ Работа через брандмауэры

**Недостатки:**
- ❌ Дополнительная задержка из-за брокера
- ❌ Накладные расходы протокола
- ❌ Требует отдельный брокер

## Мониторинг и диагностика

### Проверка статуса всех компонентов
```bash
./check_status.sh
```

### Проверка TCP соединения
```bash
# Проверка доступности TCP порта
nc -zv localhost 9999

# Просмотр активных TCP соединений
netstat -an | grep 9999

# Проверка через API
curl http://localhost:8080/health
curl http://localhost:8081/health
```

### Просмотр логов

Логи sender (TCP клиент):
```bash
tail -f logs/sender.log | grep -i tcp
```

Логи recipient (TCP сервер):
```bash
tail -f logs/recipient.log | grep -i tcp
```

## Рекомендации по выбору протокола

### Используйте TCP когда:
- Требуется минимальная задержка
- Передаются большие объемы данных
- Есть прямой сетевой доступ между sender и recipient
- Важна максимальная пропускная способность
- Нет необходимости в маршрутизации сообщений

### Используйте MQTT когда:
- Требуется гарантированная доставка (QoS)
- Необходима персистентность сообщений
- Нужна гибкая маршрутизация
- Есть несколько получателей данных
- Сеть нестабильная или есть ограничения firewall

## Тестовые сценарии

### 1. Стресс-тест TCP
```bash
# Максимальное количество потоков
curl -X POST http://localhost:8080/test/batch \
  -d '{"protocol":"tcp","thread_count":100,"packet_size":524288,"total_messages":50000,"duration":300}'

# Максимальная частота отправки
curl -X POST http://localhost:8080/test/stream \
  -d '{"protocol":"tcp","messages_per_sec":10000,"packet_size":102400,"duration":60}'

# Максимальный размер пакетов
curl -X POST http://localhost:8080/test/large \
  -d '{"protocol":"tcp","thread_count":5,"packet_size_mb":100,"duration":120}'
```

### 2. Тест надежности
```bash
# Запустите долгий тест
curl -X POST http://localhost:8080/test/stream \
  -d '{"protocol":"tcp","messages_per_sec":100,"packet_size":102400,"duration":3600}'

# В другом терминале периодически проверяйте статистику
watch -n 5 'curl -s http://localhost:8080/stats | jq .'
```

### 3. Тест восстановления соединения
```bash
# Запустите тест
curl -X POST http://localhost:8080/test/stream \
  -d '{"protocol":"tcp","messages_per_sec":10,"packet_size":1024,"duration":300}'

# Перезапустите recipient (TCP сервер) во время теста
# Проверьте, что соединение восстановлено автоматически
```

## Устранение проблем

### TCP сервер не запускается
- Проверьте, что порт 9999 не занят: `lsof -i :9999`
- Проверьте права доступа и firewall
- Убедитесь, что `tcp.enabled: true` в конфигурации

### TCP клиент не подключается
- Проверьте правильность адреса в конфигурации
- Убедитесь, что TCP сервер запущен
- Проверьте сетевую доступность: `telnet localhost 9999`

### Потеря сообщений
- Увеличьте таймауты в конфигурации
- Проверьте размер буферов TCP
- Мониторьте сетевые ошибки: `netstat -s | grep -i error`

### Низкая производительность
- Увеличьте количество потоков для пакетного теста
- Оптимизируйте размер пакетов
- Проверьте загрузку CPU и сети
- Рассмотрите использование TCP_NODELAY для малых сообщений

## Дополнительные команды Makefile

```bash
# Проверка TCP соединения
make test-tcp-connection

# Запуск пакетного теста через TCP
make test-tcp-batch

# Запуск потокового теста через TCP
make test-tcp-stream

# Запуск теста с большими пакетами через TCP
make test-tcp-large

# Сравнительный тест MQTT vs TCP
make test-mqtt-vs-tcp

# Интерактивное меню тестирования
make run-test-menu
```

## Метрики производительности

При тестировании обращайте внимание на следующие метрики:

- **Throughput (пропускная способность)**: сообщений/сек или МБ/сек
- **Latency (задержка)**: min/avg/max/p50/p95/p99 в миллисекундах
- **Error rate (частота ошибок)**: процент потерянных сообщений
- **CPU usage (загрузка процессора)**: для sender и recipient
- **Memory usage (использование памяти)**: особенно при больших пакетах
- **Network I/O**: входящий/исходящий трафик

Доступ к метрикам:
- Sender metrics: http://localhost:8080/metrics
- Recipient metrics: http://localhost:8081/metrics
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)