package blade

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestFormatKeepsTheTextOfATemplateWithoutMarkup(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			"markdown keeps its indent and blank lines while the blade is rebuilt",
			"# Title\n\n@foreach($items as $item)\n* {{$item}}\n@endforeach\n\n@if($a)\n\n## Section\n@endif\n",
			"# Title\n\n@foreach ($items as $item)\n* {{ $item }}\n@endforeach\n\n@if ($a)\n\n## Section\n@endif\n",
		},
		{
			"an ignore comment keeps the next block",
			"{{-- laravel-fmt-ignore --}}\n@if($a  &&$b)\nx {{$y}}\n@endif\n@if($c)\nz\n@endif\n",
			"{{-- laravel-fmt-ignore --}}\n@if($a  &&$b)\nx {{$y}}\n@endif\n@if ($c)\nz\n@endif\n",
		},
		{
			"an ignore comment keeps the next text run",
			"{{-- laravel-fmt-ignore --}}\nHello {{$a}}\n{{$b}}\n\n{{$c}}\n",
			"{{-- laravel-fmt-ignore --}}\nHello {{$a}}\n{{$b}}\n\n{{ $c }}\n",
		},
		{
			"an indented @php keeps its body under it",
			"@foreach($xs as $x)\n    @php\n        $y = 1;\n    @endphp\n@endforeach\n",
			"@foreach ($xs as $x)\n    @php\n        $y = 1;\n    @endphp\n@endforeach\n",
		},
		{
			"a mail component body is copied whole",
			"@component('mail::message')\n{{$x}}\n@endcomponent\n",
			"@component('mail::message')\n{{$x}}\n@endcomponent\n",
		},
		{
			"one tag in a branch makes it markup",
			"@if($a)\nx\n@else\n<p>y</p>\n@endif\n",
			"@if ($a)\n    x\n@else\n    <p>y</p>\n@endif\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tt.in), config.Default())
			if string(got) != tt.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tt.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n%q\n---\n%q", got, twice)
			}
		})
	}
}
