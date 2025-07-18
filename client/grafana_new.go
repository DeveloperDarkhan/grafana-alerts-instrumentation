package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"grafana-alerts-instrumentation/models"
)

type GrafanaClient struct {
	baseURL string
	token   string
}

func NewGrafanaClient(baseURL, token string) *GrafanaClient {
	return &GrafanaClient{
		baseURL: baseURL,
		token:   token,
	}
}

func (c *GrafanaClient) GetAlerts() ([]models.AlertRule, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/provisioning/alert-rules", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Grafana API returned status %d", resp.StatusCode)
	}

	var alerts []models.AlertRule
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return nil, err
	}

	return alerts, nil
}

func (c *GrafanaClient) UpdateGroupAfterAlertMove(groupName, folderUID, interval string) error {
	// Retry logic to handle HTTP 202 "Accepted" status
	maxRetries := 10
	retryDelay := 2 * time.Second

	fmt.Printf("🔧 Setting evaluation interval '%s' for group '%s'...\n", interval, groupName)

	for i := 0; i < maxRetries; i++ {
		time.Sleep(retryDelay) // Wait before each attempt

		err := c.updateGroupEvaluationInterval(groupName, folderUID, interval)
		if err == nil {
			return nil // Успех!
		}

		// Check if it's a retryable error (HTTP 202 or "group not found yet")
		if strings.Contains(err.Error(), "status 202") ||
			err.Error() == fmt.Sprintf("group '%s' not found yet, still being created", groupName) {
			fmt.Printf("   ⏳ Attempt %d/%d: Group still being created, waiting %v...\n",
				i+1, maxRetries, retryDelay)
			continue
		}

		// Non-retryable error
		return err
	}

	return fmt.Errorf("failed after %d attempts", maxRetries)
}

func (c *GrafanaClient) GetRuleGroups() ([]models.RuleGroup, error) {
	// Use Prometheus API endpoint
	req, err := http.NewRequest("GET", c.baseURL+"/api/prometheus/grafana/api/v1/rules", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Grafana API returned status %d", resp.StatusCode)
	}

	var promResponse models.PrometheusAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&promResponse); err != nil {
		return nil, fmt.Errorf("failed to parse prometheus response: %v", err)
	}

	// Convert PrometheusRuleGroup to RuleGroup
	var ruleGroups []models.RuleGroup
	for _, promGroup := range promResponse.Data.Groups {
		// Convert interval from seconds to string format "Xs"
		intervalStr := fmt.Sprintf("%ds", promGroup.Interval)

		ruleGroup := models.RuleGroup{
			Name:      promGroup.Name,
			FolderUID: promGroup.FolderUID,
			Interval:  intervalStr,
			// Rules will be filled later if needed
		}
		ruleGroups = append(ruleGroups, ruleGroup)
	}

	return ruleGroups, nil
}

func (c *GrafanaClient) UpdateAlert(alert models.AlertRule) error {
	// Always use X-Disable-Provenance: true header to preserve UI editability
	return c.updateViaProvisioningAPIWithDisabledProvenance(alert)
}

func (c *GrafanaClient) updateViaProvisioningAPIWithDisabledProvenance(alert models.AlertRule) error {
	alertJSON, err := json.Marshal(alert)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", c.baseURL+"/api/v1/provisioning/alert-rules/"+alert.UID, bytes.NewBuffer(alertJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Disable-Provenance", "true") // Preserve UI editability

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update alert (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (c *GrafanaClient) CreateOrUpdateRuleGroup(ruleGroup models.RuleGroup) error {
	groupJSON, err := json.Marshal(ruleGroup)
	if err != nil {
		return err
	}

	// Use Grafana Ruler API
	url := c.baseURL + "/api/ruler/grafana/api/v1/rules/" + ruleGroup.FolderUID + "/" + ruleGroup.Name
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(groupJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle HTTP 202 status - operation accepted, but async
	if resp.StatusCode == 202 {
		return fmt.Errorf("group is still being created (status 202), need to wait")
	}

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create/update rule group (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (c *GrafanaClient) updateGroupEvaluationInterval(groupName, folderUID, interval string) error {
	// Get current rules for the group
	url := c.baseURL + "/api/ruler/grafana/api/v1/rules/" + folderUID + "/" + groupName
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle HTTP 202 - group is still being created
	if resp.StatusCode == 202 {
		return fmt.Errorf("failed to get current rules (status 202)")
	}

	if resp.StatusCode == 404 {
		return fmt.Errorf("group '%s' not found yet, still being created", groupName)
	}

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get current rules (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var currentGroup models.RuleGroup
	if err := json.NewDecoder(resp.Body).Decode(&currentGroup); err != nil {
		return fmt.Errorf("failed to parse current group: %v", err)
	}

	// Update interval
	currentGroup.Interval = interval

	// Update the group
	return c.CreateOrUpdateRuleGroup(currentGroup)
}
