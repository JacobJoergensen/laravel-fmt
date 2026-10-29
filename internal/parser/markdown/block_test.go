package markdown

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestParseDispatchesEachBlockForm(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want any
	}{
		{"atx heading", "# Title", (*ATXHeading)(nil)},
		{"setext heading (=)", "Title\n=====", (*SetextHeading)(nil)},
		{"thematic break", "---", (*ThematicBreak)(nil)},
		{"fenced code (backticks)", "```go\nfoo\n```", (*FencedCodeBlock)(nil)},
		{"fenced code (tildes)", "~~~\nfoo\n~~~", (*FencedCodeBlock)(nil)},
		{"indented code", "    foo", (*IndentedCodeBlock)(nil)},
		{"html block", "<div>", (*HTMLBlock)(nil)},
		{"blockquote", "> quoted", (*Blockquote)(nil)},
		{"unordered list", "- one", (*List)(nil)},
		{"ordered list", "1. one", (*List)(nil)},
		{"link ref def", "[label]: https://example.com", (*LinkRefDef)(nil)},
		{"table", "| a | b |\n| --- | --- |", (*Table)(nil)},
		{"paragraph", "just text", (*Paragraph)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			if len(file.Blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(file.Blocks))
			}

			gotType := typeName(file.Blocks[0])
			wantType := typeName(tc.want)

			if gotType != wantType {
				t.Fatalf("block is %s, want %s", gotType, wantType)
			}
		})
	}
}

func TestCommentHTMLBlockEndsAtCloser(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantBlocks  int
		wantContent string
		wantSecond  any
	}{
		{
			"comment then table",
			"<!-- laravel-fmt-ignore -->\n| a | b |\n| --- | --- |",
			2, "<!-- laravel-fmt-ignore -->", (*Table)(nil),
		},
		{
			"comment then paragraph",
			"<!-- x -->\nfollowing text",
			2, "<!-- x -->", (*Paragraph)(nil),
		},
		{
			"multiline comment ends at closer",
			"<!-- line one\nline two -->\nfollowing text",
			2, "<!-- line one\nline two -->", (*Paragraph)(nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			if len(file.Blocks) != tc.wantBlocks {
				t.Fatalf("got %d blocks, want %d", len(file.Blocks), tc.wantBlocks)
			}

			html, ok := file.Blocks[0].(*HTMLBlock)
			if !ok {
				t.Fatalf("first block is %s, want *HTMLBlock", typeName(file.Blocks[0]))
			}

			if string(html.Content) != tc.wantContent {
				t.Fatalf("comment content = %q, want %q", html.Content, tc.wantContent)
			}

			if got := typeName(file.Blocks[1]); got != typeName(tc.wantSecond) {
				t.Fatalf("second block is %s, want %s", got, typeName(tc.wantSecond))
			}
		})
	}
}

func TestNonCommentHTMLBlockRunsToBlankLine(t *testing.T) {
	file, _ := Parse([]byte("<div>\nstill html\n</div>"))

	if len(file.Blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (a tag HTML block runs to a blank line)", len(file.Blocks))
	}
}

func TestATXHeadingLevel(t *testing.T) {
	cases := []struct {
		src       string
		level     int
		isHeading bool
	}{
		{"# a", 1, true},
		{"## a", 2, true},
		{"### a", 3, true},
		{"#### a", 4, true},
		{"##### a", 5, true},
		{"###### a", 6, true},
		{"####### a", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			if tc.isHeading {
				h, ok := file.Blocks[0].(*ATXHeading)

				if !ok {
					t.Fatalf("block is %T, want *ATXHeading", file.Blocks[0])
				}

				if h.Level != tc.level {
					t.Fatalf("Level=%d, want %d", h.Level, tc.level)
				}
			} else {
				if _, ok := file.Blocks[0].(*ATXHeading); ok {
					t.Fatalf("seven #'s should not produce a heading")
				}
			}
		})
	}
}

func TestSetextPromotionRequiresPrecedingParagraph(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want any
	}{
		{"paragraph + === becomes setext", "Title\n===", (*SetextHeading)(nil)},
		{"paragraph + --- becomes setext", "Title\n---", (*SetextHeading)(nil)},
		{"bare --- is thematic break", "---", (*ThematicBreak)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			if len(file.Blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(file.Blocks))
			}

			got := typeName(file.Blocks[0])
			want := typeName(tc.want)

			if got != want {
				t.Fatalf("block is %s, want %s", got, want)
			}
		})
	}
}

func TestParagraphInterruptedByBlockStart(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"heading interrupts paragraph", "para\n# heading"},
		{"thematic break interrupts paragraph", "para\n***"},
		{"fenced code interrupts paragraph", "para\n```\ncode\n```"},
		{"blockquote interrupts paragraph", "para\n> quoted"},
		{"list marker interrupts paragraph", "para\n- item"},
		{"list marker with a tab gap interrupts paragraph", "para\n-\tx"},
		{"block-level tag interrupts paragraph", "para\n<div>x</div>"},
		{"comment interrupts paragraph", "para\n<!-- c -->"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			if len(file.Blocks) < 2 {
				t.Fatalf("got %d blocks, want >=2 (paragraph not interrupted)", len(file.Blocks))
			}

			if _, ok := file.Blocks[0].(*Paragraph); !ok {
				t.Fatalf("first block is %T, want *Paragraph", file.Blocks[0])
			}
		})
	}

	file, _ := Parse([]byte("para\n<span>x</span>"))

	if len(file.Blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (an inline tag opened a block)", len(file.Blocks))
	}
}

func TestThematicBreakBeatsListMarkerDispatch(t *testing.T) {
	file, _ := Parse([]byte("---\n"))

	if _, ok := file.Blocks[0].(*ThematicBreak); !ok {
		t.Fatalf("block is %T, want *ThematicBreak", file.Blocks[0])
	}
}

func TestListMarkerFamilyContinuity(t *testing.T) {
	file, _ := Parse([]byte("- a\n* b\n"))

	var lists int

	for _, b := range file.Blocks {
		if _, ok := b.(*List); ok {
			lists++
		}
	}

	if lists != 2 {
		t.Fatalf("got %d lists, want 2", lists)
	}
}

func TestLooseListDetected(t *testing.T) {
	file, _ := Parse([]byte("- a\n\n- b\n\n- c\n"))

	lists := 0

	var found *List

	for _, b := range file.Blocks {
		if l, ok := b.(*List); ok {
			lists++
			found = l
		}
	}

	if lists != 1 {
		t.Fatalf("got %d lists, want 1 (loose list should not split)", lists)
	}

	if !found.Loose {
		t.Fatalf("Loose=false, want true for blank-separated items")
	}

	if len(found.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(found.Items))
	}
}

func TestTightListStaysTight(t *testing.T) {
	file, _ := Parse([]byte("- a\n- b\n- c\n"))

	var found *List

	for _, b := range file.Blocks {
		if l, ok := b.(*List); ok {
			found = l
			break
		}
	}

	if found == nil {
		t.Fatalf("no List produced")
		return
	}

	if found.Loose {
		t.Fatalf("Loose=true, want false for contiguous items")
	}

	if len(found.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(found.Items))
	}
}

func TestMultiParagraphListItem(t *testing.T) {
	src := "- first paragraph\n\n  continuation of first item\n- second item\n"
	file, _ := Parse([]byte(src))

	lists := 0

	var found *List

	for _, b := range file.Blocks {
		if l, ok := b.(*List); ok {
			lists++
			found = l
		}
	}

	if lists != 1 {
		t.Fatalf("got %d lists, want 1 (multi-paragraph item should not split the list)", lists)
	}

	if !found.Loose {
		t.Fatalf("Loose=false, want true for a list whose item has an internal blank")
	}

	if len(found.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(found.Items))
	}

	firstContent := string(found.Items[0].Content)

	if !strings.Contains(firstContent, "\n\n") {
		t.Fatalf("item 0 content does not contain the paragraph separator: %q", firstContent)
	}

	if !strings.Contains(firstContent, "continuation of first item") {
		t.Fatalf("item 0 content missing the continuation paragraph: %q", firstContent)
	}
}

// Which lines join the item follows league/commonmark.
func TestListItemLazyContinuation(t *testing.T) {
	cases := []struct {
		name           string
		src            string
		wantContent    string
		wantTrailBlock any
	}{
		{
			name:        "bare line continues the open paragraph",
			src:         "- first\nsecond\n",
			wantContent: "first\nsecond",
		},
		{
			name:        "line under a wide marker continues the paragraph",
			src:         "100) first\n  second\n",
			wantContent: "first\nsecond",
		},
		{
			name:        "setext underline is escaped, not an interrupter",
			src:         "- first\n===\n",
			wantContent: "first\n\\===",
		},
		{
			name:        "link reference definition is not an interrupter",
			src:         "- first\n[a]: /b\n",
			wantContent: "first\n[a]: /b",
		},
		{
			name:        "inline tag opens no block and continues the paragraph",
			src:         "- first\n<span>x</span>\n",
			wantContent: "first\n<span>x</span>",
		},
		{
			name:        "laziness reaches the paragraph of a nested blockquote",
			src:         "- > quoted\nsecond\n",
			wantContent: "> quoted\nsecond",
		},
		{
			name:           "heading interrupts",
			src:            "- first\n# heading\n",
			wantContent:    "first",
			wantTrailBlock: (*ATXHeading)(nil),
		},
		{
			name:           "thematic break interrupts",
			src:            "- first\n---\n",
			wantContent:    "first",
			wantTrailBlock: (*ThematicBreak)(nil),
		},
		{
			name:           "opening fence interrupts",
			src:            "- first\n```\ncode\n```\n",
			wantContent:    "first",
			wantTrailBlock: (*FencedCodeBlock)(nil),
		},
		{
			name:           "blockquote interrupts",
			src:            "- first\n> quoted\n",
			wantContent:    "first",
			wantTrailBlock: (*Blockquote)(nil),
		},
		{
			name:           "block-level tag interrupts",
			src:            "- first\n<div>x</div>\n",
			wantContent:    "first",
			wantTrailBlock: (*HTMLBlock)(nil),
		},
		{
			name:           "a list marker interrupts whatever its number",
			src:            "- first\n2) second\n",
			wantContent:    "first",
			wantTrailBlock: (*List)(nil),
		},
		{
			name:           "a blank line closes the paragraph, so nothing is lazy",
			src:            "- first\n\nstray\n",
			wantContent:    "first",
			wantTrailBlock: (*Paragraph)(nil),
		},
		{
			name:           "no paragraph is open after a heading in the item",
			src:            "- # heading\nstray\n",
			wantContent:    "# heading",
			wantTrailBlock: (*Paragraph)(nil),
		},
		{
			name:           "no paragraph is open inside a fence in the item",
			src:            "- ```\n  code\nstray\n",
			wantContent:    "```\ncode",
			wantTrailBlock: (*Paragraph)(nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			list, ok := file.Blocks[0].(*List)
			if !ok {
				t.Fatalf("block 0 is %T, want *List", file.Blocks[0])
			}

			last := list.Items[len(list.Items)-1]

			if string(last.Content) != tc.wantContent {
				t.Fatalf("item content=%q, want %q", string(last.Content), tc.wantContent)
			}

			if tc.wantTrailBlock == nil {
				if len(file.Blocks) != 1 {
					t.Fatalf("got %d blocks, want 1 (the lazy line was split off)", len(file.Blocks))
				}

				return
			}

			if len(file.Blocks) < 2 {
				t.Fatalf("got %d blocks, want >=2 (interrupter not split off)", len(file.Blocks))
			}

			gotType := typeName(file.Blocks[1])
			wantType := typeName(tc.wantTrailBlock)

			if gotType != wantType {
				t.Fatalf("block 1 is %s, want %s", gotType, wantType)
			}
		})
	}
}

func TestNestedListContentStaysInsideParentItem(t *testing.T) {
	src := "- outer\n  - inner\n"
	file, _ := Parse([]byte(src))

	lists := 0

	for _, b := range file.Blocks {
		if _, ok := b.(*List); ok {
			lists++
		}
	}

	if lists != 1 {
		t.Fatalf("got %d lists, want 1 (nested item got promoted)", lists)
	}
}

func TestContainerBodyIsParsedFromItsContent(t *testing.T) {
	file, _ := Parse([]byte("- outer\n  > quoted\n"))

	list, ok := file.Blocks[0].(*List)
	if !ok {
		t.Fatalf("block 0 is %T, want *List", file.Blocks[0])
	}

	item := list.Items[0]
	if item.Body == nil || !bytes.Equal(item.Body.Src, item.Content) || len(item.Body.Blocks) != 2 {
		t.Fatalf("item Body = %+v, want its Content %q parsed into two blocks", item.Body, item.Content)
	}

	quote, ok := item.Body.Blocks[1].(*Blockquote)
	if !ok {
		t.Fatalf("item Body block 1 is %T, want *Blockquote", item.Body.Blocks[1])
	}

	if quote.Body == nil || !bytes.Equal(quote.Body.Src, quote.Content) || len(quote.Body.Blocks) != 1 {
		t.Fatalf("quote Body = %+v, want its Content %q parsed into one block", quote.Body, quote.Content)
	}
}

func TestTablePromotion(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantHeaders []string
		wantAligns  []TableAlign
		wantRows    [][]string
	}{
		{
			name:        "basic table no alignment",
			src:         "| a | b |\n| --- | --- |\n| 1 | 2 |\n",
			wantHeaders: []string{"a", "b"},
			wantAligns:  []TableAlign{TableAlignNone, TableAlignNone},
			wantRows:    [][]string{{"1", "2"}},
		},
		{
			name:        "alignments parsed left right center",
			src:         "| a | b | c |\n| :--- | ---: | :---: |\n| 1 | 2 | 3 |\n",
			wantHeaders: []string{"a", "b", "c"},
			wantAligns:  []TableAlign{TableAlignLeft, TableAlignRight, TableAlignCenter},
			wantRows:    [][]string{{"1", "2", "3"}},
		},
		{
			name:        "leading and trailing pipes optional",
			src:         "a | b\n--- | ---\n1 | 2\n",
			wantHeaders: []string{"a", "b"},
			wantAligns:  []TableAlign{TableAlignNone, TableAlignNone},
			wantRows:    [][]string{{"1", "2"}},
		},
		{
			name:        "escaped pipe inside cell",
			src:         "| a\\|b | c |\n| --- | --- |\n",
			wantHeaders: []string{"a|b", "c"},
			wantAligns:  []TableAlign{TableAlignNone, TableAlignNone},
		},
		{
			name:        "missing trailing cells padded to header width",
			src:         "| a | b | c |\n| --- | --- | --- |\n| 1 | 2 |\n",
			wantHeaders: []string{"a", "b", "c"},
			wantAligns:  []TableAlign{TableAlignNone, TableAlignNone, TableAlignNone},
			wantRows:    [][]string{{"1", "2", ""}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))
			tbl, ok := file.Blocks[0].(*Table)

			if !ok {
				t.Fatalf("block 0 is %T, want *Table", file.Blocks[0])
			}

			if len(tbl.Header) != len(tc.wantHeaders) {
				t.Fatalf("got %d headers, want %d", len(tbl.Header), len(tc.wantHeaders))
			}

			for i, h := range tbl.Header {
				if string(h.Content) != tc.wantHeaders[i] {
					t.Fatalf("Header[%d]=%q, want %q", i, string(h.Content), tc.wantHeaders[i])
				}
			}

			if len(tbl.Aligns) != len(tc.wantAligns) {
				t.Fatalf("got %d aligns, want %d", len(tbl.Aligns), len(tc.wantAligns))
			}

			for i, a := range tbl.Aligns {
				if a != tc.wantAligns[i] {
					t.Fatalf("Aligns[%d]=%d, want %d", i, a, tc.wantAligns[i])
				}
			}

			if len(tbl.Rows) != len(tc.wantRows) {
				t.Fatalf("got %d rows, want %d", len(tbl.Rows), len(tc.wantRows))
			}

			for ri, row := range tbl.Rows {
				for ci, cell := range row {
					if string(cell.Content) != tc.wantRows[ri][ci] {
						t.Fatalf("Rows[%d][%d]=%q, want %q", ri, ci, string(cell.Content), tc.wantRows[ri][ci])
					}
				}
			}
		})
	}
}

// Expected values are league/commonmark's verdicts.
func TestTableNotPromotedWithoutPipeInBothRows(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want any
	}{
		{"setext takes precedence on bare ---", "Title\n---\n", (*SetextHeading)(nil)},
		{"alignment-only separator without pipe stays paragraph", "Title\n:---:\n", (*Paragraph)(nil)},
		{"separator without a pipe stays paragraph", "|0\n-:\n", (*Paragraph)(nil)},
		{"header without a pipe stays paragraph", "0\n-:|\n", (*Paragraph)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			gotType := typeName(file.Blocks[0])
			wantType := typeName(tc.want)

			if gotType != wantType {
				t.Fatalf("block 0 is %s, want %s", gotType, wantType)
			}
		})
	}
}

// Expected values are verdicts league/commonmark and micromark agree on.
func TestLinkRefDefRejectsBadLabelAndDestination(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want any
	}{
		{"plain destination", "[a]: b", (*LinkRefDef)(nil)},
		{"dash destination", "[a]:-a", (*LinkRefDef)(nil)},
		{"brackets in destination", "[a]:1[b", (*LinkRefDef)(nil)},
		{"angle bracket in destination", "[a]:a<b", (*LinkRefDef)(nil)},
		{"balanced parens", "[a]:b(c)", (*LinkRefDef)(nil)},
		{"escaped paren", `[a]:b\)`, (*LinkRefDef)(nil)},
		{"unescaped closing paren", "[a]:b)", (*Paragraph)(nil)},
		{"unescaped opening paren", "[a]:b(", (*Paragraph)(nil)},
		{"unbalanced open then text", "[a]:b(c", (*Paragraph)(nil)},
		{"closing paren first", "[a]:)b", (*Paragraph)(nil)},
		{"unescaped bracket in label", "[a[b]: x", (*Paragraph)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			gotType := typeName(file.Blocks[0])
			wantType := typeName(tc.want)

			if gotType != wantType {
				t.Fatalf("block 0 is %s, want %s", gotType, wantType)
			}
		})
	}
}

func TestBlockquoteLazyContinuation(t *testing.T) {
	cases := []struct {
		name           string
		src            string
		wantBQContent  string
		wantTrailBlock any
	}{
		{
			name:          "bare line continues open paragraph",
			src:           "> first line\nsecond line\n",
			wantBQContent: "first line\nsecond line",
		},
		{
			name:           "blank line ends the quote",
			src:            "> first line\n\nstray\n",
			wantBQContent:  "first line",
			wantTrailBlock: (*Paragraph)(nil),
		},
		{
			name:           "heading after bare line breaks the quote",
			src:            "> first line\n# heading\n",
			wantBQContent:  "first line",
			wantTrailBlock: (*ATXHeading)(nil),
		},
		{
			name:           "list marker after bare line breaks the quote",
			src:            "> first line\n- item\n",
			wantBQContent:  "first line",
			wantTrailBlock: (*List)(nil),
		},
		{
			name:           "blank `>` line closes paragraph; bare line is outside",
			src:            "> first line\n>\nstray\n",
			wantBQContent:  "first line\n",
			wantTrailBlock: (*Paragraph)(nil),
		},
		{
			name:          "setext underline is escaped, so it stays text",
			src:           "> first line\n===\n",
			wantBQContent: "first line\n\\===",
		},
		{
			name:          "bare lines on either side of a marked line all continue",
			src:           "> first line\nsecond\n> third\nfourth\n",
			wantBQContent: "first line\nsecond\nthird\nfourth",
		},
		{
			name:           "fence opened after a bare line closes the paragraph",
			src:            "> first line\nsecond\n> ```\nstray\n",
			wantBQContent:  "first line\nsecond\n```",
			wantTrailBlock: (*Paragraph)(nil),
		},
		{
			name:           "block-level tag after bare line breaks the quote",
			src:            "> first line\n<div>x</div>\n",
			wantBQContent:  "first line",
			wantTrailBlock: (*HTMLBlock)(nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))
			bq, ok := file.Blocks[0].(*Blockquote)

			if !ok {
				t.Fatalf("block 0 is %T, want *Blockquote", file.Blocks[0])
			}

			if string(bq.Content) != tc.wantBQContent {
				t.Fatalf("Content=%q, want %q", string(bq.Content), tc.wantBQContent)
			}

			if tc.wantTrailBlock == nil {
				if len(file.Blocks) != 1 {
					t.Fatalf("got %d blocks, want 1", len(file.Blocks))
				}

				return
			}

			if len(file.Blocks) < 2 {
				t.Fatalf("got %d blocks, want >=2 (interrupter not split off)", len(file.Blocks))
			}

			gotType := typeName(file.Blocks[1])
			wantType := typeName(tc.wantTrailBlock)

			if gotType != wantType {
				t.Fatalf("block 1 is %s, want %s", gotType, wantType)
			}
		})
	}
}

// Quotes that deepen and then drop back leave a lazy line at every level. Each
// lazy check is answered once per document, so doubling the lines stays near
// quadratic; asking again inside every check made the work exponential.
func TestNestedLazyQuotesScaleNearQuadratically(t *testing.T) {
	allocs := func(lines int) float64 {
		var sb strings.Builder

		for i := range lines {
			sb.WriteString(strings.Repeat("> ", i%6) + "line\n")
		}

		src := []byte(sb.String())

		return testing.AllocsPerRun(3, func() { Parse(src) })
	}

	if ratio := allocs(96) / allocs(48); ratio > 5 {
		t.Fatalf("doubling the lines multiplied allocations by %.1f, want at most 5", ratio)
	}
}

func TestFrontmatterFenceCharacterDistinguishesFormats(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want byte
	}{
		{"yaml frontmatter", "---\ntitle: x\n---\n", '-'},
		{"toml frontmatter", "+++\ntitle = \"x\"\n+++\n", '+'},
		{"no frontmatter when first line is text", "title\n", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))
			var fm *Frontmatter

			if len(file.Blocks) > 0 {
				fm, _ = file.Blocks[0].(*Frontmatter)
			}

			if tc.want == 0 {
				if fm != nil {
					t.Fatalf("got Frontmatter on a file without a fence")
				}

				return
			}

			if fm == nil {
				t.Fatalf("no Frontmatter emitted")
				return
			}

			if fm.FenceChar != tc.want {
				t.Fatalf("FenceChar=%c, want %c", fm.FenceChar, tc.want)
			}
		})
	}
}

func TestFencedCodeClosingFenceLengthAndChar(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		closed bool
	}{
		{"matching length and char closes", "```\nfoo\n```\n", true},
		{"longer close fence closes", "```\nfoo\n`````\n", true},
		{"shorter close fence does not close", "````\nfoo\n```\n", false},
		{"different fence char does not close", "```\nfoo\n~~~\n", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))
			fcb, ok := file.Blocks[0].(*FencedCodeBlock)

			if !ok {
				t.Fatalf("block is %T, want *FencedCodeBlock", file.Blocks[0])
			}

			closed := fcb.CloseFence.End.Offset > fcb.CloseFence.Start.Offset

			if closed != tc.closed {
				t.Fatalf("closed=%v, want %v", closed, tc.closed)
			}
		})
	}
}

// Expected values are league/commonmark's verdicts.
func TestHTMLBlockEndsAtItsOwnCloser(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantContent string
		wantBlocks  int
	}{
		{"comment holds interior blanks", "<!--\na\n\nb\n-->\n\nafter\n", "<!--\na\n\nb\n-->", 2},
		{"raw text holds interior blanks", "<pre>\na\n\nb\n</pre>\n\nafter\n", "<pre>\na\n\nb\n</pre>", 2},
		{"closer matches whatever its case", "<PRE>\nx\n\n</PRE>\n\nafter\n", "<PRE>\nx\n\n</PRE>", 2},
		{"processing instruction", "<?php\n\n?>\n\nafter\n", "<?php\n\n?>", 2},
		{"declaration ends at the angle", "<!A\n\n>\n\nafter\n", "<!A\n\n>", 2},
		{"cdata", "<![CDATA[\n\n]]>\n\nafter\n", "<![CDATA[\n\n]]>", 2},
		{"unclosed raw text runs to eof", "<pre>\nx\n\n# y\n", "<pre>\nx\n\n# y", 1},
		{"block tag still ends at a blank", "<div>\na\n\nb\n", "<div>\na", 2},
		{"closer on the opening line", "<!-- x -->\n\nafter\n", "<!-- x -->", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			block, ok := file.Blocks[0].(*HTMLBlock)
			if !ok {
				t.Fatalf("block 0 is %T, want *HTMLBlock", file.Blocks[0])
			}

			if string(block.Content) != tc.wantContent {
				t.Fatalf("Content=%q, want %q", string(block.Content), tc.wantContent)
			}

			if len(file.Blocks) != tc.wantBlocks {
				t.Fatalf("got %d blocks, want %d", len(file.Blocks), tc.wantBlocks)
			}
		})
	}
}

// Expected values are league/commonmark's verdicts.
func TestListItemLooseOnlyForOwnBlocks(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantList  bool
		wantFirst bool
	}{
		{"no blank at all", "- a\n- b\n", false, false},
		{"two own blocks", "- a\n\n  b\n- c\n", true, true},
		{"blank between items", "- a\n\n- b\n", true, false},
		{"paragraph and fence", "- a\n\n  ```\n  x\n  ```\n", true, true},
		{"blank inside a nested item", "- a\n  - b\n\n    c\n", false, false},
		{"blank between nested items", "- a\n  - b\n\n  - c\n", false, false},
		{"blank inside a fence", "- ```\n  x\n\n  y\n  ```\n", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			list, ok := file.Blocks[0].(*List)
			if !ok {
				t.Fatalf("block 0 is %T, want *List", file.Blocks[0])
			}

			if list.Loose != tc.wantList {
				t.Fatalf("List.Loose=%v, want %v", list.Loose, tc.wantList)
			}

			if list.Items[0].Loose != tc.wantFirst {
				t.Fatalf("Items[0].Loose=%v, want %v", list.Items[0].Loose, tc.wantFirst)
			}
		})
	}
}

// Expected values are league/commonmark's verdicts.
func TestFencedCodeDedentsByFenceIndent(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unindented fence keeps content", "```\n    code\n```\n", "    code"},
		{"one space strips one column", " ```\n    code\n```\n", "   code"},
		{"tab expands, the columns past the indent survive", "  ```\n\tcode\n```\n", "  code"},
		{"a shorter line loses only what it has", "   ```\n     a\n  b\n```\n", "  a\nb"},
		{"whitespace-only line keeps its residue", " ```\n   \n```\n", "  "},
		{"only indentation is removed, never content", "   ```\n a\n```\n", "a"},
		{"info string does not change the strip", "  ~~~info\n   x\n  ~~~\n", " x"},
		{"an unclosed fence is stripped too", " ```\n   x\n", "  x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			fcb, ok := file.Blocks[0].(*FencedCodeBlock)
			if !ok {
				t.Fatalf("block is %T, want *FencedCodeBlock", file.Blocks[0])
			}

			if string(fcb.Content) != tc.want {
				t.Fatalf("Content=%q, want %q", string(fcb.Content), tc.want)
			}
		})
	}
}

func typeName(v any) string {
	return fmt.Sprintf("%T", v)
}

func TestStripFenceIndentChangesOnlyIndentation(t *testing.T) {
	cases := []struct {
		name    string
		content string
		strip   int
		want    string
	}{
		{"zero strip is identity", " x\n\n", 0, " x\n\n"},
		{"trailing blank line survives", " x\n\n", 1, "x\n\n"},
		{"two trailing blanks survive", " x\n\n\n", 1, "x\n\n\n"},
		{"blank-only content survives", "\n", 1, "\n"},
		{"interior blank survives", " a\n\n b\n", 1, "a\n\nb\n"},
		{"CRLF terminators are kept", " a\r\n\r\n", 1, "a\r\n\r\n"},
		{"unterminated last line", " a\n b", 1, "a\nb"},
		{"line shallower than the strip", " a\n   b\n", 2, "a\n b\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripFenceIndent([]byte(tc.content), tc.strip)

			if string(got) != tc.want {
				t.Fatalf("stripFenceIndent(%q, %d) = %q, want %q", tc.content, tc.strip, got, tc.want)
			}
		})
	}
}

// Expected values are league/commonmark's verdicts.
func TestListItemTabAfterMarkerReachesTheTabStop(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantContent string
		wantBlocks  int
	}{
		{"a line short of the stop leaves the item", "-\tfoo\n\n  bar\n", "foo", 2},
		{"a line at the stop stays in the item", "-\tfoo\n\n    bar\n", "foo\n\nbar", 1},
		{"ordered marker", "1.\tfoo\n\n   bar\n", "foo", 2},
		{"a gap past four columns keeps the tab's width", "-\t\tfoo\n", "      foo", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			list, ok := file.Blocks[0].(*List)
			if !ok {
				t.Fatalf("block 0 is %T, want *List", file.Blocks[0])
			}

			if string(list.Items[0].Content) != tc.wantContent {
				t.Fatalf("Items[0].Content=%q, want %q", list.Items[0].Content, tc.wantContent)
			}

			if len(file.Blocks) != tc.wantBlocks {
				t.Fatalf("got %d blocks, want %d", len(file.Blocks), tc.wantBlocks)
			}
		})
	}
}

func TestListItemKeepsTheBytesOfAFenceBehindANestedMarker(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantContent string
	}{
		{"bullet inside bullet", "* * ~~~\n    \t0\n    ~~~\n", "* ~~~\n  \t0\n  ~~~"},
		{"bullet inside ordered", "1. * ~~~\n     \tx\n     ~~~\n", "* ~~~\n  \tx\n  ~~~"},
		{
			"leaving the nested item ends its fence",
			"* * ~~~\n    \tcode\n  outer\n  \tmore\n",
			"* ~~~\n  \tcode\nouter\n  more",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			list, ok := file.Blocks[0].(*List)
			if !ok {
				t.Fatalf("block 0 is %T, want *List", file.Blocks[0])
			}

			if got := string(list.Items[0].Content); got != tc.wantContent {
				t.Fatalf("item content=%q, want %q", got, tc.wantContent)
			}
		})
	}
}
