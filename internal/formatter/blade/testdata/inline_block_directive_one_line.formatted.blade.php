<div>
    <span>
        {{ $a }}
        @if ($b?->capacity) /{{ $b->capacity }} @endif
    </span>
    <span>
        @if ($b)
            /{{ $b->capacity }}
        @endif
    </span>
    <span>
        @if ($bAlphaBetaGammaDeltaEpsilonZetaEtaThetaIotaKappaLambdaMuNuXiOmicronPiRhoSigmaTau)
            {{ $b->capacityWithAVeryLongName }}
        @endif
    </span>
</div>
