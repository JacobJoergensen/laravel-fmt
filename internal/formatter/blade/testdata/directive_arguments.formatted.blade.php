@include('partials.row', ['a' => 1],)
@can('update', $post,)
    <p>edit</p>
@endcan
@markdown(Hello   world)
@icon(star)
