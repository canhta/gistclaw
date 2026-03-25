package control

import "strings"

type CommandSpec struct {
	Name        string `json:"command"`
	Description string `json:"description"`
}

type Command struct {
	Name string
	Args string
}

type Registry struct {
	commands map[string]struct{}
	specs    []CommandSpec
}

func NewRegistry(specs ...CommandSpec) Registry {
	commands := make(map[string]struct{}, len(specs))
	normalizedSpecs := make([]CommandSpec, 0, len(specs))
	for _, spec := range specs {
		normalized := normalizeName(spec.Name)
		if normalized == "" {
			continue
		}
		commands[normalized] = struct{}{}
		normalizedSpecs = append(normalizedSpecs, CommandSpec{
			Name:        normalized,
			Description: strings.TrimSpace(spec.Description),
		})
	}
	return Registry{commands: commands, specs: normalizedSpecs}
}

func (r Registry) Parse(text string) (Command, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return Command{}, false
	}

	token, args, _ := strings.Cut(trimmed, " ")
	if token == "/" {
		return Command{}, false
	}

	name := strings.TrimPrefix(token, "/")
	name, _, _ = strings.Cut(name, "@")
	name = normalizeName(name)
	if name == "" || strings.Contains(name, "/") {
		return Command{}, false
	}
	if _, ok := r.commands[name]; !ok {
		return Command{}, false
	}

	return Command{
		Name: name,
		Args: strings.TrimSpace(args),
	}, true
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "/")))
}

func (r Registry) Specs() []CommandSpec {
	specs := make([]CommandSpec, len(r.specs))
	copy(specs, r.specs)
	return specs
}
