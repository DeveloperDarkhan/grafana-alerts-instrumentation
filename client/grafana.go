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
	// Попробуем альтернативный эндпоинт
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

	var groups []models.RuleGroup
	if err := json.NewDecoder(resp.Body).Decode(&groups); err != nil {
		return nil, fmt.Errorf("failed to parse rule groups: %v", err)
	}
	return groups, nil
}

func (c *GrafanaClient) UpdateAlert(alert models.AlertRule) error {
	// Всегда используем заголовок X-Disable-Provenance: true для сохранения UI-редактируемости
	return c.updateViaProvisioningAPIWithDisabledProvenance(alert)
}

// updateViaRulerAPI - обновление через Ruler API (сохраняет UI-редактируемость)
func (c *GrafanaClient) updateViaRulerAPI(alert models.AlertRule) error {
	// Попробуем использовать внутренний API, который использует UI Grafana
	url := fmt.Sprintf("%s/api/v1/rules", c.baseURL)

	// Создаем структуру как для UI
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

// updateViaAlertingAPI - обновление через стандартный alerting API
func (c *GrafanaClient) updateViaAlertingAPI(alert models.AlertRule) error {
	// Попробуем использовать тот же API, что использует UI
	// Сначала получим полную информацию об алерте
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

	// Обновляем только нужные поля
	if alert.For != "" {
		fullAlert["for"] = alert.For
	}
	if alert.KeepFiringFor != "" {
		// Для keep_firing_for может потребоваться другое поле
		if annotations, ok := fullAlert["annotations"].(map[string]interface{}); ok {
			annotations["__keep_firing_for__"] = alert.KeepFiringFor
		}
	}

	// Пробуем обновить через UI API
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

// updateViaProvisioningAPIWithDisabledProvenance - обновление через provisioning API с сохранением UI-редактируемости
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

	// МАГИЧЕСКИЙ ЗАГОЛОВОК! Сохраняет UI-редактируемость
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

// updateViaProvisioningAPI - обновление через provisioning API (делает алерт read-only)
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

// restoreUIEditability - попытка восстановить возможность редактирования в UI
func (c *GrafanaClient) restoreUIEditability(alert models.AlertRule) error {
	// Пытаемся "сбросить" статус provisioned несколькими способами:

	// Способ 1: Удаляем поле provenance через внутренний API
	if err := c.clearProvenance(alert.UID); err == nil {
		return nil
	}

	// Способ 2: Попробуем экспорт-импорт через dashboard API
	if err := c.reimportAsNonProvisioned(alert); err == nil {
		return nil
	}

	// Способ 3: Обновление через имитацию UI запроса
	if err := c.updateViaUIEmulation(alert); err == nil {
		return nil
	}

	return fmt.Errorf("all methods to restore UI editability failed")
}

// clearProvenance - попытка очистить статус provenance
func (c *GrafanaClient) clearProvenance(alertUID string) error {
	// Пробуем очистить provenance через внутренний API
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

// reimportAsNonProvisioned - попытка переимпорта алерта как не-provisioned
func (c *GrafanaClient) reimportAsNonProvisioned(alert models.AlertRule) error {
	// Получаем полные данные алерта
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

	// Удаляем поля, связанные с provenance
	delete(fullAlert, "provenance")
	delete(fullAlert, "id") // Удаляем ID для пересоздания

	// Создаем новый алерт через UI API
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
		// Если создание прошло успешно, удаляем старый provisioned алерт
		c.deleteProvisionedAlert(alert.UID)
		return nil
	}

	return fmt.Errorf("failed to recreate alert: %d", resp.StatusCode)
}

// updateViaUIEmulation - обновление через полную имитацию UI
func (c *GrafanaClient) updateViaUIEmulation(alert models.AlertRule) error {
	// Пытаемся использовать точно такие же заголовки и структуру, как UI
	url := fmt.Sprintf("%s/api/ruler/grafana/api/v1/rules", c.baseURL)

	// Создаем структуру как в UI без provisioning полей
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

	// Имитируем заголовки из UI
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

// deleteProvisionedAlert - удаление старого provisioned алерта
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

	return nil // Игнорируем ошибки удаления
}

// ExportAlertAsYAML - экспорт алерта в YAML формате
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
