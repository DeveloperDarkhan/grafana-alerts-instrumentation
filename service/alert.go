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
	alerts, err := s.client.GetAlerts()
	if err != nil {
		return fmt.Errorf("failed to get alerts: %v", err)
	}

	count := 0
	for _, alert := range alerts {
		if strings.Contains(strings.ToLower(alert.Title), strings.ToLower(s.config.SearchName)) {
			count++
			s.printAlert(alert)
		}
	}

	fmt.Printf("Found %d alerts matching '%s'\n", count, s.config.SearchName)
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
