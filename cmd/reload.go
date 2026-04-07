package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hugo-andrade/avellcc/internal/config"
	"github.com/hugo-andrade/avellcc/internal/keyboard"
	"github.com/hugo-andrade/avellcc/internal/lightbar"
)

var reloadCmd = &cobra.Command{
	Use:           "reload",
	Short:         "Reload saved keyboard and lightbar state",
	Args:          cobra.NoArgs,
	RunE:          runReload,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(reloadCmd)
}

func runReload(cmd *cobra.Command, args []string) error {
	bundle := config.LoadStateBundle()
	if len(bundle) == 0 {
		fmt.Println("No saved state found.")
		return nil
	}

	reloaded := false

	// Keyboard
	if kbState, ok := bundle["keyboard"].(map[string]any); ok && len(kbState) > 0 {
		ctrl := keyboard.NewITE8295(nil)
		if err := ctrl.Open(); err != nil {
			fmt.Printf("Keyboard: %v, skipping.\n", err)
		} else {
			reloadKeyboardState(ctrl, kbState)
			_ = ctrl.Close()
			fmt.Println("Keyboard reloaded.")
			reloaded = true
		}
	}

	// Lightbar
	if lbState, ok := bundle["lightbar"].(map[string]any); ok && len(lbState) > 0 {
		if err := restoreLightbarState(lbState, nil); err != nil {
			fmt.Printf("Lightbar: %v, skipping.\n", err)
		} else {
			fmt.Println("Lightbar reloaded.")
			reloaded = true
		}
	}

	if !reloaded {
		fmt.Println("No saved state found.")
	}

	return nil
}

func reloadKeyboardState(ctrl *keyboard.ITE8295, kbState map[string]any) {
	mode, _ := kbState["mode"].(string)
	switch mode {
	case "off":
		_ = ctrl.Off()
	case "effect":
		effect, _ := kbState["effect"].(string)
		if effect != "" {
			speed := 3
			if s, ok := config.GetInt(kbState, "speed"); ok {
				speed = s
			}
			if animID, ok := keyboard.EffectNames[strings.ToLower(effect)]; ok {
				_ = ctrl.SetHWAnimation(animID)
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
		}
	case "static":
		if colorArr, ok := kbState["color"].([]any); ok && len(colorArr) == 3 {
			r, _ := config.GetInt(map[string]any{"v": colorArr[0]}, "v")
			g, _ := config.GetInt(map[string]any{"v": colorArr[1]}, "v")
			b, _ := config.GetInt(map[string]any{"v": colorArr[2]}, "v")
			_ = ctrl.SetAllKeys(byte(r), byte(g), byte(b))
		}
	case "profile":
		profilePath, _ := kbState["profile"].(string)
		if profilePath != "" {
			_, _ = loadProfile(ctrl, profilePath)
		}
	}

	if brightness, ok := config.GetInt(kbState, "brightness"); ok {
		_ = ctrl.SetBrightness(brightness)
	}

	if perKey, ok := kbState["per_key"].(map[string]any); ok {
		keymap := keyboard.LoadKeymap()
		for keyName, colorVal := range perKey {
			if colorArr, ok := colorVal.([]any); ok && len(colorArr) == 3 {
				pos, found := keyboard.GetKeyPosition(keyName, keymap)
				if found {
					r, _ := config.GetInt(map[string]any{"v": colorArr[0]}, "v")
					g, _ := config.GetInt(map[string]any{"v": colorArr[1]}, "v")
					b, _ := config.GetInt(map[string]any{"v": colorArr[2]}, "v")
					_ = ctrl.SetKeyColor(pos[0], pos[1], byte(r), byte(g), byte(b))
				}
			}
		}
	}
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
		defer func() { _ = ctrl.Close() }()
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
