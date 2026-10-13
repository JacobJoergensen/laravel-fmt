<div>
    <?php echo "?> <div class='p-2 flex'>foo</div>"; ?>
</div>
<div>
    <?php echo 'a ?> <b>'; ?>
</div>
<div>
    <?php echo <<<EOT
?> <b class='x'>y</b>
EOT;
?>
</div>
<div>
    <?php
        /* ?> <b class='x'>y</b> */
        echo 1;
    ?>
</div>
<div <?php echo "?> x='1'"; ?>>z</div>
<script>
    var a = "<?php echo '?></script>'; ?>";
</script>
<p>after</p>
