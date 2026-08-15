package coerce

// String uses the comma-ok form.
func String(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// Describe uses a type switch.
func Describe(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case error:
		return t.Error()
	default:
		return "unknown"
	}
}
