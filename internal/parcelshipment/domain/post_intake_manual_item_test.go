package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: ADR-0341 决定一——请求表里系统不执行的几项才成待人工处理项：今天只有运输履约的两个「另起旅程」动作由系统执行，其余全人工；继续没有请求项。
func TestOnlyTheRequestsTheSystemDoesNotExecuteBecomeManualItems(t *testing.T) {
	want := map[domain.PostIntakeDispositionDirection]int{
		domain.PostIntakeContinue: 0, domain.PostIntakeIntercept: 2, domain.PostIntakeRedirect: 1,
		domain.PostIntakeReturn: 2, domain.PostIntakeTerminateService: 3,
	}
	for direction, count := range want {
		manual := direction.ManualOwnerRequests()
		if len(manual) != count {
			t.Errorf("%s 的人工项 %d, want %d", direction, len(manual), count)
		}
		for _, request := range manual {
			if request.Action.ExecutedBySystem() {
				t.Errorf("%s 的人工项里有系统执行的动作 %s", direction, request.Action)
			}
		}
	}
}

// Covers: ADR-0341 决定二——登记的键只收封闭表里需要人工处理的项；结果、说明、操作者缺一即拒。
func TestAManualItemKeyAndResultNeedTheirParts(t *testing.T) {
	tenant, _ := domain.NewTenantID("t")
	parcel, _ := domain.NewDeclaredParcelID("p")
	key := domain.ManualItemKey{Tenant: tenant, Parcel: parcel, DecisionVersion: 1,
		Owner: domain.DispositionOwnerNodeOperations, Action: domain.DispositionActionHoldAtControl}
	if err := domain.ManualItemKeyError(key); err != nil {
		t.Fatalf("合法键被拒：%v", err)
	}
	key.Action = domain.DispositionActionStartReturnJourney
	if domain.ManualItemKeyError(key) == nil {
		t.Error("系统执行的动作被当成人工项")
	}
	key.Action, key.DecisionVersion = domain.DispositionActionHoldAtControl, 0
	if domain.ManualItemKeyError(key) == nil {
		t.Error("决定版本 0 被接受")
	}
	if _, err := domain.ParseManualItemOutcome("DONE"); err == nil {
		t.Error("封闭集外的结果被接受")
	}
}
