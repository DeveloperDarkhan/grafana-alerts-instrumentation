# Grafana Alerts Instrumentation

CLI tool for searching and counting Grafana alerts by name substring.

## Project Structure

```
grafana-alerts-instrumentation/
├── cmd/
│   └── main.go              # Application entry point
├── config/
│   └── config.go            # Configuration and flag parsing
├── models/
│   └── alert.go             # Data models
├── client/
│   └── grafana.go           # Grafana API client
├── service/
│   └── alert.go             # Business logic for alert searching
├── go.mod                   # Go module file
└── README.md                 # This file
```

## Usage

```bash
# Build the application
go build -o grafana-alerts ./cmd/grafana-alerts

# Search alerts (read-only mode)
./grafana-alerts -url="https://your-grafana.com" -token="your-token" --name="staging"

# Search and update alerts (change mode)
./grafana-alerts -url="https://your-grafana.com" -token="your-token" --name="staging" --change --firing=5m

# Download matched alerts as YAML files
./grafana-alerts -url="https://your-grafana.com" -token="your-token" --name="staging" --download

# Or use environment variables
export GRAFANA_URL="https://your-grafana.com"
export GRAFANA_TOKEN="your-token"
./grafana-alerts --name="staging" --change --firing=1m --pending=15m
./grafana-alerts --name="staging" --download --download-dir="./my-alerts"
```

## Configuration

- `GRAFANA_URL` or `-url`: Grafana instance URL
- `GRAFANA_TOKEN` or `-token`: Grafana API token
- `--name`: Substring to search in alert names
- `--change`: Enable change mode to update matched alerts
- `--download`: Download matched alerts as YAML files
- `--download-dir`: Directory to save downloaded YAML files (default: "./alerts")
- `--firing`: New keep_firing_for duration (e.g., 1m, 5m)
- `--pending`: New pending (for) duration (e.g., 1m, 5m)

## Output Format

```
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 1m | keep_firing_for: 1m | Alert Name...
Found 2 alerts matching 'staging'
```

## Features

- Search alerts by name substring (case-insensitive)
- Display alert UID, pending time, rule group, keep firing duration
- Truncate long alert names to 25 characters
- **Update alert parameters** with `--change` flag:
  - Modify `keep_firing_for` duration
  - Modify pending (`for`) duration
- **Download alerts as YAML** with `--download` flag:
  - Export matched alerts in Grafana provisioning format
  - Save to customizable directory
  - Safe filename generation from alert titles
- Clean, modular code structure following Go best practices
- Safe operation: shows what will be changed before applying updates
- **Preserves UI editability** when updating alerts via API

## Пример вывода

### Поиск алертов
```
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 1m | keep_firing_for: 1m | Alert Name...
Found 3 alerts matching 'cpu'
```

### Изменение алертов
```
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 1m | keep_firing_for: 1m | Alert Name...
  keep_firing_for: 1m -> New: 5m
  pending (for): 15m -> New: 10m
  status: success

Found 1 alerts matching 'cpu'
Changed 1 alerts
Unchanged 0 alerts
```

### Загрузка алертов
```
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 1m | keep_firing_for: 1m | Alert Name...
  Downloaded: alertid123_Alert_Name.yaml
Found 1 alerts matching 'cpu'

Downloaded 1 alert files to ./alerts/
```