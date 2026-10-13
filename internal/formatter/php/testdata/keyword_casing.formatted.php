<?php

class Foo extends Bar
{
    public function run(): void
    {
        if (true) {
            return Foo::CONST;
        }

        echo __LINE__;
    }
}
