<html lang="{{ str_replace('_', '-', app()->getLocale()) }}">
<head>
    <title>{{ $title }}</title>
</head>
{{-- shell --}}
@include('partials.banner')
<body class="antialiased">
    <main>{{ $slot }}</main>
</body>
</html>
<div>
    <html>
        <body>
            <p>nested</p>
        </body>
    </html>
</div>
<html>
    <body>
        <p>a</p>
    </body>
    <div>stray</div>
</html>
