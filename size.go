package main

import (
	"fmt"
	"strconv"
	"strings"
)

func parseSize(s string) (int64, error) {
	trimmed := strings.TrimSpace(strings.ToUpper(s))
	if trimmed == "" {
		return 0, fmt.Errorf("empty size")
	}
	multiplier := int64(1)
	for _, suffix := range []struct {
		name string
		mul  int64
	}{
		{"KIB", 1024},
		{"KB", 1024},
		{"K", 1024},
		{"MIB", 1024 * 1024},
		{"MB", 1024 * 1024},
		{"M", 1024 * 1024},
		{"GIB", 1024 * 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"G", 1024 * 1024 * 1024},
	} {
		if strings.HasSuffix(trimmed, suffix.name) {
			multiplier = suffix.mul
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, suffix.name))
			break
		}
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("parse size %q: %w", s, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("size must be non-negative")
	}
	return int64(value * float64(multiplier)), nil
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for next := n / unit; next >= unit; next /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
