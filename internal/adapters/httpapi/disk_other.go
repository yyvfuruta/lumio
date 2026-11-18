//go:build !linux && !darwin

package httpapi

func diskUsage(path string) (*DiskUsage, error) { return nil, nil }
