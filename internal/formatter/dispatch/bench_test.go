package dispatch

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func BenchmarkFile(b *testing.B) {
	cases := []struct {
		name string
		path string
		src  string
	}{
		{
			name: "php",
			path: "x.php",
			src: `<?php
namespace App\Models;
class User
{
    public int $id;
    public string $name;
    public function getName(): string
    {
        return $this->name;
    }
    public function setName(string $name): void
    {
        $this->name = $name;
    }
}
`,
		},
		{
			name: "blade",
			path: "x.blade.php",
			src: `<div class="container mx-auto p-4">
    <h1>{{ $title }}</h1>
    @foreach($items as $item)
        <a href="{{ route('show', $item) }}">{{ $item->name }}</a>
    @endforeach
</div>
`,
		},
		{
			name: "livewire",
			path: "x.blade.php",
			src: `<?php
use Livewire\Component;
new class extends Component {
    public $name = 'World';
};
?>

<div>Hello {{ $name }}</div>
`,
		},
		{
			name: "markdown",
			path: "x.md",
			src: "# Usage\n\nRegister the service in a provider:\n\n" +
				"```php\nuse App\\Services\\Greeter;\n\npublic function register(): void\n{\n" +
				"    $this->app->singleton(Greeter::class, fn ($app) => new Greeter($app['config']['greeter']));\n}\n```\n\n" +
				"Then publish the config:\n\n```shell\nphp artisan vendor:publish --tag=greeter\n```\n\n" +
				"```json\n{\"greeter\": {\"name\": \"World\", \"loud\": false}}\n```\n",
		},
	}

	cfg := config.Default()

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			src := []byte(tc.src)
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := File(tc.path, src, cfg); err != nil {
					b.Fatalf("File returned error: %v", err)
				}
			}
		})
	}
}
