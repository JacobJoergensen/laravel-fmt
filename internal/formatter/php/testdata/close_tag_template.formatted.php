<div>
    <?php foreach ($elements as $element) { ?>
        <span class="item"><?php echo htmlspecialchars($element) ?></span>
    <?php } ?>
    <?php if ($footer !== '') { ?>
        <footer><?php echo $footer ?></footer>
    <?php } ?>
</div>
