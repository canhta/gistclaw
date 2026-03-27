package execution

type PathScope struct {
	Root string `json:"root,omitempty"`
	Path string `json:"path"`
}

type Target struct {
	CWD        string      `json:"cwd"`
	ReadRoots  []PathScope `json:"read_roots,omitempty"`
	WriteRoots []PathScope `json:"write_roots,omitempty"`
	ExecHost   string      `json:"exec_host"`
}

type Request struct {
	ExplicitPath string
	StickyCWD    string
}

const ExecHostLocal = "host"
