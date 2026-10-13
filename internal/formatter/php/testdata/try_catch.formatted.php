<?php

try {
    doRisky();
} catch (Exception $e) {
    handle();
} finally {
    cleanup();
}
