package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func customerAccountScope(t *testing.T, tenant, account string) domain.AdmissionScope {
	t.Helper()
	scope, err := domain.NewCustomerAccountAdmissionScope(
		mustValue(t, domain.NewTenantID, tenant),
		mustValue(t, domain.NewCustomerAccountID, account),
	)
	if err != nil {
		t.Fatalf("new customer account admission scope: %v", err)
	}
	return scope
}

// Covers: 按客户账户立的拟受理范围由同一处构造与认回——归属目录凭认回的租户决定答不答，认回的必须是立快照时那一组。
// 值里带 / 与 % 的两例钉住转义：不转义时「A/B」+「C」与「A」+「B/C」拼出同一个引用，目录会把一个租户的范围认成另一个。
func TestCustomerAccountAdmissionScopeRoundTrips(t *testing.T) {
	for name, pair := range map[string][2]string{
		"plain":            {"SYN-TENANT-01", "SYN-ACCOUNT-01"},
		"slash in tenant":  {"SYN-TENANT/01", "SYN-ACCOUNT-01"},
		"slash in account": {"SYN-TENANT", "01/SYN-ACCOUNT-01"},
		"percent escapes":  {"SYN-%2F-TENANT", "SYN-%25-ACCOUNT"},
	} {
		t.Run(name, func(t *testing.T) {
			scope := customerAccountScope(t, pair[0], pair[1])
			tenant, account, ok := scope.CustomerAccount()
			if !ok || tenant.String() != pair[0] || account.String() != pair[1] {
				t.Fatalf("认回 = (%q, %q, %v), want (%q, %q, true)", tenant, account, ok, pair[0], pair[1])
			}
		})
	}

	if customerAccountScope(t, "SYN-TENANT/01", "SYN-ACCOUNT-01") == customerAccountScope(t, "SYN-TENANT", "01/SYN-ACCOUNT-01") {
		t.Fatal("两组不同的（租户、客户账户）立出了同一份范围")
	}
	if got := customerAccountScope(t, "SYN-TENANT-01", "SYN-ACCOUNT-01").Reference().String(); got != "customer-account:SYN-TENANT-01/SYN-ACCOUNT-01" {
		t.Fatalf("引用 = %q", got)
	}
}

// Covers: 不是这一形状的范围认不出租户。引用开头对得上而摘要对不上的那一例最要紧：只看开头，一份手拼的范围就能冒认
// 某个租户。
func TestOnlyACustomerAccountScopeYieldsItsTenant(t *testing.T) {
	genuine := customerAccountScope(t, "SYN-TENANT-01", "SYN-ACCOUNT-01")
	for name, scope := range map[string]domain.AdmissionScope{
		"another shape":         mustScope(t, "SYN-ADM-SCOPE/shipment-intake", "sha256:syn-adm-scope-shipment-intake"),
		"reference, wrong hash": mustScope(t, genuine.Reference().String(), "sha256:forged"),
		"no separator":          mustScope(t, "customer-account:SYN-TENANT-01", genuine.Digest().String()),
		"blank account":         mustScope(t, "customer-account:SYN-TENANT-01/ ", genuine.Digest().String()),
		"zero value":            {},
	} {
		t.Run(name, func(t *testing.T) {
			if tenant, account, ok := scope.CustomerAccount(); ok {
				t.Fatalf("认出了 (%q, %q)", tenant, account)
			}
		})
	}
}

func TestCustomerAccountAdmissionScopeRefusesBlankParts(t *testing.T) {
	if _, err := domain.NewCustomerAccountAdmissionScope(domain.TenantID{}, mustValue(t, domain.NewCustomerAccountID, "SYN-ACCOUNT-01")); !errors.Is(err, domain.ErrInvalidAdmissionScope) {
		t.Fatalf("缺租户：err = %v, want ErrInvalidAdmissionScope", err)
	}
	if _, err := domain.NewCustomerAccountAdmissionScope(mustValue(t, domain.NewTenantID, "SYN-TENANT-01"), domain.CustomerAccountID{}); !errors.Is(err, domain.ErrInvalidAdmissionScope) {
		t.Fatalf("缺客户账户：err = %v, want ErrInvalidAdmissionScope", err)
	}
}
