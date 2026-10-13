<head>
    @hasstack('scripts')
        <div class="stack">
            @stack('scripts')
        </div>
    @else
        <p>No scripts</p>
    @endif
</head>
<body>
    @context('canonical')
        <link href="{{ $value }}" rel="canonical">
    @endcontext
    @componentFirst(['custom.alert', 'alert'])
        <strong>Whoops!</strong>
    @endComponentFirst
    @pushIf($shouldPush, 'scripts')
        <script src="/a.js"></script>
    @elsePushIf($other, 'scripts')
        <script src="/b.js"></script>
    @elsePush('scripts')
        <script src="/c.js"></script>
    @endPushIf
</body>
