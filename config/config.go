package config

import (
	"flag"
	"fmt"
	"os"
)

const MaxTitleLength = 25

type Config struct {
	GrafanaURL  string
	APIToken    string
	SearchName  string
	MaxTitleLen int
	ChangeMode  bool
	NewFiring   string
	NewPending  string
}

func ParseFlags() *Config {
	url := flag.String("url", os.Getenv("GRAFANA_URL"), "Grafana URL")
	token := flag.String("token", os.Getenv("GRAFANA_TOKEN"), "Grafana API token")
	name := flag.String("name", "", "Alert name substring to search")
	change := flag.Bool("change", false, "Enable change mode to update alerts")
	firing := flag.String("firing", "", "New keep_firing_for duration (e.g., 1m, 5m)")
	interval := flag.String("pending", "", "New pending (for) duration (e.g., 15m, 5m)")
	flag.Parse()

	return &Config{
		GrafanaURL:  *url,
		APIToken:    *token,
		SearchName:  *name,
		MaxTitleLen: MaxTitleLength,
		ChangeMode:  *change,
		NewFiring:   *firing,
		NewPending:  *interval,
	}
}

func (c *Config) Validate() error {
	if c.GrafanaURL == "" || c.APIToken == "" || c.SearchName == "" {
		return fmt.Errorf("url, token, and name are required (use flags or env)")
	}

	if c.ChangeMode && (c.NewFiring == "" && c.NewPending == "") {
		return fmt.Errorf("when using --change flag, at least one of --firing or --pending must be specified")
	}

	return nil
}
