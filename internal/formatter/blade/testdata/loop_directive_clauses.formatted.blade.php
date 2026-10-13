@foreach ($users as $user)
    <p>{{ $user->name }}</p>
@endforeach
@foreach ($items as $key => $value)
    <p>{{ $key }}</p>
@endforeach
@forelse ($posts as $post)
    <p>{{ $post }}</p>
@empty
    <p>none</p>
@endforelse
@for ($i = 0; $i < 10; $i++)
    <p>{{ $i }}</p>
@endfor
@foreach ($a as ['x' => $x, 'y' => $y])
    <p>{{ $x }}</p>
@endforeach
@foreach ($rows as &$row)
    <p>x</p>
@endforeach
@foreach ($users /* active */ as $user)
    <p>x</p>
@endforeach
@foreach ($users as $user // note
)
    <p>x</p>
@endforeach
<div>
    <ul>
        @foreach (
            $this->someVeryLongMethodName($withAnArgument, $andAnotherArgument)
                ->filter(fn ($item) => $item->isActive())
                ->values() as $item
        )
            <li>{{ $item }}</li>
        @endforeach
    </ul>
</div>
