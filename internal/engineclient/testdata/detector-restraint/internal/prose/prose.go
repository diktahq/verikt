// Package prose contains English that happens to use SQL words.
//
// Reduced from a real project (no database, no driver, no query anywhere) where
// these two lines produced two error-severity sql_concatenation findings and
// failed the build. The project had already reworded one error message to avoid
// the detector, with a comment explaining why.
package prose

// Check builds a diagnostic message. `where` is a location, not a clause.
func Check(where string, problems []string) []string {
	return append(problems, where+": names no capability, so it decides nothing - delete the block")
}

// Describe explains what a table shows, in English.
func Describe(table string) string {
	return "select the rows from " + table + " that changed"
}

// Advise returns guidance that mentions updating and inserting.
func Advise(field string) string {
	return "update the " + field + " value, or insert a new record by hand"
}
