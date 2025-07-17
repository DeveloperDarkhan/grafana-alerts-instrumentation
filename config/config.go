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
}

func ParseFlags() *Config {
	url := flag.String("url", os.Getenv("GRAFANA_URL"), "Grafana URL")
	token := flag.String("token", os.Getenv("GRAFANA_TOKEN"), "Grafana API token")
	name := flag.String("name", "", "Alert name substring to search")
	flag.Parse()

	return &Config{
		GrafanaURL:  *url,
		APIToken:    *token,
		SearchName:  *name,
		MaxTitleLen: MaxTitleLength,
	}
}

func (c *Config) Validate() error {
	if c.GrafanaURL == "" || c.APIToken == "" || c.SearchName == "" {
		return fmt.Errorf("url, token, and name are required (use flags or env)")
	}
	return nil
}
