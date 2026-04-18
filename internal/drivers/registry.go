package drivers

import (
	"fmt"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// Registry holds the concrete Driver for each endpoint id referenced in config.
type Registry struct {
	endpoints map[string]Driver
}

// BuildRegistry constructs a Registry from validated config. Returns an error
// if any endpoint uses an unsupported driver or a duplicate id slips through.
func BuildRegistry(cfg *config.Config) (*Registry, error) {
	r := &Registry{endpoints: map[string]Driver{}}
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverOpenAICompatible {
			return nil, fmt.Errorf("unsupported driver type %q", d.Driver)
		}
		for _, e := range d.Endpoints {
			if _, dup := r.endpoints[e.ID]; dup {
				return nil, fmt.Errorf("duplicate endpoint id %q", e.ID)
			}
			r.endpoints[e.ID] = NewOpenAICompatible(e.Config.APIBase, e.Config.APIKey)
		}
	}
	return r, nil
}

// Endpoint returns the driver for an endpoint id, or an error if unknown.
func (r *Registry) Endpoint(id string) (Driver, error) {
	d, ok := r.endpoints[id]
	if !ok {
		return nil, fmt.Errorf("unknown endpoint %q", id)
	}
	return d, nil
}
