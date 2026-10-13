package config

// DisableRelocatingPasses turns off the passes that move code from one part of
// a file to another.
//
// CONTRACT: call this whenever only part of the formatted output will reach the
// buffer, as range formatting does. Such a pass writes the two ends of a move
// as separate changes, so a range holding one end without the other deletes the
// moved code or duplicates it.
func DisableRelocatingPasses(cfg *File) {
	cfg.PHP.SortUses = false
}
