package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/hugo-andrade/avellcc/internal/config"
	"github.com/hugo-andrade/avellcc/internal/lightbar"
	"github.com/hugo-andrade/avellcc/internal/tui"
)

var (
	lbEffect     string
	lbColor      string
	lbBrightness int
	lbBrightSet  bool
	lbSpeed      int
	lbSpeedSet   bool
	lbOff        bool
	lbEffectCode string
	lbColorID    int
	lbColorIDSet bool

	// Debug flags
	lbDebugDescriptor  bool
	lbDebugGet         string
	lbDebugRaw         string
	lbDebugReport      string
	lbDebugCommand     string
	lbDebugData        string
	lbDebugFeatureSize string
)

var lightbarCmd = &cobra.Command{
	Use:           "lightbar",
	Aliases:       []string{"lb"},
	Short:         "Control rear lightbar",
	RunE:          runLightbar,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	effects := sortedKeys(lightbar.X58EffectCodes)
	colors := sortedKeys(lightbar.X58ColorIDs)

	f := lightbarCmd.Flags()
	f.StringVarP(&lbEffect, "effect", "e", "", fmt.Sprintf("Set effect (%s)", strings.Join(effects, ", ")))
	f.StringVarP(&lbColor, "color", "c", "", fmt.Sprintf("Set color (%s)", strings.Join(colors, ", ")))
	f.IntVarP(&lbBrightness, "brightness", "b", 0, "Set brightness (0-4)")
	f.IntVarP(&lbSpeed, "speed", "s", 0, "Set animation speed")
	f.BoolVar(&lbOff, "off", false, "Turn off lightbar")
	f.StringVar(&lbEffectCode, "effect-code", "", "Set effect by raw hex code")
	f.IntVar(&lbColorID, "color-id", 0, "Set color by raw ID")

	// Debug group
	f.BoolVar(&lbDebugDescriptor, "debug-descriptor", false, "Dump the HID report descriptor")
	f.StringVar(&lbDebugGet, "debug-get", "", "Read a HID feature report (e.g. 0x5A)")
	f.StringVar(&lbDebugRaw, "debug-raw", "", "Send raw payload bytes")
	f.StringVar(&lbDebugReport, "debug-report", "0xCD", "Report ID for --debug-raw (default: 0xCD)")
	f.StringVar(&lbDebugCommand, "debug-command", "", "Send a command byte on report 0xCD")
	f.StringVar(&lbDebugData, "debug-data", "", "Payload bytes for --debug-command")
	f.StringVar(&lbDebugFeatureSize, "debug-feature-size", fmt.Sprintf("%d", lightbar.DescriptorReportSize), "Feature frame size")

	rootCmd.AddCommand(lightbarCmd)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func runLightbar(cmd *cobra.Command, args []string) error {
	lbBrightSet = cmd.Flags().Changed("brightness")
	lbSpeedSet = cmd.Flags().Changed("speed")
	lbColorIDSet = cmd.Flags().Changed("color-id")

	// Validate
	if err := validateLightbarArgs(); err != nil {
		return err
	}

	// Resolve effect
	var effectCode *byte
	if lbEffect != "" {
		ec, err := config.ParseLightbarEffect(lbEffect)
		if err != nil {
			return err
		}
		effectCode = &ec
	} else if lbEffectCode != "" {
		ec, err := config.ParseByte(lbEffectCode)
		if err != nil {
			return err
		}
		effectCode = &ec
	}

	// Resolve color
	var colorID *byte
	if lbColor != "" {
		ci, err := config.ParseLightbarColor(lbColor)
		if err != nil {
			return err
		}
		colorID = &ci
	} else if lbColorIDSet {
		ci := byte(lbColorID)
		colorID = &ci
	}

	var brightness *int
	if lbBrightSet {
		brightness = &lbBrightness
	}
	var speed *byte
	if lbSpeedSet {
		s := byte(lbSpeed)
		speed = &s
	}

	hasUpdate := effectCode != nil || colorID != nil || brightness != nil || speed != nil

	ctrl := lightbar.NewITE8911(nil)
	if err := ctrl.Open(); err != nil {
		return err
	}
	defer func() { _ = ctrl.Close() }()

	if lbOff {
		if err := ctrl.X58Off(); err != nil {
			return err
		}
		_ = config.SaveLightbarState(map[string]any{"mode": actionOff})
		fmt.Println("Lightbar off.")
		return nil
	}

	if hasUpdate {
		currentBundle := config.LoadStateBundle()
		currentLB, _ := currentBundle["lightbar"].(map[string]any)
		if currentLB == nil {
			currentLB = map[string]any{}
		}

		updates := map[string]any{"mode": "active"}
		if effectCode != nil {
			updates["effect_code"] = float64(*effectCode)
		}
		if colorID != nil {
			updates["color_id"] = float64(*colorID)
		}
		if brightness != nil {
			updates["brightness"] = float64(*brightness)
		}
		if speed != nil {
			updates["speed"] = float64(*speed)
		}

		savedState := config.MergeLightbarState(currentLB, updates)

		// Apply full state
		var applyEffect, applyColor *byte
		var applyBrightness *int
		var applySpeed *byte

		if ec, ok := config.GetInt(savedState, "effect_code"); ok {
			b := byte(ec)
			applyEffect = &b
		}
		if ci, ok := config.GetInt(savedState, "color_id"); ok {
			b := byte(ci)
			applyColor = &b
		}
		if br, ok := config.GetInt(savedState, "brightness"); ok {
			applyBrightness = &br
		}
		if sp, ok := config.GetInt(savedState, "speed"); ok {
			b := byte(sp)
			applySpeed = &b
		}

		if err := ctrl.X58Apply(applyEffect, applyColor, applyBrightness, applySpeed); err != nil {
			return err
		}
		_ = config.SaveLightbarState(savedState)

		effectName, _ := savedState["effect"].(string)
		if effectName == "" {
			effectName = "?"
		}
		colorName := "?"
		if ci, ok := config.GetInt(savedState, "color_id"); ok {
			if name, ok := lightbar.X58ColorNames[byte(ci)]; ok {
				colorName = name
			}
		}
		br, _ := config.GetInt(savedState, "brightness")
		sp, _ := config.GetInt(savedState, "speed")
		fmt.Printf("Lightbar updated: effect=%s, color=%s, brightness=%d, speed=%d.\n", effectName, colorName, br, sp)
		return nil
	}

	// Debug actions
	if lbDebugDescriptor {
		data, err := ctrl.ReadReportDescriptor()
		if err != nil {
			return err
		}
		fmt.Printf("Descriptor (%d bytes): %s\n", len(data), config.FormatHex(data))
		return nil
	}

	if lbDebugGet != "" {
		featureSize, err := config.ParseByte(lbDebugFeatureSize)
		if err != nil {
			return err
		}
		reportID, err := config.ParseByte(lbDebugGet)
		if err != nil {
			return err
		}
		data, err := ctrl.GetFeature(reportID, int(featureSize))
		if err != nil {
			return err
		}
		fmt.Printf("Feature 0x%02x: %s\n", reportID, config.FormatHex(data))
		return nil
	}

	if lbDebugRaw != "" {
		featureSize, err := config.ParseByte(lbDebugFeatureSize)
		if err != nil {
			return err
		}
		reportID, err := config.ParseByte(lbDebugReport)
		if err != nil {
			return err
		}
		payload, err := config.ParseBytes(lbDebugRaw)
		if err != nil {
			return err
		}
		if err := ctrl.SendFeature(reportID, payload, int(featureSize)); err != nil {
			return err
		}
		fullReport := append([]byte{reportID}, payload...)
		fmt.Printf("Sent report 0x%02x: %s\n", reportID, config.FormatHex(fullReport))
		return nil
	}

	if lbDebugCommand != "" {
		featureSize, err := config.ParseByte(lbDebugFeatureSize)
		if err != nil {
			return err
		}
		cmdByte, err := config.ParseByte(lbDebugCommand)
		if err != nil {
			return err
		}
		payload, err := config.ParseBytes(lbDebugData)
		if err != nil {
			return err
		}
		if err := ctrl.SendCommand(cmdByte, payload, int(featureSize)); err != nil {
			return err
		}
		fullReport := append([]byte{lightbar.ReportIDCtrl, cmdByte}, payload...)
		fmt.Printf("Sent command 0x%02x: %s\n", cmdByte, config.FormatHex(fullReport))
		return nil
	}

	// Default: interactive TUI panel (or plain text if not a terminal)
	if _, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), unix.TCGETS); err != nil {
		lightbarStatus(ctrl)
		return nil
	}
	panel := tui.NewLightbarPanel(ctrl)
	p := tea.NewProgram(panel)
	_, err := p.Run()
	return err
}

func validateLightbarArgs() error {
	if lbEffect != "" && lbEffectCode != "" {
		return fmt.Errorf("choose either --effect or --effect-code")
	}
	if lbColor != "" && lbColorIDSet {
		return fmt.Errorf("choose either --color or --color-id")
	}
	if lbDebugData != "" && lbDebugCommand == "" {
		return fmt.Errorf("--debug-data requires --debug-command")
	}

	debugActions := 0
	if lbDebugDescriptor {
		debugActions++
	}
	if lbDebugGet != "" {
		debugActions++
	}
	if lbDebugRaw != "" {
		debugActions++
	}
	if lbDebugCommand != "" {
		debugActions++
	}
	if debugActions > 1 {
		return fmt.Errorf("choose only one lightbar debug action at a time")
	}

	hasUpdate := lbEffect != "" || lbEffectCode != "" || lbColor != "" || lbColorIDSet || lbBrightSet || lbSpeedSet
	if lbOff && (hasUpdate || debugActions > 0) {
		return fmt.Errorf("--off cannot be combined with other lightbar options")
	}
	if hasUpdate && debugActions > 0 {
		return fmt.Errorf("semantic lightbar options cannot be combined with --debug-* actions")
	}

	return nil
}

func lightbarStatus(ctrl *lightbar.ITE8911) {
	fmt.Printf("Device: %s\n", ctrl.Path())
	bundle := config.LoadStateBundle()
	lbState, _ := bundle["lightbar"].(map[string]any)
	if lbState == nil {
		lbState = map[string]any{}
	}
	mode, _ := lbState["mode"].(string)
	if mode == actionOff {
		fmt.Println("State: off")
	} else {
		effect, _ := lbState["effect"].(string)
		if effect == "" {
			effect = "?"
		}
		colorName := "?"
		if ci, ok := config.GetInt(lbState, "color_id"); ok {
			if name, ok := lightbar.X58ColorNames[byte(ci)]; ok {
				colorName = name
			}
		}
		br, _ := config.GetInt(lbState, "brightness")
		sp, _ := config.GetInt(lbState, "speed")
		fmt.Printf("State: effect=%s, color=%s, brightness=%d, speed=%d\n", effect, colorName, br, sp)
	}

	effects := sortedKeys(lightbar.X58EffectCodes)
	colors := sortedKeys(lightbar.X58ColorIDs)
	fmt.Printf("\nEffects: %s\n", strings.Join(effects, ", "))
	fmt.Printf("Colors:  %s\n", strings.Join(colors, ", "))
}
