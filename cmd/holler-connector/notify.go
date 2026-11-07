package main

import (
	"context"
	"github.com/72olabs/holler-connectors/notify"
	"io"
)

func runNotify(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := commandFlags("notify", stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	result, err := notify.Run(ctx, stdin)
	if err != nil {
		return err
	}
	return writeJSON(stdout, result)
}
