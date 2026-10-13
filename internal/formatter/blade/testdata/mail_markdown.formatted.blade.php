<x-mail::message>
# Thank You for Contacting Us!

Hi {{ $name }},

Thank you for reaching out to us! We have received your message and will get back to you as soon as possible.

## Your Submission

**Subject:** {{ $topic }}

**Message:**

{{ $message }}

We appreciate you taking the time to contact us.

<x-mail::button :url="$url">
Visit Our Website
</x-mail::button>

Best regards,<br>
{{ config('app.name') }}
</x-mail::message>

<x-mail::panel>
Your order has shipped and is on its way to the delivery address you gave us at checkout earlier today, so watch your inbox.
</x-mail::panel>
