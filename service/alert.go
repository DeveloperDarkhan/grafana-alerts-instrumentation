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
	// Если нужно показать только группы, вызываем соответствующую функцию
	if s.config.ListGroups {
		return s.ListGroups()
	}

	// Получаем алерты
	alerts, err := s.client.GetAlerts()
	if err != nil {
		return fmt.Errorf("failed to get alerts: %v", err)
	}

	// Получаем группы правил для получения реальных интервалов
	ruleGroups, err := s.client.GetRuleGroups()
	if err != nil {
		// Если не удается получить группы, используем заглушку
		fmt.Printf("Warning: failed to get rule groups, using default intervals: %v\n", err)
	}

	// Создаем карту группа -> интервал из реальных данных групп
	groupIntervals := make(map[string]string)
	for _, group := range ruleGroups {
		groupIntervals[group.Name] = group.Interval
	}

	// Для групп без данных используем заглушку
	for _, alert := range alerts {
		if _, exists := groupIntervals[alert.RuleGroup]; !exists {
			groupIntervals[alert.RuleGroup] = "1m" // заглушка для неизвестных групп
		}
	}

	count := 0
	var matchedAlerts []models.AlertRule

	for _, alert := range alerts {
		// Если ReadAllMode или DownloadMode с пустым SearchName, добавляем все алерты
		// Иначе фильтруем по названию
		if s.config.ReadAllMode || s.config.SearchName == "" || strings.Contains(strings.ToLower(alert.Title), strings.ToLower(s.config.SearchName)) {
			count++
			matchedAlerts = append(matchedAlerts, alert)

			// Показываем детали алерта только если не в режиме загрузки
			if !s.config.DownloadMode {
				s.printAlertWithGroup(alert, groupIntervals[alert.RuleGroup])
			}
		}
	}

	if s.config.ReadAllMode {
		fmt.Printf("Found %d total alerts\n", count)
	} else if s.config.SearchName == "" {
		fmt.Printf("Found %d total alerts\n", count)
	} else {
		fmt.Printf("Found %d alerts matching '%s'\n", count, s.config.SearchName)
	}

	if s.config.ChangeMode && count > 0 {
		return s.updateAlerts(matchedAlerts)
	}

	if s.config.DownloadMode && count > 0 {
		return s.downloadAlertsSimple(matchedAlerts)
	}

	return nil
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

		if s.config.NewGroup != "" {
			alert.RuleGroup = s.config.NewGroup
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
			if s.config.NewGroup != "" {
				fmt.Printf("  evaluation_group: %s -> New: %s\n", originalAlert.RuleGroup, alert.RuleGroup)
				if s.config.NewInterval != "" {
					fmt.Printf("  group_interval: will be set to %s\n", s.config.NewInterval)
				}
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

func (s *AlertService) downloadAlertsSimple(alerts []models.AlertRule) error {
	if err := os.MkdirAll(s.config.DownloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %v", err)
	}

	// Получаем реальные интервалы групп
	ruleGroups, err := s.client.GetRuleGroups()
	groupIntervals := make(map[string]string)
	if err != nil {
		fmt.Printf("Warning: failed to get rule groups, using default interval: %v\n", err)
	} else {
		for _, group := range ruleGroups {
			groupIntervals[group.Name] = group.Interval
		}
	}

	// Получаем папки для преобразования UID в названия
	folders, err := s.client.GetFolders()
	folderNames := make(map[string]string)
	if err != nil {
		fmt.Printf("Warning: failed to get folders, using folder UIDs: %v\n", err)
	} else {
		for _, folder := range folders {
			folderNames[folder.UID] = folder.Title
		}
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
		// Получаем реальный интервал группы или используем заглушку
		interval := groupIntervals[groupName]
		if interval == "" {
			interval = "1m" // заглушка для неизвестных групп
		}

		// Получаем имя папки вместо UID
		folderUID := groupAlerts[0].FolderUID
		folderName := folderNames[folderUID]
		if folderName == "" {
			folderName = folderUID // используем UID если имя не найдено
		}

		yamlContent.WriteString(fmt.Sprintf("  - orgId: %d\n", groupAlerts[0].OrgID))
		yamlContent.WriteString(fmt.Sprintf("    folder: %s\n", folderName))
		yamlContent.WriteString(fmt.Sprintf("    name: %s\n", groupName))
		yamlContent.WriteString(fmt.Sprintf("    interval: %s\n", interval))
		yamlContent.WriteString("    rules:\n")

		for _, alert := range groupAlerts {
			yamlContent.WriteString(fmt.Sprintf("      - uid: %s\n", alert.UID))
			yamlContent.WriteString(fmt.Sprintf("        title: %s\n", alert.Title))
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

func (s *AlertService) ListGroups() error {
	// Получаем алерты для подсчета
	alerts, err := s.client.GetAlerts()
	if err != nil {
		return fmt.Errorf("failed to get alerts: %v", err)
	}

	// Получаем реальные интервалы групп
	ruleGroups, err := s.client.GetRuleGroups()
	groupIntervals := make(map[string]string)
	if err != nil {
		fmt.Printf("Warning: failed to get rule groups, using default intervals: %v\n", err)
	} else {
		for _, group := range ruleGroups {
			groupIntervals[group.Name] = group.Interval
		}
	}

	// Получаем папки для отображения имен
	folders, err := s.client.GetFolders()
	folderNames := make(map[string]string)
	if err != nil {
		fmt.Printf("Warning: failed to get folders, using folder UIDs: %v\n", err)
	} else {
		for _, folder := range folders {
			folderNames[folder.UID] = folder.Title
		}
	}

	// Подсчитываем алерты по группам
	groupCounts := make(map[string]int)
	groupFolders := make(map[string]string)
	for _, alert := range alerts {
		groupCounts[alert.RuleGroup]++
		if groupFolders[alert.RuleGroup] == "" {
			folderName := folderNames[alert.FolderUID]
			if folderName == "" {
				folderName = alert.FolderUID
			}
			groupFolders[alert.RuleGroup] = folderName
		}
	}

	fmt.Println("Available evaluation groups:")
	fmt.Println("============================")

	for groupName, count := range groupCounts {
		interval := groupIntervals[groupName]
		if interval == "" {
			interval = "1m"
		}
		folder := groupFolders[groupName]

		fmt.Printf("%s\n", folder)
		fmt.Printf("  └── %s (%s) - %d alert(s)\n", groupName, interval, count)
		fmt.Println()
	}

	return nil
}
