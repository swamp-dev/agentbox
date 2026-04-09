package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/swamp-dev/agentbox/internal/store"
)

// renderWatchDashboard renders a full dashboard view as a string for the
// watch-mode TUI. It uses strings.Builder and reuses helpers from status.go
// (renderProgressBar, statusIcon, truncate).
func renderWatchDashboard(data *store.DashboardData, width int) string {
	if width < 40 {
		width = 40
	}

	var b strings.Builder

	// Header.
	writeHeader(&b, width)

	// Session info.
	writeSessionInfo(&b, data.Session)

	// Progress bar + task stats.
	writeTaskStats(&b, data.TaskStats, width)

	// Quality trend.
	writeQuality(&b, data.QualityTrend, data.TestPassRate)

	// Resource usage.
	if data.TotalUsage != nil {
		writeResourceUsage(&b, data.TotalUsage)
	}

	// Recent journal.
	if len(data.RecentJournal) > 0 {
		writeRecentJournal(&b, data.RecentJournal, width)
	}

	// Footer with refresh timestamp.
	ts := time.Now().Format("15:04:05")
	prefix := "── refreshed " + ts + " "
	prefixWidth := 22
	remaining := width - prefixWidth
	fmt.Fprintf(&b, "%s", prefix)
	if remaining > 0 {
		b.WriteString(strings.Repeat("─", remaining))
	}
	b.WriteByte('\n')

	return b.String()
}

func writeHeader(b *strings.Builder, width int) {
	title := " Agentbox Dashboard "
	pad := width - len(title)
	left := pad / 2
	right := pad - left
	if left < 0 {
		left = 0
	}
	if right < 0 {
		right = 0
	}
	b.WriteString(strings.Repeat("═", left))
	b.WriteString(title)
	b.WriteString(strings.Repeat("═", right))
	b.WriteByte('\n')
	b.WriteByte('\n')
}

func writeSessionInfo(b *strings.Builder, sess *store.Session) {
	fmt.Fprintf(b, "  Session #%d  %s  %s\n", sess.ID, statusIcon(sess.Status), sess.Status)
	if sess.RepoURL != "" {
		fmt.Fprintf(b, "  Repo:    %s\n", sess.RepoURL)
	}
	if sess.BranchName != "" {
		fmt.Fprintf(b, "  Branch:  %s\n", sess.BranchName)
	}
	fmt.Fprintf(b, "  Started: %s\n", sess.StartedAt.Format("2006-01-02 15:04:05"))
	b.WriteByte('\n')
}

func writeTaskStats(b *strings.Builder, stats *store.TaskStats, width int) {
	b.WriteString("  Tasks\n")

	var pct float64
	if stats.Total > 0 {
		pct = float64(stats.Completed) / float64(stats.Total) * 100
	}

	barWidth := width - 20
	if barWidth < 10 {
		barWidth = 10
	}
	bar := renderProgressBar(pct, barWidth)
	fmt.Fprintf(b, "  %s %5.1f%%\n", bar, pct)

	fmt.Fprintf(b, "  ✓ %d completed  ▶ %d in progress  ○ %d pending  ✗ %d failed",
		stats.Completed, stats.InProgress, stats.Pending, stats.Failed)
	if stats.Deferred > 0 {
		fmt.Fprintf(b, "  ⊘ %d deferred", stats.Deferred)
	}
	b.WriteByte('\n')
	b.WriteByte('\n')
}

func writeQuality(b *strings.Builder, trend string, passRate float64) {
	b.WriteString("  Quality\n")
	fmt.Fprintf(b, "  Trend: %s  │  Test Pass Rate: %.1f%%\n", trend, passRate*100)
	b.WriteByte('\n')
}

func writeResourceUsage(b *strings.Builder, usage *store.ResourceUsage) {
	b.WriteString("  Resources\n")
	fmt.Fprintf(b, "  Iterations: %d  │  Tokens: %d  │  Container: %s\n",
		usage.Iteration, usage.EstimatedTokens, formatDuration(usage.ContainerTimeMs))
	b.WriteByte('\n')
}

func writeRecentJournal(b *strings.Builder, entries []*store.JournalEntry, width int) {
	b.WriteString("  Recent Activity\n")

	start := len(entries) - 5
	if start < 0 {
		start = 0
	}

	maxSummary := width - 24
	if maxSummary < 20 {
		maxSummary = 20
	}

	for _, e := range entries[start:] {
		summary := truncate(e.Summary, maxSummary)
		fmt.Fprintf(b, "  %s %s  %s\n",
			statusIcon(e.Kind), e.Timestamp.Format("15:04"), summary)
	}
	b.WriteByte('\n')
}

// clearScreen writes ANSI escape codes to move cursor home and clear the screen.
func clearScreen(w io.Writer) {
	fmt.Fprint(w, "\033[H\033[2J")
}

// runWatchMode polls ExportDashboardData every 2 seconds, clears the screen,
// and redraws the dashboard. It exits on SIGINT or SIGTERM.
func runWatchMode(s *store.Store, sessionID int64) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	const width = 80

	data, err := s.ExportDashboardData(sessionID)
	if err != nil {
		return fmt.Errorf("loading dashboard data: %w", err)
	}
	clearScreen(os.Stdout)
	fmt.Fprint(os.Stdout, renderWatchDashboard(data, width))

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			data, err := s.ExportDashboardData(sessionID)
			if err != nil {
				return fmt.Errorf("loading dashboard data: %w", err)
			}
			clearScreen(os.Stdout)
			fmt.Fprint(os.Stdout, renderWatchDashboard(data, width))
		}
	}
}

// formatDuration converts milliseconds to a human-readable duration string.
func formatDuration(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	secs := ms / 1000
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	remainSecs := secs % 60
	return fmt.Sprintf("%dm%ds", mins, remainSecs)
}
