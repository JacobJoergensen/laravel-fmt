<?php

final class Account
{
    final public const STATUS = 1;

    private readonly string $name;

    public private(set) int $count;

    final protected static function make(): self
    {
        return new self();
    }

    public function __construct(private readonly string $id)
    {
        $this->name = $id;
    }

    public static function ok(): void
    {
        //
    }
}

final readonly class Money
{
    public function __construct(public int $amount)
    {
        //
    }
}

abstract class Base {}
