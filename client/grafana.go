package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"grafana-alerts-instrumentation/models"
)

type GrafanaClient struct {
	baseURL string
	token   string
}

// parseIntervalToSeconds converts interval strings like "5m", "300s", "1h" to seconds
func parseIntervalToSeconds(interval string) (int64, error) {
	if interval == "" {
		return 0, fmt.Errorf("empty interval")
	}

	// Handle pure number (assume seconds)
	if num, err := strconv.ParseInt(interval, 10, 64); err == nil {
		return num, nil
	}

	// Handle suffixed intervals
	switch {
	case strings.HasSuffix(interval, "s"):
		numStr := strings.TrimSuffix(interval, "s")
		return strconv.ParseInt(numStr, 10, 64)
	case strings.HasSuffix(interval, "m"):
		numStr := strings.TrimSuffix(interval, "m")
		minutes, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return 0, err
		}
		return minutes * 60, nil
	case strings.HasSuffix(interval, "h"):
		numStr := strings.TrimSuffix(interval, "h")
		hours, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return 0, err
		}
		return hours * 3600, nil
	default:
		return 0, fmt.Errorf("unsupported interval format: %s", interval)
	}
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
		return nil, fmt.Errorf("grafana API returned status %d", resp.StatusCode)
	}

	var alerts []models.AlertRule
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return nil, err
	}

	return alerts, nil
}

func (c *GrafanaClient) UpdateGroupAfterAlertMove(groupName, folderUID, interval string) error {
	// Retry logic to handle HTTP 202 "Accepted" status
	maxRetries := 3
	retryDelay := 2 * time.Second

	fmt.Printf("🔧 Setting evaluation interval '%s' for group '%s'...\n", interval, groupName)

	for i := 0; i < maxRetries; i++ {
		if i > 0 { // Don't wait before first attempt, as we already waited in service
			time.Sleep(retryDelay)
		}

		err := c.updateGroupEvaluationInterval(groupName, folderUID, interval)
		if err == nil {
			fmt.Printf("   ✅ Successfully set interval '%s' for group '%s'\n", interval, groupName)
			return nil // Success!
		}

		// Check if it's a retryable error (HTTP 202 or "group not found yet")
		if strings.Contains(err.Error(), "status 202") ||
			err.Error() == fmt.Sprintf("group '%s' not found yet, still being created", groupName) {
			fmt.Printf("   ⏳ Attempt %d/%d: %s, waiting %v...\n",
				i+1, maxRetries, err.Error(), retryDelay)
			continue
		}

		// Non-retryable error
		fmt.Printf("   ❌ Non-retryable error: %v\n", err)
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
		return nil, fmt.Errorf("grafana API returned status %d", resp.StatusCode)
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
	// Get current rules for the group using official provisioning API
	url := c.baseURL + "/api/v1/provisioning/folder/" + folderUID + "/rule-groups/" + groupName
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Disable-Provenance", "true")

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

	// Read the raw JSON response first
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to get current rules (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse JSON into a generic map to preserve all fields
	var groupData map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &groupData); err != nil {
		return fmt.Errorf("failed to parse current group JSON: %v", err)
	}

	// Get current interval for logging
	currentInterval, _ := groupData["interval"].(string)

	// Convert string interval to seconds (int64)
	intervalSeconds, err := parseIntervalToSeconds(interval)
	if err != nil {
		return fmt.Errorf("failed to parse interval '%s': %v", interval, err)
	}

	fmt.Printf("   📊 Current group interval: %s -> setting to: %s (%d seconds)\n", currentInterval, interval, intervalSeconds)

	// Update the interval field with int64 value
	groupData["interval"] = intervalSeconds

	// Convert back to JSON
	updatedJSON, err := json.Marshal(groupData)
	if err != nil {
		return fmt.Errorf("failed to marshal updated group: %v", err)
	}

	// Send PUT request to update the group
	putReq, err := http.NewRequest("PUT", url, bytes.NewBuffer(updatedJSON))
	if err != nil {
		return err
	}
	putReq.Header.Set("Authorization", "Bearer "+c.token)
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("X-Disable-Provenance", "true")

	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		return err
	}
	defer putResp.Body.Close()

	// Handle HTTP 202 status - operation accepted, but async
	if putResp.StatusCode == 202 {
		return fmt.Errorf("group update still being processed (status 202), need to wait")
	}

	if putResp.StatusCode != 200 && putResp.StatusCode != 201 {
		bodyBytes, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("failed to update rule group via PUT (status %d): %s", putResp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (c *GrafanaClient) GetFolders() ([]models.Folder, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/folders", nil)
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
		return nil, fmt.Errorf("grafana API returned status %d", resp.StatusCode)
	}

	var folders []models.Folder
	if err := json.NewDecoder(resp.Body).Decode(&folders); err != nil {
		return nil, err
	}

	return folders, nil
}

func (c *GrafanaClient) GetGroupDetails(groupName, folderUID string) (*models.RuleGroup, error) {
	// Get specific group details using Ruler API
	url := c.baseURL + "/api/ruler/grafana/api/v1/rules/" + folderUID + "/" + groupName
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("group '%s' not found", groupName)
	}

	// Handle status 202 (group still being created) but try to parse the response anyway
	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get group details (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var group models.RuleGroup
	if err := json.NewDecoder(resp.Body).Decode(&group); err != nil {
		return nil, fmt.Errorf("failed to parse group details: %v", err)
	}

	// If status was 202, log a warning
	if resp.StatusCode == 202 {
		fmt.Printf("⚠️  Group '%s' is still being created (status 202), but returning available data\n", groupName)
	}

	return &group, nil
}
