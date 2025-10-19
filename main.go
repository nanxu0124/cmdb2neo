package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"cmdb2neo/ioc"
)

func main() {
	env := flag.String("env", "local", "configuration environment: local|test|prod")
	configPath := flag.String("config", "", "path to configuration file (overrides -env)")
	flag.Parse()

	path, err := resolveConfigPath(*env, *configPath)
	if err != nil {
		log.Fatalf("resolve config path failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		log.Fatalf("config file not found: %s (%v)", path, err)
	}

	ioc.SetConfigPath(path)
	log.Printf("using config: %s", path)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app, cleanup, err := InitApp(ctx)
	if err != nil {
		log.Fatalf("init app failed: %v", err)
	}
	defer cleanup()

	if err := app.Run(ctx); err != nil {
		log.Fatalf("app run failed: %v", err)
	}
}

func resolveConfigPath(env, override string) (string, error) {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed, nil
	}
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "", "local":
		return "configs/config.local.yaml", nil
	case "test", "testing":
		return "configs/config.test.yaml", nil
	case "prod", "production", "online":
		return "configs/config.prod.yaml", nil
	default:
		return "", fmt.Errorf("unknown env %q", env)
	}
}
