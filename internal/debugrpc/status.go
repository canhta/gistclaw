package debugrpc

import "errors"

var ErrUnknownProbe = errors.New("debugrpc: unknown probe")

const (
	ProbeStatus          = "status"
	ProbeConnectorHealth = "connector_health"
	ProbeActiveProject   = "active_project"
	ProbeScheduleStatus  = "schedule_status"
)

type Status struct {
	Notice  string  `json:"notice,omitempty"`
	Summary Summary `json:"summary"`
	Probes  []Probe `json:"probes"`
	Result  Result  `json:"result"`
}

type Summary struct {
	ProbeCount    int    `json:"probe_count"`
	ReadOnly      bool   `json:"read_only"`
	DefaultProbe  string `json:"default_probe"`
	SelectedProbe string `json:"selected_probe"`
}

type Probe struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Result struct {
	Probe           string         `json:"probe"`
	Label           string         `json:"label"`
	Summary         string         `json:"summary"`
	ExecutedAt      string         `json:"executed_at"`
	ExecutedAtLabel string         `json:"executed_at_label"`
	Data            map[string]any `json:"data,omitempty"`
}

func Catalog() []Probe {
	return []Probe{
		{
			Name:        ProbeStatus,
			Label:       "Status",
			Description: "Inspect active runs, approvals, and storage health.",
		},
		{
			Name:        ProbeConnectorHealth,
			Label:       "Connector health",
			Description: "Inspect configured connector health snapshots.",
		},
		{
			Name:        ProbeActiveProject,
			Label:       "Active project",
			Description: "Inspect the current project scope and workspace path.",
		},
		{
			Name:        ProbeScheduleStatus,
			Label:       "Scheduler",
			Description: "Inspect schedule counters and the next scheduler wake time.",
		},
	}
}

func ResolveProbe(raw string) (Probe, bool) {
	name := raw
	if name == "" {
		name = ProbeStatus
	}

	for _, probe := range Catalog() {
		if probe.Name == name {
			return probe, true
		}
	}
	return Probe{}, false
}

func FallbackStatus(selected, notice string) Status {
	probe, ok := ResolveProbe(selected)
	if !ok {
		probe, _ = ResolveProbe("")
	}

	return Status{
		Notice: notice,
		Summary: Summary{
			ProbeCount:    len(Catalog()),
			ReadOnly:      true,
			DefaultProbe:  ProbeStatus,
			SelectedProbe: probe.Name,
		},
		Probes: Catalog(),
		Result: Result{
			Probe:           probe.Name,
			Label:           probe.Label,
			Summary:         notice,
			ExecutedAt:      "",
			ExecutedAtLabel: "Unavailable",
			Data:            map[string]any{},
		},
	}
}
