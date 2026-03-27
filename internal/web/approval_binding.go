package web

import "github.com/canhta/gistclaw/internal/authority"

func approvalBindingSummary(bindingJSON []byte) string {
	return authority.BindingSummaryJSON(bindingJSON)
}
