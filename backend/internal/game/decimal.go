// Package game implements the exact finite congestion-game model used by the
// application. Decimal intentionally stores rational values, so all values
// entered as finite decimals and every F/S comparison remain exact.
package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

// maxExactLiteralLength keeps arbitrary-precision input bounded while still
// allowing an exact result (notably a PoA fraction) to round-trip through
// SQLite and the public JSON API.  Four thousand decimal digits is far below
// the HTTP-body limit, yet comfortably above the size produced by the
// supported model limits.
const maxExactLiteralLength = 4096

// Decimal is an immutable exact rational number. Input accepts only finite
// decimal notation; a result that is not a terminating decimal is rendered as
// an exact fraction (for example, 4/3 for the price of anarchy in preset A).
// The zero value is a valid zero.
type Decimal struct {
	rational *big.Rat
}

// ParseDecimal parses a finite decimal without accepting exponent notation,
// NaN, Infinity, or an arbitrary fraction supplied by a client.
func ParseDecimal(text string) (Decimal, error) {
	text = strings.TrimSpace(text)
	if len(text) > maxExactLiteralLength {
		return Decimal{}, fmt.Errorf("десятичное число длиннее %d символов", maxExactLiteralLength)
	}
	if !validDecimalText(text) {
		return Decimal{}, fmt.Errorf("некорректное десятичное число %q", text)
	}

	sign := 1
	if text[0] == '+' || text[0] == '-' {
		if text[0] == '-' {
			sign = -1
		}
		text = text[1:]
	}

	whole, fraction, hasFraction := strings.Cut(text, ".")
	if whole == "" {
		whole = "0"
	}
	digits := whole
	if hasFraction {
		digits += fraction
	}
	if digits == "" {
		digits = "0"
	}

	numerator := new(big.Int)
	if _, ok := numerator.SetString(digits, 10); !ok {
		return Decimal{}, fmt.Errorf("не удалось разобрать десятичное число %q", text)
	}
	if sign < 0 {
		numerator.Neg(numerator)
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(fraction))), nil)
	return decimalFromRat(new(big.Rat).SetFrac(numerator, denominator)), nil
}

// MustDecimal is intended for static presets and tests.
func MustDecimal(text string) Decimal {
	value, err := ParseDecimal(text)
	if err != nil {
		panic(err)
	}
	return value
}

// Zero returns an exact zero.
func Zero() Decimal { return decimalFromRat(new(big.Rat)) }

// DecimalFromInt returns an exact integer.
func DecimalFromInt(value int) Decimal {
	return decimalFromRat(new(big.Rat).SetInt64(int64(value)))
}

func decimalFromRat(value *big.Rat) Decimal {
	return Decimal{rational: new(big.Rat).Set(value)}
}

func (d Decimal) rat() *big.Rat {
	if d.rational == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(d.rational)
}

// Add returns d+other without mutating either operand.
func (d Decimal) Add(other Decimal) Decimal {
	return decimalFromRat(new(big.Rat).Add(d.rat(), other.rat()))
}

// Sub returns d-other without mutating either operand.
func (d Decimal) Sub(other Decimal) Decimal {
	return decimalFromRat(new(big.Rat).Sub(d.rat(), other.rat()))
}

// Mul returns d*other without mutating either operand.
func (d Decimal) Mul(other Decimal) Decimal {
	return decimalFromRat(new(big.Rat).Mul(d.rat(), other.rat()))
}

// MulInt returns d multiplied by an integer.
func (d Decimal) MulInt(other int) Decimal {
	return decimalFromRat(new(big.Rat).Mul(d.rat(), new(big.Rat).SetInt64(int64(other))))
}

// Div returns d/other. Division by zero is reported rather than producing a
// non-finite number.
func (d Decimal) Div(other Decimal) (Decimal, error) {
	if other.IsZero() {
		return Decimal{}, fmt.Errorf("деление на ноль")
	}
	return decimalFromRat(new(big.Rat).Quo(d.rat(), other.rat())), nil
}

// Cmp compares two exact values.
func (d Decimal) Cmp(other Decimal) int { return d.rat().Cmp(other.rat()) }

func (d Decimal) IsZero() bool     { return d.Cmp(Zero()) == 0 }
func (d Decimal) IsNegative() bool { return d.Cmp(Zero()) < 0 }
func (d Decimal) IsPositive() bool { return d.Cmp(Zero()) > 0 }

// String returns either a canonical finite decimal or a canonical fraction.
func (d Decimal) String() string {
	return decimalOrFraction(d.rat())
}

// MarshalJSON always uses a string. JSON numbers are binary floating literals
// in many clients; a string preserves the exact server result at the boundary.
func (d Decimal) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON accepts a decimal JSON string or number for ergonomic input,
// plus the canonical p/q form emitted for non-terminating exact results. This
// makes saved server responses round-trip without loss of precision.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("ожидалось десятичное число")
	}
	var text string
	if trimmed[0] == '"' {
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return fmt.Errorf("прочитать десятичную строку: %w", err)
		}
	} else {
		text = string(trimmed)
	}
	value, err := parseExactValue(text)
	if err != nil {
		return err
	}
	*d = value
	return nil
}

func parseExactValue(text string) (Decimal, error) {
	text = strings.TrimSpace(text)
	if len(text) > maxExactLiteralLength {
		return Decimal{}, fmt.Errorf("точное число длиннее %d символов", maxExactLiteralLength)
	}
	if !strings.Contains(text, "/") {
		return ParseDecimal(text)
	}
	if strings.Count(text, "/") != 1 {
		return Decimal{}, fmt.Errorf("некорректная точная дробь %q", text)
	}
	numeratorText, denominatorText, _ := strings.Cut(text, "/")
	if numeratorText == "" || denominatorText == "" || !validIntegerText(numeratorText) || !validUnsignedIntegerText(denominatorText) {
		return Decimal{}, fmt.Errorf("некорректная точная дробь %q", text)
	}
	numerator, ok := new(big.Int).SetString(numeratorText, 10)
	if !ok {
		return Decimal{}, fmt.Errorf("не удалось разобрать числитель %q", numeratorText)
	}
	denominator, ok := new(big.Int).SetString(denominatorText, 10)
	if !ok || denominator.Sign() == 0 {
		return Decimal{}, fmt.Errorf("знаменатель должен быть положительным")
	}
	return decimalFromRat(new(big.Rat).SetFrac(numerator, denominator)), nil
}

func validDecimalText(text string) bool {
	if text == "" {
		return false
	}
	if text[0] == '+' || text[0] == '-' {
		text = text[1:]
	}
	if text == "" {
		return false
	}
	dots := 0
	digits := 0
	for _, char := range text {
		switch {
		case char == '.':
			dots++
			if dots > 1 {
				return false
			}
		case unicode.IsDigit(char) && char <= '9':
			digits++
		default:
			return false
		}
	}
	return digits > 0
}

func validIntegerText(text string) bool {
	if text == "" {
		return false
	}
	if text[0] == '+' || text[0] == '-' {
		text = text[1:]
	}
	return validUnsignedIntegerText(text)
}

func validUnsignedIntegerText(text string) bool {
	if text == "" {
		return false
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func decimalOrFraction(value *big.Rat) string {
	denominator := new(big.Int).Set(value.Denom())
	two := big.NewInt(2)
	five := big.NewInt(5)
	remainder := new(big.Int)
	twoExponent := 0
	fiveExponent := 0
	for {
		remainder.Mod(denominator, two)
		if remainder.Sign() != 0 {
			break
		}
		denominator.Quo(denominator, two)
		twoExponent++
	}
	for {
		remainder.Mod(denominator, five)
		if remainder.Sign() != 0 {
			break
		}
		denominator.Quo(denominator, five)
		fiveExponent++
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return value.RatString()
	}

	scale := twoExponent
	if fiveExponent > scale {
		scale = fiveExponent
	}
	numerator := new(big.Int).Set(value.Num())
	if twoExponent < scale {
		numerator.Mul(numerator, new(big.Int).Exp(two, big.NewInt(int64(scale-twoExponent)), nil))
	}
	if fiveExponent < scale {
		numerator.Mul(numerator, new(big.Int).Exp(five, big.NewInt(int64(scale-fiveExponent)), nil))
	}

	sign := ""
	if numerator.Sign() < 0 {
		sign = "-"
		numerator.Abs(numerator)
	}
	digits := numerator.String()
	if scale == 0 {
		return sign + digits
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	cut := len(digits) - scale
	integer, fraction := digits[:cut], strings.TrimRight(digits[cut:], "0")
	if fraction == "" {
		return sign + integer
	}
	return sign + integer + "." + fraction
}
