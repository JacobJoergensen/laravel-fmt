<?php

abstract class BaseAction
{
    abstract public function handle(): void;

    final public function run(): void
    {
        $this->handle();
    }
}

final class ConcreteAction extends BaseAction
{
    public function handle(): void {}
}
