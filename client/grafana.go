package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("failed to parse alerts: %v", err)
	}
	return alerts, nil
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

// updateViaRulerAPI - update via Ruler API (preserves UI editability)
func (c *GrafanaClient) updateViaRulerAPI(alert models.AlertRule) error {
	// Try to use the internal API that Grafana UI uses
	url := fmt.Sprintf("%s/api/v1/rules", c.baseURL)

	// Create structure like for UI
	updateRequest := map[string]interface{}{
		"uid":             alert.UID,
		"title":           alert.Title,
		"condition":       alert.Condition,
		"data":            alert.Data,
		"intervalSeconds": 60, // default
		"noDataState":     alert.NoDataState,
		"execErrState":    alert.ExecErrState,
		"for":             alert.For,
		"ruleGroup":       alert.RuleGroup,
		"folderUID":       alert.FolderUID,
		"isPaused":        alert.IsPaused,
	}

	jsonData, err := json.Marshal(updateRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %v", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
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

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("rules API returned status %d", resp.StatusCode)
}

// updateViaAlertingAPI - update via standard alerting API
func (c *GrafanaClient) updateViaAlertingAPI(alert models.AlertRule) error {
	// Try to use the same API that UI uses
	// First get full alert information
	getURL := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s", c.baseURL, alert.UID)

	req, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to get current alert state: %d", resp.StatusCode)
	}

	var fullAlert map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&fullAlert); err != nil {
		return err
	}

	// Update only needed fields
	if alert.For != "" {
		fullAlert["for"] = alert.For
	}
	if alert.KeepFiringFor != "" {
		// For keep_firing_for might require a different field
		if annotations, ok := fullAlert["annotations"].(map[string]interface{}); ok {
			annotations["__keep_firing_for__"] = alert.KeepFiringFor
		}
	}

	// Try to update via UI API
	updateURL := fmt.Sprintf("%s/api/ruler/grafana/api/v1/rules/%s", c.baseURL, alert.RuleGroup)

	jsonData, err := json.Marshal(fullAlert)
	if err != nil {
		return err
	}

	req, err = http.NewRequest("POST", updateURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("UI-like API returned status %d", resp.StatusCode)
}

// updateViaProvisioningAPIWithDisabledProvenance - update via provisioning API preserving UI editability
func (c *GrafanaClient) updateViaProvisioningAPIWithDisabledProvenance(alert models.AlertRule) error {
	url := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s", c.baseURL, alert.UID)

	jsonData, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %v", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	// MAGIC HEADER! Preserves UI editability
	req.Header.Set("X-Disable-Provenance", "true")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("API returned status %d", resp.StatusCode)
}

// updateViaProvisioningAPI - update via provisioning API (makes alert read-only)
func (c *GrafanaClient) updateViaProvisioningAPI(alert models.AlertRule) error {
	url := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s", c.baseURL, alert.UID)

	jsonData, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %v", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
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

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("provisioning API returned status %d", resp.StatusCode)
}

// restoreUIEditability - attempt to restore UI editability
func (c *GrafanaClient) restoreUIEditability(alert models.AlertRule) error {
	// Try to "reset" provisioned status using several methods:

	// Method 1: Remove provenance field via internal API
	if err := c.clearProvenance(alert.UID); err == nil {
		return nil
	}

	// Method 2: Try export-import via dashboard API
	if err := c.reimportAsNonProvisioned(alert); err == nil {
		return nil
	}

	// Method 3: Update via UI request emulation
	if err := c.updateViaUIEmulation(alert); err == nil {
		return nil
	}

	return fmt.Errorf("all methods to restore UI editability failed")
}

// clearProvenance - attempt to clear provenance status
func (c *GrafanaClient) clearProvenance(alertUID string) error {
	// Try to clear provenance via internal API
	url := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s/provenance", c.baseURL, alertUID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("failed to clear provenance: %d", resp.StatusCode)
}

// reimportAsNonProvisioned - attempt to reimport alert as non-provisioned
func (c *GrafanaClient) reimportAsNonProvisioned(alert models.AlertRule) error {
	// Get full alert data
	getURL := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s", c.baseURL, alert.UID)

	req, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to get alert: %d", resp.StatusCode)
	}

	var fullAlert map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&fullAlert); err != nil {
		return err
	}

	// Remove provenance-related fields
	delete(fullAlert, "provenance")
	delete(fullAlert, "id") // Remove ID for recreation

	// Create new alert via UI API
	createURL := fmt.Sprintf("%s/api/v1/rules", c.baseURL)

	jsonData, err := json.Marshal(fullAlert)
	if err != nil {
		return err
	}

	req, err = http.NewRequest("POST", createURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// If creation was successful, delete old provisioned alert
		c.deleteProvisionedAlert(alert.UID)
		return nil
	}

	return fmt.Errorf("failed to recreate alert: %d", resp.StatusCode)
}

// updateViaUIEmulation - update via full UI emulation
func (c *GrafanaClient) updateViaUIEmulation(alert models.AlertRule) error {
	// Try to use exactly the same headers and structure as UI
	url := fmt.Sprintf("%s/api/ruler/grafana/api/v1/rules", c.baseURL)

	// Create structure like in UI without provisioning fields
	uiRequest := map[string]interface{}{
		"uid":          alert.UID,
		"title":        alert.Title,
		"condition":    alert.Condition,
		"data":         alert.Data,
		"noDataState":  alert.NoDataState,
		"execErrState": alert.ExecErrState,
		"for":          alert.For,
		"folderUID":    alert.FolderUID,
		"ruleGroup":    alert.RuleGroup,
		"isPaused":     alert.IsPaused,
	}

	jsonData, err := json.Marshal(uiRequest)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	// Emulate headers from UI
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Grafana-Org-Id", "1")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", c.baseURL+"/alerting/list")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("UI emulation failed: %d", resp.StatusCode)
}

// deleteProvisionedAlert - delete old provisioned alert
func (c *GrafanaClient) deleteProvisionedAlert(alertUID string) error {
	url := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s", c.baseURL, alertUID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil // Ignore deletion errors
}

// ExportAlertAsYAML - export alert in YAML format
func (c *GrafanaClient) ExportAlertAsYAML(alertUID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s/export?format=yaml", c.baseURL, alertUID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/yaml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to export alert %s: status %d", alertUID, resp.StatusCode)
	}

	yamlData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return yamlData, nil
}
