package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.2.0"

var rootCmd = &cobra.Command{
	Use:   "avellcc",
	Short: "Avell Storm 590X Control Center for Linux",
	Long:  "Control keyboard RGB, lightbar, and fans on Avell/Clevo laptops.",
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = version
}
