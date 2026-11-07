//go:build darwin || linux

package codexqueue

import (
	"os"
	"syscall"
)

func ownedByDaemon(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}
