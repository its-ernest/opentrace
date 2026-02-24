package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/its-ernest/opentrace/core"
	"github.com/its-ernest/opentrace/installer"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "opentrace",
		Short: "Modular OSINT pipeline runner",
	}

	root.AddCommand(runCmd(), installCmd(), uninstallCmd(), modulesCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "run <pipeline.yaml>",
		Short:   "Run a pipeline",
		Aliases: []string{"-r"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := core.Load(args[0])
			if err != nil {
				return err
			}

			reg := installer.LoadRegistry()
			for _, m := range p.Modules {
				if _, ok := reg[m.Name]; !ok {
					return fmt.Errorf("module %q not installed — run: opentrace install %s", m.Name, m.Name)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			return core.Run(ctx, p, installer.BinDir())
		},
	}
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <module>",
		Short: "Install a module from opentrace-modules",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return installer.Install(args[0])
		},
	}
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <module>",
		Short: "Uninstall a module",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return installer.Uninstall(args[0])
		},
	}
}

func modulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "modules",
		Short: "List installed modules",
		Run: func(cmd *cobra.Command, args []string) {
			installer.List()
		},
	}
}