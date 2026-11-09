# Changelog

## Unreleased
* Improved `laravel-fmt.json` to reject duplicate keys, wrong-case keys like `PrintWidth`, `null` and invalid UTF-8
* Improved the error for a wrong value in `laravel-fmt.json`, `pint.json` or `mago.json` to name its key, like `preset must be a string`
* Improved `devcontainer.json` and `.vscode/*.json` to be formatted as JSONC
* Improved JSONC to keep a trailing comma in a multi-line list
* Fixed `pint.json`, `composer.json`, `package.json` and `mago.json` keys matching in any case, like `"Preset"` for `preset`
* Fixed JSON that is not UTF-8 being formatted instead of reported
* Fixed a flat JSON array or object before a comma going one column past `printWidth`
* Fixed JSONC comments around a comma swapping places
* Fixed invalid YAML like `branches: [main]tags: [v1]`, `-\tname: x` or `--- a: b` being reshaped instead of reported
* Fixed YAML block scalar lines holding only tabs or spaces being removed
* Fixed XML attributes with spaces around `=` being reported as invalid
* Fixed XML text gaining indent after `laravel-fmt-ignore` or before a close tag
* Fixed invalid XML like `<a b="1"c="2"/>` or a bare `&` being formatted instead of reported
* Fixed Pint's `per`, `psr12` and `symfony` presets, rule sets like `@PER-CS` and the order of `pint.json` rules not matching Pint
* Fixed a Pint rule set to `true` or only partly configured keeping the preset's options
* Fixed `ordered_imports` with `sort_algorithm: none` still sorting imports
* Fixed laravel-fmt and Pint's `psr12` and `symfony` presets undoing each other's empty bodies and `new class ()`
* Fixed `pint.json` with `"rules": []` being rejected and an unknown preset being accepted
* Fixed `.prettierrc.json` being read as YAML and a byte order mark in `.prettierrc` dropping the first option
* Fixed Prettier options like `"tabWidth": "4"` being misread or ignored instead of reported
* Fixed an empty `"prettier"` key in `package.json` hiding `.prettierrc`, and a shared config in `.prettierrc` being reported as invalid
* Fixed Prettier `overrides` globs like `./src/*.php` or `!*.md` matching other files than Prettier does
* Fixed Mago values like `same_line`, `Crlf`, `"preserve"` or the `pint` preset being ignored or refused
* Fixed Mago's `tempest` and `hack` presets missing and its `inline-empty-*-braces` settings being ignored
* Fixed Mago settings that Mago itself rejects being accepted
* Fixed a `mago.toml` without a `[formatter]` table letting a parent folder's Mago config apply

## Version 0.9.0 (2026-11-07)
* Updated Tailwind to 4.3.5
* Improved multi-line Alpine attribute values to be spaced like single-line ones and indented by their bracket nesting
* Improved `<script>` bodies without Blade blocks like `@if` to be indented by their bracket nesting
* Improved `:` bindings like `:class` on plain HTML tags to be spaced like their `x-bind:` form
* Improved `.html` files to keep a paragraph holding a custom element on one line, as Blade does
* Fixed a Blade comment in an Alpine attribute being spaced apart
* Fixed `const`, `let` and `var` losing their space before `[` in Alpine attributes
* Fixed `-- >` in an Alpine attribute being joined into `-->`
* Fixed strings in an Alpine attribute being spaced apart after a `/` that follows a property named like a keyword, such as `cache.delete / 2`
* Fixed a `<script>` in a CRLF file with a backslash-continued string moving one level on the second format
* Fixed Tailwind sorting treating a bare utility name like `row` or `text` as a class
* Fixed Tailwind sorting moving `!p-3` and older utilities like `break-words` to the front
* Fixed Tailwind sorting placing theme values like `bg-brand` before the built-in ones
* Fixed `@apply` in a stylesheet without Tailwind v4 getting spaces after commas, which split classes like `shadow-[0_1px_rgba(0,0,0,0.1)]`
* Fixed a custom property whose value holds `{}`, such as `--x: {{a}}`, failing to parse
* Fixed an empty fallback like `var(--tw-blur,)` getting a space before its `)`
* Fixed `.html` files getting a visible space at the edge of an inline or custom element
* Fixed `.html` files adding spaces to a multi-line attribute value that ends in a line break
* Fixed the error for a file that cannot be written saying `open` twice

## Version 0.8.0 (2026-11-03)
* Added a `laravel-fmt/format` request to `laravel-fmt lsp` that formats the text it is sent, so editor plugins do not have to open the file
* Improved Tailwind sorting to follow Tailwind 4.3.4 and accept any number of `@custom-variant`s and custom breakpoints in a stylesheet
* Optimized `laravel-fmt lsp` to look up a file's config once when an editor asks for its line ending
* Fixed `laravel-fmt lsp` converting the line endings of a file it could not format or was not given
* Fixed `laravel-fmt lsp` exiting on a message that is not valid JSON
* Fixed `laravel-fmt lsp` answering requests after an editor asked it to shut down
* Fixed `laravel-fmt lsp` replying to a request method sent as a notification
* Fixed `laravel-fmt lsp` running out of memory on a corrupted message header
* Fixed `laravel-fmt lsp` range formatting reaching past the selected lines when a range position is out of bounds
* Fixed `laravel-fmt lsp` on macOS and Linux reading a folder named like `a:b` as a Windows drive
* Fixed a named file or folder with brackets like a Folio `[id].blade.php` being read as a glob, which formatted other files instead
* Fixed a file named twice in different case on Windows or macOS being formatted and counted twice
* Fixed `.gitignore` and `ignore` patterns on Windows and macOS missing a file whose name differs in case
* Fixed `.prettierignore` patterns missing a file whose name differs in case, which Prettier matches
* Fixed a path typed in another case on Windows or macOS keeping that case in the output and the GitLab report, where GitLab could not find the file
* Fixed an accented file name on macOS being formatted twice when named in both of its Unicode spellings
* Fixed `.gitignore` and `ignore` patterns on macOS missing an accented file name stored in its other Unicode spelling
* Fixed an accented file name on macOS reaching the GitLab report in a spelling GitLab could not find
* Fixed a project opened through a symlinked folder or junction formatting no files
* Fixed `--since` and `--staged` in a project opened through a symlinked folder ignoring the `--config` file's `ignore` list
* Fixed a file named twice through a symlinked folder or junction being formatted and counted twice
* Fixed `--diff` run from a subfolder of a Laravel app writing `../` paths that `git apply` rejects
* Fixed "auto" line endings turning a mostly LF file into CRLF when one line ended in CRLF
* Fixed `--staged` with "auto" line endings turning a CRLF working copy into LF under git's `core.autocrlf`
* Fixed `--staged` storing CRLF in the commit when git's line-ending conversion would store LF
* Fixed a Pint `notName` like `legacy*` skipping every file in a folder it matches, which Pint still formats
* Fixed a Pint `notPath` like `tests/Fixtures` missing `tests/FixturesTest.php`, which Pint matches as part of the path
* Fixed Pint `notName` braces like `*.{stub,tpl}.php` and plain regexes like `/Models/` in `notName` or `notPath` being ignored
* Fixed arbitrary Tailwind values like `text-[10dvh]`, `shadow-[0_0_2px_#000]` or `font-[500]` sorting by the wrong property
* Fixed arbitrary Tailwind values like `p-[3px]` sorting ahead of named ones like `p-4`
* Fixed negative and fraction Tailwind classes like `-mt-2` or `translate-x-2/3` sorting as their positive or whole form
* Fixed Tailwind variants like `min-[50rem]`, `@min-[9rem]`, `data-[a]` or `group-hover` sorting in another order than Tailwind's
* Fixed custom Tailwind breakpoints in `px` sorting by width, where Tailwind sorts them before every `rem` one
* Fixed Tailwind classes like `w-[10px]` sorting ahead of `w-[9px]`, which Tailwind compares by number
* Fixed a Tailwind class with a variant Tailwind rejects, like `hover-foo:flex`, `group-md:flex` or a breakpoint the theme removed, being sorted instead of left at the front

## Version 0.7.0 (2026-10-29)
* Improved JSON in a `<script>` with type `application/ld+json`, `application/json`, `importmap` or `speculationrules` to be formatted like any other JSON
* Improved an unclosed string, comment, block or Blade echo in CSS, JSON, XML, HTML or Blade to be reported where it opens instead of at the end of the file
* Improved `init` to write only the schema reference, so the new `laravel-fmt.json` no longer overrides `.editorconfig`, Prettier, Pint or Mago
* Improved `laravel-fmt lsp` to log why it exits when an editor ends it without a shutdown request
* Improved a Markdown code block tagged with a full extension like `blade.php` to be formatted as that file
* Optimized Markdown with nested lists and blockquotes, which formats about a third faster
* Fixed an empty or whitespace-only PHP file being given a lone line break
* Fixed a package nested in a Laravel app ignoring the app's `.prettierignore` and Pint excludes when formatted on save
* Fixed `--stdin-filepath` relative to a subfolder applying HTML layout to a Laravel Boost guideline
* Fixed `--stdin-filepath` holding `*` or `?` failing inside a project on Windows instead of formatting as it does outside one
* Fixed Markdown with nested blockquotes and lines that continue a quote without `>` hanging for minutes
* Fixed a PHP code block in Markdown with a lone CR in a string changing layout on a second run
* Fixed a form feed in a CSS value, selector or at-rule leaving the surrounding spacing uneven
* Fixed a file with CR-only line endings reporting parse errors on the wrong line, and CSS and JSON in one losing their blank lines, line breaks and comment indentation

## Version 0.6.0 (2026-10-28)
* Added support for Mago's `extends`, so formatter settings from a shared Mago config are used
* Added detection of `mago.yaml`, `mago.yml`, `mago.json` and `mago.dist.*` configs
* Added support for Pint's `extend`, so rules and excludes from a shared `pint.json` are used
* Added support for a Prettier config in `package.yaml`
* Added `laravel-fmt-ignore` comments in JSONC files like `tsconfig.json`
* Improved YAML formatting to cover a value on the line below its key, a plain value that runs onto more lines, a `- - a` nested list and a file that is one flow collection, which were left unformatted
* Fixed a parent folder that can be entered but not listed, like a `0711` home folder, failing the run
* Fixed a no-break space between XML elements, or as a Livewire component's whole template, being removed
* Fixed Tailwind class sorting keeping the spaces at the edges of a Blade `class` value, and of a one-class HTML value
* Fixed an `.editorconfig` saved with a BOM losing its `root = true` or its first section
* Fixed an invalid `package.json` failing the run instead of being passed over, as Prettier does
* Fixed a shared Prettier config named in `package.json` being skipped for a `.prettierrc` Prettier itself would ignore
* Fixed `init` run from a subfolder of a Laravel app writing a `laravel-fmt.json` that is never read
* Fixed a Blade view opening with `<?php if (...): ?>` or `<?php if (...) { ?>` failing as a broken Livewire component
* Fixed deeply nested CSS, HTML, XML or Blade crashing the whole run instead of reporting the file
* Fixed a too-deeply nested JSON or PHP file reporting hundreds of follow-on errors after the depth error
* Fixed a multi-line YAML flow collection with a comment line holding an apostrophe being rejected as unterminated
* Fixed YAML block scalar lines starting with `#`, like `### Thanks to all contributors!`, being dropped from the text
* Fixed a YAML list item holding an apostrophe, like `- it's`, or a flow mapping, like `- { a: 1 }`, being read as a key and reshaped
* Fixed a multi-line YAML flow collection keeping its old indent when its key moved, which could change its value
* Fixed a YAML mapping with a duplicate key being formatted instead of reported
* Fixed a YAML key indented to no open mapping, like a key lined up under a list item's dash, passing as valid instead of being reported
* Fixed an ignored YAML document with an indented body losing the indent of its first line only
* Fixed XML with CR line endings losing its blank lines and keeping CRs inside text
* Fixed XML element and attribute names with non-ASCII letters, like `<café>`, being rejected
* Fixed a stray character in an XML tag, like the `$` in `<a b="1" $/>`, being silently removed instead of reported

## Version 0.5.0 (2026-10-26)
* Added support for `.prettierrc.toml`
* Added a warning when the project's Prettier config is a JS, TS or JSON5 file, which laravel-fmt can't read
* Improved `--show-config` to show which config each layout setting comes from, and to say when the file is ignored
* Fixed the Prettier config being picked in a different order than Prettier uses when a folder has more than one
* Fixed `.mts` and `.cts` Prettier configs not being found
* Fixed the first pattern of a `.gitignore` or `.prettierignore` saved with a BOM being ignored
* Fixed `.prettierignore` patterns with a leading space or an escaped trailing space not matching
* Fixed a `.gitignore` line longer than 64 KiB failing the run
* Fixed Markdown tables with Chinese, Japanese or Korean text, or decomposed accents, being misaligned
* Fixed JSON with Chinese, Japanese or Korean text staying on one line past the print width

## Version 0.4.0 (2026-10-20)
* Added a `laravel-fmt/endOfLine` request to `laravel-fmt lsp` so editor plugins can apply the project's line ending
* Improved `laravel-fmt lsp` to show a broken config as a message in the editor
* Fixed `laravel-fmt lsp` looking up config in the wrong place for files on a network share or in WSL opened from Windows
* Fixed `laravel-fmt lsp` rewriting every line on each save when a file's line endings differed from the config
* Fixed the kept blank lines of a YAML `|+` or `>+` block being removed when it has an anchor or tag, ends the file, or sits under `laravel-fmt-ignore`
* Fixed trailing spaces being removed from an anchored or tagged YAML block like `key: &a |`
* Fixed the lines of a multi-line template literal in an Alpine or bound attribute being re-indented, which changed the string
* Fixed a Markdown line starting with a code span like ` ```code``` ` getting a backslash that broke the code span
* Fixed `markdown.proseWrap: always` showing a backslash before a `1.` or `1)` it wrapped to the start of a line
* Fixed a comment after the root value in JSON with comments, like `tsconfig.json`, being moved off its line
* Fixed a `+` or a digit in the comment after a YAML block header changing how the block is formatted

## Version 0.3.0 (2026-10-17)
* Added `--reporter gitlab` to write a GitLab Code Quality report with `--check`, so unformatted files show up in merge requests
* Added formatting of PHP, Blade, HTML, CSS, JSON, YAML and XML code blocks in Markdown, including PHP snippets without `<?php`; a block that doesn't parse is kept as written
* Improved `!` in Alpine attributes to keep a space after it (`open = ! open`), following `php.notOperatorSpace` like PHP and Blade echoes
* Fixed comments after the last statement in a `@php` block or a `<?php ?>` block in Blade being removed
* Fixed strings like `"{$a->f("x")}"` failing to format when the code inside `{$...}` has its own quoted string
* Fixed `markdown.proseWrap: always` wrapping text in lists and blockquotes past the print width
* Fixed the blank line before a comment at the end of a PHP file being removed
* Fixed a comment written after a docblock or block comment on the same line being moved onto its own line
* Fixed an unquoted attribute value holding a PHP tag, like `<div x=<?= $a ?>>`, being cut at `?>` and quoted into broken PHP
* Fixed a form feed or vertical tab between PHP tokens being formatted instead of failing, as PHP rejects them
* Fixed a lone `@` in Blade text making the surrounding text change layout on a second run
* Fixed some binary operators, concatenations and destructuring assignments changing layout on a second run
* Fixed lines of a method chain or concatenation after a multi-line operand being indented deeper than the lines before it

## Version 0.2.0 (2026-10-15)
* Added `blade.blankLineBeforeDirectives` to put a blank line before the Blade block directives you list, like `@if` and `@foreach`
* Added support for Pint's `blank_line_before_statement` Blade rule in `pint.json`
* Added Tailwind class sorting in `$attributes->class(...)` and `$attributes->merge(['class' => ...])`, slot attributes included
* Improved dynamic tags like `<{{ $as }}>` to format their attributes and body instead of keeping them as text
* Fixed `<img{!! $attributes !!}>` and other echoes glued to a tag name failing to format
* Fixed dynamic headings like `<h{{ $level }}>…</h{{ $level }}>` failing to format
* Fixed a `@name(` with no closing `)` in an attribute value being rewritten into markup that no longer parses; it now fails with an error
* Fixed Tailwind sorting moving a `(` class next to a bare directive in `class`, where Blade reads it as the directive's arguments
* Fixed an assignment chain after a multi-line list like `[...] = $a = $b` changing layout on a second run
* Fixed a lone carriage return after an echo in an unquoted attribute value turning into a line ending on a second run

## Version 0.1.0 (2026-10-14)
* Initial release
