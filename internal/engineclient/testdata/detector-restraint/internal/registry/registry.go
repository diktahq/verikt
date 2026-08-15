package registry

var defaultName string

func init() {
	defaultName = "registry"
}

// Name returns the default name.
func Name() string { return defaultName }
