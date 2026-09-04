<?php
function calculateDiscount(int $amount, int $tier): int {
    if ($amount > 1000) {
        if ($tier == 2) return $amount - 100;
        return $amount - 50;
    }
    return $amount;
}
