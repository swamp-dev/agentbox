package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/journal"
	"github.com/swamp-dev/agentbox/internal/store"
)

var journalCmd = &cobra.Command{
	Use:   "journal",
	Short: "View dev diary entries",
	Long: `Display journal entries from the agent's development diary.
The journal captures the agent's reflections, confidence levels, and
observations throughout the development process.`,
	RunE: runJournal,
}

var (
	journalLast     int
	journalSprint   int
	journalMarkdown bool
	journalExport   bool
)

func init() {
	journalCmd.Flags().IntVar(&journalLast, "last", 0, "show last N entries")
	journalCmd.Flags().IntVar(&journalSprint, "sprint", 0, "filter by sprint number")
	journalCmd.Flags().BoolVar(&journalMarkdown, "markdown", false, "render as markdown")
	journalCmd.Flags().BoolVar(&journalExport, "export", false, "export to .agentbox/journal.md")
}

func runJournal(cmd *cobra.Command, args []string) error {
	s, sessionID, err := openLatestSession()
	if err != nil {
		return err
	}
	defer s.Close()

	j := journal.New(s, sessionID)

	if journalExport || journalMarkdown {
		md, err := j.ExportMarkdown()
		if err != nil {
			return fmt.Errorf("exporting journal: %w", err)
		}

		if journalExport {
			cwd, _ := os.Getwd()
			path := cwd + "/.agentbox/journal.md"
			os.MkdirAll(cwd+"/.agentbox", 0755)
			if err := os.WriteFile(path, []byte(md), 0644); err != nil {
				return err
			}
			fmt.Printf("Exported: %s\n", path)
			return nil
		}

		fmt.Print(md)
		return nil
	}

	opts := &store.JournalQuery{}
	if journalLast > 0 {
		opts.Limit = journalLast
	}
	if journalSprint > 0 {
		opts.Sprint = journalSprint
	}

	entries, err := j.Entries(opts)
	if err != nil {
		return fmt.Errorf("loading journal entries: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No journal entries found.")
		return nil
	}

	for _, e := range entries {
		fmt.Print(journal.RenderEntry(e))
		fmt.Println("---")
	}

	return nil
}
