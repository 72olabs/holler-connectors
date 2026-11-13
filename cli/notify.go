package cli

import (
	"context"
	"github.com/72olabs/holler-connectors/notify"
	"io"
)

// Notify is the "notify" command: the wake-adapter entry point. It reads one
// notify.Request from stdin, delivers it with notify.Run and prints the
// notify.Result as JSON. A daemon started with this binary as its connector
// binary runs it for every wake it hands to a harness adapter.
func Notify(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
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
