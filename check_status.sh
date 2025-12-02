#!/bin/bash

# Скрипт проверки статуса всех компонентов системы InfoDiode

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     Проверка статуса системы InfoDiode        ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}"
echo ""

# Функция проверки доступности сервиса
check_service() {
    local name=$1
    local url=$2
    local expected_code=${3:-200}

    if curl -s -o /dev/null -w "%{http_code}" "$url" | grep -q "$expected_code"; then
        echo -e "${GREEN}✓${NC} $name: ${GREEN}Работает${NC}"
        return 0
    else
        echo -e "${RED}✗${NC} $name: ${RED}Недоступен${NC}"
        return 1
    fi
}

# Функция проверки TCP порта
check_tcp_port() {
    local name=$1
    local host=$2
    local port=$3

    if nc -z -w1 "$host" "$port" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} $name (TCP $host:$port): ${GREEN}Открыт${NC}"
        return 0
    else
        echo -e "${RED}✗${NC} $name (TCP $host:$port): ${RED}Закрыт${NC}"
        return 1
    fi
}

# Функция проверки Docker контейнера
check_docker_container() {
    local name=$1
    local container=$2

    if docker ps --format "{{.Names}}" 2>/dev/null | grep -q "^$container$"; then
        echo -e "${GREEN}✓${NC} $name: ${GREEN}Запущен${NC}"
        return 0
    else
        echo -e "${RED}✗${NC} $name: ${RED}Не запущен${NC}"
        return 1
    fi
}

# Проверка Docker контейнеров
echo -e "${YELLOW}1. Docker контейнеры:${NC}"
check_docker_container "MQTT Broker" "mqtt-broker" || true
check_docker_container "Prometheus" "prometheus" || true
check_docker_container "Grafana" "grafana" || true
echo ""

# Проверка основных сервисов
echo -e "${YELLOW}2. Основные сервисы:${NC}"
check_service "Sender API" "http://localhost:8080/health"
check_service "Sender Metrics" "http://localhost:8080/metrics"
check_service "Recipient API" "http://localhost:8081/health"
check_service "Recipient Metrics" "http://localhost:8081/metrics"
echo ""

# Проверка TCP портов
echo -e "${YELLOW}3. TCP порты:${NC}"
check_tcp_port "TCP Server (Recipient)" "localhost" "9999"
check_tcp_port "MQTT Broker" "localhost" "1883"
check_tcp_port "Prometheus" "localhost" "9090"
check_tcp_port "Grafana" "localhost" "3000"
echo ""

# Проверка мониторинга
echo -e "${YELLOW}4. Системы мониторинга:${NC}"
check_service "Prometheus" "http://localhost:9090/-/healthy"
check_service "Grafana" "http://localhost:3000/api/health"
echo ""

# Проверка подключения к MQTT
echo -e "${YELLOW}5. Проверка MQTT брокера:${NC}"
if command -v mqtt-broker &> /dev/null; then
    timeout 2 mqtt-broker -h localhost -p 1883 -t "test/ping" -C 1 2>/dev/null
    if [ $? -eq 124 ]; then
        echo -e "${GREEN}✓${NC} MQTT Broker: ${GREEN}Доступен для подключения${NC}"
    else
        echo -e "${YELLOW}⚠${NC} MQTT Broker: ${YELLOW}Проверьте настройки${NC}"
    fi
else
    echo -e "${YELLOW}⚠${NC} mqtt-broker не установлен, пропуск проверки MQTT"
fi
echo ""

# Проверка конфигурации TCP
echo -e "${YELLOW}6. Конфигурация TCP:${NC}"
if [ -f "sender/config.yaml" ]; then
    tcp_enabled=$(grep -A 1 "^tcp:" sender/config.yaml | grep "enabled:" | awk '{print $2}')
    if [ "$tcp_enabled" = "true" ]; then
        echo -e "${GREEN}✓${NC} TCP в Sender: ${GREEN}Включен${NC}"
        tcp_address=$(grep -A 2 "^tcp:" sender/config.yaml | grep "address:" | awk '{print $2}')
        echo -e "  Адрес сервера: ${BLUE}$tcp_address${NC}"
    else
        echo -e "${YELLOW}⚠${NC} TCP в Sender: ${YELLOW}Выключен${NC}"
    fi
fi

if [ -f "recipient/config.yaml" ]; then
    tcp_enabled=$(grep -A 1 "^tcp:" recipient/config.yaml | grep "enabled:" | awk '{print $2}')
    if [ "$tcp_enabled" = "true" ]; then
        echo -e "${GREEN}✓${NC} TCP в Recipient: ${GREEN}Включен${NC}"
        tcp_address=$(grep -A 2 "^tcp:" recipient/config.yaml | grep "address:" | awk '{print $2}')
        echo -e "  Адрес прослушивания: ${BLUE}$tcp_address${NC}"
    else
        echo -e "${YELLOW}⚠${NC} TCP в Recipient: ${YELLOW}Выключен${NC}"
    fi
fi
echo ""

# Проверка логов
echo -e "${YELLOW}7. Проверка логов:${NC}"
if [ -d "logs" ]; then
    sender_log=$(find logs -name "sender*.log" -type f 2>/dev/null | head -1)
    recipient_log=$(find logs -name "recipient*.log" -type f 2>/dev/null | head -1)

    if [ -n "$sender_log" ]; then
        last_sender_error=$(grep -i "error" "$sender_log" | tail -1)
        if [ -n "$last_sender_error" ]; then
            echo -e "${YELLOW}⚠${NC} Последняя ошибка Sender: ${YELLOW}Найдены ошибки в логах${NC}"
            echo "  $last_sender_error" | head -c 1000
            echo "..."
        else
            echo -e "${GREEN}✓${NC} Логи Sender: ${GREEN}Без ошибок${NC}"
        fi
    fi

    if [ -n "$recipient_log" ]; then
        last_recipient_error=$(grep -i "error" "$recipient_log" | tail -1)
        if [ -n "$last_recipient_error" ]; then
            echo -e "${YELLOW}⚠${NC} Последняя ошибка Recipient: ${YELLOW}Найдены ошибки в логах${NC}"
            echo "  $last_recipient_error" | head -c 1000
            echo "..."
        else
            echo -e "${GREEN}✓${NC} Логи Recipient: ${GREEN}Без ошибок${NC}"
        fi
    fi
else
    echo -e "${YELLOW}⚠${NC} Директория логов не найдена"
fi
echo ""

# Проверка текущего теста
echo -e "${YELLOW}8. Статус тестирования:${NC}"
stats=$(curl -s "http://localhost:8080/stats" 2>/dev/null)
if [ $? -eq 0 ] && [ -n "$stats" ]; then
    active=$(echo "$stats" | jq -r '.active' 2>/dev/null)
    if [ "$active" = "true" ]; then
        echo -e "${BLUE}►${NC} Тест активен"
        current_test=$(echo "$stats" | jq -r '.current_test' 2>/dev/null)
        echo -e "  Тип: ${BLUE}$current_test${NC}"
        messages_sent=$(echo "$stats" | jq -r '.test.messages_sent' 2>/dev/null)
        echo -e "  Отправлено сообщений: ${BLUE}$messages_sent${NC}"
    else
        echo -e "${GREEN}✓${NC} Нет активных тестов"
    fi
else
    echo -e "${YELLOW}⚠${NC} Не удалось получить статистику"
fi
echo ""

# Итоговый статус
echo -e "${BLUE}════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Итоговый статус:${NC}"

all_good=true

# Проверка критических сервисов
if ! check_service "Sender" "http://localhost:8080/health" > /dev/null 2>&1; then
    echo -e "${RED}✗ Sender не работает - запустите: cd sender && go run cmd/main.go${NC}"
    all_good=false
fi

if ! check_service "Recipient" "http://localhost:8081/health" > /dev/null 2>&1; then
    echo -e "${RED}✗ Recipient не работает - запустите: cd recipient && go run cmd/main.go${NC}"
    all_good=false
fi

if ! check_tcp_port "TCP Server" "localhost" "9999" > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠ TCP сервер недоступен - проверьте конфигурацию recipient${NC}"
fi

if ! check_tcp_port "MQTT" "localhost" "1883" > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠ MQTT брокер недоступен - запустите: docker-compose up -d mosquitto${NC}"
fi

if [ "$all_good" = true ]; then
    echo -e "${GREEN}✓ Все основные компоненты работают нормально${NC}"
    echo ""
    echo -e "${BLUE}Доступные интерфейсы:${NC}"
    echo -e "  • Sender API:    ${BLUE}http://localhost:8080${NC}"
    echo -e "  • Recipient API: ${BLUE}http://localhost:8081${NC}"
    echo -e "  • Prometheus:    ${BLUE}http://localhost:9090${NC}"
    echo -e "  • Grafana:       ${BLUE}http://localhost:3000${NC} (admin/admin)"
    echo -e "  • TCP Server:    ${BLUE}localhost:9999${NC}"
    echo ""
    echo -e "${GREEN}Система готова к тестированию!${NC}"
    echo -e "Запустите ${BLUE}./run_test.sh${NC} для интерактивного тестирования"
else
    echo -e "${RED}✗ Обнаружены проблемы. Проверьте компоненты выше.${NC}"
fi

echo -e "${BLUE}════════════════════════════════════════════════${NC}"
