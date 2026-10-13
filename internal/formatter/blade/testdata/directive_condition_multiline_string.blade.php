@if ($order->isPending && $order->hasStock && $order->customer->isVerified && $order->paymentMethod && $note === 'first line of the note
second line of the note')
    <p>ready to ship</p>
@endif
