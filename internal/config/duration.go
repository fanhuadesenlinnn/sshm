package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Duration is a YAML duration using Go duration syntax plus a day suffix.
type Duration struct {
	time.Duration
}

// ParseHumanDuration parses a positive duration. In addition to
// time.ParseDuration syntax, a trailing d means 24 hours.
func ParseHumanDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("时长不能为空")
	}
	if strings.HasSuffix(value, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(value, "d"), 64)
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("无效时长 %q", value)
		}
		duration := time.Duration(days * float64(24*time.Hour))
		if duration <= 0 {
			return 0, fmt.Errorf("无效时长 %q", value)
		}
		return duration, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("无效时长 %q", value)
	}
	return duration, nil
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return fmt.Errorf("时长必须使用 30s、5m、7d 等格式")
	}
	duration, err := ParseHumanDuration(value)
	if err != nil {
		return err
	}
	d.Duration = duration
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	if d.Duration == 0 {
		return "", nil
	}
	if d.Duration%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d.Duration/(24*time.Hour))), nil
	}
	return d.String(), nil
}
