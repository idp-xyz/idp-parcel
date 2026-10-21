package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// customerPlace 是场外揽收的接货位置：客户场所一类，不是物流节点码，也不在任何计划节点上。
const customerPlace = "customer-warehouse-2"

// Covers: ADR-0249 决定二与 CONTEXT「当前可控节点」——场外揽收是一类立得起复核触发的控制依据（有效网络收寄的来源），
// 却不建立当前可控节点；节点收寄与交进接收节点的权威运输交接才建立。揽收地点原样随触发保全。
func TestAnOffsitePickupTriggersAReassessmentWithoutEstablishingAControllableNode(t *testing.T) {
	spec := triggerSpec(t, domain.OffsitePickupControl)
	spec.Location = mustValue(t, domain.NewActualLocationReference, customerPlace)
	trigger, err := domain.NewReassessmentTrigger(spec)
	if err != nil {
		t.Fatalf("new pickup trigger: %v", err)
	}
	if trigger.Location().String() != customerPlace {
		t.Fatalf("location = %q; 揽收地点要原样随触发保全", trigger.Location())
	}
	if trigger.Control().EstablishesControllableNode() {
		t.Fatal("场外揽收建立了当前可控节点")
	}
	for _, control := range []domain.ControlEvidenceKind{domain.NodeIntakeControl, domain.TransportHandoverControl} {
		if !control.EstablishesControllableNode() {
			t.Fatalf("%s 不建立当前可控节点", control)
		}
	}
	if domain.SourceHint.EstablishesControllableNode() || domain.ControlEvidenceKindInvalid.EstablishesControllableNode() {
		t.Fatal("线索或空值建立了当前可控节点")
	}
	if domain.OffsitePickupControl.String() == domain.TransportHandoverControl.String() {
		t.Fatal("场外揽收与权威运输交接共用一个码值，复核依据分不出两者")
	}
}

// Covers: AT-NR-034 的领域半边（ADR-0249 决定一）——揽收触发的适用性复核不拿揽收地点比计划节点：地点不在计划上
// 也仍适用；只看选中候选的硬约束，被有效限制淘汰即失效、依据点名限制来源而不是地点；限制状态未知照旧交回缺口。
func TestAPickupReviewChecksHardConstraintsOnlyAndNeverComparesThePlaceWithPlanNodes(t *testing.T) {
	plan := reviewedPlan(t)
	place := mustValue(t, domain.NewActualLocationReference, customerPlace)

	kept, err := domain.ReviewPlanApplicability(plan, domain.OffsitePickupControl, place, nil)
	if err != nil {
		t.Fatalf("review pickup: %v", err)
	}
	if kept.Outcome() != domain.PlanStillApplicable {
		basis, _ := kept.LapseBasis()
		t.Fatalf("outcome = %q basis = %q, want STILL_APPLICABLE——揽收地点不是节点，不与计划节点比", kept.Outcome(), basis)
	}

	restricted, err := domain.ReviewPlanApplicability(plan, domain.OffsitePickupControl, place,
		[]domain.HardConstraintFinding{restrictionOn(t, "cand-1")})
	if err != nil {
		t.Fatalf("review restricted pickup: %v", err)
	}
	if restricted.Outcome() != domain.PlanNoLongerApplicable {
		t.Fatalf("outcome = %q, want NO_LONGER_APPLICABLE", restricted.Outcome())
	}
	if basis, _ := restricted.LapseBasis(); basis.String() != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-9" {
		t.Fatalf("basis = %q; 揽收复核的失效依据只能是硬约束", basis)
	}

	unclear, err := domain.ReviewPlanApplicability(plan, domain.OffsitePickupControl, place,
		[]domain.HardConstraintFinding{unknownConstraintOn(t, "cand-1")})
	if err != nil {
		t.Fatalf("review unclear pickup: %v", err)
	}
	if unclear.Outcome() != domain.PlanReviewUndecided {
		t.Fatalf("outcome = %q, want REVIEW_UNDECIDED", unclear.Outcome())
	}

	// 节点上的复核照旧比节点：同一个不在计划上的位置在节点收寄下仍判失效。
	atNode, err := domain.ReviewPlanApplicability(plan, domain.NodeIntakeControl, place, nil)
	if err != nil {
		t.Fatalf("review node intake: %v", err)
	}
	if atNode.Outcome() != domain.PlanNoLongerApplicable {
		t.Fatalf("outcome = %q; 节点收寄不在计划节点上必须失效", atNode.Outcome())
	}

	for _, control := range []domain.ControlEvidenceKind{domain.ControlEvidenceKindInvalid, domain.SourceHint} {
		if _, err := domain.ReviewPlanApplicability(plan, control, place, nil); !errors.Is(err, domain.ErrInvalidPlanReview) {
			t.Fatalf("control %d: err = %v, want ErrInvalidPlanReview", control, err)
		}
	}
}

// Covers: ADR-0249 决定二「揽收复核的已执行前缀为空，全部计划段都在未执行范围里」——与扫描线索那一格分开：扫描是
// 不知道包裹在哪、切不出前缀；揽收是确知包裹还没进任何节点、什么都没执行。剩余段数是整条计划。
func TestAPickupLeavesTheWholePlanUnexecuted(t *testing.T) {
	nodes := []string{"node-origin", "node-hub", "node-destination"}
	prefix := domain.JudgeExecutedPrefix(nodes, domain.OffsitePickupControl, customerPlace)
	if prefix.Grade() != domain.ExecutedPrefixNothingExecuted {
		t.Fatalf("grade = %d, want ExecutedPrefixNothingExecuted", prefix.Grade())
	}
	if len(prefix.Prefix()) != 0 {
		t.Fatalf("prefix = %v; 揽收不切前缀", prefix.Prefix())
	}
	if got := prefix.Remaining(); len(got) != 3 || prefix.RemainingSegments() != 2 {
		t.Fatalf("remaining = %v segments %d, want the whole plan", got, prefix.RemainingSegments())
	}
	if !prefix.OnlyUnexecutedAffected() {
		t.Fatal("揽收复核把已执行前缀算成受影响")
	}

	// 揽收地点碰巧与计划节点同名也不切前缀：揽收不在节点上。
	same := domain.JudgeExecutedPrefix(nodes, domain.OffsitePickupControl, "node-origin")
	if same.Grade() != domain.ExecutedPrefixNothingExecuted || len(same.Prefix()) != 0 {
		t.Fatalf("grade = %d prefix = %v; 揽收按地点同名切了前缀", same.Grade(), same.Prefix())
	}

	if domain.JudgeExecutedPrefix(nodes, domain.SourceHint, "node-origin").OnlyUnexecutedAffected() {
		t.Fatal("扫描线索切不出前缀，却答了只改未执行部分")
	}
	if !domain.JudgeExecutedPrefix(nodes, domain.NodeIntakeControl, "node-hub").OnlyUnexecutedAffected() {
		t.Fatal("节点收寄切出前缀之后应只改未执行部分")
	}
}
