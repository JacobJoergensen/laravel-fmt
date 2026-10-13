<?php

#[Deprecated]
class Old
{
    #[Getter]
    public string $name;

    #[Route('/home')]
    public function home(): void {}
}
