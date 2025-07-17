package models

type AlertRule struct {
	ID            int           `json:"id,omitempty"`
	UID           string        `json:"uid"`
	Title         string        `json:"title"`
	For           string        `json:"for"`
	KeepFiringFor string        `json:"keep_firing_for"`
	RuleGroup     string        `json:"ruleGroup"`
	OrgID         int           `json:"orgID,omitempty"`
	FolderUID     string        `json:"folderUID,omitempty"`
	Condition     string        `json:"condition,omitempty"`
	Data          []interface{} `json:"data,omitempty"`
	NoDataState   string        `json:"noDataState,omitempty"`
	ExecErrState  string        `json:"execErrState,omitempty"`
	IsPaused      bool          `json:"isPaused,omitempty"`
}

type RuleGroup struct {
	Name      string      `json:"name"`
	OrgID     int         `json:"orgID"`
	FolderUID string      `json:"folderUID"`
	Interval  string      `json:"interval"`
	Rules     []AlertRule `json:"rules"`
}

type AlertGroup struct {
	Name     string      `json:"name"`
	Interval string      `json:"interval"`
	Rules    []AlertRule `json:"rules"`
}

type APIResponse struct {
	Groups []AlertGroup `json:"groups"`
}

// Structure for Prometheus API response
type PrometheusAPIResponse struct {
	Status string              `json:"status"`
	Data   PrometheusRulesData `json:"data"`
}

type PrometheusRulesData struct {
	Groups []PrometheusRuleGroup `json:"groups"`
}

type PrometheusRuleGroup struct {
	Name      string        `json:"name"`
	File      string        `json:"file"`
	FolderUID string        `json:"folderUid"`
	Interval  int           `json:"interval"` // in seconds
	Rules     []interface{} `json:"rules"`    // rules can be of different types
}
