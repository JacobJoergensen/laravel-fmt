<?php

use Livewire\Component;

new class extends Component {
    public string $name = 'World';
};
?>

<div>
    <p>Hello {{ $name }}</p>

    <p>Goodbye {{ $name }}</p>
</div>
