//go:build !linux && !darwin

package app

func diskUsage(path string) (*DiskUsage, error) { return nil, nil }
