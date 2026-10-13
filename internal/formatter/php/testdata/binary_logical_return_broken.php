<?php

return $token->isActive() && $token->notExpired() && $token->hasScope($scope);
