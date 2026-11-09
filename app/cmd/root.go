package cmd

import (
	"fmt"
	"github.com/go-bumbu/reference-app/app/metainfo"
	"github.com/spf13/cobra"
	"os"
	"runtime"
)

// Execute is the entry point for the command line
func Execute() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

const appName = "refapp" // TODO replace the app name

func rootCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   appName,
		Short: fmt.Sprintf("%s short description", appName),
		// a runtime error (bad config, port in use) is not a usage error:
		// don't print the usage, and let Execute print the error once.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		_ = cmd.Help()
		return nil
	})

	cmd.AddCommand(
		serverCmd(),
		versionCmd(),
	)

	return cmd
}

func versionCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:   "version",
		Short: "print version and build information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Version: %s\n", metainfo.Version)
			fmt.Printf("Build date: %s\n", metainfo.BuildTime)
			fmt.Printf("Commit sha: %s\n", metainfo.ShaVer)
			fmt.Printf("Compiler: %s\n", runtime.Version())
		},
	}
	return &cmd
}
