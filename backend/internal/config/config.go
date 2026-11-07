package config

import (
	"flag"
	"os"
	"time"
)

type Config struct {
	Addr        string
	DBPath      string
	DevURL      string
	StaticDir   string
	MLURL       string
	MLTimeout   time.Duration
	Workers     int
	MaxAttempts int
}

func Load(args []string) (Config, error) {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	var c Config
	fs.StringVar(&c.Addr, "addr", env("CQA_ADDR", "127.0.0.1:8080"), "listen address (CQA_ADDR)")
	fs.StringVar(&c.DBPath, "db", env("CQA_DB", "resources/cqa.db"), "SQLite file path (CQA_DB)")
	fs.StringVar(&c.DevURL, "dev-url", env("CQA_DEV_URL", ""), "signed URL of the DEV dataset (CQA_DEV_URL); never logged")
	fs.StringVar(&c.StaticDir, "static", env("CQA_STATIC", "../frontend"), "directory with the frontend to serve at / (CQA_STATIC)")
	fs.StringVar(&c.MLURL, "ml-url", env("CQA_ML_URL", ""), "ML evaluator endpoint (CQA_ML_URL); empty uses the stub evaluator")
	fs.DurationVar(&c.MLTimeout, "ml-timeout", envDuration("CQA_ML_TIMEOUT", 10*time.Minute), "timeout per ML call (CQA_ML_TIMEOUT)")
	fs.IntVar(&c.Workers, "workers", envInt("CQA_WORKERS", 1), "parallel evaluations (CQA_WORKERS)")
	fs.IntVar(&c.MaxAttempts, "max-attempts", envInt("CQA_MAX_ATTEMPTS", 3), "attempts per job before it is failed (CQA_MAX_ATTEMPTS)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(env(key, "")); err == nil {
		return d
	}
	return def
}

func envInt(key string, def int) int {
	n := 0
	for _, ch := range env(key, "") {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	if n == 0 {
		return def
	}
	return n
}
