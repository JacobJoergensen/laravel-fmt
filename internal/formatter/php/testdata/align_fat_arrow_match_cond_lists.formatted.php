<?php

$tier = match ($plan) {
    self::Free, self::Trial, self::Basic                             => Tier::Entry,
    self::Pro, self::Team                                            => Tier::Standard,
    self::Business, self::Enterprise, self::Custom, self::Government => Tier::Premium,
};
