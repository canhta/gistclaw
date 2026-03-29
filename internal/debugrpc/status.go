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
