<?php

return [
    'name'         => config('app.name'),
    'appUrl'       => config('app.url'),
    'locale'       => app()->getLocale(),
    'translations' => $this->getTranslations(),
    'features'     => [
        'filmSearch' => Feature::active(FilmSearch::class),
        'newsletter' => Feature::active(NewsletterSignup::class),
    ],
    'flash' => [
        'success'            => $request->session()->get('success'),
        'error'              => $request->session()->get('error'),
        'newsletter_success' => $request->session()->get('newsletter_success'),
    ],
];
