package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"grafana-alerts-instrumentation/client"
	"grafana-alerts-instrumentation/config"
	"grafana-alerts-instrumentation/models"
)

type AlertService struct {
	client *client.GrafanaClient
	config *config.Config
}

func NewAlertService(cfg *config.Config) *AlertService {
	grafanaClient := client.NewGrafanaClient(cfg.GrafanaURL, cfg.APIToken)
	return &AlertService{
		client: grafanaClient,
		config: cfg,
	}
}

func (s *AlertService) SearchAlerts() error {
	// Получаем алерты
	alerts, err := s.client.GetAlerts()
	if err != nil {
		return fmt.Errorf("failed to get alerts: %v", err)
	}

	// Создаем карту группа -> интервал из самих алертов (если доступно)
	// Пока используем заглушку, так как API groups может не работать
	groupIntervals := make(map[string]string)
	for _, alert := range alerts {
		if _, exists := groupIntervals[alert.RuleGroup]; !exists {
			groupIntervals[alert.RuleGroup] = "1m" // заглушка, пока не найдем правильный API
		}
	}

	count := 0
	var matchedAlerts []models.AlertRule

	for _, alert := range alerts {
		if strings.Contains(strings.ToLower(alert.Title), strings.ToLower(s.config.SearchName)) {
			count++
			matchedAlerts = append(matchedAlerts, alert)

			// Показываем детали алерта только если не в режиме загрузки
			if !s.config.DownloadMode {
				s.printAlertWithGroup(alert, groupIntervals[alert.RuleGroup])
			}
		}
	}

	fmt.Printf("Found %d alerts matching '%s'\n", count, s.config.SearchName)

	if s.config.ChangeMode && count > 0 {
		return s.updateAlerts(matchedAlerts)
	}

	if s.config.DownloadMode && count > 0 {
		return s.downloadAlertsSimple(matchedAlerts)
	}

	return nil
}

func (s *AlertService) printAlert(alert models.AlertRule) {
	title := alert.Title
	if len(title) > s.config.MaxTitleLen {
		title = title[:s.config.MaxTitleLen] + "..."
	}
	fmt.Printf("uid: %s | pending: %s | group: %s | keep_firing_for: %s | %s\n",
		alert.UID, alert.For, alert.RuleGroup, alert.KeepFiringFor, title)
}

func (s *AlertService) printAlertWithGroup(alert models.AlertRule, groupInterval string) {
	title := alert.Title
	if len(title) > s.config.MaxTitleLen {
		title = title[:s.config.MaxTitleLen] + "..."
	}
	fmt.Printf("uid: %s | pending: %s | group: %s | eval_interval: %s | keep_firing_for: %s | %s\n",
		alert.UID, alert.For, alert.RuleGroup, groupInterval, alert.KeepFiringFor, title)
}

func (s *AlertService) updateAlerts(alerts []models.AlertRule) error {
	changedCount := 0
	unchangedCount := 0

	for _, alert := range alerts {
		originalAlert := alert
		updated := false

		if s.config.NewFiring != "" {
			alert.KeepFiringFor = s.config.NewFiring
			updated = true
		}

		if s.config.NewPending != "" {
			alert.For = s.config.NewPending
			updated = true
		}

		if updated {
			// Показываем изменения
			if s.config.NewFiring != "" {
				fmt.Printf("  keep_firing_for: %s -> New: %s\n", originalAlert.KeepFiringFor, alert.KeepFiringFor)
			}
			if s.config.NewPending != "" {
				fmt.Printf("  pending (for): %s -> New: %s\n", originalAlert.For, alert.For)
			}

			if err := s.client.UpdateAlert(alert); err != nil {
				fmt.Printf("  status: error - %v\n", err)
				unchangedCount++
				continue
			}

			fmt.Printf("  status: success\n")
			changedCount++
		} else {
			unchangedCount++
		}
	}

	fmt.Printf("\nChanged %d alerts\n", changedCount)
	fmt.Printf("Unchanged %d alerts\n", unchangedCount)

	return nil
}

func (s *AlertService) downloadAlerts(alerts []models.AlertRule) error {
	if err := os.MkdirAll(s.config.DownloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %v", err)
	}

	downloadedCount := 0

	for _, alert := range alerts {
		yamlData, err := s.client.ExportAlertAsYAML(alert.UID)
		if err != nil {
			fmt.Printf("  Failed to download alert %s: %v\n", alert.UID, err)
			continue
		}

		// Создаем безопасное имя файла
		safeTitle := strings.ReplaceAll(alert.Title, "/", "_")
		safeTitle = strings.ReplaceAll(safeTitle, " ", "_")
		filename := fmt.Sprintf("%s_%s.yaml", alert.UID, safeTitle)
		if len(filename) > 100 {
			filename = fmt.Sprintf("%s.yaml", alert.UID)
		}

		filepath := filepath.Join(s.config.DownloadDir, filename)

		if err := os.WriteFile(filepath, yamlData, 0644); err != nil {
			fmt.Printf("  Failed to save alert %s: %v\n", alert.UID, err)
			continue
		}

		fmt.Printf("  Downloaded: %s\n", filename)
		downloadedCount++
	}

	fmt.Printf("\nDownloaded %d alert files to %s/\n", downloadedCount, s.config.DownloadDir)
	return nil
}

func (s *AlertService) downloadAlertsSimple(alerts []models.AlertRule) error {
	if err := os.MkdirAll(s.config.DownloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %v", err)
	}

	// Группируем алерты по группам
	groupMap := make(map[string][]models.AlertRule)
	for _, alert := range alerts {
		groupMap[alert.RuleGroup] = append(groupMap[alert.RuleGroup], alert)
	}

	// Создаем один YAML файл для всех алертов
	var yamlContent strings.Builder
	yamlContent.WriteString("groups:\n")

	for groupName, groupAlerts := range groupMap {
		yamlContent.WriteString(fmt.Sprintf("  - orgId: %d\n", groupAlerts[0].OrgID))
		yamlContent.WriteString(fmt.Sprintf("    name: %s\n", groupName))
		yamlContent.WriteString("    rules:\n")

		for _, alert := range groupAlerts {
			yamlContent.WriteString(fmt.Sprintf("      - uid: %s\n", alert.UID))
			yamlContent.WriteString(fmt.Sprintf("        title: %s\n", alert.Title))
			yamlContent.WriteString("        interval: 1m\n")
			yamlContent.WriteString(fmt.Sprintf("        for: %s\n", alert.For))
			yamlContent.WriteString(fmt.Sprintf("        keepFiringFor: %s\n", alert.KeepFiringFor))
		}
	}

	// Создаем имя файла с nanotimestamp
	timestamp := os.Getenv("NANO_TIME")
	if timestamp == "" {
		// Если переменная окружения не установлена, используем текущее время в наносекундах
		timestamp = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	filename := fmt.Sprintf("%s_downloads.yaml", timestamp)
	filepath := filepath.Join(s.config.DownloadDir, filename)

	if err := os.WriteFile(filepath, []byte(yamlContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to save combined alerts file: %v", err)
	}

	fmt.Printf("  Downloaded: %s\n", filename)
	fmt.Printf("\nDownloaded %d alerts in 1 combined file to %s/\n", len(alerts), s.config.DownloadDir)
	return nil
}
