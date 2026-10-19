package pilotgovernance_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/pilotgovernance"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 编译期钉住合成目录确实是目录：形状不合当场编不过，靠装配时才发现的表现是一个看不出
// 原因的`权威未确定`（与本包三个窄口的同款钉法一致）。
var _ adapter.GovernanceScopeDirectory = adapter.IsolatedGovernanceScopeDirectory{}

const (
	isolatedTenant      = "SYN-TENANT-01"
	isolatedObjectScope = "SYN-PILOT-SCOPE/shipment-intake@v1"
	isolatedCapability  = "SYN-CAP/shipment-intake"
	isolatedFactKind    = "SYN-FACT/shipment-request"
	isolatedPilotScope  = "SYN-PILOT-SCOPE/shipment-intake@v1"
)

func isolatedDirectory(t *testing.T) adapter.IsolatedGovernanceScopeDirectory {
	t.Helper()
	directory, err := adapter.NewIsolatedGovernanceScopeDirectory(
		isolatedTenant, isolatedObjectScope, isolatedCapability, isolatedFactKind, isolatedPilotScope)
	if err != nil {
		t.Fatalf("构造合成目录：%v", err)
	}
	return directory
}

// Covers: ADR-0091 决定二与决定五——合成目录只交治理坐标，不替归属作答；坐标是装配
// 注入的合成常量而不从拟受理范围派生，因此开关租户换一个客户账户来问，交回的仍是同一组。
//
// 「换一份范围仍同一组」不是在描述实现，是在钉住隔离环境的前提：那里只有一组合成坐标。
// 哪天它要按范围分叉，分叉规则就是 `PAR-GOV-03..07` 的实例半边语义，不该由一个合成目录替它拟。
func TestIsolatedDirectoryAnswersInjectedCoordinatesForTheSwitchTenant(t *testing.T) {
	directory := isolatedDirectory(t)

	for name, scope := range map[string]psdomain.AdmissionScope{
		"甲账户": customerAccountScope(t, isolatedTenant, "SYN-ACCOUNT-01"),
		"乙账户": customerAccountScope(t, isolatedTenant, "SYN-ACCOUNT-02"),
	} {
		t.Run(name, func(t *testing.T) {
			governance, found, err := directory.FindGovernanceScope(context.Background(), scope)
			if err != nil {
				t.Fatalf("查治理坐标：%v", err)
			}
			if !found {
				t.Fatal("开关租户的范围答查无此范围——隔离环境里它总该有坐标")
			}
			if governance.ObjectScope != isolatedObjectScope {
				t.Fatalf("对象范围 = %q, want %q", governance.ObjectScope, isolatedObjectScope)
			}
			if governance.Capability != isolatedCapability {
				t.Fatalf("能力 = %q, want %q", governance.Capability, isolatedCapability)
			}
			if governance.FactKind != isolatedFactKind {
				t.Fatalf("事实类型 = %q, want %q", governance.FactKind, isolatedFactKind)
			}
			if governance.PilotScope.String() != isolatedPilotScope {
				t.Fatalf("试点范围版本 = %q, want %q", governance.PilotScope.String(), isolatedPilotScope)
			}
		})
	}
}

// Covers: ADR-0091 决定六「也不因它出现而获得任何真实租户」——提交口挂的是真渠道，任何租户的委托都走得到归属这一步；
// 开关租户之外的范围答显式未配置，归属据此答`权威未确定`。另一个合成租户也在拦的一侧：目录认的是开关的那个值，不是前缀。
// 认不出租户的范围同样不答。
func TestIsolatedDirectoryAnswersNothingOutsideTheSwitchTenant(t *testing.T) {
	directory := isolatedDirectory(t)

	for name, scope := range map[string]psdomain.AdmissionScope{
		"另一个租户":     customerAccountScope(t, "TENANT-PROD-1", "ACCOUNT-PROD-1"),
		"另一个合成租户":   customerAccountScope(t, "SYN-TENANT-02", "SYN-ACCOUNT-01"),
		"认不出租户的范围":  admissionScope(t),
		"冒认开关租户的引用": forgedSwitchTenantScope(t),
	} {
		t.Run(name, func(t *testing.T) {
			governance, found, err := directory.FindGovernanceScope(context.Background(), scope)
			if err != nil {
				t.Fatalf("查治理坐标：%v", err)
			}
			if found || governance != (adapter.GovernanceScope{}) {
				t.Fatalf("交出了坐标 %+v——非开关租户会凭合成治理区间落成`已提交`", governance)
			}
		})
	}
}

// Covers: 残缺输入在构造期就拒，不留到第一个请求。各缺一次而不是合成一条：
// 交回半组坐标的目录会让归属去登记册里查一行永远命不中的键，而那答出来的
// `权威未确定`与「压根没配目录」一模一样——两种缺席的恢复动作却不同。缺租户的目录
// 则认不出任何一份范围属于谁。
func TestIsolatedDirectoryRefusesIncompleteCoordinates(t *testing.T) {
	for name, argument := range map[string][5]string{
		"缺租户":     {"", isolatedObjectScope, isolatedCapability, isolatedFactKind, isolatedPilotScope},
		"缺对象范围":   {isolatedTenant, "", isolatedCapability, isolatedFactKind, isolatedPilotScope},
		"缺能力":     {isolatedTenant, isolatedObjectScope, "", isolatedFactKind, isolatedPilotScope},
		"缺事实类型":   {isolatedTenant, isolatedObjectScope, isolatedCapability, "", isolatedPilotScope},
		"缺试点范围版本": {isolatedTenant, isolatedObjectScope, isolatedCapability, isolatedFactKind, ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := adapter.NewIsolatedGovernanceScopeDirectory(
				argument[0], argument[1], argument[2], argument[3], argument[4])
			if err == nil {
				t.Fatal("残缺输入也装得起来")
			}
		})
	}
}

func customerAccountScope(t *testing.T, tenant, account string) psdomain.AdmissionScope {
	t.Helper()
	tenantID, err := psdomain.NewTenantID(tenant)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	accountID, err := psdomain.NewCustomerAccountID(account)
	if err != nil {
		t.Fatalf("customer account: %v", err)
	}
	scope, err := psdomain.NewCustomerAccountAdmissionScope(tenantID, accountID)
	if err != nil {
		t.Fatalf("customer account admission scope: %v", err)
	}
	return scope
}

// forgedSwitchTenantScope 的引用与开关租户那一份逐字相同，摘要却不是按那一形算的。
func forgedSwitchTenantScope(t *testing.T) psdomain.AdmissionScope {
	t.Helper()
	genuine := customerAccountScope(t, isolatedTenant, "SYN-ACCOUNT-01")
	digest, err := psdomain.NewAdmissionScopeDigest("sha256:forged")
	if err != nil {
		t.Fatalf("admission scope digest: %v", err)
	}
	scope, err := psdomain.NewAdmissionScope(genuine.Reference(), digest)
	if err != nil {
		t.Fatalf("admission scope: %v", err)
	}
	return scope
}
