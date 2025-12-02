#!/bin/bash

# Скрипт для запуска тестов с выбором протокола (MQTT или TCP)

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# URL API сервера
API_URL="http://localhost:8080"

# Функция для вывода меню
show_menu() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}     Тестирование передачи данных      ${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    echo "Выберите протокол:"
    echo "  1) MQTT"
    echo "  2) TCP"
    echo "  3) Сравнение MQTT vs TCP"
    echo ""
    echo "  0) Выход"
    echo ""
}

# Функция для выбора типа теста
show_test_type_menu() {
    local protocol=$1
    echo -e "${YELLOW}Выберите тип теста для $protocol:${NC}"
    echo "  1) Пакетная отправка (batch)"
    echo "  2) Потоковая отправка (stream)"
    echo "  3) Большие пакеты (large)"
    echo "  4) Пользовательские параметры"
    echo ""
    echo "  0) Назад"
    echo ""
}

# Функция для запуска пакетного теста
run_batch_test() {
    local protocol=$1
    echo -e "${GREEN}Запуск пакетного теста через $protocol${NC}"

    read -p "Количество потоков (1-100): " threads
    read -p "Размер пакета в байтах: " packet_size
    read -p "Общее количество сообщений: " total_messages
    read -p "Длительность теста (секунд): " duration

    echo -e "${YELLOW}Отправка запроса...${NC}"

    response=$(curl -s -X POST "$API_URL/test/batch" \
        -H "Content-Type: application/json" \
        -d "{
            \"protocol\": \"$protocol\",
            \"thread_count\": $threads,
            \"packet_size\": $packet_size,
            \"total_messages\": $total_messages,
            \"duration\": $duration
        }")

    echo -e "${GREEN}Ответ сервера:${NC}"
    echo "$response" | jq .
}

# Функция для запуска потокового теста
run_stream_test() {
    local protocol=$1
    echo -e "${GREEN}Запуск потокового теста через $protocol${NC}"

    read -p "Сообщений в секунду: " msg_per_sec
    read -p "Размер пакета в байтах: " packet_size
    read -p "Длительность теста (секунд): " duration

    echo -e "${YELLOW}Отправка запроса...${NC}"

    response=$(curl -s -X POST "$API_URL/test/stream" \
        -H "Content-Type: application/json" \
        -d "{
            \"protocol\": \"$protocol\",
            \"messages_per_sec\": $msg_per_sec,
            \"packet_size\": $packet_size,
            \"duration\": $duration
        }")

    echo -e "${GREEN}Ответ сервера:${NC}"
    echo "$response" | jq .
}

# Функция для запуска теста с большими пакетами
run_large_test() {
    local protocol=$1
    echo -e "${GREEN}Запуск теста с большими пакетами через $protocol${NC}"

    read -p "Количество потоков (1-20): " threads
    read -p "Размер пакета в МБ (1-100): " packet_size_mb
    read -p "Длительность теста (секунд): " duration

    echo -e "${YELLOW}Отправка запроса...${NC}"

    response=$(curl -s -X POST "$API_URL/test/large" \
        -H "Content-Type: application/json" \
        -d "{
            \"protocol\": \"$protocol\",
            \"thread_count\": $threads,
            \"packet_size_mb\": $packet_size_mb,
            \"duration\": $duration
        }")

    echo -e "${GREEN}Ответ сервера:${NC}"
    echo "$response" | jq .
}

# Функция для сравнительного теста
run_comparison_test() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}    Сравнительный тест MQTT vs TCP     ${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""

    echo "Выберите сценарий:"
    echo "  1) Маленькие пакеты (100KB, 1000 msg/sec)"
    echo "  2) Средние пакеты (1MB, 100 msg/sec)"
    echo "  3) Большие пакеты (10MB, 10 msg/sec)"
    echo ""
    read -p "Выбор: " scenario

    case $scenario in
        1)
            echo -e "${YELLOW}Тест с маленькими пакетами${NC}"
            packet_size=102400
            msg_rate=1000
            ;;
        2)
            echo -e "${YELLOW}Тест со средними пакетами${NC}"
            packet_size=1048576
            msg_rate=100
            ;;
        3)
            echo -e "${YELLOW}Тест с большими пакетами${NC}"
            packet_size=10485760
            msg_rate=10
            ;;
        *)
            echo -e "${RED}Неверный выбор${NC}"
            return
            ;;
    esac

    read -p "Длительность каждого теста (секунд): " duration

    # Запуск теста через MQTT
    echo -e "\n${GREEN}Запуск теста через MQTT...${NC}"
    mqtt_response=$(curl -s -X POST "$API_URL/test/stream" \
        -H "Content-Type: application/json" \
        -d "{
            \"protocol\": \"mqtt\",
            \"messages_per_sec\": $msg_rate,
            \"packet_size\": $packet_size,
            \"duration\": $duration
        }")

    echo "Тест MQTT запущен. Ожидание завершения..."
    sleep $((duration + 5))

    # Получение статистики MQTT
    mqtt_stats=$(curl -s "$API_URL/stats")
    echo -e "${GREEN}Результаты MQTT:${NC}"
    echo "$mqtt_stats" | jq .

    # Остановка теста
    curl -s -X POST "$API_URL/test/stop" > /dev/null

    echo -e "\n${YELLOW}Пауза перед TCP тестом...${NC}"
    sleep 5

    # Запуск теста через TCP
    echo -e "\n${GREEN}Запуск теста через TCP...${NC}"
    tcp_response=$(curl -s -X POST "$API_URL/test/stream" \
        -H "Content-Type: application/json" \
        -d "{
            \"protocol\": \"tcp\",
            \"messages_per_sec\": $msg_rate,
            \"packet_size\": $packet_size,
            \"duration\": $duration
        }")

    echo "Тест TCP запущен. Ожидание завершения..."
    sleep $((duration + 5))

    # Получение статистики TCP
    tcp_stats=$(curl -s "$API_URL/stats")
    echo -e "${GREEN}Результаты TCP:${NC}"
    echo "$tcp_stats" | jq .

    # Остановка теста
    curl -s -X POST "$API_URL/test/stop" > /dev/null

    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}         Сравнение завершено            ${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# Функция для получения статистики
get_stats() {
    echo -e "${YELLOW}Получение статистики...${NC}"
    response=$(curl -s "$API_URL/stats")
    echo -e "${GREEN}Текущая статистика:${NC}"
    echo "$response" | jq .
}

# Функция для остановки теста
stop_test() {
    echo -e "${YELLOW}Остановка текущего теста...${NC}"
    response=$(curl -s -X POST "$API_URL/test/stop")
    echo -e "${GREEN}Ответ:${NC}"
    echo "$response" | jq .
}

# Проверка наличия jq
if ! command -v jq &> /dev/null; then
    echo -e "${YELLOW}Предупреждение: jq не установлен. Установите его для красивого вывода JSON.${NC}"
    echo "  Ubuntu/Debian: sudo apt-get install jq"
    echo "  MacOS: brew install jq"
    echo ""
fi

# Проверка доступности API
echo -e "${YELLOW}Проверка доступности API сервера...${NC}"
if curl -s -X GET "$API_URL/health" | head -n 1 | grep "healthy" > /dev/null; then
    echo -e "${GREEN}API сервер доступен${NC}"
else
    echo -e "${RED}API сервер недоступен. Убедитесь, что sender сервис запущен.${NC}"
    exit 1
fi

# Главный цикл
while true; do
    show_menu
    read -p "Выберите опцию: " choice

    case $choice in
        1)
            # MQTT тесты
            protocol="mqtt"
            while true; do
                show_test_type_menu "MQTT"
                read -p "Выберите тип теста: " test_type

                case $test_type in
                    1) run_batch_test "$protocol" ;;
                    2) run_stream_test "$protocol" ;;
                    3) run_large_test "$protocol" ;;
                    4)
                        echo "Пользовательские параметры"
                        run_batch_test "$protocol"
                        ;;
                    0) break ;;
                    *) echo -e "${RED}Неверный выбор${NC}" ;;
                esac

                if [[ $test_type != "0" ]]; then
                    echo ""
                    read -p "Нажмите Enter для продолжения..."
                fi
            done
            ;;
        2)
            # TCP тесты
            protocol="tcp"
            while true; do
                show_test_type_menu "TCP"
                read -p "Выберите тип теста: " test_type

                case $test_type in
                    1) run_batch_test "$protocol" ;;
                    2) run_stream_test "$protocol" ;;
                    3) run_large_test "$protocol" ;;
                    4)
                        echo "Пользовательские параметры"
                        run_batch_test "$protocol"
                        ;;
                    0) break ;;
                    *) echo -e "${RED}Неверный выбор${NC}" ;;
                esac

                if [[ $test_type != "0" ]]; then
                    echo ""
                    read -p "Нажмите Enter для продолжения..."
                fi
            done
            ;;
        3)
            # Сравнение протоколов
            run_comparison_test
            echo ""
            read -p "Нажмите Enter для продолжения..."
            ;;
        0)
            echo -e "${GREEN}Выход из программы${NC}"
            exit 0
            ;;
        s|S)
            # Скрытая опция для получения статистики
            get_stats
            echo ""
            read -p "Нажмите Enter для продолжения..."
            ;;
        x|X)
            # Скрытая опция для остановки теста
            stop_test
            echo ""
            read -p "Нажмите Enter для продолжения..."
            ;;
        *)
            echo -e "${RED}Неверный выбор. Попробуйте снова.${NC}"
            sleep 1
            ;;
    esac
done
