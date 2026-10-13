<?php

class Point
{
    public function __construct(
        public int $x {
            get => $this->x;
            set(int $value) => $this->x = max(0, $value);
        },
        private string $label = 'origin' {
            final get => strtoupper($this->label);
        },
    ) {}
}
