#!/bin/bash

# Скрипт для перезапуска мониторинга InfoDiode

echo "🔄 Перезапуск мониторинга InfoDiode..."

# Остановка контейнеров мониторинга
echo "📦 Остановка контейнеров..."
docker-compose --profile monitoring down

# Очистка volumes (опционально, закомментировано)
# echo "🧹 Очистка volumes..."
# docker volume rm infodiode_test_grafana-data infodiode_test_prometheus-data 2>/dev/null

# Пересоздание контейнеров
echo "🚀 Запуск контейнеров мониторинга..."
docker-compose --profile monitoring up -d

# Ожидание запуска
echo "⏳ Ожидание запуска сервисов..."
sleep 5

# Проверка статуса
echo "✅ Проверка статуса..."
docker-compose --profile monitoring ps

echo ""
echo "📊 Сервисы доступны по адресам:"
echo "   - Grafana: http://localhost:3000 (admin/admin)"
echo "   - Prometheus: http://localhost:9090"
echo ""
echo "💡 Для просмотра логов используйте:"
echo "   docker-compose --profile monitoring logs -f grafana"
echo "   docker-compose --profile monitoring logs -f prometheus"
