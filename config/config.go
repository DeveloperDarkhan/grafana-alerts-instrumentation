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
	ListGroups   bool
	DownloadDir  string
	NewFiring    string
	NewPending   string
	NewGroup     string
	NewInterval  string
}

func ParseFlags() *Config {
	url := flag.String("url", os.Getenv("GRAFANA_URL"), "Grafana URL")
	token := flag.String("token", os.Getenv("GRAFANA_TOKEN"), "Grafana API token")
	name := flag.String("name", "", "Alert name substring to search")
	change := flag.Bool("change", false, "Enable change mode to update alerts")
	download := flag.Bool("download", false, "Download matched alerts as YAML files")
	readAll := flag.Bool("read-all", false, "Read and display all alerts without filtering")
	listGroups := flag.Bool("list-groups", false, "List all evaluation groups with their intervals and alert counts")
	downloadDir := flag.String("download-dir", "./downloads", "Directory to save downloaded alert YAML files")
	firing := flag.String("firing", "", "New keep_firing_for duration (e.g., 1m, 5m)")
	interval := flag.String("pending", "", "New pending (for) duration (e.g., 15m, 5m)")
	group := flag.String("group", "", "Move alert to specified evaluation group")
	groupInterval := flag.String("interval", "", "Set interval for new evaluation group (e.g., 300s, 5m)")
	flag.Parse()

	return &Config{
		GrafanaURL:   *url,
		APIToken:     *token,
		SearchName:   *name,
		MaxTitleLen:  MaxTitleLength,
		ChangeMode:   *change,
		DownloadMode: *download,
		ReadAllMode:  *readAll,
		ListGroups:   *listGroups,
		DownloadDir:  *downloadDir,
		NewFiring:    *firing,
		NewPending:   *interval,
		NewGroup:     *group,
		NewInterval:  *groupInterval,
	}
}

func (c *Config) Validate() error {
	if c.GrafanaURL == "" || c.APIToken == "" {
		return fmt.Errorf("url and token are required (use flags or env)")
	}

	// SearchName обязателен только если не в режиме скачивания всех алертов, не в режиме чтения всех алертов, и не в режиме списка групп
	if !c.DownloadMode && !c.ReadAllMode && !c.ListGroups && c.SearchName == "" {
		return fmt.Errorf("name is required for search/change operations (use --name flag)")
	}

	// Для режима скачивания требуется либо --read-all, либо --name
	if c.DownloadMode && !c.ReadAllMode && c.SearchName == "" {
		return fmt.Errorf("download mode requires either --read-all flag or --name parameter")
	}

	if c.ChangeMode && (c.NewFiring == "" && c.NewPending == "" && c.NewGroup == "") {
		return fmt.Errorf("when using --change flag, at least one of --firing, --pending, or --group must be specified")
	}

	// Если указана новая группа с интервалом, проверяем что задан --change
	if c.NewInterval != "" && c.NewGroup == "" {
		return fmt.Errorf("--interval can only be used together with --group")
	}

	return nil
}
