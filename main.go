package main

import (
	"context"
	"log"
)

func main() {
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
