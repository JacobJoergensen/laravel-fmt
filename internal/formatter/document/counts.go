package document

// Count is the fewest and most times an item can print.
type Count struct {
	Min, Max int
}

// Tag marks a leaf as printing item ID Count times each time the leaf prints.
type Tag struct {
	ID    int
	Count Count
}

// PrintCounts reports how many times each tagged item in d can print. A doc in
// tags counts as its tags alone, whatever it holds.
func PrintCounts(d *Doc, tags map[*Doc][]Tag) map[int]Count {
	if own, ok := tags[d]; ok {
		out := make(map[int]Count, len(own))
		for _, t := range own {
			c := out[t.ID]
			out[t.ID] = Count{Min: c.Min + t.Count.Min, Max: c.Max + t.Count.Max}
		}

		return out
	}

	if d.kind == kindCondGroup || d.kind == kindIfBreak {
		out := PrintCounts(d.parts[0], tags)
		for _, p := range d.parts[1:] {
			out = either(out, PrintCounts(p, tags))
		}

		return out
	}

	var out map[int]Count
	for _, p := range d.parts {
		out = both(out, PrintCounts(p, tags))
	}

	return out
}

// both adds b into a; every map PrintCounts builds is its own.
func both(a, b map[int]Count) map[int]Count {
	if a == nil {
		return b
	}

	for id, c := range b {
		sum := a[id]
		a[id] = Count{Min: sum.Min + c.Min, Max: sum.Max + c.Max}
	}

	return a
}

// either is the counts of printing one of two docs: an item missing from one
// can print zero times.
func either(a, b map[int]Count) map[int]Count {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}

	out := make(map[int]Count, max(len(a), len(b)))

	for id, c := range a {
		other := b[id]
		out[id] = Count{Min: min(c.Min, other.Min), Max: max(c.Max, other.Max)}
	}

	for id, c := range b {
		if _, seen := a[id]; !seen {
			out[id] = Count{Min: 0, Max: c.Max}
		}
	}

	return out
}
