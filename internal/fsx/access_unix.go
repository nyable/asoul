//go:build !windows

package fsx

// Unix mode bits are applied by AtomicWriteFile before the rename.
func preserveFileAccess(destination, source string) error { return nil }
