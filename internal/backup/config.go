package backup

type Config struct {
	Repo       string   `json:"repo"`
	Remote     string   `json:"remote"`
	Identity   string   `json:"identity"`
	Recipients []string `json:"recipients"`
}

type Options struct {
	ConfigStore    *ConfigStore
	ConfigPath     string
	Repo           string
	Remote         string
	Identity       string
	Recipients     []string
	Push           bool
	SkipPull       bool
	AsyncPush      bool
	PushQueueLimit int
	Progress       func(format string, args ...any)
}

func DefaultConfig() Config {
	return Config{
		Repo:     "~/Projects/backup-gog",
		Identity: "~/.gog/age.key",
	}
}
