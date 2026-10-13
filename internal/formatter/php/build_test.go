package php

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

func buildPHP(t *testing.T, src string) string {
	t.Helper()

	file, errs := parser.Parse([]byte(src))
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}

	return string(BuildFile(file, config.Default()))
}

// PHP's lexer eats the one newline after `?>`, so `?>\n\n` leaves a trailing
// whitespace-only inline-HTML run and `?>\n` leaves none.
func TestBuildFileIsFixedPointWithBlankHTMLAfterCloseTag(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"one blank line", "<?php 0?>\n\n"},
		{"eaten newline only", "<?php 0?>\n"},
		{"no newline", "<?php 0?>"},
		{"several blank lines", "<?php 0?>\n\n\n"},
		{"spaces on the blank line", "<?php 0?>\n \n"},
		{"real trailing text", "<?php 0?>\n\nx\n"},
		{"leading markup", "x\n<?php 0?>\n\n"},
		{"tag stands in for the terminator", "<?php if ($a): endif ?>\n\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := buildPHP(t, tc.src)

			if second := buildPHP(t, first); second != first {
				t.Fatalf("BuildFile output is not a fixed point:\n=== first ===\n%s\n=== second ===\n%s", first, second)
			}
		})
	}
}

func TestBuildFileWithOnlyAnOpenTagEndsAfterIt(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"no newline", "<?php", "<?php\n"},
		{"one newline", "<?php\n", "<?php\n"},
		{"crlf", "<?php\r\n", "<?php\n"},
		{"trailing blank lines", "<?php\n\n\n\n", "<?php\n"},
		{"a comment still follows a blank line", "<?php\n// c\n", "<?php\n\n// c\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, tc.src)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := buildPHP(t, got); again != got {
				t.Fatalf("not idempotent: %q then %q", got, again)
			}
		})
	}
}

func TestBuildCallFitsOnOneLine(t *testing.T) {
	got := buildPHP(t, "<?php foo($a, $b, $c);")
	want := "<?php\n\nfoo($a, $b, $c);\n"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildCallBreaksWhenTooWide(t *testing.T) {
	src := "<?php processData($firstArgument, $secondArgument, $thirdArgument, $fourthArgument, $fifthArgument, $sixthArgument, $seventhArgument, $eighthArgument);"
	got := buildPHP(t, src)
	want := "<?php\n\nprocessData(\n" +
		"    $firstArgument,\n" +
		"    $secondArgument,\n" +
		"    $thirdArgument,\n" +
		"    $fourthArgument,\n" +
		"    $fifthArgument,\n" +
		"    $sixthArgument,\n" +
		"    $seventhArgument,\n" +
		"    $eighthArgument,\n" +
		");\n"

	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildClassCanonicalShape(t *testing.T) {
	src := "<?php\nclass UserService extends BaseService implements ServiceContract\n{\n" +
		"    private int $count = 0;\n\n" +
		"    public function handle(array $items, string $mode): Collection\n    {\n" +
		"        foreach ($items as $key => $item) {\n" +
		"            if ($item->isActive() && $mode === 'strict') {\n" +
		"                $this->process($item);\n            }\n        }\n\n" +
		"        return collect($items)->map(fn ($x) => $x->value)->filter();\n    }\n}"

	got := buildPHP(t, src)
	want := "<?php\n\nclass UserService extends BaseService implements ServiceContract\n{\n" +
		"    private int $count = 0;\n\n" +
		"    public function handle(array $items, string $mode): Collection\n    {\n" +
		"        foreach ($items as $key => $item) {\n" +
		"            if ($item->isActive() && $mode === 'strict') {\n" +
		"                $this->process($item);\n            }\n        }\n\n" +
		"        return collect($items)->map(fn ($x) => $x->value)->filter();\n    }\n}\n"

	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildChainBreaksOnePerLine(t *testing.T) {
	src := "<?php $result = User::query()->where('active', true)->where('role', 'administrator')->orderBy('created_at', 'desc')->limit(10)->offset(20)->get();"
	got := buildPHP(t, src)
	want := "<?php\n\n$result = User::query()\n" +
		"    ->where('active', true)\n" +
		"    ->where('role', 'administrator')\n" +
		"    ->orderBy('created_at', 'desc')\n" +
		"    ->limit(10)\n" +
		"    ->offset(20)\n" +
		"    ->get();\n"

	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildShortChainStaysInline(t *testing.T) {
	got := buildPHP(t, "<?php $x = $this->service->handle($a, $b);")
	want := "<?php\n\n$x = $this->service->handle($a, $b);\n"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildPreservesComments(t *testing.T) {
	src := "<?php\n// top\nclass Repo\n{\n" +
		"    /**\n     * Items.\n     */\n    private array $items = []; // inline\n\n" +
		"    // method\n    public function all(): array\n    {\n        return $this->items;\n    }\n}"

	got := buildPHP(t, src)

	for _, want := range []string{"// top", "/**\n", "* Items.", "// inline", "// method"} {
		if !contains(got, want) {
			t.Fatalf("comment %q missing from:\n%s", want, got)
		}
	}

	if !contains(got, "= []; // inline") {
		t.Fatalf("inline comment did not stay trailing:\n%s", got)
	}
}

func TestBuildPreservesInteriorComments(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			"array element trailing",
			"<?php\nreturn [\n    'a' => 1, // first\n    'b' => 2, // second\n];",
			[]string{"// first", "// second"},
		},
		{
			"array leading banner",
			"<?php\nreturn [\n    // section\n    'a' => 1,\n];",
			[]string{"// section"},
		},
		{
			"empty array comment",
			"<?php\n$opts = [\n    // commented out entry\n];",
			[]string{"// commented out entry"},
		},
		{
			"match arm trailing",
			"<?php\n$x = match ($v) {\n    1 => 'a', // one\n    default => 'b', // other\n};",
			[]string{"// one", "// other"},
		},
		{
			"call argument trailing",
			"<?php\nf(\n    $a, // an argument\n);",
			[]string{"// an argument"},
		},
		{
			"binary operand",
			"<?php\n$x = $a . // joiner\n    $b;",
			[]string{"// joiner"},
		},
		{
			"ternary branch",
			"<?php\n$x = $cond\n    // why\n    ? $a\n    : $b;",
			[]string{"// why"},
		},
		{
			"chain step trailing",
			"<?php\n$x = $q->where('a', 1) // step\n    ->get();",
			[]string{"// step"},
		},
		{
			"property hook comments",
			"<?php\nclass Foo\n{\n    public string $name {\n        // normalize\n        set => strtolower($value);\n        get => $this->name; // read\n    }\n}",
			[]string{"// normalize", "// read"},
		},
		{
			"empty class body comment",
			"<?php\nclass Foo\n{\n    // keep me\n}",
			[]string{"// keep me"},
		},
		{
			"empty anonymous class body comment",
			"<?php\n$a = new class {\n    // placeholder\n};",
			[]string{"// placeholder"},
		},
		{
			"empty function body comment",
			"<?php\nfunction bar()\n{\n    // todo\n}",
			[]string{"// todo"},
		},
		{
			"empty method body comment",
			"<?php\nclass C\n{\n    public function up(): void\n    {\n        // migrate\n    }\n}",
			[]string{"// migrate"},
		},
		{
			"empty interface body comment",
			"<?php\ninterface Baz\n{\n    /* dangling */\n}",
			[]string{"/* dangling */"},
		},
		{
			"empty enum body comment",
			"<?php\nenum Qux\n{\n    // cases later\n}",
			[]string{"// cases later"},
		},
		{
			"empty trait body comment",
			"<?php\ntrait Tr\n{\n    // nothing yet\n}",
			[]string{"// nothing yet"},
		},
		{
			"empty control block comment",
			"<?php\nif ($x) {\n    // empty branch\n}",
			[]string{"// empty branch"},
		},
		{
			"empty closure body comment",
			"<?php\n$f = function () {\n    // later\n};",
			[]string{"// later"},
		},
		{
			"own-line and inline comment pair before nested array",
			"<?php\n$x = [\n    /* a */\n    /* b */ [0],\n];",
			[]string{"/* a */", "/* b */"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := buildPHP(t, tc.src)

			for _, want := range tc.want {
				if !contains(first, want) {
					t.Fatalf("comment %q dropped:\n%s", want, first)
				}
			}

			file, errs := parser.Parse([]byte(first))
			if len(errs) > 0 {
				t.Fatalf("re-parse: %v", errs)
			}

			if second := string(BuildFile(file, config.Default())); first != second {
				t.Fatalf("not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
			}
		})
	}
}

func TestBuildParamAttributes(t *testing.T) {
	inline := buildPHP(t, "<?php\nclass C\n{\n    public function f(#[SensitiveParameter] string $token): void {}\n}")
	if !contains(inline, "f(#[SensitiveParameter] string $token)") {
		t.Fatalf("inline attribute not preserved:\n%s", inline)
	}

	broken := buildPHP(t, "<?php\nclass C\n{\n    public function __construct(#[Required] private string $firstLongParameterName, #[Config] private int $secondLongParameterName) {}\n}")
	if !contains(broken, "#[Required]\n        private string $firstLongParameterName,") {
		t.Fatalf("broken attribute not on its own line:\n%s", broken)
	}

	file, errs := parser.Parse([]byte(broken))
	if len(errs) > 0 {
		t.Fatalf("re-parse: %v", errs)
	}

	if second := string(BuildFile(file, config.Default())); broken != second {
		t.Fatalf("not idempotent:\n--- first ---\n%s\n--- second ---\n%s", broken, second)
	}
}

func TestBuildDocShapeReflow(t *testing.T) {
	wide := "<?php\nclass C\n{\n    /**\n     * @return array{total: int, sent: int, delivered: int, opened: int, acknowledged: int, flagged: int, overdue: int, pending: int}\n     */\n    public function stats(): array {}\n}"

	got := buildPHP(t, wide)
	if !contains(got, "* @return array{\n") || !contains(got, "     *     total: int,\n") {
		t.Fatalf("wide shape did not break per member:\n%s", got)
	}

	for _, line := range splitLines(got) {
		if len(line) > config.Default().PrintWidth {
			t.Fatalf("doc shape left a line over width:\n%s", got)
		}
	}

	file, errs := parser.Parse([]byte(got))
	if len(errs) > 0 {
		t.Fatalf("re-parse: %v", errs)
	}

	if second := string(BuildFile(file, config.Default())); got != second {
		t.Fatalf("doc shape not idempotent:\n--- first ---\n%s\n--- second ---\n%s", got, second)
	}

	narrow := buildPHP(t, "<?php\nclass C\n{\n    /**\n     * @return array{id: int, name: string}\n     */\n    public function one(): array {}\n}")
	if !contains(narrow, "* @return array{id: int, name: string}") {
		t.Fatalf("narrow shape should stay inline:\n%s", narrow)
	}
}

func TestBuildLastArgHug(t *testing.T) {
	arr := buildPHP(t, "<?php\nresponse($pixel, 200, ['Content-Type' => 'image/gif', 'Cache-Control' => 'no-store, no-cache, must-revalidate, max-age=0', 'Pragma' => 'no-cache']);")
	if !contains(arr, "response($pixel, 200, [\n") {
		t.Fatalf("array last arg did not hug:\n%s", arr)
	}

	clo := buildPHP(t, "<?php\nit('formats a long descriptive label for the test case here', function () {\n    expect(true)->toBeTrue();\n});")
	if !contains(clo, "', function () {\n") {
		t.Fatalf("closure last arg did not hug:\n%s", clo)
	}

	short := buildPHP(t, "<?php\nfoo($a, [1, 2]);")
	if !contains(short, "foo($a, [1, 2]);") {
		t.Fatalf("fitting call should stay flat:\n%s", short)
	}

	named := buildPHP(t, "<?php\nmake(alpha: $firstValueGoesHereAndItIsLong, beta: $secondValueGoesHereAlsoLong, gamma: ['x' => 1, 'y' => 2, 'z' => 3, 'w' => 4]);")
	if !contains(named, "make(\n    alpha:") {
		t.Fatalf("all-named args should stack:\n%s", named)
	}
}

func TestBuildMatchArmConditionBreak(t *testing.T) {
	wide := buildPHP(t, "<?php\n$x = match ($v) {\n"+
		"    Status::Alpha, Status::Bravo, Status::Charlie, Status::Delta,"+
		" Status::Echo, Status::Foxtrot, Status::Golf, Status::Hotel => 'group',\n"+
		"    default => 'none',\n};")

	if !contains(wide, "Status::Alpha,\n") || !contains(wide, "Status::Hotel => 'group',") {
		t.Fatalf("wide match arm did not break per condition:\n%s", wide)
	}

	for _, line := range splitLines(wide) {
		if len(line) > config.Default().PrintWidth {
			t.Fatalf("match arm left a line over width:\n%s", wide)
		}
	}

	short := buildPHP(t, "<?php\n$x = match ($v) {\n    1, 2 => 'low',\n    default => 'high',\n};")
	if !contains(short, "    1, 2 => 'low',") {
		t.Fatalf("short match arm should stay on one line:\n%s", short)
	}
}

func TestBuildTernaryBreak(t *testing.T) {
	wide := buildPHP(t, "<?php\n$label = $cond"+
		" ? 'the user account is active and is currently signed in right here'"+
		" : 'the user account is inactive or has been signed out elsewhere';")

	if !contains(wide, "$cond\n    ? '") || !contains(wide, "\n    : '") {
		t.Fatalf("wide ternary did not break at ?/::\n%s", wide)
	}

	elvis := buildPHP(t, "<?php\n$name = $value"+
		" ?: 'a fairly long fallback display name that is used whenever none at all is provided by the caller here';")
	if !contains(elvis, "$value\n    ?: '") {
		t.Fatalf("wide elvis did not break at ?::\n%s", elvis)
	}

	short := buildPHP(t, "<?php\n$x = $cond ? $a : $b;")
	if !contains(short, "$x = $cond ? $a : $b;") {
		t.Fatalf("fitting ternary should stay flat:\n%s", short)
	}
}

func TestBuildBinaryBreak(t *testing.T) {
	logical := buildPHP(t, "<?php\n$ok = $conditionOne"+
		" && $conditionTwoIsHere && $conditionThreeIsAlsoPresent"+
		" && $conditionFourMakesItOverflowTheLimit && $five;")

	if !contains(logical, "$conditionOne\n    && $conditionTwoIsHere") {
		t.Fatalf("logical run did not wrap with operator leading:\n%s", logical)
	}

	for _, line := range splitLines(logical) {
		if len(line) > config.Default().PrintWidth {
			t.Fatalf("binary left a line over width:\n%s", logical)
		}
	}

	short := buildPHP(t, "<?php\n$x = $a && $b && $c;")
	if !contains(short, "$x = $a && $b && $c;") {
		t.Fatalf("short logical run should stay flat:\n%s", short)
	}

	concat := buildPHP(t, "<?php\n$s = 'the first piece of a considerably longer string here'"+
		" . $variableInTheMiddle . 'and then a final trailing piece that makes it overflow';")
	if !contains(concat, "\n    .") {
		t.Fatalf("concat did not fill onto continuation lines:\n%s", concat)
	}
}

func TestBuildAnonClass(t *testing.T) {
	src := "<?php\nreturn new class() extends Migration implements Runnable {\n" +
		"    public function up(): void\n    {\n        Schema::create('t', fn () => null);\n    }\n};"

	got := buildPHP(t, src)

	// The anonymous-class brace defaults to the next line.
	if !contains(got, "new class() extends Migration implements Runnable\n{\n") {
		t.Fatalf("anon class header/body not laid out:\n%s", got)
	}

	if !contains(got, "    public function up(): void\n    {\n") {
		t.Fatalf("anon class member not formatted:\n%s", got)
	}

	file, errs := parser.Parse([]byte(got))
	if len(errs) > 0 {
		t.Fatalf("re-parse: %v", errs)
	}

	if second := string(BuildFile(file, config.Default())); got != second {
		t.Fatalf("anon class not idempotent:\n--- first ---\n%s\n--- second ---\n%s", got, second)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}

	return false
}

func TestBuildIdempotentByConstruction(t *testing.T) {
	srcs := []string{
		"<?php foo($a, $b, $c);",
		"<?php someFunction($argumentOne, $argumentTwo, $argumentThree, $argumentFour, $argumentFive);",
		"<?php $x = [1, 2, 3];",
		"<?php return $value ? $whenTrue : $whenFalse;",
		"<?php echo $a, $b;",
		"<?php $config = ['key' => 'value', 'nested' => ['deep' => $thing], 'another' => $longerValueHere];",
		"<?php function greet(string $name): string { return \"hi $name\"; }",
		"<?php foreach ($items as $i => $item) { process($item); }",
		"<?php $fn = match ($status) { 1, 2 => 'low', default => 'high' };",
		"<?php try { risky(); } catch (Throwable $e) { report($e); } finally { cleanup(); }",
		"<?php namespace App; use App\\Models\\User; enum Status: string { case On = 'on'; }",
		"<?php for ($i = 0; $i < 10; $i++) { echo $i; }",
		"<?php switch ($x) { case 1: doOne(); break; default: doDefault(); }",
		"<?php interface Repo { public function find(int $id): ?Model; }",
		"<?php trait HasName { public function name(): string { return $this->name; } }",
	}

	for _, src := range srcs {
		first := buildPHP(t, src)

		file, errs := parser.Parse([]byte(first))
		if len(errs) > 0 {
			t.Fatalf("re-parse %q: %v", first, errs)
		}

		second := string(BuildFile(file, config.Default()))
		if first != second {
			t.Fatalf("not idempotent for %q:\n--- first ---\n%s\n--- second ---\n%s", src, first, second)
		}
	}
}

func TestBuildTrailingSemicolonCounted(t *testing.T) {
	src := "<?php $result = compute($firstParameter, $secondParameter, $thirdParameterX);"
	got := buildPHP(t, src)

	for _, line := range splitLines(got) {
		if len(line) > config.Default().PrintWidth {
			t.Fatalf("line over width:\n%s", got)
		}
	}
}

func splitLines(s string) []string {
	var lines []string

	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}

	lines = append(lines, s[start:])

	return lines
}

func TestBuildFirstArgHug(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"array too wide for its own line",
			"<?php\n$writer->writeRow([$record->identifier, $record->fullName, $record->emailAddress, $record->createdTimestamp, $record->updatedTimestamp], null, 'B'.$line);\n",
			"<?php\n\n$writer->writeRow([\n    $record->identifier,\n    $record->fullName,\n    $record->emailAddress,\n" +
				"    $record->createdTimestamp,\n    $record->updatedTimestamp,\n], null, 'B'.$line);\n",
		},
		{
			"array that fits its own line",
			"<?php\n$writer->writeRow([$record->identifier, $record->fullName, $record->emailAddress, $record->createdTimestamp], null, 'B'.$line);\n",
			"<?php\n\n$writer->writeRow(\n    [$record->identifier, $record->fullName, $record->emailAddress, $record->createdTimestamp],\n" +
				"    null,\n    'B'.$line,\n);\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, tc.src)
			if got != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if buildPHP(t, got) != got {
				t.Fatal("not idempotent")
			}
		})
	}
}

func TestBuildFirstArgHugKeepsALongTailThatFitsTheCloser(t *testing.T) {
	src := "<?php\nreturn array_filter([\n    'search' => $this->search,\n    'type' => $this->type,\n" +
		"], static fn (?string $value): bool => $value !== null && $value !== '' && $value !== '0');\n"

	if got := buildPHP(t, src); got != "<?php\n\n"+src[len("<?php\n"):] {
		t.Fatalf("first-arg hug with a long tail mismatch.\n=== got ===\n%s", got)
	}
}

func TestBuildFirstArgMinorArrayGoesOnePerLine(t *testing.T) {
	src := "<?php\n$table->index([$columnNames['model_morph_key'], 'model_type'], 'model_has_permissions_model_id_model_type_index_tenant_x');\n"
	want := "<?php\n\n$table->index(\n    [$columnNames['model_morph_key'], 'model_type'],\n    'model_has_permissions_model_id_model_type_index_tenant_x',\n);\n"

	if got := buildPHP(t, src); got != want {
		t.Fatalf("minor first arg mismatch.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	if got := buildPHP(t, src); buildPHP(t, got) != got {
		t.Fatalf("minor first arg not idempotent")
	}
}

func TestBuildTrailingMethodBreak(t *testing.T) {
	broke := buildPHP(t, "<?php\nRoute::get('/newsletters/track/{token}/pixel.gif', [EmailOpenTrackingController::class, 'pixel'])->name('newsletters.tracking.pixel');\n")
	if !strings.Contains(broke, "])\n    ->name(") {
		t.Fatalf("chain did not break at the arrow:\n%s", broke)
	}

	hug := buildPHP(t, "<?php\nRoute::get('/dashboard', fn () => Inertia::render('Dashboard', ['stats' => $statistics, 'notifications' => $alerts, 'more' => $extra]))->name('dash');\n")
	if !strings.Contains(hug, "]))->name('dash')") {
		t.Fatalf("block-like head should keep the method inline:\n%s", hug)
	}
}

// `\f` and `\v` are PHP whitespace like a space, so the source measure must not
// count them.
func TestBuildOpenerWidthIgnoresVanishingWhitespace(t *testing.T) {
	callee := strings.Repeat("0", 59)
	arg := strings.Repeat("0", 51)

	space := buildPHP(t, "<?php (0("+callee+" ('\n"+arg+"')));")

	for _, sep := range []string{"\f", "\v", "\t", "\r", "\n", "  "} {
		t.Run(fmt.Sprintf("%q", sep), func(t *testing.T) {
			got := buildPHP(t, "<?php (0("+callee+sep+"('\n"+arg+"')));")

			if got != space {
				t.Fatalf("separator %q changed the layout.\n=== with %q ===\n%s\n=== with a space ===\n%s", sep, sep, got, space)
			}
		})
	}
}

func TestBuildBreakingArgListOmitsCommaForLanguageConstructs(t *testing.T) {
	cond := "$someVeryLongVariableNameHereOk && $anotherQuiteLongVariableNameHere" +
		" && $thirdLongVariableNameIsHereToo && $fourthOneIsHere"

	broken := "\n    $someVeryLongVariableNameHereOk\n" +
		"    && $anotherQuiteLongVariableNameHere\n" +
		"    && $thirdLongVariableNameIsHereToo\n" +
		"    && $fourthOneIsHere"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"empty", "<?php $q = empty(" + cond + ");", "$q = empty(" + broken + "\n);"},
		{"empty is case-insensitive", "<?php $q = EMPTY(" + cond + ");", "$q = EMPTY(" + broken + "\n);"},
		{"eval", "<?php $q = eval(" + cond + ");", "$q = eval(" + broken + "\n);"},
		{"exit", "<?php exit(" + cond + ");", "exit(" + broken + "\n);"},
		{"die", "<?php die(" + cond + ");", "die(" + broken + "\n);"},
		// A leading backslash makes it an ordinary call to an undefined global
		// function, not the construct, so the comma stays.
		{"qualified name is a plain call", `<?php $q = \empty(` + cond + ");", `$q = \empty(` + broken + ",\n);"},
		{"method named empty is a plain call", "<?php $q = $o->empty(" + cond + ");", "$q = $o->empty(" + broken + ",\n);"},
		{"plain call keeps the comma", "<?php $q = f(" + cond + ");", "$q = f(" + broken + ",\n);"},
		{"isset keeps the comma", "<?php $q = isset(" + cond + ");", "$q = isset(" + broken + ",\n);"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, tc.src)
			want := "<?php\n\n" + tc.want + "\n"

			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestBuildReindentsBlockCommentContinuationLines(t *testing.T) {
	body := func(open string) string {
		return "<?php\n\nfunction f()\n{\n\tif (true) {\n\t\t" + open +
			"\n\t\t * One.\n\t\t * Two.\n\t\t */\n\t\treturn 1;\n\t}\n}\n"
	}

	want := "<?php\n\nfunction f()\n{\n    if (true) {\n        %s\n" +
		"         * One.\n         * Two.\n         */\n        return 1;\n    }\n}\n"

	t.Run("block comment re-homes", func(t *testing.T) {
		got := buildPHP(t, body("/*"))

		if got != fmt.Sprintf(want, "/*") {
			t.Fatalf("got %q, want %q", got, fmt.Sprintf(want, "/*"))
		}
	})

	t.Run("docblock re-homes the same way", func(t *testing.T) {
		got := buildPHP(t, body("/**"))

		if got != fmt.Sprintf(want, "/**") {
			t.Fatalf("got %q, want %q", got, fmt.Sprintf(want, "/**"))
		}
	})

	// The strip is the smallest continuation indent, so a hand-aligned interior
	// shifts as one block instead of collapsing.
	t.Run("interior alignment survives", func(t *testing.T) {
		src := "<?php\n\nfunction f()\n{\n    /*\n      a\n          b\n    */\n    return 1;\n}\n"

		got := buildPHP(t, src)

		if !strings.Contains(got, "\n      a\n          b\n") {
			t.Fatalf("relative alignment changed:\n%s", got)
		}
	})

	// A frozen region is the opposite contract: it keeps its source columns.
	t.Run("laravel-fmt-ignore still freezes", func(t *testing.T) {
		src := "<?php\n\n// laravel-fmt-ignore\n$m = [\n      1,   0,\n          0,   1,\n];\n"

		got := buildPHP(t, src)

		if !strings.Contains(got, "\n      1,   0,\n          0,   1,\n") {
			t.Fatalf("frozen region was re-homed:\n%s", got)
		}
	})
}

// Line terminators between tokens follow the configured EOL; a CR inside a
// string literal is the string's value and stays.
func TestBuildNormalizesVerbatimLineTerminators(t *testing.T) {
	chain := "<?php\r\n\r\nclass C\r\n{\r\n    public function h()\r\n    {\r\n" +
		"        return Model::query()\r\n            ->where(\"a\", 1) // keep\r\n" +
		"            ->get();\r\n    }\r\n}\r\n"

	frozen := "<?php\r\n\r\n// laravel-fmt-ignore\r\n$m   =   \"a\rb\";\r\n$other   =   1;\r\n"

	t.Run("a CRLF file comes back all LF", func(t *testing.T) {
		got := buildPHP(t, chain)

		if strings.Contains(got, "\r") {
			t.Fatalf("carriage returns survived into an LF file:\n%q", got)
		}
	})

	t.Run("a CR inside a string literal is the value and survives", func(t *testing.T) {
		got := buildPHP(t, frozen)

		if !strings.Contains(got, "\"a\rb\"") {
			t.Fatalf("string literal CR was rewritten:\n%q", got)
		}
	})

	t.Run("an LF file takes CRLF when configured", func(t *testing.T) {
		cfg := config.Default()
		cfg.EndOfLine = config.EndOfLineCRLF

		src := strings.ReplaceAll(chain, "\r\n", "\n")

		file, errs := parser.Parse([]byte(src))
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}

		got := string(BuildFile(file, cfg))

		if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
			t.Fatalf("a bare LF survived into a CRLF file:\n%q", got)
		}
	})
}

func TestBuildHugsBraceAfterWrappedSignature(t *testing.T) {
	wrapped := "<?php\nclass C\n{\n    public function __construct(" +
		"protected ContainerContract $container, protected DispatcherContract $events, " +
		"protected string $applicationName)\n    {\n        $this->boot();\n    }\n}\n"

	short := "<?php\nfunction tally(int $a): int\n{\n    return $a;\n}\n"

	cases := []struct {
		name    string
		style   string
		src     string
		want    string
		absent  string
		comment string
	}{
		{
			name:  "next-line hugs a wrapped signature",
			style: config.BraceStyleNextLine,
			src:   wrapped,
			want:  "    ) {\n",
		},
		{
			name:   "next-line still breaks an unwrapped signature",
			style:  config.BraceStyleNextLine,
			src:    short,
			want:   "function tally(int $a): int\n{\n",
			absent: "int {",
		},
		{
			name:   "always-next-line refuses to hug",
			style:  config.BraceStyleAlwaysNextLine,
			src:    wrapped,
			want:   "    )\n    {\n",
			absent: ") {",
		},
		{
			name:  "same-line always hugs",
			style: config.BraceStyleSameLine,
			src:   short,
			want:  "function tally(int $a): int {\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.FunctionBraceStyle = tc.style
			cfg.PHP.MethodBraceStyle = tc.style

			file, errs := parser.Parse([]byte(tc.src))
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			got := string(BuildFile(file, cfg))

			if !contains(got, tc.want) {
				t.Fatalf("missing %q:\n%s", tc.want, got)
			}

			if tc.absent != "" && contains(got, tc.absent) {
				t.Fatalf("unexpectedly contains %q:\n%s", tc.absent, got)
			}

			second, errs := parser.Parse([]byte(got))
			if len(errs) > 0 {
				t.Fatalf("re-parse: %v", errs)
			}

			if twice := string(BuildFile(second, cfg)); twice != got {
				t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestBuildKeepsAuthoredNewParens(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"bare stays bare", "new Foo"},
		{"parens stay", "new Foo()"},
		{"arguments are untouched", "new Foo($a, $b)"},
		{"qualified bare", "new \\App\\Foo"},
		{"php 8.4 member access keeps its parens", "new Foo()->bar()"},
		{"immediate invoke keeps its parens", "new Foo()($a)"},
		{"immediate invoke with no arguments", "new Foo()()"},
		{"index access keeps its parens", "new Foo()['a']"},
		{"static access keeps its parens", "new Foo()::BAR"},
		{"anonymous class bare", "new class {}"},
		{"anonymous class with parens", "new class() {}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, "<?php $a = "+tc.expr+";")
			want := "<?php\n\n$a = " + tc.expr + ";\n"

			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestBuildAnonClassBraceFollowsConfig(t *testing.T) {
	src := "<?php\nreturn new class extends Migration\n{\n    public $a = 1;\n};\n"

	cases := []struct {
		name  string
		style string
		want  string
	}{
		{"default is next line", config.BraceStyleNextLine, "new class extends Migration\n{"},
		{"same line when configured", config.BraceStyleSameLine, "new class extends Migration {"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.AnonClassBraceStyle = tc.style

			file, errs := parser.Parse([]byte(src))
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			got := string(BuildFile(file, cfg))

			if !contains(got, tc.want) {
				t.Fatalf("missing %q:\n%s", tc.want, got)
			}

			second, errs := parser.Parse([]byte(got))
			if len(errs) > 0 {
				t.Fatalf("re-parse: %v", errs)
			}

			if twice := string(BuildFile(second, cfg)); twice != got {
				t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}

	// The named-class rule is separate and must not have moved with it.
	named := buildPHP(t, "<?php\nclass C\n{\n    public $a = 1;\n}\n")
	if !contains(named, "class C\n{") {
		t.Fatalf("named class brace changed:\n%s", named)
	}
}

func TestBuildKeepsCommentAtEndOfBody(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"trailing line comment on the last statement",
			"<?php\nfunction f()\n{\n    $a = 1;\n    return $a; // last\n}\n",
			"function f()\n{\n    $a = 1;\n\n    return $a; // last\n}",
		},
		{
			"own-line comment before the closing brace",
			"<?php\nfunction f()\n{\n    $a = 1;\n    // note\n}\n",
			"function f()\n{\n    $a = 1;\n    // note\n}",
		},
		{
			"block comment before the closing brace",
			"<?php\nfunction f()\n{\n    $a = 1;\n    /* note */\n}\n",
			"function f()\n{\n    $a = 1;\n    /* note */\n}",
		},
		{
			"class member body",
			"<?php\nclass C\n{\n    public $a = 1;\n    // note\n}\n",
			"class C\n{\n    public $a = 1;\n    // note\n}",
		},
		{
			"if body",
			"<?php\nif ($a) {\n    $b = 1;\n    // note\n}\n",
			"if ($a) {\n    $b = 1;\n    // note\n}",
		},
		{
			"foreach body",
			"<?php\nforeach ($a as $b) {\n    $c = 1;\n    // note\n}\n",
			"foreach ($a as $b) {\n    $c = 1;\n    // note\n}",
		},
		{
			"try body",
			"<?php\ntry {\n    $a = 1;\n    // note\n} catch (E $e) {\n    $b = 2;\n    // caught\n}\n",
			"try {\n    $a = 1;\n    // note\n} catch (E $e) {\n    $b = 2;\n    // caught\n}",
		},
		{
			"two trailing comments keep their order",
			"<?php\nfunction f()\n{\n    $a = 1;\n    // one\n    // two\n}\n",
			"function f()\n{\n    $a = 1;\n    // one\n    // two\n}",
		},
		{
			"authored blank above the trailing comment survives",
			"<?php\nfunction f()\n{\n    $a = 1;\n\n    // note\n}\n",
			"function f()\n{\n    $a = 1;\n\n    // note\n}",
		},
		// The empty-body path already worked and must keep working.
		{
			"sole comment in an empty body",
			"<?php\nfunction f()\n{\n    // only\n}\n",
			"function f()\n{\n    // only\n}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, tc.src)
			want := "<?php\n\n" + tc.want + "\n"

			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}

			if twice := buildPHP(t, got); twice != got {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestBuildKeepsAdjacentUnarySignsApart(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want string
	}{
		{"double minus on a variable", "- -$b", "- -$b"},
		{"double plus on a variable", "+ +$b", "+ +$b"},
		{"double minus on a literal", "- -0", "- -0"},
		{"triple minus", "- - -$b", "- - -$b"},
		{"minus before pre-decrement", "- --$b", "- --$b"},
		{"plus before pre-increment", "+ ++$b", "+ ++$b"},
		// No PHP operator spells these, so they may still close up.
		{"tilde pair has no token", "~ ~$b", "~~$b"},
		{"mixed signs do not fuse", "- +$b", "-+$b"},
		{"mixed signs reversed", "+ -$b", "+-$b"},
		{"parenthesized stays parenthesized", "-(-5)", "-(-5)"},
		{"single unary is tight", "-$b", "-$b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, "<?php $a = "+tc.expr+";")
			want := "<?php\n\n$a = " + tc.want + ";\n"

			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestBuildKeepsBinaryThenUnary(t *testing.T) {
	cases := []string{"$b - -$c", "$b + +$c", "$b - --$c", "$b + ++$c", "1 - -0"}

	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			got := buildPHP(t, "<?php $a = "+expr+";")
			want := "<?php\n\n$a = " + expr + ";\n"

			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

// PHP's DNUM leaves the fraction optional, so a trailing dot belongs to the
// literal: `1. + 2` is 3.0, while `1 . +2` is the string '12'.
func TestBuildKeepsNumericLiteralsVerbatim(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"trailing dot in additive context", "1. + 2"},
		{"trailing dot with subtraction", "5. - 2"},
		{"trailing dot exponent", "1.e3"},
		{"trailing dot negative exponent", "1.e-300"},
		{"trailing dot alone", "0."},
		{"trailing dot with separator", "1_000."},
		{"leading dot", ".5"},
		{"leading dot exponent", ".5e3"},
		{"both parts", "1.5"},
		{"uppercase exponent", "1.5E-3"},
		{"hex", "0x1F"},
		{"octal", "0o755"},
		{"separators", "1_000_000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, "<?php $a = "+tc.expr+";")
			want := "<?php\n\n$a = " + tc.expr + ";\n"

			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestBuildAcceptsCloseTagInsideSwitchCase(t *testing.T) {
	src := "<?php\nswitch ($a) {\n    case 1: ?>\n        <p>html</p>\n    <?php break;\n}\n"

	got := buildPHP(t, src)

	for _, want := range []string{"case 1: ?>", "<p>html</p>", "<?php break;"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}

	if twice := buildPHP(t, got); twice != got {
		t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestBuildTerminatesAFrozenStatementWhoseCloseTagFollowsAComment(t *testing.T) {
	cases := map[string]string{
		"a hash comment":  "<?php\n// laravel-fmt-ignore\n$total#?>",
		"a slash comment": "<?php\n// laravel-fmt-ignore\n$total // note ?>",
		"a block comment": "<?php\n// laravel-fmt-ignore\n$total /* note */ ?>",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := buildPHP(t, src)

			if !strings.Contains(got, "$total;") {
				t.Fatalf("the statement lost its terminator:\n%s", got)
			}

			if twice := buildPHP(t, got); twice != got {
				t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

// list() is the long form of [...] destructuring, so it can skip an element
// too.
func TestBuildAcceptsElidedListElement(t *testing.T) {
	cases := []string{
		"list(, $b) = $x;",
		"list($a, , $c) = $y;",
		"list($a, list($b, $c)) = $z;",
		"list('k' => $v) = $m;",
		"[, $b] = $x;",
	}

	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			got := buildPHP(t, "<?php\n"+expr+"\n")
			want := "<?php\n\n" + expr + "\n"

			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestBuildExprNormalizesLayoutCRBesideACRLiteral(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"CR beside a CR-bearing heredoc", "<<<a\r\na\r%#\n0", "<<<a\na\n%#\n0"},
		{"CR beside a CR-free heredoc", "<<<a\na\r%#\n0", "<<<a\na\n%#\n0"},
		{"CR inside a string is value", "\"a\rb\" . $c", "'a\rb'.$c"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildExprString(t, tc.src)

			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := buildExprString(t, got); twice != got {
				t.Fatalf("not idempotent:\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func buildExprString(t *testing.T, src string) string {
	t.Helper()

	expr, tokens, errs := parser.ParseFullExpression([]byte(src))
	if len(errs) > 0 || expr == nil {
		t.Fatalf("parse %q: %v", src, errs)
	}

	return string(BuildExpr([]byte(src), tokens, expr, config.Default(), 0))
}

func TestChainPreservationChargesTheStatementPrefix(t *testing.T) {
	const chain = "SomeFactoryClass0000000000()::CONST_NAME00000->chain0" +
		"->method00000000('000000000000000000000000000000000000000')->tail()"

	cases := []struct {
		name string
		stmt string
	}{
		{"assignment to a property", "$obj->prop = " + chain + ";"},
		{"assignment to a long variable", "$aaaaaaa = " + chain + ";"},
		{"return", "return " + chain + ";"},
		{"cast", "return (int) " + chain + ";"},
		{"parenthesised", "$aaaaaaa = (" + chain + ");"},
		{"negated", "$aaaaaaa = ! " + chain + ";"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := buildPHP(t, "<?php\n"+tc.stmt+"\n")

			if twice := buildPHP(t, once); twice != once {
				t.Fatalf("not a fixed point:\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestBuildArrayValueBelowKeyKeepsAComparisonWhole(t *testing.T) {
	key := "'" + strings.Repeat("k", 76) + "'"
	src := "<?php\n$classes = [\n    " + key + " => $foo->current_identifier === $bar->id && ! in_array($bar->id, $selected, true) && ! $bar->is_archived_for_good_now,\n];\n"
	want := "<?php\n\n$classes = [\n    " + key + " =>\n        $foo->current_identifier === $bar->id\n" +
		"            && ! in_array($bar->id, $selected, true)\n            && ! $bar->is_archived_for_good_now,\n];\n"

	got := buildPHP(t, src)
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if buildPHP(t, got) != got {
		t.Fatal("not idempotent")
	}
}

func TestBuildTwoArrayCallSharesTheBracketSeam(t *testing.T) {
	long := "'" + strings.Repeat("b", 60) + "' => $first, '" + strings.Repeat("c", 60) + "' => $second"

	cases := []struct {
		name string
		call string
		want string
	}{
		{
			name: "both arrays broken",
			call: "f([\n    'a' => $a,\n], [\n    'b' => $b,\n]);",
			want: "f([\n    'a' => $a,\n], [\n    'b' => $b,\n]);",
		},
		{
			name: "short second array rides the closer",
			call: "f([\n    'a' => $a,\n], ['b' => $b]);",
			want: "f([\n    'a' => $a,\n], ['b' => $b]);",
		},
		{
			name: "second array too long for the closer line breaks",
			call: "f([\n    'a' => $a,\n], [" + long + "]);",
			want: "f([\n    'a' => $a,\n], [\n    '" + strings.Repeat("b", 60) + "' => $first,\n    '" +
				strings.Repeat("c", 60) + "' => $second,\n]);",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := "<?php\n\n" + tc.want + "\n"

			got := buildPHP(t, "<?php\n"+tc.call+"\n")
			if got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if buildPHP(t, got) != got {
				t.Fatal("not idempotent")
			}
		})
	}
}

func TestBuildArrowHugNeedsSomethingToBreakBelowIt(t *testing.T) {
	arrow := "fn (PaperQueryBuilder $query): PaperQueryBuilder => $query->where('published', false)"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"short call body that overflows",
			"function f() {\n    $authors = Author::with('posts', " + arrow + ")->get();\n}",
			"function f()\n{\n    $authors = Author::with(\n        'posts',\n        " + arrow + ",\n    )->get();\n}",
		},
		{"short call body that fits", "Author::with('posts', fn ($q) => $q->where('published', false));", "Author::with('posts', fn ($q) => $q->where('published', false));"},
		{
			"array body",
			"$rows = Author::with('posts', fn (PaperQueryBuilder $query): array => ['published' => false, 'title' => 'Some longer title here'])->get();",
			"$rows = Author::with('posts', fn (PaperQueryBuilder $query): array => [\n    'published' => false,\n    'title' => 'Some longer title here',\n])->get();",
		},
		{
			"lone short call body",
			"$names = $collection->filter(fn (PaperQueryBuilderInterface $queryBuilder): bool => $queryBuilder->where('published', false));",
			"$names = $collection->filter(\n    fn (PaperQueryBuilderInterface $queryBuilder): bool => $queryBuilder->where('published', false),\n);",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, "<?php\n"+tc.src+"\n")
			if want := "<?php\n\n" + tc.want + "\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if buildPHP(t, got) != got {
				t.Fatal("not idempotent")
			}
		})
	}
}

func TestBuildCallWrappingOnlyAnArrayHugsAsTheArrayDoes(t *testing.T) {
	const lead = "File::put(public_path('images/test-frames/'.ImageAnalyzerService::MANIFEST), "

	body := "[\n        'stored.webp' => ['aspectRatio' => '2.39:1'],\n    ]"

	cases := []struct {
		name string
		last string
		want string
	}{
		{"call wrapping only an array", "json_encode(" + body + ")", "    " + lead + "json_encode(" + body + "));"},
		{"bare array", body, "    " + lead + body + ");"},
		{
			"call with an argument before its array",
			"__('messages.key', " + body + ")",
			"    File::put(\n        public_path('images/test-frames/'.ImageAnalyzerService::MANIFEST),\n" +
				"        __('messages.key', [\n            'stored.webp' => ['aspectRatio' => '2.39:1'],\n        ]),\n    );",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<?php\nfunction f() {\n    " + lead + tc.last + ");\n}\n"

			got := buildPHP(t, src)
			if want := "<?php\n\nfunction f()\n{\n" + tc.want + "\n}\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if buildPHP(t, got) != got {
				t.Fatal("not idempotent")
			}
		})
	}
}
