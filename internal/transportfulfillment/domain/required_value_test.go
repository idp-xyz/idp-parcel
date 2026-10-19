package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 tf-face-algebra/01 修复方向 2——必备值含 NUL 就拒（登记册 text 列存不下）；前后带空白的非空值照旧原样收。
func TestRequiredValuesRefuseNUL(t *testing.T) {
	constructors := map[string]func(string) error{
		"装载分配版本": func(v string) error { _, err := domain.NewLoadAssignmentVersion(v); return err },
		"载运对象":   func(v string) error { _, err := domain.NewCarriedObjectReference(v); return err },
		"移动地点":   func(v string) error { _, err := domain.NewMovementLocationReference(v); return err },
		"尝试证据":   func(v string) error { _, err := domain.NewAttemptEvidenceReference(v); return err },
	}
	for name, construct := range constructors {
		for _, value := range []string{"SYN-REF\x00", "\x00SYN-REF", "SYN-\x00-REF"} {
			if err := construct(value); !errors.Is(err, domain.ErrValueContainsNUL) {
				t.Fatalf("%s %q：err = %v，期望 ErrValueContainsNUL", name, value, err)
			}
		}
		if err := construct("\x00"); err == nil {
			t.Fatalf("%s 只含 NUL：期望拒", name)
		}
	}
	padded, err := domain.NewLoadAssignmentVersion(" SYN-LA-V1 ")
	if err != nil || padded.String() != " SYN-LA-V1 " {
		t.Fatalf("前后带空白的非空值 = %q, %v，期望原样收", padded.String(), err)
	}
}
