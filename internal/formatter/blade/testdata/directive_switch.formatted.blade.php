@switch($type)
    @case('a')
        <p>handling A</p>
        @break
    @case('b')
        <p>handling B</p>
        @break
    @default
        <p>handling default</p>
@endswitch
