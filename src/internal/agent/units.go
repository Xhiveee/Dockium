package agent

import (
	"strconv"
	"strings"
	"time"
)

func parsePercent(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "%"))
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || number < 0 {
		return 0
	}
	return number
}

func parseHumanSize(value string) uint64 {
	return parseHumanSizeBytes(value)
}

func parseHumanSizeBytes(value string) uint64 {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "n/a") {
		return 0
	}
	units := []struct {
		suffix string
		factor float64
	}{
		{"TiB", 1024 * 1024 * 1024 * 1024},
		{"GiB", 1024 * 1024 * 1024},
		{"MiB", 1024 * 1024},
		{"KiB", 1024},
		{"TB", 1000 * 1000 * 1000 * 1000},
		{"GB", 1000 * 1000 * 1000},
		{"MB", 1000 * 1000},
		{"kB", 1000},
		{"KB", 1000},
		{"B", 1},
	}
	for _, unit := range units {
		if strings.HasSuffix(value, unit.suffix) {
			number := strings.TrimSpace(strings.TrimSuffix(value, unit.suffix))
			parsed, err := strconv.ParseFloat(number, 64)
			if err != nil || parsed < 0 {
				return 0
			}
			return uint64(parsed * unit.factor)
		}
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || number < 0 {
		return 0
	}
	return uint64(number)
}

func parseDockerTime(value string) int64 {
	value = strings.TrimSpace(value)
	layouts := []string{
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().UnixMilli()
		}
	}
	return 0
}

func parseRFC3339Millis(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0
	}
	millis := parsed.UTC().UnixMilli()
	if millis < 0 {
		return 0
	}
	return millis
}
