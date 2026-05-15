// Package metrics provides quality trend tracking, resource monitoring, and budget enforcement.
package metrics

import (
	"fmt"

	"github.com/swamp-dev/agentbox/internal/store"
)

// Collector provides a query facade over the store for metrics aggregation.
type Collector struct {
	store     *store.Store
	sessionID int64
}

// NewCollector creates a metrics collector for the given session.
func NewCollector(s *store.Store, sessionID int64) *Collector {
	return &Collector{store: s, sessionID: sessionID}
}

// TestPassRate returns the test pass rate over the last N quality snapshots.
func (c *Collector) TestPassRate(lastN int) (float64, error) {
	return c.store.TestPassRate(c.sessionID, lastN)
}

// FailingTestTrend returns failing test names with their failure counts.
func (c *Collector) FailingTestTrend(lastN int) (map[string]int, error) {
	return c.store.FailingTestTrend(c.sessionID, lastN)
}

// QualityTrend returns "improving", "stable", or "degrading".
func (c *Collector) QualityTrend(lastN int) (string, error) {
	return c.store.QualityTrend(c.sessionID, lastN)
}

// TotalUsage returns aggregate resource consumption.
func (c *Collector) TotalUsage() (*store.ResourceUsage, error) {
	return c.store.TotalUsage(c.sessionID)
}

// RecordQuality records a quality snapshot.
func (c *Collector) RecordQuality(q *store.QualitySnapshot) error {
	q.SessionID = c.sessionID
	return c.store.RecordQuality(q)
}

// RecordUsage records resource usage.
func (c *Collector) RecordUsage(u *store.ResourceUsage) error {
	u.SessionID = c.sessionID
	return c.store.RecordUsage(u)
}

// Summary returns a formatted metrics summary.
func (c *Collector) Summary() (string, error) {
	usage, err := c.TotalUsage()
	if err != nil {
		return "", err
	}
	trend, err := c.QualityTrend(10)
	if err != nil {
		return "", err
	}
	rate, err := c.TestPassRate(10)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"Iterations: %d | Tokens: %d | Container: %dms | Quality: %s | Pass Rate: %.1f%%",
		usage.Iteration, usage.EstimatedTokens, usage.ContainerTimeMs,
		trend, rate*100,
	), nil
}
