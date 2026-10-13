<?php

class Service
{
    public function handle(
        #[Audited]
        string $token,
        FirstDependency $first,
        SecondDependency $second,
    ): SuccessResult|FailureResult
    {
        return $result;
    }
}
