// Package config handles state persistence, color parsing, and profile management.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hugo-andrade/avellcc/internal/lightbar"
)

func ConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "avellcc")
}

func StateFile() string {
	return filepath.Join(ConfigDir(), "state.json")
}

// --- Color parsing ---

var namedColors = map[string][3]byte{
	"red":     {255, 0, 0},
	"green":   {0, 255, 0},
	"blue":    {0, 0, 255},
	"white":   {255, 255, 255},
	"black":   {0, 0, 0},
	"yellow":  {255, 255, 0},
	"cyan":    {0, 255, 255},
	"magenta": {255, 0, 255},
	"orange":  {255, 128, 0},
	"purple":  {128, 0, 255},
	"pink":    {255, 100, 200},
}

// ParseColor parses a color string into R, G, B.
// Supported: named colors, #RRGGBB, RRGGBB, R,G,B.
func ParseColor(s string) (r, g, b byte, err error) {
	s = strings.TrimSpace(strings.ToLower(s))

	if rgb, ok := namedColors[s]; ok {
		return rgb[0], rgb[1], rgb[2], nil
	}

	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		if len(parts) == 3 {
			rv, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			gv, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			bv, e3 := strconv.Atoi(strings.TrimSpace(parts[2]))
			if e1 != nil || e2 != nil || e3 != nil {
				return 0, 0, 0, fmt.Errorf("cannot parse color: '%s'", s)
			}
			return byte(rv & 0xFF), byte(gv & 0xFF), byte(bv & 0xFF), nil
		}
	}

	hex := strings.TrimPrefix(s, "#")
	if len(hex) == 6 {
		rv, err := strconv.ParseUint(hex[0:2], 16, 8)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("cannot parse color: '%s'", s)
		}
		gv, err := strconv.ParseUint(hex[2:4], 16, 8)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("cannot parse color: '%s'", s)
		}
		bv, err := strconv.ParseUint(hex[4:6], 16, 8)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("cannot parse color: '%s'", s)
		}
		return byte(rv), byte(gv), byte(bv), nil
	}

	return 0, 0, 0, fmt.Errorf("cannot parse color: '%s'", s)
}

// ParseByte parses a single byte in decimal or hex notation.
func ParseByte(s string) (byte, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	base := 10
	if strings.HasPrefix(s, "0x") {
		base = 16
		s = s[2:]
	} else {
		for _, c := range s {
			if (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
				base = 16
				break
			}
		}
	}
	val, err := strconv.ParseUint(s, base, 8)
	if err != nil {
		return 0, fmt.Errorf("byte out of range: %s", s)
	}
	return byte(val), nil
}

// ParseBytes parses space/comma-separated bytes.
func ParseBytes(spec string) ([]byte, error) {
	if spec == "" {
		return nil, nil
	}
	re := regexp.MustCompile(`[\s,;:]+`)
	parts := re.Split(strings.TrimSpace(spec), -1)
	var result []byte
	for _, p := range parts {
		if p == "" {
			continue
		}
		b, err := ParseByte(p)
		if err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, nil
}

// NormalizeName normalizes effect/color names.
func NormalizeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	re := regexp.MustCompile(`[\s_]+`)
	return re.ReplaceAllString(s, "-")
}

// ParseLightbarEffect parses effect name or hex code.
func ParseLightbarEffect(value string) (byte, error) {
	s := NormalizeName(value)
	aliases := map[string]string{
		"changecolor": "change-color",
		"colorwave":   "color-wave",
	}
	if aliased, ok := aliases[s]; ok {
		s = aliased
	}
	if code, ok := lightbar.X58EffectCodes[s]; ok {
		return code, nil
	}
	return ParseByte(value)
}

// ParseLightbarColor parses color name, hex, or raw ID.
func ParseLightbarColor(value string) (byte, error) {
	s := NormalizeName(value)
	aliases := map[string]string{
		"yellow-green": "lime",
		"chartreuse":   "lime",
		"violet":       "purple",
	}
	if aliased, ok := aliases[s]; ok {
		s = aliased
	}
	if id, ok := lightbar.X58ColorIDs[s]; ok {
		return id, nil
	}

	hexVal := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(value)), "#")
	if len(hexVal) == 8 {
		hexVal = hexVal[2:]
	}
	if len(hexVal) == 6 {
		for colorID, knownHex := range lightbar.X58ColorHex {
			if strings.TrimPrefix(knownHex, "#") == hexVal {
				return colorID, nil
			}
		}
	}

	return ParseByte(value)
}

// FormatHex formats bytes as space-separated hex.
func FormatHex(data []byte) string {
	parts := make([]string, len(data))
	for i, b := range data {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, " ")
}

// --- State persistence ---

// StateBundle holds keyboard and lightbar state.
type StateBundle struct {
	Keyboard map[string]any `json:"keyboard,omitempty"`
	Lightbar map[string]any `json:"lightbar,omitempty"`
}

// LoadState loads the raw state file.
func LoadState() (map[string]any, error) {
	data, err := os.ReadFile(StateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}

// SaveState saves state to the config file.
func SaveState(state map[string]any) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StateFile(), data, 0644)
}

// LoadStateBundle loads the state bundle with backward compatibility.
func LoadStateBundle() map[string]any {
	raw, err := LoadState()
	if err != nil || raw == nil {
		return map[string]any{}
	}
	if _, ok := raw["keyboard"]; ok {
		return raw
	}
	if _, ok := raw["lightbar"]; ok {
		return raw
	}
	// Backward compat: keyboard-only schema
	return map[string]any{"keyboard": raw}
}

// SaveStateBundle saves the bundle.
func SaveStateBundle(bundle map[string]any) error {
	state := map[string]any{}
	if kb, ok := bundle["keyboard"]; ok && kb != nil {
		state["keyboard"] = kb
	}
	if lb, ok := bundle["lightbar"]; ok && lb != nil {
		state["lightbar"] = lb
	}
	return SaveState(state)
}

// DefaultLightbarState returns the default lightbar state.
func DefaultLightbarState() map[string]any {
	return map[string]any{
		"mode":        "active",
		"effect":      lightbar.X58DefaultEffect,
		"effect_code": float64(lightbar.X58DefaultEffectCode),
		"color_id":    float64(lightbar.X58DefaultColorID),
		"brightness":  float64(lightbar.X58DefaultBrightness),
		"speed":       float64(lightbar.X58DefaultSpeed),
	}
}

// MergeLightbarState merges saved state with updates.
func MergeLightbarState(state map[string]any, updates map[string]any) map[string]any {
	merged := DefaultLightbarState()
	for k, v := range state {
		merged[k] = v
	}
	for k, v := range updates {
		if v != nil {
			merged[k] = v
		}
	}
	// Sync effect ↔ effect_code
	if effect, ok := merged["effect"].(string); ok {
		if code, ok := lightbar.X58EffectCodes[effect]; ok {
			merged["effect_code"] = float64(code)
		}
	}
	if code, ok := toFloat(merged["effect_code"]); ok {
		if name, ok := lightbar.X58EffectNames[byte(code)]; ok {
			if _, hasEffect := merged["effect"]; !hasEffect || merged["effect"] == "?" {
				merged["effect"] = name
			}
		}
	}
	return merged
}

func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case byte:
		return float64(val), true
	}
	return 0, false
}

// GetInt extracts an int from a state map (handles JSON float64).
func GetInt(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	f, ok := toFloat(v)
	return int(f), ok
}

// GetString extracts a string from a state map.
func GetString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SaveLightbarState saves only the lightbar portion.
func SaveLightbarState(lightbarState map[string]any) error {
	bundle := LoadStateBundle()
	if lightbarState != nil {
		bundle["lightbar"] = lightbarState
	} else {
		delete(bundle, "lightbar")
	}
	return SaveStateBundle(bundle)
}

// LoadProfile loads a profile JSON file and applies it.
func LoadProfile(profilePath string) (map[string]any, error) {
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		configPath := filepath.Join(ConfigDir(), "profiles", profilePath)
		if _, err := os.Stat(configPath); err == nil {
			profilePath = configPath
		} else {
			return nil, fmt.Errorf("profile not found: %s", profilePath)
		}
	}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, err
	}
	var profile map[string]any
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}
	return profile, nil
}
