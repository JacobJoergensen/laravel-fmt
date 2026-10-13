# Laravel Fmt

[![Latest Version](https://img.shields.io/packagist/v/jacobjoergensen/laravel-fmt.svg)](https://packagist.org/packages/jacobjoergensen/laravel-fmt)
[![CI](https://github.com/JacobJoergensen/laravel-fmt/actions/workflows/ci.yml/badge.svg)](https://github.com/JacobJoergensen/laravel-fmt/actions)
[![License](https://img.shields.io/github/license/JacobJoergensen/laravel-fmt)](LICENSE.md)

Laravel Fmt formats PHP, Blade, and Livewire 4, with first-class Tailwind class sorting. It also formats HTML, CSS, Markdown, JSON, YAML, and XML.

> [!WARNING]
> **Status: beta.** Not recommended for production code yet. Expect rough edges, and please [report issues](https://github.com/JacobJoergensen/laravel-fmt/issues).

## Why Laravel Fmt?

Livewire 4 (Single Components), Blade and Tailwind class sorting in one binary. No Node, no Prettier plugin, and it fits the toolchain you already run: drop it in next to your pint.json, .php-cs-fixer.php or mago.toml, and it reads and adopts the formatting settings it understands.

## Get Started

```sh
composer require jacobjoergensen/laravel-fmt --dev
```

Format your project:

```sh
vendor/bin/laravel-fmt
```

With no arguments it walks the project. Pass paths to narrow it down:

```sh
vendor/bin/laravel-fmt app/Models resources/views
```

## Checking without writing

`--check` reports which files would change and writes nothing.
`--diff` prints the changes as a unified diff.

```sh
vendor/bin/laravel-fmt --check
vendor/bin/laravel-fmt --diff
```

## Configuration

The defaults need no config. `init` writes a `laravel-fmt.json` you can edit:

```sh
vendor/bin/laravel-fmt init
```

Configuration covers project facts (print width, indent, end-of-line, PHP options), not style preferences. It also reads `.editorconfig`, `.prettierrc`, `pint.json`, `.php-cs-fixer.php`, and `mago.toml` when present and adopts the settings it models.

## Format on commit

### Zero dependencies (recommended)

`laravel-fmt --staged` formats the *staged content* of each staged file and
re-stages it. Unstaged edits are never touched; partially staged files keep
their working-tree copy intact. No stash dance, no extra tooling.

```sh
# .git/hooks/pre-commit (chmod +x), or .husky/pre-commit
vendor/bin/laravel-fmt --staged
```

### husky + lint-staged

```json
{
  "lint-staged": {
    "*.php": "vendor/bin/laravel-fmt"
  }
}
```

## Continuous integration

```sh
vendor/bin/laravel-fmt --check   # exits 1 if anything would change, 2 on failure
```

On GitHub Actions, parse errors and unformatted files are annotated inline on the pull request and listed in the job summary.

```yaml
name: format
on:
  pull_request:
  push:
    branches: [ main ]

jobs:
  laravel-fmt:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v7
      - uses: shivammathur/setup-php@v2
        with:
          php-version: '8.4'

      - uses: ramsey/composer-install@v4
      - run: vendor/bin/laravel-fmt --check
```

Windows runners: git's `autocrlf` converts checkouts to CRLF, which the
default `endOfLine: lf` reports as drift. Add `* text=auto eol=lf` to
`.gitattributes`, or set `endOfLine` in `laravel-fmt.json` if your project
genuinely uses CRLF.

## Editor integration

Editors that can pipe a buffer through a command use `--stdin-filepath`:

```sh
vendor/bin/laravel-fmt --stdin-filepath app/Models/User.php < input.php
```

A dedicated LSP server and editor extensions are in active development.

## Documentation

A full configuration reference and guides live at [laravel-fmt.com](https://laravel-fmt.com).

## Contributing

See [CONTRIBUTING.md](.github/CONTRIBUTING.md) for filing bugs and submitting PRs.

## License

MIT. See [LICENSE](LICENSE.md).
