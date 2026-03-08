// Package fan provides fan monitoring and control for Clevo/Avell laptops.
package fan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Backend represents the fan control backend.
type Backend string

const (
	BackendTuxedoIO Backend = "tuxedo_io"
	BackendACPICall Backend = "acpi_call"
	BackendHwmon    Backend = "hwmon"
	BackendNone     Backend = "none"
)

// TempReading holds a temperature sensor reading.
type TempReading struct {
	Name  string
	Label string
	Value float64
}

// FanStatus holds fan RPM and duty readings.
type FanStatus map[string]int

// FanController reads and controls fan speeds.
type FanController struct {
	backend Backend
}

// NewFanController creates a new controller with auto-detected backend.
func NewFanController() *FanController {
	return &FanController{backend: detectBackend()}
}

// Backend returns the detected backend name.
func (fc *FanController) Backend() Backend {
	return fc.backend
}

func detectBackend() Backend {
	if _, err := os.Stat("/dev/tuxedo_io"); err == nil {
		return BackendTuxedoIO
	}
	if _, err := os.Stat("/proc/acpi/call"); err == nil {
		return BackendACPICall
	}
	matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/fan*_input")
	if len(matches) > 0 {
		return BackendHwmon
	}
	return BackendNone
}

// GetFanRPM returns current fan RPMs and duty cycles.
func (fc *FanController) GetFanRPM() FanStatus {
	switch fc.backend {
	case BackendACPICall:
		return fc.acpiGetFans()
	case BackendHwmon:
		return fc.hwmonGetFans()
	case BackendTuxedoIO:
		return fc.hwmonGetFans()
	}
	return FanStatus{}
}

// GetTemperatures returns CPU and GPU temperature readings.
func (fc *FanController) GetTemperatures() []TempReading {
	skip := map[string]bool{
		"AC": true, "BAT0": true, "hidpp_battery_2": true,
		"ucsi_source_psy_USBC000:001": true, "ucsi_source_psy_USBC000:002": true,
		"acpi_fan": true,
	}

	var temps []TempReading
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	sort.Strings(hwmons)

	for _, hwmon := range hwmons {
		nameData, err := os.ReadFile(filepath.Join(hwmon, "name"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(nameData))
		if skip[name] {
			continue
		}

		inputs, _ := filepath.Glob(filepath.Join(hwmon, "temp*_input"))
		sort.Strings(inputs)

		for _, tempInput := range inputs {
			valData, err := os.ReadFile(tempInput)
			if err != nil {
				continue
			}
			val, err := strconv.Atoi(strings.TrimSpace(string(valData)))
			if err != nil {
				continue
			}

			labelPath := strings.Replace(tempInput, "_input", "_label", 1)
			var label string
			if labelData, err := os.ReadFile(labelPath); err == nil {
				label = strings.TrimSpace(string(labelData))
			} else {
				base := filepath.Base(tempInput)
				idx := strings.TrimSuffix(strings.TrimPrefix(base, "temp"), "_input")
				label = "temp" + idx
			}

			var key string
			if name == "coretemp" {
				key = label
			} else {
				hwmonIdx := strings.TrimPrefix(filepath.Base(hwmon), "hwmon")
				key = fmt.Sprintf("%s[%s]: %s", name, hwmonIdx, label)
			}

			temps = append(temps, TempReading{
				Name:  key,
				Label: label,
				Value: float64(val) / 1000.0,
			})
		}
	}
	return temps
}

// SetFanSpeed sets fan speed as percentage (0-100). fanID: 0=both, 1=fan1, 2=fan2.
func (fc *FanController) SetFanSpeed(fanID, dutyPercent int) error {
	duty := dutyPercent * 255 / 100
	if duty < 0 {
		duty = 0
	}
	if duty > 255 {
		duty = 255
	}
	switch fc.backend {
	case BackendACPICall:
		return fc.acpiSetFan(fanID, duty)
	default:
		return fmt.Errorf("backend '%s' does not support fan control; install acpi_call: sudo modprobe acpi_call", fc.backend)
	}
}

// SetAuto returns fans to automatic control.
func (fc *FanController) SetAuto() error {
	switch fc.backend {
	case BackendACPICall:
		_, err := acpiCall(0x69, 0x0F)
		return err
	default:
		return fmt.Errorf("backend '%s' does not support fan control", fc.backend)
	}
}

// --- acpi_call backend ---

func acpiCall(method, arg int) (int, error) {
	cmd := fmt.Sprintf("\\_SB.WMI.WMBB 0x00 %#x %#x", method, arg)
	if err := os.WriteFile("/proc/acpi/call", []byte(cmd), 0); err != nil {
		return 0, fmt.Errorf("acpi_call write: %w", err)
	}
	data, err := os.ReadFile("/proc/acpi/call")
	if err != nil {
		return 0, fmt.Errorf("acpi_call read: %w", err)
	}
	result := strings.TrimRight(strings.TrimSpace(string(data)), "\x00")
	if !strings.HasPrefix(result, "0x") {
		return 0, nil
	}
	val, err := strconv.ParseInt(strings.TrimPrefix(result, "0x"), 16, 64)
	if err != nil {
		return 0, fmt.Errorf("acpi_call parse '%s': %w", result, err)
	}
	return int(val), nil
}

func (fc *FanController) acpiGetFans() FanStatus {
	fans := FanStatus{}
	for fanNum, cmd := range map[int]int{1: 0x63, 2: 0x64} {
		raw, err := acpiCall(cmd, 0)
		if err != nil {
			continue
		}
		if raw != 0 {
			duty := raw & 0xFF
			fans[fmt.Sprintf("fan%d_duty", fanNum)] = duty
			fans[fmt.Sprintf("fan%d_duty_pct", fanNum)] = duty * 100 / 255
		}
	}
	// Supplement with hwmon RPM
	hwmonFans := fc.hwmonGetFans()
	for k, v := range hwmonFans {
		fans[k] = v
	}
	return fans
}

func (fc *FanController) acpiSetFan(fanID, duty int) error {
	var arg int
	if fanID == 0 {
		arg = (duty & 0xFF) | ((duty & 0xFF) << 8)
	} else {
		raw1, _ := acpiCall(0x63, 0)
		raw2, _ := acpiCall(0x64, 0)
		duty1 := raw1 & 0xFF
		duty2 := raw2 & 0xFF
		if duty1 == 0 {
			duty1 = duty
		}
		if duty2 == 0 {
			duty2 = duty
		}
		switch fanID {
		case 1:
			duty1 = duty
		case 2:
			duty2 = duty
		}
		arg = (duty1 & 0xFF) | ((duty2 & 0xFF) << 8)
	}
	_, err := acpiCall(0x68, arg)
	return err
}

// --- hwmon backend (read-only) ---

func (fc *FanController) hwmonGetFans() FanStatus {
	fans := FanStatus{}
	inputs, _ := filepath.Glob("/sys/class/hwmon/hwmon*/fan*_input")
	sort.Strings(inputs)
	fanIdx := 0
	for _, fanInput := range inputs {
		data, err := os.ReadFile(fanInput)
		if err != nil {
			continue
		}
		rpm, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			continue
		}
		fanIdx++
		fans[fmt.Sprintf("fan%d_rpm", fanIdx)] = rpm
	}
	return fans
}

// StatusReport generates a human-readable fan and temperature status report.
func StatusReport(fc *FanController) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Backend: %s", fc.backend))

	fans := fc.GetFanRPM()
	if len(fans) > 0 {
		lines = append(lines, "\nFans:")
		// Collect fan numbers
		fanNums := map[string]bool{}
		for k := range fans {
			if strings.HasPrefix(k, "fan") {
				parts := strings.SplitN(strings.TrimPrefix(k, "fan"), "_", 2)
				fanNums[parts[0]] = true
			}
		}
		sorted := make([]string, 0, len(fanNums))
		for n := range fanNums {
			sorted = append(sorted, n)
		}
		sort.Strings(sorted)
		for _, fn := range sorted {
			rpm, hasRPM := fans["fan"+fn+"_rpm"]
			dutyPct, hasDuty := fans["fan"+fn+"_duty_pct"]
			var parts []string
			if hasRPM {
				parts = append(parts, fmt.Sprintf("Fan %s: %d RPM", fn, rpm))
			} else {
				parts = append(parts, fmt.Sprintf("Fan %s: ? RPM", fn))
			}
			if hasDuty {
				parts = append(parts, fmt.Sprintf("(duty: %d%%)", dutyPct))
			}
			lines = append(lines, "  "+strings.Join(parts, "  "))
		}
	} else {
		lines = append(lines, "\nNo fan data available.")
	}

	temps := fc.GetTemperatures()
	if len(temps) > 0 {
		lines = append(lines, "\nTemperatures:")
		var coreTemps []float64
		for _, t := range temps {
			if strings.HasPrefix(t.Name, "Core ") {
				coreTemps = append(coreTemps, t.Value)
			} else {
				lines = append(lines, fmt.Sprintf("  %s: %.1f°C", t.Name, t.Value))
			}
		}
		if len(coreTemps) > 0 {
			min, max := coreTemps[0], coreTemps[0]
			for _, v := range coreTemps[1:] {
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
			lines = append(lines, fmt.Sprintf("  CPU Cores (%d): %.0f-%.0f°C", len(coreTemps), min, max))
		}
	}

	return strings.Join(lines, "\n")
}
