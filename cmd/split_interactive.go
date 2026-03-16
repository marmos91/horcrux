package cmd

import (
	"os"

	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/marmos91/horcrux/internal/wizard"
	"github.com/spf13/cobra"
)

// runSplitInteractive runs the interactive wizard to configure and execute a split.
func runSplitInteractive(cmd *cobra.Command, input string, info os.FileInfo) error {
	cfg, err := wizard.RunSplitWizard(os.Stdin, os.Stderr, input, info.Size())
	if err != nil {
		return err
	}

	prog, cleanup := newProgressReporter()
	defer cleanup()

	result, err := pipeline.Split(pipeline.SplitOptions{
		InputFile:    cfg.InputFile,
		OutputDir:    cfg.OutputDir,
		DataShards:   cfg.DataShards,
		ParityShards: cfg.ParityShards,
		Password:     cfg.Password,
		KeyFile:      cfg.KeyFile,
		NoEncrypt:    !cfg.Encrypt,
		Verbose:      verbose && !quiet,
		Progress:     prog,
	})
	if err != nil {
		return err
	}

	var backends []pipeline.BackendWithURI
	if len(cfg.DistributeURIs) > 0 && result.ShardFiles != nil {
		backends, err = pipeline.OpenWeightedBackends(cfg.DistributeURIs, loadedBackendConfig)
		if err != nil {
			return err
		}

		result.ShardFiles, err = pipeline.DistributeShards(cmd.Context(), result.ShardFiles, backends)
		if err != nil {
			return err
		}

		if !cfg.KeepLocal {
			if err := pipeline.CleanupLocalShards(result.ShardFiles); err != nil {
				return err
			}
		}
	}

	manifestPath, err := pipeline.SaveManifestPath(result, cfg.OutputDir)
	if err != nil {
		return err
	}

	if cfg.DistributeManifest && len(backends) > 0 {
		if err := pipeline.DistributeManifest(cmd.Context(), manifestPath, backends); err != nil {
			return err
		}
	}

	return nil
}
