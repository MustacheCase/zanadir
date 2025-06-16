package main

import (
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/logger"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/scanner"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "zanadir",
	Short: "Zanadir is a tool for scanning CI/CD configurations",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.CreateConfig(cmd)
		if err != nil {
			return err
		}

		rulesService, err := rules.NewRulesService()
		if err != nil {
			return err
		}

		suggestionService, err := suggester.NewSuggestionService()
		if err != nil {
			return err
		}

		h := handler.NewHandler(
			rulesService,
			scanner.NewScanService(scanner.NewRepositoryScanner()),
			suggestionService,
			matcher.NewMatchService(),
			output.NewOutputService(),
		)

		return h.Execute(cfg)
	},
}

func init() {
	rootCmd.Flags().String("dir", ".", "Directory to scan")
	rootCmd.Flags().Bool("debug", false, "Enable debug mode")
	rootCmd.Flags().String("output", "table", "Output format (table or json)")
	rootCmd.Flags().Bool("enforce", false, "Enable enforce mode")
	rootCmd.Flags().StringSlice("exclude", []string{}, "Categories to exclude")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		logger.GetLogger().Fatal(err.Error())
	}
}
