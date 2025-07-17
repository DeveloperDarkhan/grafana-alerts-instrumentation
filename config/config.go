package config

import (
	"flag"
	"fmt"
	"os"
)

const MaxTitleLength = 25

type Config struct {
	GrafanaURL   string
	APIToken     string
	SearchName   string
	MaxTitleLen  int
	ChangeMode   bool
	DownloadMode bool
	ReadAllMode  bool
	DownloadDir  string
	NewFiring    string
	NewPending   string
}

func ParseFlags() *Config {
	url := flag.String("url", os.Getenv("GRAFANA_URL"), "Grafana URL")
	token := flag.String("token", os.Getenv("GRAFANA_TOKEN"), "Grafana API token")
	name := flag.String("name", "", "Alert name substring to search")
	change := flag.Bool("change", false, "Enable change mode to update alerts")
	download := flag.Bool("download", false, "Download matched alerts as YAML files")
	readAll := flag.Bool("read-all", false, "Read and display all alerts without filtering")
	downloadDir := flag.String("download-dir", "./downloads", "Directory to save downloaded alert YAML files")
	firing := flag.String("firing", "", "New keep_firing_for duration (e.g., 1m, 5m)")
	interval := flag.String("pending", "", "New pending (for) duration (e.g., 15m, 5m)")
	flag.Parse()

	return &Config{
		GrafanaURL:   *url,
		APIToken:     *token,
		SearchName:   *name,
		MaxTitleLen:  MaxTitleLength,
		ChangeMode:   *change,
		DownloadMode: *download,
		ReadAllMode:  *readAll,
		DownloadDir:  *downloadDir,
		NewFiring:    *firing,
		NewPending:   *interval,
	}
}

func (c *Config) Validate() error {
	if c.GrafanaURL == "" || c.APIToken == "" {
		return fmt.Errorf("url and token are required (use flags or env)")
	}

	// SearchName is required only if not in download all alerts mode and not in read all alerts mode
	if !c.DownloadMode && !c.ReadAllMode && c.SearchName == "" {
		return fmt.Errorf("name is required for search/change operations (use --name flag)")
	}

	// For download mode either --read-all or --name is required
	if c.DownloadMode && !c.ReadAllMode && c.SearchName == "" {
		return fmt.Errorf("download mode requires either --read-all flag or --name parameter")
	}

	if c.ChangeMode && (c.NewFiring == "" && c.NewPending == "") {
		return fmt.Errorf("when using --change flag, at least one of --firing or --pending must be specified")
	}

	return nil
}
