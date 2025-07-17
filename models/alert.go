package models

type AlertRule struct {
	UID           string `json:"uid"`
	Title         string `json:"title"`
	For           string `json:"for"`
	KeepFiringFor string `json:"keep_firing_for"`
	RuleGroup     string `json:"ruleGroup"`
}

type AlertGroup struct {
	Name     string      `json:"name"`
	Interval string      `json:"interval"`
	Rules    []AlertRule `json:"rules"`
}

type APIResponse struct {
	Groups []AlertGroup `json:"groups"`
}
