<?php

try {
    $reader->parse($file);
} catch (App\Pdf\CrossReferenceException|App\Pdf\FilterException|App\Pdf\PdfParserException|App\Pdf\PdfReaderException $e) {
    report($e);
}

try {
    risky();
} catch (RuntimeException|LogicException $e) {
    handle($e);
}
