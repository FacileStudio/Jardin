package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/FacileStudio/Hive/internal/adapter"
	"github.com/FacileStudio/Hive/internal/cell"
	"github.com/FacileStudio/Hive/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var installAll bool

var installCmd = &cobra.Command{
	Use:   "install [agent]",
	Short: "Generate config for an agent",
	Long:  "Generate agent-specific config from cell rules and skills.\nAvailable agents: claude, gemini, codex, cursor, copilot, hermes",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !installAll && len(args) == 0 {
			return cmd.Help()
		}

		cfg, err := config.LoadHiveConfig()
		if err != nil {
			return err
		}

		input, err := buildInput(cfg)
		if err != nil {
			return err
		}

		if installAll {
			for _, a := range adapter.All() {
				if err := runAdapter(a, input); err != nil {
					return err
				}
			}
			return nil
		}

		a, err := adapter.Get(args[0])
		if err != nil {
			return err
		}
		return runAdapter(a, input)
	},
}

func buildInput(cfg *config.HiveConfig) (*adapter.Input, error) {
	cellPath, err := cfg.ActiveCellPath()
	if err != nil {
		return nil, err
	}

	cellCfg, err := config.LoadCellConfig(cellPath)
	if err != nil {
		return nil, err
	}

	rules, err := cell.ReadRules(cellPath, cellCfg.RuleOrder)
	if err != nil {
		return nil, fmt.Errorf("reading rules: %w", err)
	}

	skills, err := cell.ReadSkills(cellPath)
	if err != nil {
		return nil, fmt.Errorf("reading skills: %w", err)
	}

	machine, _ := cell.ReadMachine(cellPath, cfg.Machine)

	return &adapter.Input{
		Rules:       rules,
		Skills:      skills,
		Machine:     machine,
		CellName:    cfg.ActiveCell,
		MachineName: cfg.Machine,
	}, nil
}

func runAdapter(a adapter.Adapter, input *adapter.Input) error {
	out, err := a.Generate(*input)
	if err != nil {
		return fmt.Errorf("adapter %s: %w", a.Name(), err)
	}

	for path, content := range out.Files {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		target, isSymlink := resolveSymlink(path)
		if isSymlink {
			color.Yellow("  %s → %s (symlink → %s)", a.Name(), path, target)
			path = target
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
		if !isSymlink {
			color.Green("  %s → %s", a.Name(), path)
		}
	}
	return nil
}

func resolveSymlink(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return path, false
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, false
	}
	return resolved, true
}

func init() {
	installCmd.Flags().BoolVar(&installAll, "all", false, "generate configs for all agents")
	rootCmd.AddCommand(installCmd)
}
