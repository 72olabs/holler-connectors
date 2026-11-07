//go:build !darwin && !linux

package codexqueue

import "os"

func ownedByDaemon(os.FileInfo) bool { return false }
