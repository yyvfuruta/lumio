//go:build linux || darwin

package app

import "syscall"

func diskUsage(path string) (*DiskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}
	blockSize := uint64(stat.Bsize)
	return &DiskUsage{Total: stat.Blocks * blockSize, Used: (stat.Blocks - stat.Bfree) * blockSize, Available: stat.Bavail * blockSize}, nil
}
