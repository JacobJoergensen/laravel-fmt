<?php

$this->warehouse->locations()->where('id', $id)->delete();
$this->warehouse->teamMembers();
