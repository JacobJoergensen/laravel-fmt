# Changelog

## Unreleased
* Added `blade.blankLineBeforeDirectives` to put a blank line before the Blade block directives you list, like `@if` and `@foreach`
* Added support for Pint's `blank_line_before_statement` Blade rule in `pint.json`
* Added Tailwind class sorting in `$attributes->class(...)` and `$attributes->merge(['class' => ...])`, slot attributes included
* Improved dynamic tags like `<{{ $as }}>` to format their attributes and body instead of keeping them as text
* Fixed `<img{!! $attributes !!}>` and other echoes glued to a tag name failing to format
* Fixed dynamic headings like `<h{{ $level }}>…</h{{ $level }}>` failing to format
* Fixed a `@name(` with no closing `)` in an attribute value being rewritten into markup that no longer parses; it now fails with an error
* Fixed Tailwind sorting moving a `(` class next to a bare directive in `class`, where Blade reads it as the directive's arguments

## Version 0.1.0 (2026-10-14)
* Initial release
