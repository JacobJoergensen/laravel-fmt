<?php

$mail->line(
    'Your workspace export archive will be permanently removed from temporary cloud storage on '
        .$completedAt->toDateTimeString()
        .'.',
);
