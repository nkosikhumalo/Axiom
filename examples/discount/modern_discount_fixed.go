//go:build ignore

package main

// Correct rewrite — uses > not >= to match legacy logic exactly.
func CalculateDiscount(amount int, tier int) int {
	if amount > 1000 {
		if tier == 2 {
			return amount - 100
		}
		return amount - 50
	}
	return amount
}
