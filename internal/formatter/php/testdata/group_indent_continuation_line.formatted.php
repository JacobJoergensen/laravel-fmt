<?php

it('rejects an upload that is not an image', function (): void {
    expect(
        fn () => resolve(ThumbnailMaker::class)
            ->render($album->covers->first(), 'data:image/png;base64,'.base64_encode('nope'), 'crop'),
    )->toThrow(RuntimeException::class, 'not an image');
});
