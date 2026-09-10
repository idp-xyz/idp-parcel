package application_test

import (
	"context"
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 本文件是票 tf-segment-lifecycle-closure/14 完成判据 2 的两例：执行器的条件一格接的是**真适配器**
// （adapters/partycommercial.DeliveryConditionSource），PS 与 PC 两头各用读口替身。地点与时间窗两格照旧用本包既有的端口
// 替身——它们在夹具里是「已接」的，所以 PC 答引用时这一拍走到 OpenDispatchTask，任务的 Conditions 就是 PS 回指逐字。

// resolutionStub 扮 PS 的回指读口：只认 request-1 的成员（parcel-1 / parcel-2），其余按统一不可见答没有。
type resolutionStub struct {
	resolution psdomain.CommercialResolutionID
	members    map[string]bool
	calls      int
}

func (stub *resolutionStub) LoadCommercialResolutionReference(
	_ context.Context, _ psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CommercialResolutionID, bool, error) {
	stub.calls++
	if !stub.members[parcel.String()] {
		return psdomain.CommercialResolutionID{}, false, nil
	}
	return stub.resolution, true, nil
}

// conditionStub 扮 PC 的交付条件读口：对被问的回指答预设的引用（或没有）。
type conditionStub struct {
	reference pcdomain.DeliveryConditionReference
	declared  bool
	calls     int
}

func (stub *conditionStub) LoadDeliveryConditionReference(
	context.Context, pcdomain.TenantID, pcdomain.ResolutionID,
) (pcdomain.DeliveryConditionReference, bool, error) {
	stub.calls++
	return stub.reference, stub.declared, nil
}

// contractClosure 经 PC 公开领域门造一份唯一解析、采用了客户合同版本的闭包——PC 的交付条件引用只能由它产出，
// 回指也由它派生；不在测试里拼字面。
func contractClosure(t *testing.T) pcdomain.CommercialClosure {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("effective interval: %v", err)
	}
	tenant, err := pcdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	scope, err := pcdomain.NewCommercialScopeReference("scope-a")
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	objectID, err := pcdomain.NewCommercialObjectID("contract-1")
	if err != nil {
		t.Fatalf("object id: %v", err)
	}
	label, err := pcdomain.NewCommercialVersionLabel("v1")
	if err != nil {
		t.Fatalf("version label: %v", err)
	}
	digest, err := pcdomain.NewCommercialContentDigest("sha256:contract-1")
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID: tenant, Kind: pcdomain.CustomerContractObject, ObjectID: objectID, Version: label, Scope: scope, ContentDigest: digest, Effective: interval,
	})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	approvalRef, err := pcdomain.NewApprovalReference("approval-contract-1")
	if err != nil {
		t.Fatalf("approval reference: %v", err)
	}
	sourceRef, err := pcdomain.NewCommercialSourceReference("source-contract-1")
	if err != nil {
		t.Fatalf("source reference: %v", err)
	}
	basis, err := pcdomain.NewApprovalBasis(approvalRef, sourceRef, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("approval basis: %v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	registry := pcdomain.NewCommercialRegistry()
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	policyVersion, err := pcdomain.NewAnchorPolicyVersion("anchor-policy-v1")
	if err != nil {
		t.Fatalf("anchor policy: %v", err)
	}
	anchor, err := pcdomain.NewSelectionAnchor(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), policyVersion)
	if err != nil {
		t.Fatalf("anchor: %v", err)
	}
	customer, err := pcdomain.NewCustomerAccountID("customer-1")
	if err != nil {
		t.Fatalf("customer: %v", err)
	}
	legalEntity, err := pcdomain.NewLegalEntityReference("legal-1")
	if err != nil {
		t.Fatalf("legal entity: %v", err)
	}
	closure := pcdomain.ResolveCommercialClosure(registry, pcdomain.ClosureResolutionKey{
		TenantID: tenant, CustomerAccountID: customer, LegalEntityCandidate: legalEntity, Scope: scope,
		Purpose: pcdomain.AcceptanceControlPurpose, Anchor: anchor,
		RequiredBases: []pcdomain.CommercialObjectKind{pcdomain.CustomerContractObject},
	}, nil)
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("fixture closure did not resolve uniquely: %s", closure.Outcome())
	}
	return closure
}

// wireRealConditions 把执行器的条件一格换成真适配器 + 两头读口替身，其余两格照旧用端口替身。
func wireRealConditions(t *testing.T, fixture *dispatchTriggerFixture, closure pcdomain.CommercialClosure, declared bool) (*resolutionStub, *conditionStub) {
	t.Helper()
	resolution, err := psdomain.NewCommercialResolutionID(closure.ResolutionID().String())
	if err != nil {
		t.Fatalf("ps resolution: %v", err)
	}
	resolutions := &resolutionStub{resolution: resolution, members: map[string]bool{"parcel-1": true, "parcel-2": true}}
	conditions := &conditionStub{declared: declared}
	if declared {
		reference, present, err := pcdomain.DeliveryConditionReferenceFor(closure, false, true)
		if err != nil || !present {
			t.Fatalf("delivery condition reference: present=%v err=%v", present, err)
		}
		conditions.reference = reference
	}
	source, err := tfpartycommercial.NewDeliveryConditionSource(resolutions, conditions)
	if err != nil {
		t.Fatalf("new delivery condition source: %v", err)
	}
	fixture.deps.Conditions = source
	return resolutions, conditions
}

// Covers: 票面完成判据 2 前半——PC 答交付条件引用时执行器走到 OpenDispatchTask（地点与时间窗两格在夹具里已接），任务的
// Conditions 是 PS 回指逐字（ADR-0133 决定一：条件引用就是接受时固定的商业解析回指）。
func TestAPartyCommercialConditionReferenceFlowsIntoTheTaskVerbatim(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	closure := contractClosure(t)
	resolutions, conditions := wireRealConditions(t, fixture, closure, true)

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("触发：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("outcome = %q, want DISPATCH_TASK_FORMED（reason=%s missing=%v）", result.Outcome(), result.UndecidedReason(), result.Missing())
	}
	record, recorded := result.Record()
	if !recorded {
		t.Fatal("任务形成了却没交回记录")
	}
	if got := record.Task.Conditions().String(); got != closure.ResolutionID().String() || got == "" {
		t.Fatalf("任务条件 = %q, want PS 回指逐字 %q", got, closure.ResolutionID())
	}
	if resolutions.calls != 1 || conditions.calls != 1 {
		t.Fatalf("PS 被问 %d 次、PC 被问 %d 次, want 各 1", resolutions.calls, conditions.calls)
	}
}

// Covers: 票面完成判据 2 后半——PS 对集运单元答「没有」（对象无采用的合同）→ REQUIREMENT_MISSING / DELIVERY_CONDITION，任务
// 待形成、段与交接一行不动、PC 不被问、不补默认签收。「没有」是业务答案不是欠账：与 TestAnOwnerAnsweringMissingKeepsTheTaskUnformed
// 同一条纪律，不留续办引用（ADR-0114 决定三）。
func TestAConsolidationUnitWithoutAnAdoptedContractLeavesTheTaskUnformed(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "consolidation-unit-7", "FINAL_DELIVERY", handoverJudgedTime)
	resolutions, conditions := wireRealConditions(t, fixture, contractClosure(t), true)
	segmentSaves, segmentJoins := fixture.handovers.segments.saves, fixture.handovers.segments.joins
	handoverSaves := fixture.handovers.handovers.saves

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "consolidation-unit-7"))
	if err != nil {
		t.Fatalf("触发：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchRequirementMissing {
		t.Fatalf("outcome = %q, want REQUIREMENT_MISSING（reason=%s）", result.Outcome(), result.UndecidedReason())
	}
	if missing := result.Missing(); len(missing) != 1 || missing[0] != application.DeliveryConditionRequirement {
		t.Fatalf("missing = %v, want [DELIVERY_CONDITION]", missing)
	}
	if result.ContinuationReference() != "" {
		t.Fatal("所有者答「没有」是业务答案不是欠账，不该留续办引用")
	}
	if fixture.tasks.saves != 0 {
		t.Fatalf("PS 说对象无采用的合同却开了任务——填了默认条件：saves=%d", fixture.tasks.saves)
	}
	if fixture.handovers.segments.saves != segmentSaves || fixture.handovers.segments.joins != segmentJoins || fixture.handovers.handovers.saves != handoverSaves {
		t.Fatal("这一拍改动了段或交接——执行器对段登记册只读，不回滚交接")
	}
	if resolutions.calls != 1 || conditions.calls != 0 {
		t.Fatalf("PS 被问 %d 次、PC 被问 %d 次, want PS 1 / PC 0——PS 说没有就不进第二段", resolutions.calls, conditions.calls)
	}
}
