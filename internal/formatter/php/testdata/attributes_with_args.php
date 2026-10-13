<?php

#[Route('/users', methods: ['GET', 'POST'])]
#[Middleware('auth')]
class UserController
{
    #[Group('users'), WithoutMiddleware]
    public string $name;

    #[Test]
    #[DataProvider('cases')]
    public function itWorks(): void {}
}
