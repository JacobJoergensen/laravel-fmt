# Changelog

## Unreleased
* Improved `laravel-fmt lsp` to show a broken config as a message in the editor

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
