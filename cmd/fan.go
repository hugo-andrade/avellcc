package cmd

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/hugo-andrade/avellcc/internal/fan"
	"github.com/hugo-andrade/avellcc/internal/tui"
)

var (
	fanStatus  bool
	fanSpeed   int
	fanSpeedSet bool
	fanID      int
	fanIDSet   bool
	fanAuto    bool
)

var fanCmd = &cobra.Command{
	Use:           "fan",
	Short:         "Control fans and view thermals",
	RunE:          runFan,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	f := fanCmd.Flags()
	f.BoolVar(&fanStatus, "status", false, "Show fan and temperature status")
	f.IntVar(&fanSpeed, "speed", 0, "Set fan speed (0-100%)")
	f.IntVar(&fanID, "fan", 0, "Target specific fan (1 or 2)")
	f.BoolVar(&fanAuto, "auto", false, "Set fans to automatic mode")

	rootCmd.AddCommand(fanCmd)
}

func runFan(cmd *cobra.Command, args []string) error {
	fanSpeedSet = cmd.Flags().Changed("speed")
	fanIDSet = cmd.Flags().Changed("fan")

	fc := fan.NewFanController()

	if fanAuto {
		if err := fc.SetAuto(); err != nil {
			return err
		}
		fmt.Println("Fans set to automatic mode.")
		return nil
	}

	if fanSpeedSet {
		targetFan := 0
		if fanIDSet {
			targetFan = fanID
		}
		if err := fc.SetFanSpeed(targetFan, fanSpeed); err != nil {
			return err
		}
		if targetFan == 0 {
			fmt.Printf("All fans set to %d%%.\n", fanSpeed)
		} else {
			fmt.Printf("Fan %d set to %d%%.\n", targetFan, fanSpeed)
		}
		return nil
	}

	// Default: show status (TUI if interactive, text otherwise)
	if isTerminal() && !fanStatus {
		model := tui.NewFanModel(fc)
		p := tea.NewProgram(model)
		_, err := p.Run()
		return err
	}

	fmt.Println(fan.StatusReport(fc))
	return nil
}

func isTerminal() bool {
	_, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), unix.TCGETS)
	return err == nil
}
