package service

import (
	"fmt"
	"strings"

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
			s.printAlertWithGroup(alert, groupIntervals[alert.RuleGroup])
		}
	}

	fmt.Printf("Found %d alerts matching '%s'\n", count, s.config.SearchName)

	if s.config.ChangeMode && count > 0 {
		return s.updateAlerts(matchedAlerts)
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
