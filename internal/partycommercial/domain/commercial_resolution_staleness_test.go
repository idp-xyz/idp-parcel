package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件此前还有六条对单依据形态提交前重解（ValidateBeforeDecision）的用例：同范围新增候选即 STALE、
// 权威视图未变原结果可用、重解后换新解析标识、别的范围不影响本范围、权威不可读停未决、只有唯一解析
// 能被重解。那一版 2026-09-08 随票 wiring-baseline-remainder/05 删去（闭包形态 ValidateClosureBeforeDecision
// 是唯一生产路径，同一组规则的用例在 reference_closure_test.go），只为它写的用例随它走。

// Covers: party-commercial CONTEXT 第一阶段返回「解析标识、选择锚点、采用版本、有效区间
// 和当前修订标识」— 修订标识是返回契约的一部分，不是可选附加。
func TestResolutionCarriesTheAuthorityViewRevision(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

	revision, present := result.ViewRevision()
	if !present || revision.String() == "" {
		t.Fatal("a resolution carries no authority view revision")
	}
	if registry.ViewRevision(commercialValue(t, domain.NewTenantID, "tenant-1"), commercialValue(t, domain.NewCommercialScopeReference, "scope-a")) != revision {
		t.Fatal("the resolution echoed a revision the registry does not report")
	}
}
