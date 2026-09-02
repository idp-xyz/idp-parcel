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
	isolatedObjectScope = "SYN-PILOT-SCOPE/shipment-intake@v1"
	isolatedCapability  = "SYN-CAP/shipment-intake"
	isolatedFactKind    = "SYN-FACT/shipment-request"
	isolatedPilotScope  = "SYN-PILOT-SCOPE/shipment-intake@v1"
)

// Covers: ADR-0091 决定二与决定五——合成目录只交治理坐标，不替归属作答；坐标是装配
// 注入的合成常量而不从拟受理范围派生，因此换一份范围来问，交回的仍是同一组。
//
// 「换一份范围仍同一组」不是在描述实现，是在钉住隔离环境的前提：那里只有一份合成的
// 拟受理范围。哪天它要按范围分叉，分叉规则就是 `PAR-GOV-03..07` 的实例半边语义，
// 不该由一个合成目录替它拟。
func TestIsolatedDirectoryAnswersInjectedCoordinatesForAnyScope(t *testing.T) {
	directory, err := adapter.NewIsolatedGovernanceScopeDirectory(
		isolatedObjectScope, isolatedCapability, isolatedFactKind, isolatedPilotScope)
	if err != nil {
		t.Fatalf("构造合成目录：%v", err)
	}

	for name, scope := range map[string]psdomain.AdmissionScope{
		"甲范围": admissionScope(t),
		"乙范围": otherAdmissionScope(t),
	} {
		t.Run(name, func(t *testing.T) {
			governance, found, err := directory.FindGovernanceScope(context.Background(), scope)
			if err != nil {
				t.Fatalf("查治理坐标：%v", err)
			}
			if !found {
				t.Fatal("合成目录答查无此范围——隔离环境里那份合成范围总该有坐标")
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

// Covers: 残缺坐标在构造期就拒，不留到第一个请求。四维各缺一次而不是合成一条：
// 交回半组坐标的目录会让归属去登记册里查一行永远命不中的键，而那答出来的
// `权威未确定`与「压根没配目录」一模一样——两种缺席的恢复动作却不同。
func TestIsolatedDirectoryRefusesIncompleteCoordinates(t *testing.T) {
	for name, argument := range map[string][4]string{
		"缺对象范围":   {"", isolatedCapability, isolatedFactKind, isolatedPilotScope},
		"缺能力":     {isolatedObjectScope, "", isolatedFactKind, isolatedPilotScope},
		"缺事实类型":   {isolatedObjectScope, isolatedCapability, "", isolatedPilotScope},
		"缺试点范围版本": {isolatedObjectScope, isolatedCapability, isolatedFactKind, ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := adapter.NewIsolatedGovernanceScopeDirectory(
				argument[0], argument[1], argument[2], argument[3])
			if err == nil {
				t.Fatal("残缺坐标也装得起来——归属会去查一行永远命不中的键")
			}
		})
	}
}

// otherAdmissionScope 是另一份拟受理范围，与 admissionScope 逐维不同。
func otherAdmissionScope(t *testing.T) psdomain.AdmissionScope {
	t.Helper()
	reference, err := psdomain.NewAdmissionScopeReference("ADM-SCOPE-2")
	if err != nil {
		t.Fatalf("admission scope reference: %v", err)
	}
	digest, err := psdomain.NewAdmissionScopeDigest("sha256:adm-scope-2")
	if err != nil {
		t.Fatalf("admission scope digest: %v", err)
	}
	scope, err := psdomain.NewAdmissionScope(reference, digest)
	if err != nil {
		t.Fatalf("admission scope: %v", err)
	}
	return scope
}
