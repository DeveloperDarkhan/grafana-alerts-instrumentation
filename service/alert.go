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
	// If need to show only groups, call corresponding function
	if s.config.ListGroups {
		return s.ListGroups()
	}

	// If need to show group details, call corresponding function
	if s.config.GroupDetails != "" {
		return s.ShowGroupDetails()
	}

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

		if s.config.NewGroup != "" {
			// Сначала проверяем/создаем группу
			if s.config.NewInterval != "" {
				ruleGroup := models.RuleGroup{
					Name:      s.config.NewGroup,
					FolderUID: alert.FolderUID,
					Interval:  s.config.NewInterval,
					Rules:     []models.AlertRule{}, // Empty rules initially
				}
				s.client.CreateOrUpdateRuleGroup(ruleGroup)
			}

			alert.RuleGroup = s.config.NewGroup
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
			if s.config.NewGroup != "" {
				fmt.Printf("  evaluation_group: %s -> New: %s\n", originalAlert.RuleGroup, alert.RuleGroup)
			}

			// Сначала обновляем алерт (переносим в новую группу)
			if err := s.client.UpdateAlert(alert); err != nil {
				fmt.Printf("  status: error - %v\n", err)
				unchangedCount++
				continue
			}

			// ВАЖНО: Даем время Grafana обновить состояние группы после перемещения алерта
			if s.config.NewGroup != "" && s.config.NewInterval != "" {
				fmt.Printf("  📝 Alert moved successfully, waiting for group state to update...\n")
				time.Sleep(3 * time.Second) // Пауза для обновления состояния

				// Создаем новый клиент для получения свежего состояния
				freshClient := client.NewGrafanaClient(s.config.GrafanaURL, s.config.APIToken)

				if err := freshClient.UpdateGroupAfterAlertMove(s.config.NewGroup, alert.FolderUID, s.config.NewInterval); err != nil {
					fmt.Printf("  group_interval: ⚠️  Failed to set '%s' via API: %v\n", s.config.NewInterval, err)
					fmt.Printf("  📋 Manual UI setup required for evaluation interval\n")
				} else {
					fmt.Printf("  group_interval: ✅ Successfully set to '%s'\n", s.config.NewInterval)
				}
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

	// Get folders to convert UID to names
	folders, err := s.client.GetFolders()
	folderNames := make(map[string]string)
	if err != nil {
		fmt.Printf("Warning: failed to get folders, using folder UIDs: %v\n", err)
	} else {
		for _, folder := range folders {
			folderNames[folder.UID] = folder.Title
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

		// Get folder name instead of UID
		folderUID := groupAlerts[0].FolderUID
		folderName := folderNames[folderUID]
		if folderName == "" {
			folderName = folderUID // use UID if name not found
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

func (s *AlertService) ListGroups() error {
	// Get alerts for counting
	alerts, err := s.client.GetAlerts()
	if err != nil {
		return fmt.Errorf("failed to get alerts: %v", err)
	}

	// Get real group intervals
	ruleGroups, err := s.client.GetRuleGroups()
	groupIntervals := make(map[string]string)
	if err != nil {
		fmt.Printf("Warning: failed to get rule groups, using default intervals: %v\n", err)
	} else {
		for _, group := range ruleGroups {
			groupIntervals[group.Name] = group.Interval
		}
	}

	// Get folders for displaying names
	folders, err := s.client.GetFolders()
	folderNames := make(map[string]string)
	if err != nil {
		fmt.Printf("Warning: failed to get folders, using folder UIDs: %v\n", err)
	} else {
		for _, folder := range folders {
			folderNames[folder.UID] = folder.Title
		}
	}

	// Count alerts by groups
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

func (s *AlertService) ShowGroupDetails() error {
	// Parse group name and find its folder
	groupName := s.config.GroupDetails

	// Get all rule groups to find the folder UID for this group
	ruleGroups, err := s.client.GetRuleGroups()
	if err != nil {
		return fmt.Errorf("failed to get rule groups: %v", err)
	}

	var targetFolderUID string
	for _, group := range ruleGroups {
		if group.Name == groupName {
			targetFolderUID = group.FolderUID
			break
		}
	}

	if targetFolderUID == "" {
		return fmt.Errorf("group '%s' not found", groupName)
	}

	// Get detailed information about the group
	groupDetails, err := s.client.GetGroupDetails(groupName, targetFolderUID)
	if err != nil {
		return fmt.Errorf("failed to get group details: %v", err)
	}

	// Get folder information
	folders, err := s.client.GetFolders()
	folderName := targetFolderUID // fallback to UID if name not found
	if err == nil {
		for _, folder := range folders {
			if folder.UID == targetFolderUID {
				folderName = folder.Title
				break
			}
		}
	}

	fmt.Printf("Evaluation Group Details\n")
	fmt.Printf("========================\n")
	fmt.Printf("📁 Folder: %s\n", folderName)
	fmt.Printf("📊 Group: %s\n", groupDetails.Name)
	fmt.Printf("⏱️  Evaluation Interval: %s\n", groupDetails.Interval)
	fmt.Printf("🔢 Total Rules: %d\n", len(groupDetails.Rules))
	fmt.Printf("\nAlert Rules:\n")
	fmt.Printf("============\n")

	for i, rule := range groupDetails.Rules {
		// Truncate title if too long
		title := rule.Title
		if len(title) > s.config.MaxTitleLen {
			title = title[:s.config.MaxTitleLen-3] + "..."
		}

		fmt.Printf("%d. %s\n", i+1, title)
		fmt.Printf("   UID: %s\n", rule.UID)
		fmt.Printf("   Pending: %s\n", rule.For)
		fmt.Printf("   Keep Firing: %s\n", rule.KeepFiringFor)
		if rule.IsPaused {
			fmt.Printf("   Status: ⏸️  PAUSED\n")
		} else {
			fmt.Printf("   Status: ▶️  ACTIVE\n")
		}
		fmt.Printf("\n")
	}

	return nil
}
