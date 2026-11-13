package cli

import (
	"context"
	"io"
)

// run is Run with the zero Options, which the command tests in this package
// need: they exercise only built-in commands.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Run(ctx, args, stdin, stdout, stderr, Options{})
}
