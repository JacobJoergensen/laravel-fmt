<?php

class Person
{
    public string $greeting {
        get => 'hello';
        set(string $value) {
            $this;
        }
    }
    public string $abstract { get; set; }
    public string $refGet {
        &get => 'ref';
    }
    public string $withAttrs {
        #[Deprecated]
        get => 'deprecated';
    }
    public string $finalGet {
        FINAL get => 'final';
    }
    public string $finalSet {
        final set(string $value) {
            $this->finalSet = $value;
        }
    }
    public string $finalRef {
        #[Deprecated] final &get => $this->finalRef;
    }
    public string $plain = 'done';
}
