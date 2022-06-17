package gophermart

import "time"

type ArgonParams struct {
	Memory      uint32 `yaml:"memory"`
	Iterations  uint32 `yaml:"iterations"`
	Parallelism uint8  `yaml:"parallelism"`
	SaltLength  uint32 `yaml:"salt_length"`
	KeyLength   uint32 `yaml:"key_length"`
}

type Config struct {
	RunAddr           string        `yaml:"address" env:"RUN_ADDRESS"`
	DatabaseURI       string        `yaml:"database_uri" env:"DATABASE_URI"`
	AccrualSystemAddr string        `yaml:"accrual_address" env:"ACCRUAL_SYSTEM_ADDRESS"`
	CookieSecret      string        `yaml:"cookie_secret" env:"COOKIE_SECRET"`
	Argon             *ArgonParams  `yaml:"encryption"`
	PollInterval      time.Duration `yaml:"poll_interval" env:"POLL_INTERVAL"`
}

func NewConfig() *Config {
	return &Config{}
}
