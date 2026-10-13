package php

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// preprocessUseRuns expands grouped `use` imports and/or sorts consecutive use
// runs per cfg.PHP.ExpandUseGroups and SortUses. A non-use statement or a
// directive-frozen use ends a run, so surrounding statements keep their position.
func (b *builder) preprocessUseRuns(stmts []parser.Stmt) []parser.Stmt {
	if !b.cfg.PHP.SortUses && !b.cfg.PHP.ExpandUseGroups {
		return stmts
	}

	out := make([]parser.Stmt, 0, len(stmts))

	i := 0

	for i < len(stmts) {
		_, isUse := stmts[i].(*parser.UseStmt)

		if !isUse || b.hasIgnoreDirective(stmts[i].Span().Start.Offset) {
			out = append(out, stmts[i])
			i++

			continue
		}

		var run []*parser.UseStmt

		j := i

		for j < len(stmts) {
			next, ok := stmts[j].(*parser.UseStmt)

			if !ok || b.hasIgnoreDirective(stmts[j].Span().Start.Offset) {
				break
			}

			run = append(run, next)
			j++
		}

		if b.cfg.PHP.ExpandUseGroups {
			expanded := make([]*parser.UseStmt, 0, len(run))

			for _, u := range run {
				// A comment inside the statement has no place among the split
				// imports, and every piece carries the original's span, so the
				// comment fallback would print the whole group beside its
				// pieces: a duplicate import, which PHP rejects.
				if b.spanHasComment(u.Span()) || u.Prefix != nil && b.cfg.PHP.KeepBracedUseGroups {
					expanded = append(expanded, u)

					continue
				}

				expanded = append(expanded, expandUseStmt(u)...)
			}

			run = expanded
		}

		if b.cfg.PHP.SortUses {
			for k, u := range run {
				if u.Prefix != nil && !b.spanHasComment(u.Span()) {
					run[k] = sortGroupItems(u, b.cfg.PHP.ImportOrder)
				}
			}
		}

		if b.cfg.PHP.SortUses && len(run) > 1 {
			b.claimUseTrails(run)

			opener := run[0].Span().Start.Offset
			closer := run[len(run)-1]
			sortUseStmts(run, b.cfg.PHP.ImportOrder)
			b.keepRunHead(opener, run[0].Span().Start.Offset)
			b.keepRunTail(closer, run[len(run)-1])

			// Sorting reordered the run, so an author blank line before any
			// import now points at the wrong statement; the run stays one
			// contiguous group, keeping only the blank before its first import.
			for _, u := range run[1:] {
				if b.useRunInterior == nil {
					b.useRunInterior = make(map[int]bool)
				}

				b.useRunInterior[u.Span().Start.Offset] = true
			}
		}

		for _, u := range run {
			out = append(out, u)
		}

		i = j
	}

	return out
}

// keepRunHead records, when sorting changed which import opens a run, that the
// trivia before the run stays at its head. Trivia is stored on the token after
// it, so it would otherwise move with the import that opened the run.
func (b *builder) keepRunHead(opener, head int) {
	if opener == head {
		return
	}

	if b.useRunHead == nil {
		b.useRunHead = make(map[int]int)
		b.useRunHeadMoved = make(map[int]bool)
	}

	b.useRunHead[head] = opener
	b.useRunHeadMoved[opener] = true
}

// The comments claimUseTrails leaves on a run's last import (an ignore
// directive and what follows it) would otherwise ride the new last import,
// where that import's own line comment swallows them.
func (b *builder) keepRunTail(closer, tail *parser.UseStmt) {
	if closer == tail {
		return
	}

	end := closer.Span().End.Offset

	i := sort.Search(len(b.tokens), func(i int) bool { return b.tokens[i].Span.Start.Offset >= end })
	if i == len(b.tokens) {
		return
	}

	if b.useRunTailMoved == nil {
		b.useRunTailMoved = make(map[int]bool)
	}

	b.useRunTailMoved[b.tokens[i].Span.Start.Offset] = true
}

// runHeadTrivia returns where the trivia before the statement at offset was
// written: the opener of its sorted import run when sorting moved it to the
// head, reported by moved, otherwise offset itself.
func (b *builder) runHeadTrivia(offset int) (before int, moved bool) {
	if opener, ok := b.useRunHead[offset]; ok {
		return opener, true
	}

	return offset, false
}

// claimUseTrails, run before an import run is sorted, gives each import the
// comments written after it on its own line, which trivia stores on the token
// that follows. An expanded group's pieces share one span; only the last owns
// the comment.
func (b *builder) claimUseTrails(run []*parser.UseStmt) {
	for k, u := range run {
		end := u.Span().End.Offset
		if k+1 < len(run) && run[k+1].Span().End.Offset == end {
			continue
		}

		i := sort.Search(len(b.tokens), func(i int) bool { return b.tokens[i].Span.Start.Offset >= end })
		if i == len(b.tokens) {
			continue
		}

		var trail []parser.Trivia

		for _, tr := range b.tokens[i].Leading {
			if tr.Kind == parser.TriviaWhitespace && triviaLineBreaks(tr.Text) > 0 {
				break
			}

			// A directive speaks for what follows it, not for the import whose
			// line it shares, so it stays with the node it freezes.
			if isCommentTrivia(tr.Kind) && formatter.IsIgnore(tr.Text) {
				break
			}

			if isCommentTrivia(tr.Kind) {
				trail = append(trail, tr)
			}
		}

		if len(trail) == 0 {
			continue
		}

		if b.useTrail == nil {
			b.useTrail = make(map[parser.Stmt][]parser.Trivia)
			b.movedTrail = make(map[int]bool)
		}

		b.useTrail[u] = trail

		for _, tr := range trail {
			b.movedTrail[tr.Span.Start.Offset] = true
		}
	}
}

// expandUseStmt expands a multi-item or grouped use statement into one
// single-item use per target (respecting the `function`/`const` kind at both
// levels). The first expanded item inherits the original keyword's leading
// trivia; the rest start clean so they sit flush.
func expandUseStmt(u *parser.UseStmt) []*parser.UseStmt {
	if u.Prefix == nil && len(u.Items) == 1 {
		return []*parser.UseStmt{u}
	}

	result := make([]*parser.UseStmt, 0, len(u.Items))

	for i, item := range u.Items {
		kw := u.Keyword

		if i > 0 {
			kw.Leading = nil
		}

		name := item.Name

		if u.Prefix != nil {
			name = prefixName(u.Prefix, item.Name)
		}

		useKind := u.UseKind

		if useKind.Kind == parser.KindInvalid {
			useKind = item.Kind
		}

		result = append(result, &parser.UseStmt{
			Keyword: kw,
			UseKind: useKind,
			Items: []*parser.UseItem{
				{
					Name:    name,
					AsToken: item.AsToken,
					Alias:   item.Alias,
				},
			},
			Semicolon: u.Semicolon,
		})
	}

	return result
}

// prefixName concatenates the parts of prefix and inner into one qualified
// name, preserving prefix's leading backslash so a fully-qualified group prefix
// survives expansion.
func prefixName(prefix, inner *parser.Name) *parser.Name {
	parts := make([]parser.Token, 0, len(prefix.Parts)+len(inner.Parts))
	parts = append(parts, prefix.Parts...)
	parts = append(parts, inner.Parts...)

	return &parser.Name{
		LeadingBackslash: prefix.LeadingBackslash,
		Parts:            parts,
	}
}

// sortUseStmts stably sorts single-item use statements by (import-kind rank,
// lowercase name). The kind rank follows cfg.PHP.ImportOrder; the default
// const → class → function matches Pint's Laravel preset (ordered_imports).
func sortUseStmts(uses []*parser.UseStmt, order []string) {
	slices.SortStableFunc(uses, func(a, b *parser.UseStmt) int {
		rankA, rankB := importKindRank(a, order), importKindRank(b, order)

		if rankA != rankB {
			return cmp.Compare(rankA, rankB)
		}

		return cmp.Compare(useNameKey(a), useNameKey(b))
	})
}

// Pint's ordered_imports sorts inside a group it keeps. u itself is left
// untouched.
func sortGroupItems(u *parser.UseStmt, order []string) *parser.UseStmt {
	items := slices.Clone(u.Items)

	slices.SortStableFunc(items, func(a, b *parser.UseItem) int {
		rankA, rankB := kindRank(itemKind(u, a), order), kindRank(itemKind(u, b), order)
		if rankA != rankB {
			return cmp.Compare(rankA, rankB)
		}

		return cmp.Compare(nameKey(a.Name), nameKey(b.Name))
	})

	sorted := *u
	sorted.Items = items

	return &sorted
}

func itemKind(u *parser.UseStmt, item *parser.UseItem) parser.Kind {
	if u.UseKind.Kind != parser.KindInvalid {
		return u.UseKind.Kind
	}

	return item.Kind.Kind
}

// A braced group sorts by its prefix and first item, where Pint places it
// among single imports.
func useNameKey(u *parser.UseStmt) string {
	if len(u.Items) == 0 {
		return ""
	}

	key := nameKey(u.Items[0].Name)

	if u.Prefix != nil {
		key = nameKey(u.Prefix) + "\\" + key
	}

	return key
}

func nameKey(n *parser.Name) string {
	if n == nil {
		return ""
	}

	var name strings.Builder

	for k, part := range n.Parts {
		if k > 0 {
			name.WriteByte('\\')
		}

		name.Write(part.Text)
	}

	return strings.ToLower(name.String())
}

// importKindRank returns u's import-kind position in order (smaller sorts
// earlier). A kind absent from order ranks after every listed kind, keeping the
// order total even for a partial imports_order from pint.json.
func importKindRank(u *parser.UseStmt, order []string) int {
	return kindRank(u.UseKind.Kind, order)
}

func kindRank(useKind parser.Kind, order []string) int {
	var kind string

	switch useKind {
	case parser.KindFunction:
		kind = config.ImportKindFunction
	case parser.KindConst:
		kind = config.ImportKindConst
	default:
		kind = config.ImportKindClass
	}

	if i := slices.Index(order, kind); i >= 0 {
		return i
	}

	return len(order)
}

func (b *builder) useStmt(n *parser.UseStmt) *document.Doc {
	head := document.Text("use ")
	if has(n.UseKind) {
		head = document.Concat(head, b.tok(n.UseKind), document.Text(" "))
	}

	if n.Prefix != nil {
		items := make([]*document.Doc, len(n.Items))
		for i, it := range n.Items {
			items[i] = b.useItem(it)
		}

		return document.Concat(head, b.name(n.Prefix), document.Text("\\{"), document.Join(document.Text(", "), items), document.Text("};"))
	}

	items := make([]*document.Doc, len(n.Items))
	for i, it := range n.Items {
		items[i] = b.useItem(it)
	}

	return document.Concat(head, document.Join(document.Text(", "), items), document.Text(";"))
}

func (b *builder) useItem(it *parser.UseItem) *document.Doc {
	item := b.name(it.Name)
	if has(it.Kind) {
		item = document.Concat(b.tok(it.Kind), document.Text(" "), item)
	}

	if has(it.AsToken) {
		item = document.Concat(item, document.Text(" as "), b.tok(it.Alias))
	}

	return item
}
