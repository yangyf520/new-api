package common

import "github.com/shopspring/decimal"

// RoundDecimal rounds a value to 4 decimal places (matches decimal(12,4)).
func RoundDecimal(v float64) float64 {
	f, _ := decimal.NewFromFloat(v).Round(4).Float64()
	return f
}

// SubtractDecimal returns a - b rounded to 4 decimal places.
func SubtractDecimal(a, b float64) float64 {
	result, _ := decimal.NewFromFloat(a).Sub(decimal.NewFromFloat(b)).Round(4).Float64()
	return result
}

// DecimalGT reports whether a is strictly greater than b after normalization.
func DecimalGT(a, b float64) bool {
	return decimal.NewFromFloat(RoundDecimal(a)).GreaterThan(decimal.NewFromFloat(RoundDecimal(b)))
}

// DecimalLT reports whether a is strictly less than b after normalization.
func DecimalLT(a, b float64) bool {
	return decimal.NewFromFloat(RoundDecimal(a)).LessThan(decimal.NewFromFloat(RoundDecimal(b)))
}

// SumExceeds reports whether used + delta is strictly greater than limit.
func SumExceeds(used, delta, limit float64) bool {
	usedDec := decimal.NewFromFloat(RoundDecimal(used))
	deltaDec := decimal.NewFromFloat(RoundDecimal(delta))
	limitDec := decimal.NewFromFloat(RoundDecimal(limit))
	return usedDec.Add(deltaDec).GreaterThan(limitDec)
}
