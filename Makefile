# Makefile для проекта infodiode_test

# Переменные
SENDER_DIR = sender
RECIPIENT_DIR = recipient
SHARED_DIR = shared
DATA_DIR = data

# Цвета для вывода
GREEN = \033[0;32m
YELLOW = \033[0;33m
RED = \033[0;31m
NC = \033[0m # No Color

# Go команды
GOCMD = go
GOBUILD = $(GOCMD) build
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
GOMOD = $(GOCMD) mod
GOFMT = $(GOCMD) fmt
GOVET = $(GOCMD) vet

# Бинарные файлы
SENDER_BINARY = $(SENDER_DIR)/bin/sender
RECIPIENT_BINARY = $(RECIPIENT_DIR)/bin/recipient
GENERATOR_BINARY = $(SENDER_DIR)/bin/generator

# Версия
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date +%Y%m%d-%H%M%S)
LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: all build clean test run-sender run-recipient run-all docker-build docker-run help

# Основные цели
all: deps build ## Установить зависимости и собрать все компоненты

help: ## Показать справку
	@echo "$(GREEN)Доступные команды:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2}'

# Сборка
build: build-sender build-recipient ## Собрать все сервисы

build-sender: ## Собрать sender сервис
	@echo "$(GREEN)Сборка sender...$(NC)"
	@mkdir -p $(SENDER_DIR)/bin
	@cd $(SENDER_DIR) && $(GOBUILD) $(LDFLAGS) -o bin/sender cmd/main.go
	@echo "$(GREEN)✓ Sender собран$(NC)"

build-recipient: ## Собрать recipient сервис
	@echo "$(GREEN)Сборка recipient...$(NC)"
	@mkdir -p $(RECIPIENT_DIR)/bin
	@cd $(RECIPIENT_DIR) && $(GOBUILD) $(LDFLAGS) -o bin/recipient cmd/main.go
	@echo "$(GREEN)✓ Recipient собран$(NC)"

build-generator: ## Собрать генератор данных
	@echo "$(GREEN)Сборка генератора данных...$(NC)"
	@mkdir -p $(SENDER_DIR)/bin
	@cd $(SENDER_DIR) && $(GOBUILD) $(LDFLAGS) -o bin/generator cmd/generator/main.go
	@echo "$(GREEN)✓ Генератор собран$(NC)"

# Запуск
run-sender: build-sender ## Запустить sender сервис
	@echo "$(GREEN)Запуск sender...$(NC)"
	@cd $(SENDER_DIR) && ./bin/sender -config config.yaml

run-recipient: build-recipient ## Запустить recipient сервис
	@echo "$(GREEN)Запуск recipient...$(NC)"
	@cd $(RECIPIENT_DIR) && ./bin/recipient -config config.yaml

run-all: ## Запустить все сервисы в отдельных терминалах (требует tmux)
	@if command -v tmux >/dev/null 2>&1; then \
		tmux new-session -d -s infodiode; \
		tmux send-keys -t infodiode "make run-mqtt" C-m; \
		tmux split-window -h -t infodiode; \
		tmux send-keys -t infodiode "sleep 10 && make run-recipient" C-m; \
		tmux split-window -v -t infodiode; \
		tmux send-keys -t infodiode "sleep 10 && make run-sender" C-m; \
		tmux attach -t infodiode; \
	else \
		echo "$(RED)Требуется tmux для запуска всех сервисов$(NC)"; \
		echo "Установите tmux или запустите сервисы в отдельных терминалах"; \
	fi

# Генерация данных
generate-data: build-generator ## Сгенерировать все тестовые данные
	@echo "$(GREEN)Генерация тестовых данных...$(NC)"
	@mkdir -p $(DATA_DIR)/{small,medium,large}
	@cd $(SENDER_DIR) && ./bin/generator -config config.yaml -all
	@echo "$(GREEN)✓ Данные сгенерированы$(NC)"

generate-data-small: build-generator ## Сгенерировать маленькие пакеты данных
	@echo "$(GREEN)Генерация маленьких пакетов...$(NC)"
	@mkdir -p $(DATA_DIR)/small
	@cd $(SENDER_DIR) && ./bin/generator -config config.yaml -size small
	@echo "$(GREEN)✓ Маленькие пакеты сгенерированы$(NC)"

generate-data-medium: build-generator ## Сгенерировать средние пакеты данных
	@echo "$(GREEN)Генерация средних пакетов...$(NC)"
	@mkdir -p $(DATA_DIR)/medium
	@cd $(SENDER_DIR) && ./bin/generator -config config.yaml -size medium
	@echo "$(GREEN)✓ Средние пакеты сгенерированы$(NC)"

generate-data-large: build-generator ## Сгенерировать большие пакеты данных
	@echo "$(GREEN)Генерация больших пакетов...$(NC)"
	@mkdir -p $(DATA_DIR)/large
	@cd $(SENDER_DIR) && ./bin/generator -config config.yaml -size large
	@echo "$(GREEN)✓ Большие пакеты сгенерированы$(NC)"

clean-data: ## Очистить сгенерированные данные
	@echo "$(YELLOW)Очистка данных...$(NC)"
	@rm -rf $(DATA_DIR)/*
	@echo "$(GREEN)✓ Данные очищены$(NC)"

# Зависимости
deps: ## Установить зависимости
	@echo "$(GREEN)Установка зависимостей...$(NC)"
	@cd $(SHARED_DIR) && $(GOMOD) tidy
	@cd $(SENDER_DIR) && $(GOMOD) tidy
	@cd $(RECIPIENT_DIR) && $(GOMOD) tidy
	@echo "$(GREEN)✓ Зависимости установлены$(NC)"

# Тестирование
test: test-shared test-sender test-recipient ## Запустить все тесты

test-shared: ## Запустить тесты shared модуля
	@echo "$(GREEN)Тестирование shared...$(NC)"
	@cd $(SHARED_DIR) && $(GOTEST) -v -cover ./...

test-sender: ## Запустить тесты sender
	@echo "$(GREEN)Тестирование sender...$(NC)"
	@cd $(SENDER_DIR) && $(GOTEST) -v -cover ./...

test-recipient: ## Запустить тесты recipient
	@echo "$(GREEN)Тестирование recipient...$(NC)"
	@cd $(RECIPIENT_DIR) && $(GOTEST) -v -cover ./...

test-integration: ## Запустить интеграционные тесты
	@echo "$(GREEN)Запуск интеграционных тестов...$(NC)"
	@./scripts/integration_test.sh

# Проверка кода
lint: ## Запустить линтеры
	@echo "$(GREEN)Проверка кода...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint не установлен. Установите: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)"; \
	fi

fmt: ## Форматировать код
	@echo "$(GREEN)Форматирование кода...$(NC)"
	@$(GOFMT) ./...
	@echo "$(GREEN)✓ Код отформатирован$(NC)"

vet: ## Запустить go vet
	@echo "$(GREEN)Запуск go vet...$(NC)"
	@cd $(SENDER_DIR) && $(GOVET) ./...
	@cd $(RECIPIENT_DIR) && $(GOVET) ./...
	@cd $(SHARED_DIR) && $(GOVET) ./...
	@echo "$(GREEN)✓ Проверка завершена$(NC)"

# Docker
docker-build: ## Собрать Docker образы
	@echo "$(GREEN)Сборка Docker образов...$(NC)"
	@docker-compose build
	@echo "$(GREEN)✓ Docker образы собраны$(NC)"

docker-run: ## Запустить все сервисы в Docker (с профилем full)
	@echo "$(GREEN)Запуск Docker контейнеров...$(NC)"
	@docker-compose --profile full up -d
	@echo "$(GREEN)✓ Контейнеры запущены$(NC)"

docker-run-base: ## Запустить только базовые сервисы (mosquitto)
	@echo "$(GREEN)Запуск базовых контейнеров...$(NC)"
	@docker-compose up -d
	@echo "$(GREEN)✓ Базовые контейнеры запущены$(NC)"

docker-run-monitoring: ## Запустить с мониторингом (full + monitoring профили)
	@echo "$(GREEN)Запуск контейнеров с мониторингом...$(NC)"
	@docker-compose --profile full --profile monitoring up -d
	@echo "$(GREEN)✓ Контейнеры с мониторингом запущены$(NC)"

docker-stop: ## Остановить Docker контейнеры
	@echo "$(YELLOW)Остановка Docker контейнеров...$(NC)"
	@docker-compose down
	@echo "$(GREEN)✓ Контейнеры остановлены$(NC)"

docker-logs: ## Показать логи Docker контейнеров
	@docker-compose logs -f

# MQTT
run-mqtt: ## Запустить MQTT брокер (Mosquitto) в Docker
	@echo "$(GREEN)Запуск MQTT брокера...$(NC)"
	@docker-compose up -d mosquitto
	@echo "$(GREEN)✓ MQTT брокер запущен$(NC)"
	@echo "$(YELLOW)Ожидание готовности MQTT...$(NC)"
	@sleep 5
	@echo "$(GREEN)✓ MQTT брокер готов к работе$(NC)"

stop-mqtt: ## Остановить MQTT брокер
	@echo "$(YELLOW)Остановка MQTT брокера...$(NC)"
	@docker-compose stop mosquitto
	@echo "$(GREEN)✓ MQTT брокер остановлен$(NC)"

# Очистка
clean: ## Очистить бинарные файлы и временные файлы
	@echo "$(YELLOW)Очистка...$(NC)"
	@rm -rf $(SENDER_DIR)/bin
	@rm -rf $(RECIPIENT_DIR)/bin
	@rm -rf logs/
	@rm -rf /tmp/mqtt-*-store
	@find . -name "*.log" -delete
	@find . -name "*.test" -delete
	@find . -name "*.out" -delete
	@echo "$(GREEN)✓ Очистка завершена$(NC)"

clean-all: clean clean-data ## Полная очистка включая данные
	@echo "$(GREEN)✓ Полная очистка завершена$(NC)"

# Мониторинг
monitor-sender: ## Мониторинг метрик sender
	@echo "$(GREEN)Открытие метрик sender...$(NC)"
	@open http://localhost:8080/metrics || xdg-open http://localhost:8080/metrics

monitor-recipient: ## Мониторинг метрик recipient
	@echo "$(GREEN)Открытие метрик recipient...$(NC)"
	@open http://localhost:8081/metrics || xdg-open http://localhost:8081/metrics

monitor-prometheus: ## Открыть интерфейс Prometheus
	@echo "$(GREEN)Открытие Prometheus...$(NC)"
	@open http://localhost:9090 || xdg-open http://localhost:9090

monitor-grafana: ## Открыть интерфейс Grafana
	@echo "$(GREEN)Открытие Grafana...$(NC)"
	@echo "$(YELLOW)Логин: admin, Пароль: admin$(NC)"
	@open http://localhost:3000 || xdg-open http://localhost:3000

monitor-all: ## Открыть все интерфейсы мониторинга
	@echo "$(GREEN)Открытие всех интерфейсов мониторинга...$(NC)"
	@open http://localhost:8080/metrics || xdg-open http://localhost:8080/metrics
	@open http://localhost:8081/metrics || xdg-open http://localhost:8081/metrics
	@open http://localhost:9090 || xdg-open http://localhost:9090
	@open http://localhost:3000 || xdg-open http://localhost:3000
	@echo "$(YELLOW)Grafana логин: admin, пароль: admin$(NC)"

health-check: ## Проверка состояния сервисов
	@echo "$(GREEN)Проверка состояния сервисов...$(NC)"
	@curl -s http://localhost:8080/health | jq . || echo "$(RED)Sender недоступен$(NC)"
	@curl -s http://localhost:8081/health | jq . || echo "$(RED)Recipient недоступен$(NC)"

# Benchmark
benchmark: ## Запустить бенчмарки
	@echo "$(GREEN)Запуск бенчмарков...$(NC)"
	@cd $(SENDER_DIR) && $(GOTEST) -bench=. -benchmem ./...
	@cd $(RECIPIENT_DIR) && $(GOTEST) -bench=. -benchmem ./...

# Утилиты
logs: ## Показать логи всех сервисов
	@tail -f logs/*.log

mqtt-logs: ## Показать логи MQTT брокера
	@docker-compose logs -f mosquitto

prometheus-logs: ## Показать логи Prometheus
	@docker-compose logs -f prometheus

grafana-logs: ## Показать логи Grafana
	@docker-compose logs -f grafana

monitoring-logs: ## Показать логи всех компонентов мониторинга
	@docker-compose logs -f prometheus grafana

version: ## Показать версию
	@echo "$(GREEN)Версия: $(VERSION)$(NC)"

# Тестирование системы
test-system: ## Быстрое тестирование всей системы
	@echo "$(GREEN)Тестирование системы InfoDiode...$(NC)"
	@echo "$(YELLOW)1. Проверка статуса сервисов...$(NC)"
	@make health-check
	@echo "$(YELLOW)2. Проверка метрик Prometheus...$(NC)"
	@curl -s "http://localhost:9090/api/v1/query?query=up" | jq '.data.result[] | {job: .metric.job, value: .value[1]}'
	@echo "$(YELLOW)3. Тест отправки сообщения...$(NC)"
	@curl -s -X POST http://localhost:8080/test/batch -H "Content-Type: application/json" -d '{"threads": 1, "duration": "5s", "size": "small"}' | jq .
	@echo "$(GREEN)✓ Тестирование завершено$(NC)"

test-quick: ## Быстрая проверка работоспособности
	@echo "$(GREEN)Быстрая проверка системы...$(NC)"
	@docker ps --format "table {{.Names}}\t{{.Status}}" | grep -E "(mqtt-broker|sender-service|recipient-service|prometheus|grafana)" || echo "$(RED)Не все контейнеры запущены$(NC)"
	@curl -s http://localhost:8080/health > /dev/null && echo "$(GREEN)✓ Sender работает$(NC)" || echo "$(RED)✗ Sender недоступен$(NC)"
	@curl -s http://localhost:8081/health > /dev/null && echo "$(GREEN)✓ Recipient работает$(NC)" || echo "$(RED)✗ Recipient недоступен$(NC)"
	@curl -s http://localhost:9090/-/healthy > /dev/null && echo "$(GREEN)✓ Prometheus работает$(NC)" || echo "$(RED)✗ Prometheus недоступен$(NC)"
	@curl -s http://localhost:3000/api/health > /dev/null && echo "$(GREEN)✓ Grafana работает$(NC)" || echo "$(RED)✗ Grafana недоступен$(NC)"

check-datasources: ## Проверить датасорсы Grafana
	@echo "$(GREEN)Проверка датасорсов Grafana...$(NC)"
	@curl -s -u admin:admin http://localhost:3000/api/datasources | jq '.[] | {name: .name, type: .type, url: .url, uid: .uid}' || echo "$(RED)Ошибка проверки датасорсов$(NC)"

test-grafana-connection: ## Тест подключения к Prometheus через Grafana
	@echo "$(GREEN)Тестирование подключения Grafana -> Prometheus...$(NC)"
	@curl -s -u admin:admin -X POST http://localhost:3000/api/datasources/proxy/1/api/v1/query -H "Content-Type: application/x-www-form-urlencoded" -d "query=up" | jq '.data.result[] | {job: .metric.job, value: .value[1]}' || echo "$(RED)Ошибка подключения$(NC)"

demo: ## Демонстрация всей системы InfoDiode
	@echo "$(GREEN) Демонстрация системы InfoDiode$(NC)"
	@echo "$(YELLOW)════════════════════════════════════════$(NC)"
	@echo "$(YELLOW)1. Проверка статуса всех сервисов...$(NC)"
	@make test-quick
	@echo ""
	@echo "$(YELLOW)2. Проверка подключения Grafana -> Prometheus...$(NC)"
	@make test-grafana-connection
	@echo ""
	@echo "$(YELLOW)3. Открытие интерфейсов мониторинга...$(NC)"
	@echo "$(GREEN) Открываем Grafana (логин: admin, пароль: admin)$(NC)"
	@make monitor-grafana
	@echo "$(GREEN) Открываем Prometheus$(NC)"
	@make monitor-prometheus
	@echo ""
	@echo "$(YELLOW)4. Доступные интерфейсы:$(NC)"
	@echo "$(GREEN)   • Sender API:      http://localhost:8080$(NC)"
	@echo "$(GREEN)   • Recipient API:   http://localhost:8081$(NC)"
	@echo "$(GREEN)   • Prometheus:      http://localhost:9090$(NC)"
	@echo "$(GREEN)   • Grafana:         http://localhost:3000$(NC)"
	@echo "$(GREEN)   • MQTT Broker:     mqtt://localhost:1883$(NC)"
	@echo ""
	@echo "$(YELLOW)5. Примеры использования:$(NC)"
	@echo "$(GREEN)   make health-check     - проверка состояния$(NC)"
	@echo "$(GREEN)   make test-system      - полное тестирование$(NC)"
	@echo "$(GREEN)   make monitor-all      - открыть все интерфейсы$(NC)"
	@echo "$(GREEN)   make docker-logs      - просмотр логов$(NC)"
	@echo "$(YELLOW)════════════════════════════════════════$(NC)"
	@echo "$(GREEN)✓ Система InfoDiode готова к использованию!$(NC)"

# TCP тестирование
test-tcp-connection: ## Проверить TCP соединение между sender и recipient
	@echo "$(GREEN)Проверка TCP соединения...$(NC)"
	@nc -zv localhost 9999 2>&1 | grep -q succeeded && echo "$(GREEN)✓ TCP порт 9999 доступен$(NC)" || echo "$(RED)✗ TCP порт 9999 недоступен$(NC)"

test-tcp-batch: ## Запустить пакетный тест через TCP
	@echo "$(GREEN)Запуск пакетного теста через TCP...$(NC)"
	@curl -s -X POST http://localhost:8080/test/batch \
		-H "Content-Type: application/json" \
		-d '{"protocol": "tcp", "thread_count": 10, "packet_size": 1048576, "total_messages": 100, "duration": 30}' | jq .

test-tcp-stream: ## Запустить потоковый тест через TCP
	@echo "$(GREEN)Запуск потокового теста через TCP...$(NC)"
	@curl -s -X POST http://localhost:8080/test/stream \
		-H "Content-Type: application/json" \
		-d '{"protocol": "tcp", "messages_per_sec": 1000, "packet_size": 102400, "duration": 30}' | jq .

test-tcp-large: ## Запустить тест с большими пакетами через TCP
	@echo "$(GREEN)Запуск теста с большими пакетами через TCP...$(NC)"
	@curl -s -X POST http://localhost:8080/test/large \
		-H "Content-Type: application/json" \
		-d '{"protocol": "tcp", "thread_count": 5, "packet_size_mb": 10, "duration": 60}' | jq .

test-mqtt-vs-tcp: ## Сравнительный тест MQTT vs TCP
	@echo "$(GREEN)Сравнительный тест MQTT vs TCP$(NC)"
	@echo "$(YELLOW)════════════════════════════════════════$(NC)"
	@echo "$(YELLOW)1. Тест MQTT (30 секунд)...$(NC)"
	@curl -s -X POST http://localhost:8080/test/stream \
		-H "Content-Type: application/json" \
		-d '{"protocol": "mqtt", "messages_per_sec": 100, "packet_size": 102400, "duration": 30}' | jq .
	@sleep 35
	@echo "$(YELLOW)Статистика MQTT:$(NC)"
	@curl -s http://localhost:8080/stats | jq .
	@curl -s -X POST http://localhost:8080/test/stop > /dev/null
	@sleep 5
	@echo ""
	@echo "$(YELLOW)2. Тест TCP (30 секунд)...$(NC)"
	@curl -s -X POST http://localhost:8080/test/stream \
		-H "Content-Type: application/json" \
		-d '{"protocol": "tcp", "messages_per_sec": 100, "packet_size": 102400, "duration": 30}' | jq .
	@sleep 35
	@echo "$(YELLOW)Статистика TCP:$(NC)"
	@curl -s http://localhost:8080/stats | jq .
	@curl -s -X POST http://localhost:8080/test/stop > /dev/null
	@echo "$(YELLOW)════════════════════════════════════════$(NC)"
	@echo "$(GREEN)✓ Сравнительный тест завершен$(NC)"

run-test-menu: ## Запустить интерактивное меню тестирования
	@echo "$(GREEN)Запуск интерактивного меню тестирования...$(NC)"
	@./run_test.sh

# Дополнительные команды для работы с разными машинами
deploy-sender: build-sender ## Развернуть sender на машине-источнике
	@echo "$(GREEN)Развертывание sender...$(NC)"
	@echo "$(YELLOW)Копирование бинарных файлов и конфигурации...$(NC)"
	@mkdir -p deploy/sender
	@cp $(SENDER_DIR)/bin/sender deploy/sender/
	@cp $(SENDER_DIR)/config.yaml deploy/sender/
	@cp -r $(DATA_DIR) deploy/sender/ 2>/dev/null || true
	@echo "$(GREEN)✓ Sender готов к развертыванию из deploy/sender$(NC)"

deploy-recipient: build-recipient ## Развернуть recipient на машине-приемнике
	@echo "$(GREEN)Развертывание recipient...$(NC)"
	@echo "$(YELLOW)Копирование бинарных файлов и конфигурации...$(NC)"
	@mkdir -p deploy/recipient
	@cp $(RECIPIENT_DIR)/bin/recipient deploy/recipient/
	@cp $(RECIPIENT_DIR)/config.yaml deploy/recipient/
	@echo "$(GREEN)✓ Recipient готов к развертыванию из deploy/recipient$(NC)"

.DEFAULT_GOAL := help
