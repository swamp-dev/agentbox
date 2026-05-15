package cli

import (
	"fmt"
	"os"
	"path/filepath"

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
	journalProject  string
	journalLast     int
	journalSprint   int
	journalMarkdown bool
	journalExport   bool
)

func init() {
	journalCmd.Flags().StringVarP(&journalProject, "project", "p", ".", "project directory")
	journalCmd.Flags().IntVar(&journalLast, "last", 0, "show last N entries")
	journalCmd.Flags().IntVar(&journalSprint, "sprint", 0, "filter by sprint number")
	journalCmd.Flags().BoolVar(&journalMarkdown, "markdown", false, "render as markdown")
	journalCmd.Flags().BoolVar(&journalExport, "export", false, "export to .agentbox/journal.md")
}

func runJournal(cmd *cobra.Command, args []string) error {
	dbPath := filepath.Join(journalProject, ".agentbox", "agentbox.db")
	s, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("no agentbox database found at %s: %w", dbPath, err)
	}
	defer s.Close()

	sess, err := s.LatestSession()
	if err != nil {
		return fmt.Errorf("no sessions found: %w", err)
	}
	sessionID := sess.ID

	j := journal.New(s, sessionID)

	if journalExport || journalMarkdown {
		md, err := j.ExportMarkdown()
		if err != nil {
			return fmt.Errorf("exporting journal: %w", err)
		}

		if journalExport {
			path := filepath.Join(journalProject, ".agentbox", "journal.md")
			_ = os.MkdirAll(filepath.Join(journalProject, ".agentbox"), 0755)
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
