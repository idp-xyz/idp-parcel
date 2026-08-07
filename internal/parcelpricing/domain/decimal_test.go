package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func TestDecimalUsesExactBase10Arithmetic(t *testing.T) {
	result, err := decimal(t, "0.1").Add(decimal(t, "0.2"))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if result.String() != "0.3" {
		t.Fatalf("0.1 + 0.2 = %s", result.String())
	}

	canonical := decimal(t, "12.500")
	if canonical.CanonicalString() != "12.5" {
		t.Fatalf("canonical = %q", canonical.CanonicalString())
	}
	if _, err := domain.ParseCanonical("12.500"); !errors.Is(err, domain.ErrInvalidDecimal) {
		t.Fatalf("non-canonical parse error = %v", err)
	}
	if _, err := domain.ParseCanonical("12.5"); err != nil {
		t.Fatalf("canonical parse: %v", err)
	}
	for _, raw := range []string{" 12.5", "12.5 ", "+12.5", ".5", "01"} {
		if _, err := domain.ParseCanonical(raw); !errors.Is(err, domain.ErrInvalidDecimal) {
			t.Fatalf("non-canonical %q error = %v", raw, err)
		}
	}
}

func TestDecimalRejectsScientificNotationAndExcessPrecision(t *testing.T) {
	if _, err := domain.ParseDecimal("1e3"); !errors.Is(err, domain.ErrInvalidDecimal) {
		t.Fatalf("scientific notation error = %v", err)
	}
	tooManyDigits := strings.Repeat("9", domain.DecimalMaxDigits+1)
	if _, err := domain.ParseDecimal(tooManyDigits); !errors.Is(err, domain.ErrDecimalPrecisionExceeded) {
		t.Fatalf("precision error = %v", err)
	}
	tooManyLeadingZeros := strings.Repeat("0", domain.DecimalMaxTextLength+1) + "1"
	if _, err := domain.ParseDecimal(tooManyLeadingZeros); !errors.Is(err, domain.ErrDecimalPrecisionExceeded) {
		t.Fatalf("input length error = %v", err)
	}
}

func TestDecimalMultiplicationNormalizesBeforeCheckingScaleLimit(t *testing.T) {
	left := decimal(t, "0."+strings.Repeat("0", domain.DecimalMaxScale-1)+"2")
	result, err := left.Mul(decimal(t, "0.5"))
	if err != nil {
		t.Fatalf("multiply: %v", err)
	}
	want := "0." + strings.Repeat("0", domain.DecimalMaxScale-1) + "1"
	if result.String() != want {
		t.Fatalf("result = %s, want %s", result.String(), want)
	}
}

func TestDecimalRoundToIncrementHandlesExactAndBoundaryValues(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		increment string
		mode      domain.RoundingMode
		want      string
	}{
		{"exact ceiling", "17", "1", domain.RoundingCeiling, "17"},
		{"just above ceiling", "17.01", "1", domain.RoundingCeiling, "18"},
		{"none", "17.01", "1", domain.RoundingNone, "17.01"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decimal(t, test.value).RoundToIncrement(decimal(t, test.increment), test.mode)
			if err != nil {
				t.Fatalf("round: %v", err)
			}
			if got.String() != test.want {
				t.Fatalf("rounded = %s, want %s", got.String(), test.want)
			}
		})
	}
}

func FuzzDecimalCeilingToWhole(f *testing.F) {
	f.Add(uint64(0), uint8(0))
	f.Add(uint64(17), uint8(1))
	f.Add(uint64(999999), uint8(99))
	f.Fuzz(func(t *testing.T, whole uint64, hundredths uint8) {
		hundredths %= 100
		if whole > 1_000_000_000_000 {
			whole %= 1_000_000_000_000
		}
		valueText := strings.TrimRight(strings.TrimRight(
			formatHundredths(whole, hundredths), "0"), ".")
		value, err := domain.ParseDecimal(valueText)
		if err != nil {
			t.Skip()
		}
		rounded, err := value.RoundToIncrement(domain.NewDecimalFromInt64(1), domain.RoundingCeiling)
		if err != nil {
			t.Fatalf("round: %v", err)
		}
		if rounded.Cmp(value) < 0 {
			t.Fatalf("rounded %s is below %s", rounded, value)
		}
		difference, err := rounded.Sub(value)
		if err != nil {
			t.Fatalf("difference: %v", err)
		}
		if difference.Cmp(domain.NewDecimalFromInt64(1)) >= 0 {
			t.Fatalf("difference %s is not below one", difference)
		}
	})
}

func formatHundredths(whole uint64, hundredths uint8) string {
	return strings.Join([]string{uintToString(whole), twoDigits(hundredths)}, ".")
}

func uintToString(value uint64) string {
	if value == 0 {
		return "0"
	}
	buffer := make([]byte, 0, 20)
	for value > 0 {
		buffer = append(buffer, byte('0'+value%10))
		value /= 10
	}
	for left, right := 0, len(buffer)-1; left < right; left, right = left+1, right-1 {
		buffer[left], buffer[right] = buffer[right], buffer[left]
	}
	return string(buffer)
}

func twoDigits(value uint8) string {
	return string([]byte{'0' + value/10, '0' + value%10})
}
