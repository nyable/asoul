package agent

import (
	"fmt"
	"sort"
	"strings"
)

// ChannelInfo describes a model-config capable agent channel.
type ChannelInfo struct {
	Name         string `json:"name"`
	Label        string `json:"label,omitempty"`
	Description  string `json:"description"`
	ConfigFormat string `json:"configFormat"`
}

// DisplayLabel returns the human-friendly label, falling back to the name.
func (c ChannelInfo) DisplayLabel() string {
	if c.Label != "" {
		return c.Label
	}
	return c.Name
}

type adapterFactory struct {
	info    ChannelInfo
	factory func() AgentAdapter
}

var adapterRegistry = map[string]adapterFactory{}

// Register registers a model-config adapter factory for an agent channel.
// Adapters register themselves from their package init functions so that the
// service and TUI can enumerate and instantiate channels without hardcoding.
func Register(info ChannelInfo, factory func() AgentAdapter) {
	if info.Name == "" || factory == nil {
		return
	}
	adapterRegistry[info.Name] = adapterFactory{info: info, factory: factory}
}

// NewAdapter returns a new adapter for the named agent channel.
func NewAdapter(name string) (AgentAdapter, error) {
	reg, ok := adapterRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unsupported agent channel %q (supported: %s)", name, strings.Join(SupportedChannelNames(), ", "))
	}
	return reg.factory(), nil
}

// SupportedChannels returns registered channels sorted by name.
func SupportedChannels() []ChannelInfo {
	infos := make([]ChannelInfo, 0, len(adapterRegistry))
	for _, reg := range adapterRegistry {
		infos = append(infos, reg.info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

// SupportedChannelNames returns the names of registered channels.
func SupportedChannelNames() []string {
	infos := SupportedChannels()
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name)
	}
	return names
}

// ChannelSupported reports whether name is a registered channel.
func ChannelSupported(name string) bool {
	_, ok := adapterRegistry[name]
	return ok
}
