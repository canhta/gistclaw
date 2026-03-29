package extensionstatus

type Status struct {
	Summary  Summary   `json:"summary"`
	Surfaces []Surface `json:"surfaces"`
	Tools    []Tool    `json:"tools"`
}

type Summary struct {
	ShippedSurfaces    int `json:"shipped_surfaces"`
	ConfiguredSurfaces int `json:"configured_surfaces"`
	InstalledTools     int `json:"installed_tools"`
	ReadyCredentials   int `json:"ready_credentials"`
	MissingCredentials int `json:"missing_credentials"`
}

type Surface struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Kind                 string `json:"kind"`
	Configured           bool   `json:"configured"`
	Active               bool   `json:"active"`
	CredentialState      string `json:"credential_state"`
	CredentialStateLabel string `json:"credential_state_label"`
	Summary              string `json:"summary"`
	Detail               string `json:"detail"`
}

type Tool struct {
	Name        string `json:"name"`
	Family      string `json:"family"`
	Risk        string `json:"risk"`
	Approval    string `json:"approval"`
	SideEffect  string `json:"side_effect"`
	Description string `json:"description"`
}
