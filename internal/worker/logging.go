package worker

func withAttrs(attrs []any, extra ...any) []any {
	combined := make([]any, 0, len(attrs)+len(extra))
	combined = append(combined, attrs...)
	combined = append(combined, extra...)
	return combined
}
