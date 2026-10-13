<?php

$prepared = match (true) {
    $e instanceof BackedEnumCaseNotFoundException => new NotFoundHttpException($e->getMessage(), $e),
    $e instanceof ModelNotFoundException => new NotFoundHttpException($e->getMessage(), $e),
    $e instanceof AuthorizationException && $e->hasStatus() => new HttpException(
        $e->status(),
        $e->response()?->message(),
        $e
    ),
    default => $e,
};

$authorBroken = match ($kind) {
    'a' => 1,
    'bbbbbb' => new Thing(
        $x,
        $y,
    ),
    'cc' => 3,
};
