package accessidentity_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

var (
	mappingDay   = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	loadDecision = accessidentity.AdmissionRequirement{
		Capability: accessidentity.CapabilityOperationDecision.String(),
		FactKind:   accessidentity.DecisionLoadAssignment.String(),
	}
)

func mappingInterval(t *testing.T, startsAt, endsAt time.Time) accessidentity.EffectiveInterval {
	t.Helper()
	interval, err := accessidentity.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("生效区间：%v", err)
	}
	return interval
}

func scopeMapping(t *testing.T, id, scope string, startsAt, endsAt time.Time) accessidentity.AdmissionScopeMapping {
	t.Helper()
	mapping, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", id, loadDecision, scope,
		mappingInterval(t, startsAt, endsAt), "SYN-BASIS-MAPPING")
	if err != nil {
		t.Fatalf("对照：%v", err)
	}
	return mapping
}

func recordedMapping(t *testing.T, mapping accessidentity.AdmissionScopeMapping, endedAt time.Time) accessidentity.RecordedAdmissionScopeMapping {
	t.Helper()
	var end *accessidentity.AdmissionScopeMappingEnd
	if !endedAt.IsZero() {
		ending, err := accessidentity.NewAdmissionScopeMappingEnd(mapping.TenantID(), mapping.MappingID(), endedAt, "SYN-BASIS-END")
		if err != nil {
			t.Fatalf("结束：%v", err)
		}
		end = &ending
	}
	recorded, err := accessidentity.NewRecordedAdmissionScopeMapping(mapping, end)
	if err != nil {
		t.Fatalf("册上的一行：%v", err)
	}
	return recorded
}

// Covers: ADR-0180 决定四——对照只认两族认证方会提的准入要求；集合外的能力或事实类型、还没定词的能力面，都在登记时拒，
// 不让拼错的一行与「没登」答同一张脸。缺件一律拒。
func TestAnAdmissionScopeMappingTakesOnlyRequirementsAFaceAsksFor(t *testing.T) {
	interval := mappingInterval(t, mappingDay, time.Time{})
	for name, requirement := range map[string]accessidentity.AdmissionRequirement{
		"operation decision with a kind": loadDecision,
		"external fact with a type": {
			Capability: accessidentity.ExternalFactAdmissionCapability,
			FactKind:   accessidentity.FactExternalFunds.String(),
		},
		// 商品代录的能力词与事实类型词都是决定字面量（ADR-0384 决定八）。认不下这一对，对照就登不进去，代录永远答不在准入范围。
		"goods recorded on behalf": {
			Capability: accessidentity.DecisionGoodsRecordOnBehalf.String(),
			FactKind:   accessidentity.DecisionGoodsRecordOnBehalf.String(),
		},
		"inbound pre-advice recorded on behalf": {
			Capability: accessidentity.DecisionInboundPreAdviceRecordOnBehalf.String(),
			FactKind:   accessidentity.DecisionInboundPreAdviceRecordOnBehalf.String(),
		},
		"inbound pre-advice close": {
			Capability: accessidentity.DecisionInboundPreAdviceClose.String(),
			FactKind:   accessidentity.DecisionInboundPreAdviceClose.String(),
		},
		"receipt discrepancy conclusion on behalf": {
			Capability: accessidentity.DecisionReceiptDiscrepancyRecordOnBehalf.String(),
			FactKind:   accessidentity.DecisionReceiptDiscrepancyRecordOnBehalf.String(),
		},
		// 货主写口两格都是该口字面量，不用 CUSTOMER_COMMAND（ADR-0384 决定八）。
		"goods maintenance": {
			Capability: accessidentity.CustomerCapabilityGoodsMaintenance.String(),
			FactKind:   accessidentity.CustomerCapabilityGoodsMaintenance.String(),
		},
		"inbound pre-advice submission": {
			Capability: accessidentity.CustomerCapabilityInboundPreAdviceSubmission.String(),
			FactKind:   accessidentity.CustomerCapabilityInboundPreAdviceSubmission.String(),
		},
		"inbound pre-advice arrival notice": {
			Capability: accessidentity.CustomerCapabilityInboundPreAdviceArrivalNotice.String(),
			FactKind:   accessidentity.CustomerCapabilityInboundPreAdviceArrivalNotice.String(),
		},
		"inbound pre-advice withdrawal": {
			Capability: accessidentity.CustomerCapabilityInboundPreAdviceWithdrawal.String(),
			FactKind:   accessidentity.CustomerCapabilityInboundPreAdviceWithdrawal.String(),
		},
		"receipt discrepancy conclusion": {
			Capability: accessidentity.CustomerCapabilityReceiptDiscrepancyConclusion.String(),
			FactKind:   accessidentity.CustomerCapabilityReceiptDiscrepancyConclusion.String(),
		},
		"warehouse measurement work fact": {
			Capability: accessidentity.CapabilityWorkFactRegistration.String(),
			FactKind:   accessidentity.WorkFactWarehouseMeasurement.String(),
		},
		"unclaimed arrival work fact": {
			Capability: accessidentity.CapabilityWorkFactRegistration.String(),
			FactKind:   accessidentity.WorkFactUnclaimedArrival.String(),
		},
		"unclaimed arrival claim": {
			Capability: accessidentity.DecisionUnclaimedArrivalClaim.String(),
			FactKind:   accessidentity.DecisionUnclaimedArrivalClaim.String(),
		},
		"unclaimed arrival disposition": {
			Capability: accessidentity.DecisionUnclaimedArrivalDisposition.String(),
			FactKind:   accessidentity.DecisionUnclaimedArrivalDisposition.String(),
		},
		"stock import submission": {
			Capability: accessidentity.DecisionStockImportSubmission.String(),
			FactKind:   accessidentity.DecisionStockImportSubmission.String(),
		},
		"wave release": {
			Capability: accessidentity.DecisionWaveRelease.String(),
			FactKind:   accessidentity.DecisionWaveRelease.String(),
		},
		"outbound request on behalf": {
			Capability: accessidentity.DecisionOutboundRequestRecordOnBehalf.String(),
			FactKind:   accessidentity.DecisionOutboundRequestRecordOnBehalf.String(),
		},
		"stock adjustment approval": {
			Capability: accessidentity.DecisionStockAdjustmentApproval.String(),
			FactKind:   accessidentity.DecisionStockAdjustmentApproval.String(),
		},
		"customer outbound submission": {
			Capability: accessidentity.CustomerCapabilityOutboundRequestSubmission.String(),
			FactKind:   accessidentity.CustomerCapabilityOutboundRequestSubmission.String(),
		},
		"customer return pre-advice": {
			Capability: accessidentity.CustomerCapabilityReturnPreAdviceSubmission.String(),
			FactKind:   accessidentity.CustomerCapabilityReturnPreAdviceSubmission.String(),
		},
	} {
		if _, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-1", requirement, "SYN-PILOT-SCOPE/x@v1", interval, "SYN-BASIS"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, requirement := range map[string]accessidentity.AdmissionRequirement{
		"synthetic capability word":           {Capability: "SYN-CAP/operation-decision", FactKind: accessidentity.DecisionLoadAssignment.String()},
		"decision capability, fact type":      {Capability: accessidentity.CapabilityOperationDecision.String(), FactKind: accessidentity.FactExternalFunds.String()},
		"external fact, decision kind":        {Capability: accessidentity.ExternalFactAdmissionCapability, FactKind: accessidentity.DecisionLoadAssignment.String()},
		"face without admission words":        {Capability: accessidentity.CapabilityWorkFactRegistration.String(), FactKind: "NODE_RECEPTION"},
		"lower-cased word":                    {Capability: "operation_decision", FactKind: accessidentity.DecisionLoadAssignment.String()},
		"goods capability, other kind":        {Capability: accessidentity.DecisionGoodsRecordOnBehalf.String(), FactKind: accessidentity.DecisionManualMeasurementEntry.String()},
		"pre-advice capability, other kind":   {Capability: accessidentity.DecisionInboundPreAdviceRecordOnBehalf.String(), FactKind: accessidentity.DecisionGoodsRecordOnBehalf.String()},
		"close capability, other kind":        {Capability: accessidentity.DecisionInboundPreAdviceClose.String(), FactKind: accessidentity.DecisionGoodsRecordOnBehalf.String()},
		"claim capability, other kind":        {Capability: accessidentity.DecisionUnclaimedArrivalClaim.String(), FactKind: accessidentity.DecisionUnclaimedArrivalDisposition.String()},
		"dispose capability, other kind":      {Capability: accessidentity.DecisionUnclaimedArrivalDisposition.String(), FactKind: accessidentity.DecisionUnclaimedArrivalClaim.String()},
		"stock import capability, other kind": {Capability: accessidentity.DecisionStockImportSubmission.String(), FactKind: accessidentity.DecisionUnclaimedArrivalClaim.String()},
		"customer write, other fact":          {Capability: accessidentity.CustomerCapabilityInboundPreAdviceSubmission.String(), FactKind: accessidentity.CustomerCapabilityInboundPreAdviceWithdrawal.String()},
		"customer write under command word":   {Capability: accessidentity.CustomerCommandAdmissionCapability, FactKind: accessidentity.CustomerCapabilityInboundPreAdviceSubmission.String()},
		"goods write under command word":      {Capability: accessidentity.CustomerCommandAdmissionCapability, FactKind: accessidentity.CustomerCapabilityGoodsMaintenance.String()},
		"goods write, other fact":             {Capability: accessidentity.CustomerCapabilityGoodsMaintenance.String(), FactKind: accessidentity.CustomerCapabilityInboundPreAdviceSubmission.String()},
		"warehouse read is not admission":     {Capability: accessidentity.CustomerCapabilityWarehouseRead.String(), FactKind: accessidentity.CustomerCapabilityWarehouseRead.String()},
	} {
		if _, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-1", requirement, "SYN-PILOT-SCOPE/x@v1", interval, "SYN-BASIS"); !errors.Is(err, accessidentity.ErrAdmissionRequirementUnknown) {
			t.Fatalf("%s: err = %v, want ErrAdmissionRequirementUnknown", name, err)
		}
	}
	for name, build := range map[string]func() error{
		"blank tenant": func() error {
			_, err := accessidentity.NewAdmissionScopeMapping(" ", "SYN-MAP-1", loadDecision, "SYN-PILOT-SCOPE/x@v1", interval, "SYN-BASIS")
			return err
		},
		"blank scope": func() error {
			_, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-1", loadDecision, "", interval, "SYN-BASIS")
			return err
		},
		"no interval": func() error {
			_, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-1", loadDecision, "SYN-PILOT-SCOPE/x@v1", accessidentity.EffectiveInterval{}, "SYN-BASIS")
			return err
		},
		"blank basis": func() error {
			_, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-1", loadDecision, "SYN-PILOT-SCOPE/x@v1", interval, "")
			return err
		},
		"end without a time": func() error {
			_, err := accessidentity.NewAdmissionScopeMappingEnd("SYN-TENANT-01", "SYN-MAP-1", time.Time{}, "SYN-BASIS")
			return err
		},
	} {
		if err := build(); !errors.Is(err, accessidentity.ErrIncompleteAdmissionScopeMapping) {
			t.Fatalf("%s: err = %v, want ErrIncompleteAdmissionScopeMapping", name, err)
		}
	}
}

// Covers: ADR-0180 决定二——一行对照在区间内、结束之前生效；同键两行的生效期有交集就算重叠，被结束的那一行只算到
// 结束时刻，所以「旧行结束、新行自结束时刻起」首尾相接不算重叠；别的键永不与它重叠。
func TestAdmissionScopeMappingsOverlapOnlyWhereBothAreEffective(t *testing.T) {
	switchover := mappingDay.Add(30 * 24 * time.Hour)
	open := recordedMapping(t, scopeMapping(t, "SYN-MAP-OLD", "SYN-PILOT-SCOPE/x@v1", mappingDay, time.Time{}), time.Time{})
	endedAtSwitchover := recordedMapping(t, scopeMapping(t, "SYN-MAP-OLD", "SYN-PILOT-SCOPE/x@v1", mappingDay, time.Time{}), switchover)
	boundedToSwitchover := recordedMapping(t, scopeMapping(t, "SYN-MAP-OLD", "SYN-PILOT-SCOPE/x@v1", mappingDay, switchover), time.Time{})
	endedBeforeStart := recordedMapping(t, scopeMapping(t, "SYN-MAP-OLD", "SYN-PILOT-SCOPE/x@v1", switchover, time.Time{}), mappingDay)

	next := scopeMapping(t, "SYN-MAP-NEW", "SYN-PILOT-SCOPE/x@v2", switchover, time.Time{})
	earlier := scopeMapping(t, "SYN-MAP-NEW", "SYN-PILOT-SCOPE/x@v2", mappingDay.Add(-24*time.Hour), mappingDay)
	otherKind, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-OTHER", accessidentity.AdmissionRequirement{
		Capability: accessidentity.CapabilityOperationDecision.String(),
		FactKind:   accessidentity.DecisionParticipationTermination.String(),
	}, "SYN-PILOT-SCOPE/x@v1", mappingInterval(t, mappingDay, time.Time{}), "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct {
		recorded  accessidentity.RecordedAdmissionScopeMapping
		candidate accessidentity.AdmissionScopeMapping
		overlaps  bool
	}{
		"open row, later start":                    {open, next, true},
		"row ended at the switchover":              {endedAtSwitchover, next, false},
		"row bounded at the switchover":            {boundedToSwitchover, next, false},
		"row ended before it started":              {endedBeforeStart, next, false},
		"candidate ends where the open row starts": {open, earlier, false},
		"another fact kind":                        {open, otherKind, false},
	}
	for name, testCase := range cases {
		if got := testCase.recorded.OverlapsWith(testCase.candidate); got != testCase.overlaps {
			t.Fatalf("%s: overlaps = %v, want %v", name, got, testCase.overlaps)
		}
	}

	if !endedAtSwitchover.EffectiveAt(switchover.Add(-time.Nanosecond)) || endedAtSwitchover.EffectiveAt(switchover) {
		t.Fatal("结束时刻起（含该时刻）不再生效，之前仍生效")
	}
	if open.EffectiveAt(mappingDay.Add(-time.Nanosecond)) || !open.EffectiveAt(mappingDay) {
		t.Fatal("区间含起点")
	}
}

// Covers: ADR-0180 决定二——同一个键在 at 那一刻生效的那一行给出对象范围；一行都不生效答没有；两行同时生效是记录错误，
// 不任选一行。结束挂到别的行上建不成。
func TestTheEffectiveMappingResolvesTheObjectScope(t *testing.T) {
	switchover := mappingDay.Add(30 * 24 * time.Hour)
	old := recordedMapping(t, scopeMapping(t, "SYN-MAP-OLD", "SYN-PILOT-SCOPE/x@v1", mappingDay, time.Time{}), switchover)
	current := recordedMapping(t, scopeMapping(t, "SYN-MAP-NEW", "SYN-PILOT-SCOPE/x@v2", switchover, time.Time{}), time.Time{})
	both := []accessidentity.RecordedAdmissionScopeMapping{old, current}

	for name, testCase := range map[string]struct {
		at    time.Time
		scope string
		found bool
	}{
		"before any row":       {mappingDay.Add(-time.Hour), "", false},
		"under the old row":    {mappingDay.Add(time.Hour), "SYN-PILOT-SCOPE/x@v1", true},
		"at the switchover":    {switchover, "SYN-PILOT-SCOPE/x@v2", true},
		"after the switchover": {switchover.Add(time.Hour), "SYN-PILOT-SCOPE/x@v2", true},
	} {
		scope, found, err := accessidentity.ResolveAdmissionObjectScope(both, testCase.at)
		if err != nil || found != testCase.found || scope != testCase.scope {
			t.Fatalf("%s: scope = %q, found = %v, err = %v", name, scope, found, err)
		}
	}
	if _, found, err := accessidentity.ResolveAdmissionObjectScope(nil, switchover); found || err != nil {
		t.Fatalf("空键：found = %v, err = %v", found, err)
	}

	clash := recordedMapping(t, scopeMapping(t, "SYN-MAP-CLASH", "SYN-PILOT-SCOPE/y@v1", mappingDay, time.Time{}), time.Time{})
	if _, _, err := accessidentity.ResolveAdmissionObjectScope([]accessidentity.RecordedAdmissionScopeMapping{current, clash}, switchover); !errors.Is(err, accessidentity.ErrConflictingAdmissionScopeMappings) {
		t.Fatalf("两行同时生效：err = %v", err)
	}

	foreignEnd, err := accessidentity.NewAdmissionScopeMappingEnd("SYN-TENANT-01", "SYN-MAP-ELSE", switchover, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessidentity.NewRecordedAdmissionScopeMapping(current.Mapping(), &foreignEnd); !errors.Is(err, accessidentity.ErrAdmissionScopeMappingEndDoesNotMatch) {
		t.Fatalf("挂错的结束：err = %v", err)
	}
}
