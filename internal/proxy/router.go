package proxy 

import (
	"strings"
	"github.com/BrandonMager/CacheProxyfy/internal/ecosystem"
)

type Router struct {
	handlers map[string]ecosystem.Handler
}

func NewRouter(enabledEcosystems[] string) *Router {
	all := map[string]ecosystem.Handler{
		"npm":   ecosystem.NewNPM(),
		"pypi":  ecosystem.NewPyPI(),
		"maven": ecosystem.NewMaven(),
	}

	enabled := make(map[string]bool, len(enabledEcosystems))
	for _, e := range enabledEcosystems {
		enabled[strings.ToLower(e)] = true
	}

	handlers := make(map[string]ecosystem.Handler, len(enabledEcosystems))
	for name, h := range all {
		if enabled[name] {
			handlers[name] = h
		}
	}

	return &Router{handlers: handlers}
}

func (r *Router) Match(path string) (string, ecosystem.Handler, bool) {
	trimmed := strings.TrimPrefix(path, "/")
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		return "", nil, false
	}

	prefix := trimmed[:slash]
	h, ok := r.handlers[prefix]
	return prefix, h, ok
}

func (r *Router) Ecosystems() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}

	return names
}
