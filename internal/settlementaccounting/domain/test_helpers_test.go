package domain_test

import "testing"

type settlementStringValue interface {
	String() string
}

func settlementValue[T settlementStringValue](t *testing.T, constructor func(string) (T, error), value string) T {
	t.Helper()
	got, err := constructor(value)
	if err != nil {
		t.Fatalf("construct %q: %v", value, err)
	}
	return got
}
