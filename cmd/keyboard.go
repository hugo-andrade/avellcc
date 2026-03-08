package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/hugo-andrade/avellcc/internal/config"
	"github.com/hugo-andrade/avellcc/internal/keyboard"
	"github.com/hugo-andrade/avellcc/internal/lightbar"
)

var (
	kbColor      string
	kbKey        string
	kbEffect     string
	kbSpeed      int
	kbSpeedSet   bool
	kbBrightness int
	kbBrightSet  bool
	kbOff        bool
	kbRestore    bool
	kbProfile    string
	kbVerbose    bool
)

var keyboardCmd = &cobra.Command{
	Use:               "keyboard [keys|calibrate|firmware]",
	Aliases:           []string{"kb"},
	Short:             "Control keyboard RGB LEDs",
	Args:              cobra.MaximumNArgs(1),
	RunE:              runKeyboard,
	SilenceUsage:      true,
	SilenceErrors:     true,
}

func init() {
	f := keyboardCmd.Flags()
	f.StringVarP(&kbColor, "color", "c", "", "Set color (hex, name, or R,G,B)")
	f.StringVarP(&kbKey, "key", "k", "", "Target a specific key")

	allEffects := allEffectNames()
	f.StringVarP(&kbEffect, "effect", "e", "", fmt.Sprintf("Set effect (%s)", strings.Join(allEffects, ", ")))
	f.IntVarP(&kbSpeed, "speed", "s", 3, "Effect speed (0-10)")
	f.IntVarP(&kbBrightness, "brightness", "b", 0, "Set brightness (0-10)")
	f.BoolVar(&kbOff, "off", false, "Turn off keyboard LEDs")
	f.BoolVar(&kbRestore, "restore", false, "Restore saved state")
	f.StringVarP(&kbProfile, "profile", "p", "", "Load a profile JSON file")
	f.BoolVarP(&kbVerbose, "verbose", "v", false, "Show grid positions (with keys action)")

	rootCmd.AddCommand(keyboardCmd)
}

func allEffectNames() []string {
	names := make(map[string]bool)
	for k := range keyboard.EffectNames {
		names[k] = true
	}
	for k := range keyboard.SoftwareEffects {
		names[k] = true
	}
	result := make([]string, 0, len(names))
	for k := range names {
		result = append(result, k)
	}
	return result
}

func runKeyboard(cmd *cobra.Command, args []string) error {
	kbSpeedSet = cmd.Flags().Changed("speed")
	kbBrightSet = cmd.Flags().Changed("brightness")

	// Determine action
	var action string
	if len(args) > 0 {
		action = args[0]
		switch action {
		case "keys", "calibrate", "firmware":
			// valid
		default:
			return fmt.Errorf("unknown action: %s (valid: keys, calibrate, firmware)", action)
		}
	}

	// Validate args
	if err := validateKeyboardArgs(action); err != nil {
		return err
	}

	// Sub-actions
	switch action {
	case "keys":
		return kbKeys()
	case "calibrate":
		return kbCalibrate()
	case "firmware":
		return kbFirmware()
	}

	// LED control
	ctrl := keyboard.NewITE8295(nil)
	if err := ctrl.Open(); err != nil {
		return err
	}
	defer ctrl.Close()

	bundle := config.LoadStateBundle()
	state := map[string]any{}

	if kbOff {
		if err := ctrl.Off(); err != nil {
			return err
		}
		state["mode"] = "off"
		bundle["keyboard"] = state
		config.SaveStateBundle(bundle)
		fmt.Println("Keyboard LEDs off.")
		return nil
	}

	if kbRestore {
		if len(bundle) == 0 {
			fmt.Println("No saved state found.")
			return nil
		}
		if err := restoreState(ctrl, bundle); err != nil {
			return err
		}
		fmt.Println("State restored.")
		return nil
	}

	applyBrightnessAfterProfile := kbBrightSet && kbProfile != ""

	if kbBrightSet && !applyBrightnessAfterProfile {
		if err := ctrl.SetBrightness(kbBrightness); err != nil {
			return err
		}
		state["brightness"] = float64(kbBrightness)
		fmt.Printf("Brightness set to %d.\n", kbBrightness)
	}

	if kbEffect != "" {
		speed := kbSpeed
		// Check if it's a hardware effect
		if animID, ok := keyboard.EffectNames[strings.ToLower(kbEffect)]; ok {
			if err := ctrl.SetHWAnimation(animID); err != nil {
				return err
			}
			state["mode"] = "effect"
			state["effect"] = kbEffect
			state["speed"] = float64(speed)
			fmt.Printf("Hardware effect '%s' activated.\n", kbEffect)
		} else {
			// Software effect
			swName := strings.ToLower(kbEffect)
			if !strings.HasPrefix(swName, "sw_") {
				swName = "sw_" + swName
			}
			fn, ok := keyboard.SoftwareEffects[swName]
			if !ok {
				// Try without sw_ prefix
				fn, ok = keyboard.SoftwareEffects[strings.ToLower(kbEffect)]
				if !ok {
					return fmt.Errorf("unknown effect '%s'; available: %s", kbEffect, strings.Join(allEffectNames(), ", "))
				}
				swName = strings.ToLower(kbEffect)
			}
			runner := keyboard.NewEffectRunner(ctrl, 30)
			opts := keyboard.DefaultEffectOpts()
			opts.Speed = speed
			runner.Start(fn, opts)
			state["mode"] = "effect"
			state["effect"] = kbEffect
			state["speed"] = float64(speed)
			bundle["keyboard"] = state
			config.SaveStateBundle(bundle)
			fmt.Printf("Software effect '%s' running (speed=%d). Press Ctrl+C to stop.\n", kbEffect, speed)
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
			<-sig
			runner.Stop()
			fmt.Println("\nEffect stopped.")
			return nil
		}
	} else if kbColor != "" {
		r, g, b, err := config.ParseColor(kbColor)
		if err != nil {
			return err
		}

		if kbKey != "" {
			keymap := keyboard.LoadKeymap()
			pos, ok := keyboard.GetKeyPosition(kbKey, keymap)
			if !ok {
				return fmt.Errorf("unknown key: '%s'; use 'avellcc keyboard keys' to list keys", kbKey)
			}
			if err := ctrl.SetKeyColor(pos[0], pos[1], r, g, b); err != nil {
				return err
			}
			perKey, _ := state["per_key"].(map[string]any)
			if perKey == nil {
				perKey = map[string]any{}
			}
			perKey[strings.ToLower(kbKey)] = []any{float64(r), float64(g), float64(b)}
			state["per_key"] = perKey
			fmt.Printf("Key '%s' set to (%d, %d, %d).\n", kbKey, r, g, b)
		} else {
			if err := ctrl.SetAllKeys(r, g, b); err != nil {
				return err
			}
			state["mode"] = "static"
			state["color"] = []any{float64(r), float64(g), float64(b)}
			fmt.Printf("All keys set to (%d, %d, %d).\n", r, g, b)
		}
	} else if kbProfile != "" {
		lbState, err := loadProfile(ctrl, kbProfile)
		if err != nil {
			return err
		}
		state["mode"] = "profile"
		state["profile"] = kbProfile
		if lbState != nil {
			bundle["lightbar"] = lbState
		}
		fmt.Printf("Profile '%s' loaded.\n", kbProfile)
	}

	if applyBrightnessAfterProfile {
		if err := ctrl.SetBrightness(kbBrightness); err != nil {
			return err
		}
		state["brightness"] = float64(kbBrightness)
		fmt.Printf("Brightness set to %d.\n", kbBrightness)
	}

	if len(state) > 0 {
		bundle["keyboard"] = state
		config.SaveStateBundle(bundle)
	}

	return nil
}

func validateKeyboardArgs(action string) error {
	hasFlags := kbColor != "" || kbKey != "" || kbEffect != "" || kbSpeedSet || kbBrightSet || kbOff || kbRestore || kbProfile != ""

	if action != "" {
		if action != "keys" && kbVerbose {
			return fmt.Errorf("--verbose is only valid with 'keyboard keys'")
		}
		if hasFlags {
			return fmt.Errorf("'keyboard %s' does not accept LED control flags", action)
		}
		return nil
	}

	if kbVerbose {
		return fmt.Errorf("--verbose is only valid with 'keyboard keys'")
	}
	if kbKey != "" && kbColor == "" {
		return fmt.Errorf("--key requires --color")
	}
	if kbSpeedSet && kbEffect == "" {
		return fmt.Errorf("--speed requires --effect")
	}
	if kbEffect != "" && kbColor != "" {
		return fmt.Errorf("choose either --effect or --color")
	}
	if kbOff && (kbRestore || kbColor != "" || kbKey != "" || kbEffect != "" || kbSpeedSet || kbBrightSet || kbProfile != "") {
		return fmt.Errorf("--off cannot be combined with other keyboard options")
	}
	if kbRestore && (kbOff || kbColor != "" || kbKey != "" || kbEffect != "" || kbSpeedSet || kbBrightSet || kbProfile != "") {
		return fmt.Errorf("--restore cannot be combined with other keyboard options")
	}
	if kbProfile != "" && (kbColor != "" || kbKey != "" || kbEffect != "" || kbSpeedSet || kbOff || kbRestore) {
		return fmt.Errorf("--profile can only be combined with --brightness")
	}
	if !hasFlags {
		return fmt.Errorf("choose a keyboard action or LED update")
	}
	return nil
}

func kbKeys() error {
	keymap := keyboard.LoadKeymap()
	keys := keyboard.ListKeys(keymap)
	if kbVerbose {
		for _, k := range keys {
			pos := keymap[k]
			fmt.Printf("  %-20s -> row=%d, col=%d\n", k, pos[0], pos[1])
		}
	} else {
		for i := 0; i < len(keys); i += 8 {
			end := i + 8
			if end > len(keys) {
				end = len(keys)
			}
			parts := make([]string, end-i)
			for j, k := range keys[i:end] {
				parts[j] = fmt.Sprintf("%-15s", k)
			}
			fmt.Println(strings.Join(parts, "  "))
		}
	}
	return nil
}

func kbCalibrate() error {
	ctrl := keyboard.NewITE8295(nil)
	if err := ctrl.Open(); err != nil {
		return err
	}
	defer ctrl.Close()

	keymap := map[string][2]int{}
	fmt.Println("=== Keyboard LED Calibration ===")
	fmt.Println("Each LED will light up RED one at a time.")
	fmt.Println("Type the key name (e.g., 'esc', 'a', 'f1') or press Enter to skip.")
	fmt.Println("Type 'q' to quit and save progress.")
	fmt.Println()

	ctrl.SetAllKeys(0, 0, 0)
	time.Sleep(500 * time.Millisecond)

outer:
	for row := 0; row < keyboard.GridRows; row++ {
		for col := 0; col < keyboard.GridCols; col++ {
			ctrl.SetKeyColor(row, col, 255, 0, 0)
			fmt.Printf("  LED (%d,%2d): ", row, col)

			var answer string
			_, err := fmt.Scanln(&answer)
			if err != nil {
				answer = ""
			}
			answer = strings.TrimSpace(strings.ToLower(answer))

			ctrl.SetKeyColor(row, col, 0, 0, 0)

			if answer == "q" {
				break outer
			}
			if answer != "" {
				keymap[answer] = [2]int{row, col}
				fmt.Printf("    -> mapped '%s' to (%d, %d)\n", answer, row, col)
			}
		}
	}

	if len(keymap) > 0 {
		if err := keyboard.SaveKeymap(keymap); err != nil {
			return err
		}
		fmt.Printf("\nSaved %d key mappings to %s\n", len(keymap), "~/.config/avellcc/keymap.json")
	} else {
		fmt.Println("\nNo keys mapped.")
	}
	return nil
}

func kbFirmware() error {
	ctrl := keyboard.NewITE8295(nil)
	if err := ctrl.Open(); err != nil {
		return err
	}
	defer ctrl.Close()

	data, err := ctrl.GetFirmwareInfo()
	if err != nil {
		return err
	}
	fmt.Printf("Firmware report 0x5A: %s\n", config.FormatHex(data))
	return nil
}

func restoreState(ctrl *keyboard.ITE8295, bundle map[string]any) error {
	kbState, _ := bundle["keyboard"].(map[string]any)
	if kbState == nil {
		kbState = map[string]any{}
	}

	mode, _ := kbState["mode"].(string)
	switch mode {
	case "off":
		ctrl.Off()
	case "effect":
		effect, _ := kbState["effect"].(string)
		if effect != "" {
			speed := 3
			if s, ok := config.GetInt(kbState, "speed"); ok {
				speed = s
			}
			if animID, ok := keyboard.EffectNames[strings.ToLower(effect)]; ok {
				ctrl.SetHWAnimation(animID)
			} else {
				swName := strings.ToLower(effect)
				if !strings.HasPrefix(swName, "sw_") {
					swName = "sw_" + swName
				}
				if fn, ok := keyboard.SoftwareEffects[swName]; ok {
					runner := keyboard.NewEffectRunner(ctrl, 30)
					opts := keyboard.DefaultEffectOpts()
					opts.Speed = speed
					runner.Start(fn, opts)
					// Runner keeps going in background
				}
			}
		}
	case "static":
		if colorArr, ok := kbState["color"].([]any); ok && len(colorArr) == 3 {
			r, _ := config.GetInt(map[string]any{"v": colorArr[0]}, "v")
			g, _ := config.GetInt(map[string]any{"v": colorArr[1]}, "v")
			b, _ := config.GetInt(map[string]any{"v": colorArr[2]}, "v")
			ctrl.SetAllKeys(byte(r), byte(g), byte(b))
		}
	case "profile":
		profilePath, _ := kbState["profile"].(string)
		if profilePath != "" {
			loadProfile(ctrl, profilePath)
		}
	}

	if brightness, ok := config.GetInt(kbState, "brightness"); ok {
		ctrl.SetBrightness(brightness)
	}

	// Restore per-key colors
	if perKey, ok := kbState["per_key"].(map[string]any); ok {
		keymap := keyboard.LoadKeymap()
		for keyName, colorVal := range perKey {
			if colorArr, ok := colorVal.([]any); ok && len(colorArr) == 3 {
				pos, found := keyboard.GetKeyPosition(keyName, keymap)
				if found {
					r, _ := config.GetInt(map[string]any{"v": colorArr[0]}, "v")
					g, _ := config.GetInt(map[string]any{"v": colorArr[1]}, "v")
					b, _ := config.GetInt(map[string]any{"v": colorArr[2]}, "v")
					ctrl.SetKeyColor(pos[0], pos[1], byte(r), byte(g), byte(b))
				}
			}
		}
	}

	// Restore lightbar
	if lbState, ok := bundle["lightbar"].(map[string]any); ok {
		restoreLightbarState(lbState, nil)
	}

	return nil
}

func restoreLightbarState(lbState map[string]any, ctrl *lightbar.ITE8911) error {
	if lbState == nil {
		return nil
	}

	state := config.MergeLightbarState(lbState, nil)
	ownsCtrl := ctrl == nil
	if ctrl == nil {
		ctrl = lightbar.NewITE8911(nil)
		if err := ctrl.Open(); err != nil {
			return err
		}
	}
	if ownsCtrl {
		defer ctrl.Close()
	}

	mode, _ := state["mode"].(string)
	if mode == "off" {
		return ctrl.X58Off()
	}

	var effectCode *byte
	var colorID *byte
	var brightness *int
	var speed *byte

	if ec, ok := config.GetInt(state, "effect_code"); ok {
		b := byte(ec)
		effectCode = &b
	}
	if ci, ok := config.GetInt(state, "color_id"); ok {
		b := byte(ci)
		colorID = &b
	}
	if br, ok := config.GetInt(state, "brightness"); ok {
		brightness = &br
	}
	if sp, ok := config.GetInt(state, "speed"); ok {
		b := byte(sp)
		speed = &b
	}

	return ctrl.X58Apply(effectCode, colorID, brightness, speed)
}

func loadProfile(ctrl *keyboard.ITE8295, profilePath string) (map[string]any, error) {
	profile, err := config.LoadProfile(profilePath)
	if err != nil {
		return nil, err
	}

	if brightness, ok := config.GetInt(profile, "brightness"); ok {
		ctrl.SetBrightness(brightness)
	}

	if effect, ok := profile["effect"].(string); ok {
		speed := 3
		if s, ok := config.GetInt(profile, "speed"); ok {
			speed = s
		}
		if animID, ok := keyboard.EffectNames[strings.ToLower(effect)]; ok {
			ctrl.SetHWAnimation(animID)
		} else {
			swName := strings.ToLower(effect)
			if !strings.HasPrefix(swName, "sw_") {
				swName = "sw_" + swName
			}
			if fn, ok := keyboard.SoftwareEffects[swName]; ok {
				runner := keyboard.NewEffectRunner(ctrl, 30)
				opts := keyboard.DefaultEffectOpts()
				opts.Speed = speed
				runner.Start(fn, opts)
			}
		}
	} else if colorVal, ok := profile["color"]; ok {
		var r, g, b byte
		switch c := colorVal.(type) {
		case string:
			r, g, b, err = config.ParseColor(c)
			if err != nil {
				return nil, err
			}
		case []any:
			if len(c) == 3 {
				rv, _ := config.GetInt(map[string]any{"v": c[0]}, "v")
				gv, _ := config.GetInt(map[string]any{"v": c[1]}, "v")
				bv, _ := config.GetInt(map[string]any{"v": c[2]}, "v")
				r, g, b = byte(rv), byte(gv), byte(bv)
			}
		}
		ctrl.SetAllKeys(r, g, b)
	}

	// Per-key colors
	if keysMap, ok := profile["keys"].(map[string]any); ok {
		keymap := keyboard.LoadKeymap()
		for keyName, colorVal := range keysMap {
			pos, found := keyboard.GetKeyPosition(keyName, keymap)
			if !found {
				continue
			}
			var r, g, b byte
			switch c := colorVal.(type) {
			case string:
				r, g, b, _ = config.ParseColor(c)
			case []any:
				if len(c) == 3 {
					rv, _ := config.GetInt(map[string]any{"v": c[0]}, "v")
					gv, _ := config.GetInt(map[string]any{"v": c[1]}, "v")
					bv, _ := config.GetInt(map[string]any{"v": c[2]}, "v")
					r, g, b = byte(rv), byte(gv), byte(bv)
				}
			}
			ctrl.SetKeyColor(pos[0], pos[1], r, g, b)
		}
	}

	// Lightbar section
	if lbRaw, ok := profile["lightbar"].(map[string]any); ok {
		mode, _ := lbRaw["mode"].(string)
		if mode == "off" {
			appliedState := map[string]any{"mode": "off"}
			restoreLightbarState(appliedState, nil)
			return appliedState, nil
		}

		var effectCode *byte
		var colorID *byte

		if effect, ok := lbRaw["effect"]; ok {
			ec, err := config.ParseLightbarEffect(fmt.Sprintf("%v", effect))
			if err == nil {
				effectCode = &ec
			}
		}
		if color, ok := lbRaw["color"]; ok {
			ci, err := config.ParseLightbarColor(fmt.Sprintf("%v", color))
			if err == nil {
				colorID = &ci
			}
		}

		updates := map[string]any{}
		if effectCode != nil {
			updates["effect_code"] = float64(*effectCode)
		}
		if colorID != nil {
			updates["color_id"] = float64(*colorID)
		}
		if br, ok := config.GetInt(lbRaw, "brightness"); ok {
			updates["brightness"] = float64(br)
		}
		if sp, ok := config.GetInt(lbRaw, "speed"); ok {
			updates["speed"] = float64(sp)
		}

		appliedState := config.MergeLightbarState(nil, updates)
		restoreLightbarState(appliedState, nil)
		return appliedState, nil
	}

	return nil, nil
}

// Helper to marshal profile data (used for JSON debugging)
func toJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}
