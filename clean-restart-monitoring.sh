#!/bin/bash

# Скрипт для полной очистки и перезапуска мониторинга InfoDiode

echo "🧹 Полная очистка и перезапуск мониторинга InfoDiode..."

# Остановка всех контейнеров
echo "📦 Остановка контейнеров..."
docker-compose --profile monitoring down
docker-compose --profile full down

# Очистка volumes
echo "🗑️  Удаление старых volumes..."
docker volume rm infodiode_test_grafana-data 2>/dev/null || true
docker volume rm infodiode_test_prometheus-data 2>/dev/null || true

# Очистка старых дашбордов из provisioning директории
echo "📂 Очистка старых конфигураций..."
rm -f grafana/provisioning/dashboards/*.json 2>/dev/null || true
rm -f grafana/provisioning/dashboards/dashboard.yml 2>/dev/null || true

# Проверка структуры директорий
echo "✅ Проверка структуры директорий..."
echo "   Дашборды в: grafana/dashboards/"
ls -la grafana/dashboards/*.json 2>/dev/null || echo "   ⚠️  Дашборды не найдены!"
echo ""
echo "   Конфигурация provisioning:"
ls -la grafana/provisioning/dashboards/dashboards.yaml 2>/dev/null || echo "   ⚠️  dashboards.yaml не найден!"
ls -la grafana/provisioning/datasources/prometheus.yaml 2>/dev/null || echo "   ⚠️  prometheus.yaml не найден!"

# Запуск контейнеров мониторинга
echo ""
echo "🚀 Запуск контейнеров мониторинга..."
docker-compose --profile monitoring up -d

# Ожидание запуска
echo "⏳ Ожидание запуска сервисов..."
sleep 10

# Проверка статуса
echo ""
echo "📊 Проверка статуса контейнеров..."
docker-compose --profile monitoring ps

# Проверка логов Grafana на наличие ошибок
echo ""
echo "🔍 Проверка логов Grafana..."
docker-compose logs grafana 2>&1 | tail -20 | grep -E "(error|Error|ERROR|provisioning|dashboard)" || echo "   ✅ Ошибок не обнаружено"

echo ""
echo "==========================================="
echo "📊 Сервисы доступны по адресам:"
echo "   - Grafana: http://localhost:3000"
echo "     Логин: admin"
echo "     Пароль: admin"
echo "   - Prometheus: http://localhost:9090"
echo ""
echo "📁 Дашборды должны находиться в папке 'InfoDiode'"
echo "   1. Overview Dashboard - общий обзор"
echo "   2. Sender Dashboard - метрики отправителя"
echo "   3. Recipient Dashboard - метрики получателя"
echo ""
echo "💡 Команды для отладки:"
echo "   docker-compose --profile monitoring logs -f grafana"
echo "   docker-compose --profile monitoring logs -f prometheus"
echo "   docker exec -it grafana ls -la /var/lib/grafana/dashboards/"
echo "==========================================="

# Дополнительная проверка загрузки дашбордов
echo ""
echo "🔍 Проверка загрузки дашбордов в контейнере..."
sleep 5
docker exec grafana ls -la /var/lib/grafana/dashboards/ 2>/dev/null || echo "   ⚠️  Не удалось проверить дашборды в контейнере"
