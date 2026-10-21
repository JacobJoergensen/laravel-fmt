# Laravel Fmt

[![Latest Version](https://img.shields.io/packagist/v/jacobjoergensen/laravel-fmt.svg)](https://packagist.org/packages/jacobjoergensen/laravel-fmt)
[![Documentation](https://img.shields.io/badge/docs-laravel--fmt.com-blue)](https://laravel-fmt.com/docs/getting-started/)
[![CI](https://github.com/JacobJoergensen/laravel-fmt/actions/workflows/ci.yml/badge.svg)](https://github.com/JacobJoergensen/laravel-fmt/actions)
[![License](https://img.shields.io/github/license/JacobJoergensen/laravel-fmt)](LICENSE.md)

Laravel Fmt formats PHP, Blade, and Livewire 4, with first-class Tailwind class sorting. It also formats HTML, CSS, Markdown, JSON, YAML, and XML.

> [!WARNING]
> **Status: beta.** Not recommended for production code yet. Expect rough edges, and please [report issues](https://github.com/JacobJoergensen/laravel-fmt/issues).

## Why Laravel Fmt?

Blade, Livewire 4 single-file components, and Tailwind class sorting in one binary. No Node, no Prettier plugin. Drop it in next to your pint.json, .php-cs-fixer.php or mago.toml and it adopts the formatting settings it understands.

## Get Started

```sh
composer require jacobjoergensen/laravel-fmt --dev
```

Format your project:

```sh
vendor/bin/laravel-fmt
```

With no arguments it walks the project. Pass paths to narrow it down, or `--check` to report what would change without writing:

```sh
vendor/bin/laravel-fmt app/Models resources/views
vendor/bin/laravel-fmt --check
```

## Documentation

Find the full documentation at [laravel-fmt.com](https://laravel-fmt.com/docs/getting-started/).

## Contributing

See [CONTRIBUTING.md](.github/CONTRIBUTING.md) for filing bugs and submitting PRs.

## License

MIT. See [LICENSE](LICENSE.md).
