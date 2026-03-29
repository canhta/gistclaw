package nodeinventory

type Status struct {
	Notice       string            `json:"notice,omitempty"`
	Summary      Summary           `json:"summary"`
	Connectors   []ConnectorStatus `json:"connectors"`
	Runs         []RunNode         `json:"runs"`
	Capabilities []Capability      `json:"capabilities"`
}

func FallbackStatus(notice string) Status {
	return Status{
		Notice: notice,
		Summary: Summary{
			Connectors:        0,
			HealthyConnectors: 0,
			RunNodes:          0,
			ApprovalNodes:     0,
			Capabilities:      0,
		},
		Connectors:   []ConnectorStatus{},
		Runs:         []RunNode{},
		Capabilities: []Capability{},
	}
}

type Summary struct {
	Connectors        int `json:"connectors"`
	HealthyConnectors int `json:"healthy_connectors"`
	RunNodes          int `json:"run_nodes"`
	ApprovalNodes     int `json:"approval_nodes"`
	Capabilities      int `json:"capabilities"`
}

type ConnectorStatus struct {
	ID               string   `json:"id"`
	Aliases          []string `json:"aliases"`
	Exposure         string   `json:"exposure"`
	State            string   `json:"state"`
	StateLabel       string   `json:"state_label"`
	Summary          string   `json:"summary"`
	CheckedAtLabel   string   `json:"checked_at_label"`
	RestartSuggested bool     `json:"restart_suggested"`
}

type RunNode struct {
	ID               string `json:"id"`
	ShortID          string `json:"short_id"`
	ParentRunID      string `json:"parent_run_id"`
	Kind             string `json:"kind"`
	AgentID          string `json:"agent_id"`
	Status           string `json:"status"`
	StatusLabel      string `json:"status_label"`
	ObjectivePreview string `json:"objective_preview"`
	StartedAtLabel   string `json:"started_at_label"`
	UpdatedAtLabel   string `json:"updated_at_label"`
}

type Capability struct {
	Name        string `json:"name"`
	Family      string `json:"family"`
	Description string `json:"description"`
}
