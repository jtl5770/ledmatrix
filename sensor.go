package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type AmbientLightSensor struct {
	devicePath     string
	lastBrightness int
}

func NewAmbientLightSensor() (*AmbientLightSensor, error) {
	devices, err := filepath.Glob("/sys/bus/iio/devices/iio:device*")
	if err != nil {
		return nil, err
	}

	for _, dev := range devices {
		name, err := os.ReadFile(filepath.Join(dev, "name"))
		if err == nil && strings.TrimSpace(string(name)) == "als" {
			return &AmbientLightSensor{devicePath: dev, lastBrightness: -1}, nil
		}
	}

	return nil, fmt.Errorf("ambient light sensor (als) not found in /sys/bus/iio/devices/")
}

func (s *AmbientLightSensor) ReadRaw() (int, error) {
	data, err := os.ReadFile(filepath.Join(s.devicePath, "in_illuminance_raw"))
	if err != nil {
		return 0, err
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}

	return val, nil
}

// Simple mapping from raw sensor value to matrix brightness (2-150)
func (s *AmbientLightSensor) MapToBrightness(raw int) int {
	// Framework ALS typically goes from 0 to ~4000+
	// We want to scale this to 2-150 (minimum 2 so it's not totally off)

	// Adjusted constants for better indoor sensitivity
	const maxSensorValue = 2000.0 // Full brightness reached at a lower light level
	const minBrightness = 2.0
	const maxBrightness = 200.0

	var brightness float64
	if raw <= 0 {
		brightness = minBrightness
	} else {
		brightness = (float64(raw) / maxSensorValue) * maxBrightness
		if brightness < minBrightness {
			brightness = minBrightness
		}
		if brightness > maxBrightness {
			brightness = maxBrightness
		}
	}

	b := int(brightness)
	if b != s.lastBrightness {
		s.lastBrightness = b
	}
	return b
}
