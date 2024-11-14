// cmd/tig/main.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tig/internal/parcel"
	"tig/shared/types"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var logger, _ = zap.NewDevelopment()

var rootCmd = &cobra.Command{
	Use:   "tig",
	Short: "Tig is a semantic version control system",
	Long: `Tig is a next-generation version control system that tracks why code changes, 
not just what changed. It provides semantic grouping of changes and intelligent 
dependency tracking.`,
}

var PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
	// Initialize logger
	var err error
	logger, err = zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("initializing logger: %w", err)
	}
	return nil
}

func init() {
	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Tig repository",
		Long:  `Tig is a next-generation version control system that tracks what changed and why.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}

			if err := parcel.Initialize(dir); err != nil {
				return fmt.Errorf("initializing repository: %w", err)
			}

			fmt.Println("Initialized empty Tig repository in", dir)
			return nil
		},
	}

	var gateCmd = &cobra.Command{
		Use:   "gate [paths...]",
		Short: "Gate specified paths",
		Long:  `Gates the specified paths. Use '.' to gate all files.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize Parcel
			parcelInstance, err := initParcel()
			if err != nil {
				return fmt.Errorf("initializing parcel: %w", err)
			}

			// Gate the specified paths
			if err := parcelInstance.Gate(args); err != nil {
				if parcelInstance.DB != nil {
					parcelInstance.DB.Close()
				}
				return fmt.Errorf("gating changes: %w", err)
			}

			// Close DB properly
			if parcelInstance.DB != nil {
				if err := parcelInstance.DB.Close(); err != nil {
					return fmt.Errorf("closing database: %w", err)
				}
			}

			fmt.Println("Changes gated successfully")
			return nil
		},
	}

	// Add ungate command
	var ungateCmd = &cobra.Command{
		Use:   "ungate [paths...]",
		Short: "Remove files from gated changes",
		Long:  `Remove files from the set of gated changes. Similar to git reset.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("specify files to ungate")
			}

			// Initialize Parcel
			parcelInstance, err := initParcel()
			if err != nil {
				return fmt.Errorf("initializing parcel: %w", err)
			}
			defer parcelInstance.DB.Close()

			// Ungate the specified paths
			if err := parcelInstance.Workspace.Ungate(args); err != nil {
				return fmt.Errorf("ungating files: %w", err)
			}

			fmt.Println("Changes ungated successfully")
			return nil
		},
	}

	// Cleanup command
	var cleanupCmd = &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup gated changes referencing missing content files",
		Long:  `Automatically removes gated changes that reference non-existent content files to maintain consistency.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize workspace
			p, err := initParcel()
			if err != nil {
				return err
			}
			defer p.DB.Close()

			// Perform cleanup
			if err := p.Workspace.CleanupGatedChanges(); err != nil {
				return fmt.Errorf("cleanup failed: %w", err)
			}

			fmt.Println("Cleanup completed successfully.")
			return nil
		},
	}

	var listIntentsCmd = &cobra.Command{
		Use:   "list",
		Short: "List all intents",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize workspace
			ws, err := initParcel()
			if err != nil {
				return err
			}
			defer ws.DB.Close()

			intents, err := ws.ListIntents()
			if err != nil {
				return fmt.Errorf("listing intents: %w", err)
			}

			if len(intents) == 0 {
				fmt.Println("No intents found")
				return nil
			}

			fmt.Println("\nIntents:")
			for _, i := range intents {
				fmt.Printf("%s  %s  %s  [%s]\n",
					i.ID[:8],
					i.CreatedAt.Format(time.RFC3339),
					i.Type,
					i.Description,
				)
			}

			return nil
		},
	}

	// Stream commands
	var streamCmd = &cobra.Command{
		Use:   "stream",
		Short: "Work with Tig streams",
		Long:  `Create and manage streams, which are continuous flows of related changes.`,
	}

	var createStreamCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new stream",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			streamType, _ := cmd.Flags().GetString("type")

			// Initialize workspace
			ws, err := initParcel()
			if err != nil {
				return err
			}
			defer ws.DB.Close()

			stream, err := ws.CreateStream(name, streamType)
			if err != nil {
				return fmt.Errorf("creating stream: %w", err)
			}

			fmt.Printf("Created stream %s: %s\n", stream.ID, stream.Name)
			return nil
		},
	}

	var listStreamsCmd = &cobra.Command{
		Use:   "list",
		Short: "List all streams",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize workspace
			ws, err := initParcel()
			if err != nil {
				return err
			}
			defer ws.DB.Close()

			streams, err := ws.ListStreams()
			if err != nil {
				return fmt.Errorf("listing streams: %w", err)
			}

			if len(streams) == 0 {
				fmt.Println("No streams found")
				return nil
			}

			fmt.Println("\nStreams:")
			for _, s := range streams {
				status := "inactive"
				if s.State.Active {
					status = "active"
				}
				fmt.Printf("%s  %s  %s  [%s]  %s\n",
					s.ID[:8],
					s.CreatedAt.Format(time.RFC3339),
					s.Type,
					s.Name,
					status,
				)
			}

			return nil
		},
	}

	var addIntentCmd = &cobra.Command{
		Use:   "add-intent",
		Short: "Add an intent to a stream",
		RunE: func(cmd *cobra.Command, args []string) error {
			streamID, _ := cmd.Flags().GetString("stream")
			intentID, _ := cmd.Flags().GetString("intent")

			// Initialize workspace
			ws, err := initParcel()
			if err != nil {
				return err
			}
			defer ws.DB.Close()

			if err := ws.AddIntentToStream(streamID, intentID); err != nil {
				return fmt.Errorf("adding intent to stream: %w", err)
			}

			fmt.Printf("Added intent %s to stream %s\n", intentID, streamID)
			return nil
		},
	}

	// changeCmd represents the change tracking commands
	var changeCmd = &cobra.Command{
		Use:   "change",
		Short: "Work with changes in the workspace",
		Long:  `Track, untrack, and view changes in your workspace.`,
	}

	// Add a corresponding untrack command for completeness
	var untrackCmd = &cobra.Command{
		Use:   "untrack [paths...]",
		Short: "Stop tracking specified files or directories",
		Long: `Stop tracking specified files or directories.
	Files that are no longer tracked will not appear in status or be included in intents.`,
		Example: `  tig untrack file.txt
	  tig untrack src/
	  tig untrack .`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("specify files or directories to untrack")
			}

			p, err := initParcel()
			if err != nil {
				return err
			}
			defer p.Close()

			// Untrack the specified paths
			if err := p.Untrack(args); err != nil {
				return fmt.Errorf("untracking paths: %w", err)
			}

			// Print success message
			if len(args) == 1 && args[0] == "." {
				fmt.Println("Stopped tracking all files in current directory")
			} else {
				fmt.Println("Stopped tracking:")
				for _, path := range args {
					fmt.Printf("  %s\n", path)
				}
			}

			return nil
		},
	}

	// Enhanced status command that uses the change tracking system
	// in cmd/tig/main.go

	var statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show working tree status",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize parcel
			p, err := initParcel()
			if err != nil {
				return err
			}
			// Ensure we close the DB properly
			defer func() {
				if p != nil && p.DB != nil {
					p.DB.Close()
				}
			}()

			// Load gated changes
			if err := p.Workspace.LoadGatedChanges(); err != nil {
				return fmt.Errorf("loading gated changes: %w", err)
			}

			// Get status
			changes, err := p.Workspace.Status()
			if err != nil {
				return fmt.Errorf("getting status: %w", err)
			}

			// Group changes by type
			var (
				gated     []shared.Change
				modified  []shared.Change
				untracked []shared.Change
				deleted   []shared.Change
			)

			for _, c := range changes {
				switch {
				case c.Gated:
					gated = append(gated, c)
				case c.Type == "modify":
					modified = append(modified, c)
				case c.Type == "untracked":
					untracked = append(untracked, c)
				case c.Type == "delete":
					deleted = append(deleted, c)
				}
			}

			// Use colors
			green := color.New(color.FgGreen).SprintFunc()
			red := color.New(color.FgRed).SprintFunc()
			yellow := color.New(color.FgYellow).SprintFunc()
			blue := color.New(color.FgBlue).SprintFunc()

			// Print summary header if there are changes
			totalChanges := len(gated) + len(modified) + len(untracked) + len(deleted)
			if totalChanges == 0 {
				fmt.Println("No changes detected (working tree clean)")
				return nil
			}

			fmt.Printf("\nChanges in working tree:\n\n")

			// Print gated files
			if len(gated) > 0 {
				fmt.Println("Changes ready for intent (gated):")
				fmt.Println("  (use \"tig intent create <description>\" to create a new intent)")
				for _, c := range gated {
					fmt.Printf("\t%s %s\n", green("✓"), c.Path)
				}
				fmt.Println()
			}

			// Print modified files
			if len(modified) > 0 {
				fmt.Println("Modified files:")
				fmt.Println("  (use \"tig gate <file>...\" to include in next intent)")
				for _, c := range modified {
					fmt.Printf("\t%s %s\n", yellow("M"), c.Path)
				}
				fmt.Println()
			}

			// Print untracked files
			if len(untracked) > 0 {
				fmt.Println("Untracked files:")
				fmt.Println("  (use \"tig gate <file>...\" to include in next intent)")
				for _, c := range untracked {
					fmt.Printf("\t%s %s\n", blue("?"), c.Path)
				}
				fmt.Println()
			}

			// Print deleted files
			if len(deleted) > 0 {
				fmt.Println("Deleted files:")
				fmt.Println("  (use \"tig gate <file>...\" to include deletion in next intent)")
				for _, c := range deleted {
					fmt.Printf("\t%s %s\n", red("D"), c.Path)
				}
				fmt.Println()
			}

			return nil
		},
	}

	// Modified intent command to use change tracking
	var intentCmd = &cobra.Command{
		Use:   "intent",
		Short: "Work with Tig intents",
		Long:  `Create and manage intents, which are semantic groupings of changes.`,
	}

	// Update the createIntentCmd implementation
	var createIntentCmd = &cobra.Command{
		Use:   "create [description]",
		Short: "Create a new intent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			description := args[0]
			intentType, _ := cmd.Flags().GetString("type")

			p, err := initParcel()
			if err != nil {
				return err
			}
			defer p.Close()

			// Create changeset first
			cs, err := p.Tracker.CreateChangeSet(description)
			if err != nil {
				return fmt.Errorf("creating changeset: %w", err)
			}

			// Create intent
			intent, err := p.CreateIntent(description, intentType)
			if err != nil {
				return fmt.Errorf("creating intent: %w", err)
			}

			// Update intent with changeset ID
			intent.ChangeSetID = cs.ID
			if err := p.UpdateIntent(intent); err != nil {
				return fmt.Errorf("updating intent: %w", err)
			}

			fmt.Printf("Created intent %s with %d changes\n", intent.ID, len(cs.Changes))
			return nil
		},
	}

	var diffCmd = &cobra.Command{
		Use:   "diff [paths...]",
		Short: "Show changes between the working tree and the previous state",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := initParcel()
			if err != nil {
				return fmt.Errorf("initializing parcel: %w", err)
			}
			defer p.Close()

			if p.Tracker == nil {
				return fmt.Errorf("tracker not initialized")
			}

			// If no paths specified, get all changed files from status
			if len(args) == 0 {
				changes, err := p.Tracker.Status()
				if err != nil {
					return fmt.Errorf("getting status: %w", err)
				}

				for _, change := range changes {
					if change.Type == "delete" {
						continue // Skip deleted files
					}
					result, err := p.Tracker.ShowFileDiff(change.Path)
					if err != nil {
						return fmt.Errorf("showing diff for %s: %w", change.Path, err)
					}
					fmt.Printf("\ndiff --tig a/%s b/%s\n", change.Path, change.Path)
					printColoredDiff(result.Format())
				}
				return nil
			}

			// Process specific paths
			for _, path := range args {
				// Ensure path is relative to repository root
				absPath := filepath.Join(p.Root, path)
				relPath, err := filepath.Rel(p.Root, absPath)
				if err != nil {
					return fmt.Errorf("getting relative path: %w", err)
				}

				// Check if file exists
				if _, err := os.Stat(absPath); err != nil {
					if os.IsNotExist(err) {
						fmt.Printf("File does not exist: %s\n", path)
						continue
					}
					return fmt.Errorf("accessing file %s: %w", path, err)
				}

				// Get and show diff
				result, err := p.Tracker.ShowFileDiff(relPath)
				if err != nil {
					return fmt.Errorf("showing diff for %s: %w", path, err)
				}

				fmt.Printf("\ndiff --tig a/%s b/%s\n", relPath, relPath)
				printColoredDiff(result.Format())
			}

			return nil
		},
	}

	// Add flags
	createIntentCmd.Flags().StringP("description", "d", "", "Intent description")
	createIntentCmd.Flags().StringP("type", "t", "feature", "Intent type (feature, fix, refactor, security, performance)")
	createIntentCmd.MarkFlagRequired("description")

	createStreamCmd.Flags().StringP("name", "n", "", "Stream name")
	createStreamCmd.Flags().StringP("type", "t", "feature", "Stream type (feature, release, hotfix)")
	createStreamCmd.MarkFlagRequired("name")

	addIntentCmd.Flags().StringP("stream", "s", "", "Stream ID")
	addIntentCmd.Flags().StringP("intent", "i", "", "Intent ID")
	addIntentCmd.MarkFlagRequired("stream")
	addIntentCmd.MarkFlagRequired("intent")

	// Add commands to root
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(intentCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(gateCmd)
	rootCmd.AddCommand(ungateCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(streamCmd)
	rootCmd.AddCommand(changeCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(untrackCmd)

	// Add intent subcommands
	intentCmd.AddCommand(createIntentCmd)
	intentCmd.AddCommand(listIntentsCmd)
	intentCmd.AddCommand(createIntentCmd)

	// Add stream subcommands
	streamCmd.AddCommand(createStreamCmd)
	streamCmd.AddCommand(listStreamsCmd)
	streamCmd.AddCommand(addIntentCmd)

	// Add change tracking commands

	changeCmd.AddCommand(untrackCmd)

}

func initParcel() (*parcel.Parcel, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting current directory: %w", err)
	}

	// Initialize logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("initializing logger: %w", err)
	}

	// Initialize Parcel with logger
	p, err := parcel.New(cwd, logger)
	if err != nil {
		return nil, fmt.Errorf("initializing parcel: %w", err)
	}

	return p, nil
}

func printColoredDiff(diff string) {
	// Create color objects
	added := color.New(color.FgGreen)
	removed := color.New(color.FgRed)
	header := color.New(color.FgCyan)

	// Process diff line by line
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			fmt.Println()
			continue
		}

		switch {
		case strings.HasPrefix(line, "@@"):
			header.Println(line)
		case strings.HasPrefix(line, "+"):
			added.Println(line)
		case strings.HasPrefix(line, "-"):
			removed.Println(line)
		default:
			fmt.Println(line)
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
