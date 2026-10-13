<?php

$classes = [
    'alpha-one beta-two gamma-three delta-four epsilon-five zeta-six eta-seven theta-eight iota-nine kappa-ten' =>
        $item->state === Status::Archived,
    'alpha-one beta-two gamma-three delta-four epsilon-five zeta-six eta-seven theta-eight iota-nine lambda' =>
        ! $items->contains('id', $user->id),
];

$enabled = match ($provider) {
    Provider::Local => true,
    Provider::RemoteStorage =>
        filled(config('services.remote.storage.access_key')) && filled(config('services.remote.storage.client_id')),
};
