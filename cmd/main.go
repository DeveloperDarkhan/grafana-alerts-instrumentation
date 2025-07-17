package main

import (
	"log"

	"grafana-alerts-instrumentation/config"
	"grafana-alerts-instrumentation/service"
)

func main() {
	cfg := config.ParseFlags()

	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}

	alertService := service.NewAlertService(cfg)
	if err := alertService.SearchAlerts(); err != nil {
		log.Fatalf("Error searching alerts: %v", err)
	}
}
