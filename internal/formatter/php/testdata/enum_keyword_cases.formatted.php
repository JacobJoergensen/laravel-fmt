<?php

enum ShippingMethod: string
{
    case Static = 'Static';
    case Print = 'Print';
    case List = 'List';
    case NextDay = 'Next Day';
}
