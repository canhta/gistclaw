package security

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/app"
	toolspkg "github.com/canhta/gistclaw/internal/tools"
)

type Severity string

const (
	SeverityWarn Severity = "warn"
	SeverityFail Severity = "fail"
)

type Finding struct {
	ID          string
	Subject     string
	Severity    Severity
	Title       string
	Detail      string
	Remediation string
}

type Report struct {
	Findings []Finding
}

func (r Report) HasFailures() bool {
	for _, finding := range r.Findings {
		if finding.Severity == SeverityFail {
			return true
		}
	}
	return false
}

type Input struct {
	Config            app.Config
	AdminTokenPresent bool
}

type auditDeps struct {
	lookPath func(string) (string, error)
}

func RunAudit(input Input) Report {
	return runAudit(input, auditDeps{lookPath: exec.LookPath})
}

func runAudit(input Input, deps auditDeps) Report {
	report := Report{}
	cfg := input.Config

	if !input.AdminTokenPresent {
		report.add(Finding{
			ID:          "admin_token.missing",
			Subject:     "admin_token",
			Severity:    SeverityFail,
			Title:       "Admin token missing",
			Detail:      "settings table does not contain an admin token",
			Remediation: "Run `gistclaw serve` once or generate and store an admin token before exposing operator surfaces.",
		})
	}

	if !isLoopbackListenAddr(cfg.Web.ListenAddr) {
		report.add(Finding{
			ID:          "web.listen_addr.exposed",
			Subject:     "web",
			Severity:    SeverityFail,
			Title:       "Web UI is bound beyond loopback",
			Detail:      fmt.Sprintf("web.listen_addr %q is reachable outside the local machine", cfg.Web.ListenAddr),
			Remediation: "Bind the web UI to 127.0.0.1 or ::1, put a trusted HTTPS reverse proxy in front of it for public access, and run `gistclaw auth set-password` before exposing the domain.",
		})
	}

	auditStorageRoot(&report, cfg)
	auditProvider(&report, cfg.Provider)
	auditResearch(&report, cfg.Research)
	auditMCP(&report, cfg.MCP, deps)
	auditWhatsApp(&report, cfg.WhatsApp)

	return report
}

func (r *Report) add(finding Finding) {
	r.Findings = append(r.Findings, finding)
}

func auditStorageRoot(report *Report, cfg app.Config) {
	if strings.TrimSpace(cfg.StorageRoot) == "" {
		report.add(Finding{
			ID:          "storage_root.missing",
			Subject:     "storage_root",
			Severity:    SeverityFail,
			Title:       "Storage root missing",
			Detail:      "storage_root is not configured",
			Remediation: "Set storage_root to a GistClaw-owned directory for logs, memory, approvals, and artifacts.",
		})
		return
	}

	info, err := os.Stat(cfg.StorageRoot)
	if err != nil {
		report.add(Finding{
			ID:          "storage_root.not_found",
			Subject:     "storage_root",
			Severity:    SeverityFail,
			Title:       "Storage root not found",
			Detail:      fmt.Sprintf("storage_root %q could not be read: %v", cfg.StorageRoot, err),
			Remediation: "Point storage_root at an existing directory that the operator account can access.",
		})
		return
	}
	if !info.IsDir() {
		report.add(Finding{
			ID:          "storage_root.not_directory",
			Subject:     "storage_root",
			Severity:    SeverityFail,
			Title:       "Storage root is not a directory",
			Detail:      fmt.Sprintf("storage_root %q is not a directory", cfg.StorageRoot),
			Remediation: "Set storage_root to a directory path instead of a file path.",
		})
		return
	}

	tmp, err := os.CreateTemp(cfg.StorageRoot, ".gistclaw-security-*")
	if err != nil {
		report.add(Finding{
			ID:          "storage_root.not_writable",
			Subject:     "storage_root",
			Severity:    SeverityFail,
			Title:       "Storage root is not writable",
			Detail:      fmt.Sprintf("storage_root %q is not writable: %v", cfg.StorageRoot, err),
			Remediation: "Grant the operator account write access to storage_root before running GistClaw there.",
		})
		return
	}
	_ = tmp.Close()
	_ = os.Remove(tmp.Name())
}

func auditProvider(report *Report, cfg app.ProviderConfig) {
	switch {
	case strings.TrimSpace(cfg.Name) == "":
		report.add(Finding{
			ID:          "provider.name.missing",
			Subject:     "provider",
			Severity:    SeverityFail,
			Title:       "Provider name missing",
			Detail:      "provider.name is required",
			Remediation: "Configure a supported provider name such as anthropic or openai.",
		})
	case cfg.Name != "anthropic" && cfg.Name != "openai":
		report.add(Finding{
			ID:          "provider.name.invalid",
			Subject:     "provider",
			Severity:    SeverityFail,
			Title:       "Provider name is invalid",
			Detail:      fmt.Sprintf("provider.name %q is not supported", cfg.Name),
			Remediation: "Set provider.name to anthropic or openai.",
		})
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		report.add(Finding{
			ID:          "provider.api_key.missing",
			Subject:     "provider",
			Severity:    SeverityFail,
			Title:       "Provider API key missing",
			Detail:      "provider.api_key is required",
			Remediation: "Provide the API key for the configured provider.",
		})
	}

	if cfg.WireAPI != "" && cfg.WireAPI != "chat_completions" && cfg.WireAPI != "responses" {
		report.add(Finding{
			ID:          "provider.wire_api.invalid",
			Subject:     "provider",
			Severity:    SeverityFail,
			Title:       "Provider wire API is invalid",
			Detail:      fmt.Sprintf("provider.wire_api %q is not supported", cfg.WireAPI),
			Remediation: "Set provider.wire_api to chat_completions or responses, or leave it empty for the default.",
		})
	}
}

func auditResearch(report *Report, cfg toolspkg.ResearchConfig) {
	if strings.TrimSpace(cfg.Provider) == "" {
		return
	}

	if cfg.Provider != "tavily" {
		report.add(Finding{
			ID:          "research.provider.invalid",
			Subject:     "research",
			Severity:    SeverityFail,
			Title:       "Research provider is invalid",
			Detail:      fmt.Sprintf("research.provider %q is not supported", cfg.Provider),
			Remediation: "Set research.provider to tavily or remove the research block until it is configured.",
		})
		return
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		report.add(Finding{
			ID:          "research.api_key.missing",
			Subject:     "research",
			Severity:    SeverityFail,
			Title:       "Research API key missing",
			Detail:      "research.api_key is required when research.provider is configured",
			Remediation: "Provide a Tavily API key or remove the research block.",
		})
	}
}

func auditMCP(report *Report, cfg toolspkg.MCPOptions, deps auditDeps) {
	for _, server := range cfg.Servers {
		if enabledMCPTools(server.Tools) == 0 {
			continue
		}

		transport := server.Transport
		if transport == "" {
			transport = "stdio"
		}
		if transport != "stdio" {
			report.add(Finding{
				ID:          "mcp.transport.invalid",
				Subject:     "mcp:" + server.ID,
				Severity:    SeverityFail,
				Title:       "MCP transport is invalid",
				Detail:      fmt.Sprintf("mcp server %q uses unsupported transport %q", server.ID, transport),
				Remediation: "Use stdio transport for shipped MCP integrations.",
			})
			continue
		}
		if len(server.Command) == 0 {
			report.add(Finding{
				ID:          "mcp.command.missing",
				Subject:     "mcp:" + server.ID,
				Severity:    SeverityFail,
				Title:       "MCP command missing",
				Detail:      fmt.Sprintf("mcp server %q does not define a command", server.ID),
				Remediation: "Set mcp.servers[].command to the server binary and arguments.",
			})
			continue
		}
		if _, err := resolveBinary(server.Command[0], deps.lookPath); err != nil {
			report.add(Finding{
				ID:          "mcp.binary.missing",
				Subject:     "mcp:" + server.ID,
				Severity:    SeverityWarn,
				Title:       "MCP binary not found",
				Detail:      fmt.Sprintf("mcp server %q binary %q could not be resolved", server.ID, server.Command[0]),
				Remediation: "Install the MCP server binary or update the command path before enabling its tools.",
			})
		}
	}
}

func auditWhatsApp(report *Report, cfg app.WhatsAppConfig) {
	configured := 0
	for _, value := range []string{cfg.PhoneNumberID, cfg.AccessToken, cfg.VerifyToken} {
		if strings.TrimSpace(value) != "" {
			configured++
		}
	}
	if configured == 0 || configured == 3 {
		return
	}

	report.add(Finding{
		ID:          "whatsapp.config.incomplete",
		Subject:     "whatsapp",
		Severity:    SeverityFail,
		Title:       "WhatsApp config is incomplete",
		Detail:      "whatsapp.phone_number_id, whatsapp.access_token, and whatsapp.verify_token must be configured together",
		Remediation: "Configure all three WhatsApp fields together, or clear the partial configuration.",
	})
}

func enabledMCPTools(tools []toolspkg.MCPToolConfig) int {
	count := 0
	for _, tool := range tools {
		if tool.Enabled {
			count++
		}
	}
	return count
}

func resolveBinary(command string, lookPath func(string) (string, error)) (string, error) {
	if command == "" {
		return "", fmt.Errorf("binary is required")
	}
	if filepath.IsAbs(command) || strings.ContainsRune(command, os.PathSeparator) {
		if _, err := os.Stat(command); err != nil {
			return "", fmt.Errorf("binary not found: %s", command)
		}
		return command, nil
	}
	resolved, err := lookPath(command)
	if err != nil {
		return "", fmt.Errorf("binary not found: %s", command)
	}
	return resolved, nil
}

func isLoopbackListenAddr(addr string) bool {
	if strings.TrimSpace(addr) == "" {
		return true
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" || host == "localhost" {
		return host == "localhost"
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
