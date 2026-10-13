@component('mail::message')
# Order Shipped

Your order has shipped.  
It arrives on {{ $date }}.

- Item one
- Item two

@component('mail::button', ['url' => $url])
View Order
@endcomponent

Thanks,<br>
{{ config('app.name') }}
@endcomponent
