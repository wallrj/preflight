package cmd

import (
	"fmt"
	"time"

	"github.com/jetstack/preflight/pkg/agent"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "start the preflight agent",
	Long: `The agent will periodically gather data for the configured data
	gatherers and send it to a remote backend for evaluation`,
	Run: agent.Run,
}

var agentInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "print several internal parameters of the agent",
	Long:  `Print several internal parameters of the agent, as the built-in OAuth2 client ID.`,
	Run: func(cmd *cobra.Command, args []string) {
		printVersion(true)
		fmt.Println()
		printOAuth2Config()
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentInfoCmd)
	agentCmd.PersistentFlags().StringVarP(
		&agent.ConfigFilePath,
		"agent-config-file",
		"c",
		"./agent.yaml",
		"Config file location, default is `agent.yaml` in the current working directory.",
	)
	agentCmd.PersistentFlags().DurationVarP(
		&agent.Period,
		"period",
		"p",
		time.Hour,
		"Time between scans (given as XhYmZs).",
	)
	agentCmd.PersistentFlags().StringVarP(
		&agent.CredentialsPath,
		"credentials-file",
		"k",
		"",
		"Location of the credentials file. For OAuth2 based authentication.",
	)
	agentCmd.PersistentFlags().BoolVarP(
		&agent.OneShot,
		"one-shot",
		"",
		false,
		"Runs agent a single time if true, or continously if false",
	)
	agentCmd.PersistentFlags().StringVarP(
		&agent.OutputPath,
		"output-path",
		"",
		"",
		"Output file path, if used, it will write data to a local file instead of uploading to the preflight server",
	)
	agentCmd.PersistentFlags().StringVarP(
		&agent.InputPath,
		"input-path",
		"",
		"",
		"Input file path, if used, it will read data from a local file instead of gathering data from clusters",
	)
	agentCmd.PersistentFlags().DurationVarP(
		&agent.BackoffMaxTime,
		"backoff-max-time",
		"",
		10*time.Minute,
		"Max time for retrying failed data gatherers (given as XhYmZs).",
	)
	agentCmd.PersistentFlags().BoolVarP(
		&agent.StrictMode,
		"strict",
		"",
		false,
		"Runs agent in strict mode. No retry attempts will be made for a missing data gatherer's data.",
	)

}
