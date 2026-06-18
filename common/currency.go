package common

import "strings"

// NormalizeCurrency normalizes a currency code for storage and comparison.
func NormalizeCurrency(currency string) string {
	currency = strings.TrimSpace(strings.ToUpper(currency))
	switch currency {
	case "", "CNY", "RMB":
		return "CNY"
	default:
		return currency
	}
}

// CurrencyEqual reports whether two currency codes match after normalization.
func CurrencyEqual(a, b string) bool {
	return NormalizeCurrency(a) == NormalizeCurrency(b)
}
