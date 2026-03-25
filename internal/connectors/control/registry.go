package control

import "strings"

type Command struct {
	Name string
	Args string
}

type Registry struct {
	commands map[string]struct{}
}

func NewRegistry(names ...string) Registry {
	commands := make(map[string]struct{}, len(names))
	for _, name := range names {
		normalized := normalizeName(name)
		if normalized == "" {
			continue
		}
		commands[normalized] = struct{}{}
	}
	return Registry{commands: commands}
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
