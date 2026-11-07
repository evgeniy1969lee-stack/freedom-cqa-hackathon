package config

import (
	"flag"
	"os"
)

type Config struct {
	Addr      string
	DBPath    string
	DevURL    string
	StaticDir string
}

func Load(args []string) (Config, error) {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	var c Config
	fs.StringVar(&c.Addr, "addr", env("CQA_ADDR", ":8080"), "listen address (CQA_ADDR)")
	fs.StringVar(&c.DBPath, "db", env("CQA_DB", "resources/cqa.db"), "SQLite file path (CQA_DB)")
	fs.StringVar(&c.DevURL, "dev-url", env("CQA_DEV_URL", ""), "signed URL of the DEV dataset (CQA_DEV_URL); never logged")
	fs.StringVar(&c.StaticDir, "static", env("CQA_STATIC", ""), "directory with the frontend to serve at / (CQA_STATIC)")
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
