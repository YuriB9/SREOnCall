package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/sre-oncall/pkg/domain"
)

// computeFingerprint returns a SHA-256 of sorted labels + source + tenant_id.
// This is the canonical dedup key per the architecture spec.
func computeFingerprint(a domain.Alert) string {
	keys := make([]string, 0, len(a.Labels))
	for k := range a.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s,", k, a.Labels[k])
	}
	fmt.Fprintf(&b, "source=%s,tenant=%s", a.Source, a.TenantID)

	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

// mapSeverity maps common severity label values to canonical AlertSeverity.
func mapSeverity(s string) domain.AlertSeverity {
	switch strings.ToLower(s) {
	case "critical":
		return domain.SeverityCritical
	case "high":
		return domain.SeverityHigh
	case "warning", "warn":
		return domain.SeverityWarning
	default:
		return domain.SeverityInfo
	}
}
