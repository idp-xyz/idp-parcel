package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func creditPolicy(t *testing.T, objectID, chargeType string, limitMinor int64) domain.CreditPolicy {
	t.Helper()
	live, err := registerable(t, domain.CreditPolicyObject, objectID, "v1", "sha256:"+objectID).
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	policy, err := domain.NewCreditPolicy(
		live,
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		commercialValue(t, domain.NewChargeTypeReference, chargeType),
		limitMinor,
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new credit policy: %v", err)
	}
	return policy
}

func creditQuery(t *testing.T, chargeType string) domain.CreditPolicyQuery {
	t.Helper()
	query, err := domain.NewCreditPolicyQuery(
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		commercialValue(t, domain.NewChargeTypeReference, chargeType),
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new credit policy query: %v", err)
	}
	return query
}

// Covers: CONTEXT「政策只提供业务判断依据，不直接修改结算余额或形成调整金额」— 解析产出
// 的是依据，不是对余额的作用。
func TestCreditPolicyYieldsABasisNotAnEffect(t *testing.T) {
	policies := []domain.CreditPolicy{creditPolicy(t, "credit-1", "charge-freight", 500000)}

	basis, err := domain.ResolveCreditPolicy(policies, creditQuery(t, "charge-freight"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if basis.AuthorizedLimitMinor() != 500000 {
		t.Fatalf("limit = %d, want 500000", basis.AuthorizedLimitMinor())
	}
	if basis.PolicyVersion().ObjectID().String() != "credit-1" {
		t.Fatal("the basis does not name the policy version it came from")
	}

	basisType := reflect.TypeOf(domain.CreditBasis{})
	forbidden := []string{"balance", "adjustment", "posted", "applied", "deducted", "settled"}
	for index := 0; index < basisType.NumField(); index++ {
		name := strings.ToLower(basisType.Field(index).Name)
		for _, word := range forbidden {
			if strings.Contains(name, word) {
				t.Fatalf("CreditBasis carries %s, which lets this context act on a settlement balance",
					basisType.Field(index).Name)
			}
		}
	}
}

// Covers: CONTEXT「按责任法人、业务角色、费用类型、金额或比例形成版本」— 四维匹配，
// 任一不同即不适用。
func TestCreditPolicyMatchesOnAllItsDimensions(t *testing.T) {
	policies := []domain.CreditPolicy{creditPolicy(t, "credit-1", "charge-freight", 500000)}

	t.Run("another charge type does not match", func(t *testing.T) {
		if _, err := domain.ResolveCreditPolicy(policies, creditQuery(t, "charge-surcharge")); !errors.Is(err, domain.ErrNoApplicableCreditPolicy) {
			t.Fatalf("error = %v; a policy answered for another charge type", err)
		}
	})

	t.Run("another authority level does not match", func(t *testing.T) {
		query, err := domain.NewCreditPolicyQuery(
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewAuthorityLevel, "level-clerk"),
			commercialValue(t, domain.NewChargeTypeReference, "charge-freight"),
			time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("new query: %v", err)
		}
		if _, err := domain.ResolveCreditPolicy(policies, query); !errors.Is(err, domain.ErrNoApplicableCreditPolicy) {
			t.Fatalf("error = %v; a policy answered across authority levels", err)
		}
	})

	t.Run("outside the interval does not match", func(t *testing.T) {
		query, err := domain.NewCreditPolicyQuery(
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
			commercialValue(t, domain.NewChargeTypeReference, "charge-freight"),
			time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("new query: %v", err)
		}
		if _, err := domain.ResolveCreditPolicy(policies, query); !errors.Is(err, domain.ErrNoApplicableCreditPolicy) {
			t.Fatalf("error = %v; an expired policy still answered", err)
		}
	})
}

// Covers: CONTEXT — 无适用信用政策不得读成「无限额」或「零额度」，重叠不得任选一条。
func TestAbsentOrOverlappingCreditPoliciesAreNeitherUnlimitedNorZero(t *testing.T) {
	t.Run("no policy is no applicable basis", func(t *testing.T) {
		basis, err := domain.ResolveCreditPolicy(nil, creditQuery(t, "charge-freight"))
		if !errors.Is(err, domain.ErrNoApplicableCreditPolicy) {
			t.Fatalf("error = %v, want ErrNoApplicableCreditPolicy", err)
		}
		if basis.AuthorizedLimitMinor() != 0 || basis.Applicable() {
			t.Fatal("an absent policy produced a usable basis; zero must not read as a granted limit")
		}
	})

	t.Run("overlapping policies conflict", func(t *testing.T) {
		policies := []domain.CreditPolicy{
			creditPolicy(t, "credit-1", "charge-freight", 500000),
			creditPolicy(t, "credit-2", "charge-freight", 900000),
		}
		if _, err := domain.ResolveCreditPolicy(policies, creditQuery(t, "charge-freight")); !errors.Is(err, domain.ErrCreditPolicyConflict) {
			t.Fatalf("error = %v; overlapping credit policies picked the larger limit", err)
		}
	})
}

// Covers: CONTEXT — 授信额度是政策携带的取值，负额度无意义；内容挂在当前可用的信用政策
// 版本上。
func TestCreditPolicyNeedsAUsableVersionAndANonNegativeLimit(t *testing.T) {
	live, err := registerable(t, domain.CreditPolicyObject, "credit-x", "v1", "sha256:x").
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}

	t.Run("refuses a negative limit", func(t *testing.T) {
		if _, err := domain.NewCreditPolicy(
			live,
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
			commercialValue(t, domain.NewChargeTypeReference, "charge-freight"),
			-1,
			mustInterval(t),
		); !errors.Is(err, domain.ErrInvalidCreditPolicy) {
			t.Fatalf("error = %v, want ErrInvalidCreditPolicy", err)
		}
	})

	t.Run("refuses another object kind", func(t *testing.T) {
		if _, err := domain.NewCreditPolicy(
			contractVersion(t, "contract-5"),
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
			commercialValue(t, domain.NewChargeTypeReference, "charge-freight"),
			500000,
			mustInterval(t),
		); !errors.Is(err, domain.ErrInvalidCreditPolicy) {
			t.Fatalf("error = %v, want ErrInvalidCreditPolicy", err)
		}
	})
}
