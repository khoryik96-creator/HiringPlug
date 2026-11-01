// Command worker processes jobs on the "core" queue (the Python AI worker in
// ai/ processes the "ai" queue). Configuration comes from HP_* environment
// variables (see .env.example).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.RunWorker(ctx, os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}
