<?php

use Livewire\Component;

new class extends Component {
    public $name = 'World';
};
?>

<div>Hello {{ $name }}</div>
