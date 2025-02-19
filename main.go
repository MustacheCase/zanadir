package main

import (
	"errors"
	"os"

	"github.com/MustacheCase/zanadir/app"
	"github.com/MustacheCase/zanadir/logger"
	"github.com/MustacheCase/zanadir/types"
)

// main function
func main() {
	l := logger.NewLogger()
	if err := run(); err != nil {
		var exitError *types.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.Code)
		}

		var userErr *types.UserError
		if errors.As(err, &userErr) {
			l.Error("User error: %v", err)
		}

		l.Error("Scanner error: %v", err)
	}
}

func run() error {
	newApp := app.NewApp()
	if err := newApp.Execute(); err != nil {
		return err
	}
	return nil
}
