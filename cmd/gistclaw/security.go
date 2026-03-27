package main

import (
	"fmt"
	"io"
	"strings"

	securitypkg "github.com/canhta/gistclaw/internal/security"
)

func runSecurity(opts globalOptions, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "audit" {
		fmt.Fprintln(stderr, "Usage: gistclaw security audit")
		return 1
	}

	cfg, err := loadConfigRawWithOverrides(opts)
	if err != nil {
		fmt.Fprintf(stdout, "FAIL config.invalid  %v\n", err)
		return 1
	}

	report := securitypkg.RunAudit(securitypkg.Input{
		Config:            cfg,
		AdminTokenPresent: strings.TrimSpace(lookupSettingFromDB(cfg.DatabasePath, "admin_token")) != "",
	})
	if len(report.Findings) == 0 {
		fmt.Fprintln(stdout, "PASS security audit  no findings")
		return 0
	}

	for _, finding := range report.Findings {
		fmt.Fprintf(stdout, "%-4s %-28s %s\n", strings.ToUpper(string(finding.Severity)), finding.ID, finding.Title)
		fmt.Fprintf(stdout, "     %s\n", finding.Detail)
		if finding.Remediation != "" {
			fmt.Fprintf(stdout, "     fix: %s\n", finding.Remediation)
		}
	}

	if report.HasFailures() {
		return 1
	}
	return 0
}
