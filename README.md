# Grafana Alert Counter

CLI-приложение для подсчёта количества алертов в Grafana по конкретному имени.

## Запуск

1. Установите переменные окружения или передайте параметры:
   - `GRAFANA_URL` — URL вашей Grafana (например, https://mon-02.freedompay.kz)
   - `GRAFANA_TOKEN` — API токен Grafana с правом чтения алертов

2. Запустите:

```sh
go run ./cmd/main.go --name "alert-name-part"
```
или
```sh
GRAFANA_URL=https://grafana.example.com GRAFANA_TOKEN=xxx go run ./cmd/main.go --name "alert-name-part"
```

## Пример вывода

```
Found 3 alerts matching 'cpu'
```