package drivers

// SetEndpointForTest replaces the driver for an existing endpoint id.
// Intended only for tests — callers in production code should go through
// BuildRegistry.
func SetEndpointForTest(r *Registry, id string, d Driver) {
	if r == nil {
		return
	}
	r.endpoints[id] = d
}
