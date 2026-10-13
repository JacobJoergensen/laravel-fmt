<div>
    <?php $total=1+2; ?>
    <?php
        $rows = ['first'=>1,'second'=>2];
        $label   =   'ready';
    ?>
    <?php use App\Support\Helper; ?>
    <?php $wide = combineTheValues($alphaValue, $betaValue, $gammaValue, $deltaValue, $epsilonValue, $zetaValue, $etaValue); ?>
    <?php if ($total): ?>
        <p>alt syntax stays a fragment</p>
    <?php elseif ($label): ?>
        <p>branch</p>
    <?php endif; ?>
    <?php if ($total) { ?>
        <p>brace syntax straddles too</p>
    <?php } ?>
    <?= $total ?>
    <?php // a lone comment is not a statement ?>
    <p>{{ $total }}</p>
</div>
