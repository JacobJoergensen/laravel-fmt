<div>
    @can('update', $post)
        <p>Editable</p>
    @elsecan('create', Post::class)
        <p>Creatable</p>
    @else
        <p>Read only</p>
    @endcan
    @unless ($active)
        <p>Inactive</p>
    @else
        <p>Active</p>
    @endunless
    @auth
        <p>Member</p>
    @else
        <p>Visitor</p>
    @endauth
    @auth('admin')
        <p>Admin</p>
    @elseauth('web')
        <p>Member</p>
    @endauth
    @guest('admin')
        <p>Not an admin</p>
    @elseguest
        <p>Visitor</p>
    @endguest
</div>
