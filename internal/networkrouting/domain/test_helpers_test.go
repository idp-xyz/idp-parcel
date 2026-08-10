package domain_test

import "testing"

type stringValue interface {
	String() string
}

func mustValue[T stringValue](t *testing.T, constructor func(string) (T, error), value string) T {
	t.Helper()
	got, err := constructor(value)
	if err != nil {
		t.Fatalf("construct %q: %v", value, err)
	}
	return got
}
