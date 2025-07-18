# Grafana Alerts Instrumentation

CLI tool for searching, updating, and downloading Grafana alerts with flexible filtering options.

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
go build -o grafana-alerts ./cmd/main.go

# Search alerts by name (read-only mode)
./grafana-alerts --name="staging"

# Read and display all alerts
./grafana-alerts --read-all

# Search and update specific alerts
./grafana-alerts --name="staging" --change --firing=5m --pending=10m

# Download specific alerts as YAML
./grafana-alerts --name="staging" --download

# Download all alerts as YAML
./grafana-alerts --read-all --download

# List all evaluation groups
./grafana-alerts --list-groups

# Move alert to existing evaluation group
./grafana-alerts --name="staging" --change --group="eval_1m"

# Move alert and change other parameters
./grafana-alerts --name="staging" --change --group="eval_1m" --firing="2m"

# Create new evaluation group (group will be created automatically)
./grafana-alerts --name="staging" --change --group="eval_10m" --interval="600s"

# Or use environment variables
export GRAFANA_URL="https://your-grafana.com"
export GRAFANA_TOKEN="your-token"
./grafana-alerts --name="staging" --change --firing=1m --pending=15m
./grafana-alerts --read-all --download --download-dir="./my-alerts"
```

## Configuration

- `GRAFANA_URL` or `--url`: Grafana instance URL
- `GRAFANA_TOKEN` or `--token`: Grafana API token
- `--name`: Substring to search in alert names (required for search/change operations)
- `--read-all`: Read and display all alerts without filtering
- `--change`: Enable change mode to update matched alerts
- `--download`: Download matched alerts as YAML files (requires --read-all or --name)
- `--download-dir`: Directory to save downloaded YAML files (default: "./downloads")
- `--firing`: New keep_firing_for duration (e.g., 1m, 5m)
- `--pending`: New pending (for) duration (e.g., 1m, 5m)
- `--list-groups`: List all evaluation groups with their intervals and alert counts
- `--group`: Move alert to specified evaluation group (use with --change)
- `--interval`: Set evaluation interval for new groups (e.g., 60s, 300s)

## Output Format

```
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 60s | keep_firing_for: 1m | Alert Name...
Found 2 alerts matching 'staging'
```

## Modes of Operation

### 1. **Search Mode** (default)
Search for alerts by name substring:
```bash
./grafana-alerts --name="staging"
```

### 2. **Read All Mode**
Display all alerts without filtering:
```bash
./grafana-alerts --read-all
```

### 3. **Update Mode**
Modify alert parameters:
```bash
./grafana-alerts --name="staging" --change --firing=5m --pending=10m
./grafana-alerts --read-all --change --firing=2m  # Update all alerts
```

### 4. **Download Mode**
Export alerts as YAML files:
```bash
./grafana-alerts --name="staging" --download           # Download filtered alerts
./grafana-alerts --read-all --download                 # Download all alerts
./grafana-alerts --read-all --download --download-dir="./backups"
```

### 5. **Evaluation Groups Management**
List and manage evaluation groups:
```bash
./grafana-alerts --list-groups                                    # List all groups
./grafana-alerts --name="staging" --change --group="eval_1m"     # Move to existing group
./grafana-alerts --name="staging" --change --group="eval_10m" --interval="600s"  # Create new group
```

## Validation Rules

- `--download` requires either `--read-all` or `--name` parameter
- `--change` requires at least one of `--firing`, `--pending`, or `--group`
- `--name` is required for search/change operations (except when using `--read-all` or `--list-groups`)
- `--interval` can only be used with `--group` when creating new evaluation groups

## Features

- **Smart filtering**: Search alerts by name substring (case-insensitive) or read all
- **Real-time data**: Displays alert UID, pending time, rule group, actual eval intervals
- **Bulk operations**: Update or download multiple alerts at once
- **Safe updates**: Shows what will be changed before applying updates
- **YAML export**: Combined file format with proper group intervals from Grafana API
- **Graceful degradation**: Falls back to default intervals if API groups unavailable
- **UI preservation**: Maintains alert editability in Grafana UI after API updates
- **Clean output**: Hides verbose details during download operations
- **Evaluation groups**: List, move alerts between groups, and create new groups with custom intervals

## Пример вывода

### Поиск конкретных алертов
```bash
$ ./grafana-alerts --name="staging"
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 60s | keep_firing_for: 1m | Test Alert Name regex [St...
uid: alertid456 | pending: 5m | group: memory | eval_interval: 300s | keep_firing_for: 2m | Staging Memory Alert...
Found 2 alerts matching 'staging'
```

### Просмотр всех алертов
```bash
$ ./grafana-alerts --read-all
uid: alert001 | pending: 15m | group: cpu | eval_interval: 60s | keep_firing_for: 1m | CPU Usage Alert...
uid: alert002 | pending: 10m | group: memory | eval_interval: 300s | keep_firing_for: 5m | Memory Usage Alert...
uid: alert003 | pending: 5m | group: disk | eval_interval: 60s | keep_firing_for: 2m | Disk Space Alert...
Found 3 total alerts
```

### Изменение параметров алертов
```bash
$ ./grafana-alerts --name="staging" --change --firing=5m --pending=10m
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 60s | keep_firing_for: 1m | Test Alert Name...
  keep_firing_for: 1m -> New: 5m
  pending (for): 15m -> New: 10m
  status: success

Found 1 alerts matching 'staging'
Changed 1 alerts
Unchanged 0 alerts
```

### Скачивание алертов в YAML
```bash
$ ./grafana-alerts --read-all --download
Found 5 total alerts
  Downloaded: 1752780626250902000_downloads.yaml

Downloaded 5 alerts in 1 combined file to ./downloads/
```

### Просмотр evaluation groups
```bash
$ ./grafana-alerts --list-groups
Available evaluation groups:
============================
📁 Observability
  └── cpu (60s) - 1 alert(s)

📁 Observability
  └── cpu usage (60s) - 1 alert(s)

📁 Exporters
  └── eval_1m (300s) - 2 alert(s)

📁 Observability
  └── new-group (60s) - 1 alert(s)
```

### Перемещение алерта между группами
```bash
$ ./grafana-alerts --name="staging" --change --group="eval_1m"
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 60s | keep_firing_for: 1m | Test Alert Name...
  evaluation_group: cpu -> New: eval_1m
  status: success

Found 1 alerts matching 'staging'
Changed 1 alerts
Unchanged 0 alerts
```

### Создание новой группы с кастомным интервалом
```bash
$ ./grafana-alerts --name="staging" --change --group="eval_10m" --interval="600s"
uid: alertid123 | pending: 15m | group: cpu | eval_interval: 60s | keep_firing_for: 1m | Test Alert Name...
  evaluation_group: cpu -> New: eval_10m
  group_interval: will be set to 600s
  status: success

Found 1 alerts matching 'staging'
Changed 1 alerts
Unchanged 0 alerts
```

### Содержимое YAML файла
```yaml
groups:
  - orgId: 1
    name: cpu
    interval: 60s
    rules:
      - uid: alertid123
        title: Test Alert Name regex [Staging]
        for: 15m
        keepFiringFor: 2m
  - orgId: 1
    name: memory
    interval: 300s
    rules:
      - uid: alertid456
        title: Memory Alert
        for: 10m
        keepFiringFor: 5m
```

## Requirements

- Go 1.21 or later
- Grafana API token with appropriate permissions:
  - `alerts:read` - for searching and reading alerts
  - `alerts:write` - for updating alerts (when using `--change`)
  - Access to provisioning API endpoints

## Troubleshooting

### Common Issues

1. **Permission Denied**
   ```
   Error: failed to get alerts: Grafana API returned status 403
   ```
   **Solution**: Check that your API token has the required permissions.

2. **API Groups Warning**
   ```
   Warning: failed to get rule groups, using default intervals
   ```
   **Impact**: The tool will use `1m` as fallback interval for all groups. Functionality is preserved.

3. **Download Mode Validation**
   ```
   Error: download mode requires either --read-all flag or --name parameter
   ```
   **Solution**: Use `--read-all --download` or `--name="filter" --download`.

4. **Change Mode Validation**
   ```
   Error: when using --change flag, at least one of --firing, --pending, or --group must be specified
   ```
   **Solution**: Add `--firing=5m`, `--pending=10m`, and/or `--group="eval_1m"` parameters.

5. **Group Creation Validation**
   ```
   Error: --interval can only be used with --group when creating new evaluation groups
   ```
   **Solution**: Use `--interval` only together with `--group` parameter.

6. **Groups API Warning**
   ```
   Warning: failed to get rule groups for evaluation groups listing
   ```
   **Impact**: The `--list-groups` command may not work properly. Check API token permissions.

### Exit Codes

- `0` - Success
- `1` - General error (invalid configuration, API errors, etc.)

## Technical Notes

- **Interval Retrieval**: The tool fetches real evaluation intervals from Grafana's Prometheus API (`/api/prometheus/grafana/api/v1/rules`)
- **UI Compatibility**: Uses `X-Disable-Provenance: true` header to maintain alert editability in Grafana UI
- **File Naming**: Downloaded YAML files use nanosecond timestamps for uniqueness
- **Error Handling**: Graceful degradation when API groups are unavailable
- **Evaluation Groups**: Groups are managed through the `ruleGroup` field in alert rules
- **Automatic Group Creation**: When moving an alert to a non-existent group, the group is created automatically
- **Group Intervals**: New groups use the specified `--interval` or default to 60s if not provided
- **Folder Integration**: Groups listing shows folder names resolved from Grafana folders API