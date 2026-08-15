package counter

// Count allocates before writing, which is the correct form.
func Count(words []string) map[string]int {
	counts := make(map[string]int, len(words))
	for _, w := range words {
		counts[w]++
	}
	return counts
}
