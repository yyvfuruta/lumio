package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yyvfuruta/lumio/internal/cli"
)

var version = "dev"
var commit = "none"
var date = "unknown"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Printf("lumio-cli %s (%s, %s)\n", version, commit, date)
		return
	}
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"help"}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.Run(ctx, args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "lumio-cli:", err)
		os.Exit(1)
	}
}
