@forelse ($users as $user)
    <li>{{ $user->name }}</li>
@empty
    <p>no users</p>
@endforelse
