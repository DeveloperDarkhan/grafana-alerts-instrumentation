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
	// Get alerts
	alerts, err := s.client.GetAlerts()
	if err != nil {
		return fmt.Errorf("failed to get alerts: %v", err)
	}

	// Get rule groups to get real intervals
	ruleGroups, err := s.client.GetRuleGroups()
	if err != nil {
		// If we can't get groups, use default placeholder
		fmt.Printf("Warning: failed to get rule groups, using default intervals: %v\n", err)
	}

	// Create map group -> interval from real group data
	groupIntervals := make(map[string]string)
	for _, group := range ruleGroups {
		groupIntervals[group.Name] = group.Interval
	}

	// For groups without data use placeholder
	for _, alert := range alerts {
		if _, exists := groupIntervals[alert.RuleGroup]; !exists {
			groupIntervals[alert.RuleGroup] = "1m" // placeholder for unknown groups
		}
	}

	count := 0
	var matchedAlerts []models.AlertRule

	for _, alert := range alerts {
		// If ReadAllMode or DownloadMode with empty SearchName, add all alerts
		// Otherwise filter by name
		if s.config.ReadAllMode || s.config.SearchName == "" || strings.Contains(strings.ToLower(alert.Title), strings.ToLower(s.config.SearchName)) {
			count++
			matchedAlerts = append(matchedAlerts, alert)

			// Show alert details only if not in download mode
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

		if updated {
			// Show changes
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

func (s *AlertService) downloadAlertsSimple(alerts []models.AlertRule) error {
	if err := os.MkdirAll(s.config.DownloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %v", err)
	}

	// Get real group intervals
	ruleGroups, err := s.client.GetRuleGroups()
	groupIntervals := make(map[string]string)
	if err != nil {
		fmt.Printf("Warning: failed to get rule groups, using default interval: %v\n", err)
	} else {
		for _, group := range ruleGroups {
			groupIntervals[group.Name] = group.Interval
		}
	}

	// Group alerts by groups
	groupMap := make(map[string][]models.AlertRule)
	for _, alert := range alerts {
		groupMap[alert.RuleGroup] = append(groupMap[alert.RuleGroup], alert)
	}

	// Create one YAML file for all alerts
	var yamlContent strings.Builder
	yamlContent.WriteString("groups:\n")

	for groupName, groupAlerts := range groupMap {
		// Get real group interval or use placeholder
		interval := groupIntervals[groupName]
		if interval == "" {
			interval = "1m" // placeholder for unknown groups
		}

		yamlContent.WriteString(fmt.Sprintf("  - orgId: %d\n", groupAlerts[0].OrgID))
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

	// Create filename with nanotimestamp
	timestamp := os.Getenv("NANO_TIME")
	if timestamp == "" {
		// If environment variable is not set, use current time in nanoseconds
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
