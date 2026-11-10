package main

import (
	"context"
	"fmt"
	"os"

	"github.com/neko233-com/buildworld/internal/selfrelease"
)

func main() {
	if err := selfrelease.Run(context.Background(), os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "BuildWorld self-release:", err)
		os.Exit(1)
	}
}
