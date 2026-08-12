package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

var suspendedAt = time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)

func entry(t *testing.T, identity string) domain.InventoryEntry {
	t.Helper()
	return domain.InventoryEntry{
		ObjectIdentity:   identity,
		CurrentFacts:     "accepted; intake adopted; in linehaul",
		CurrentAuthority: "idp-parcel",
		ResponsibleParty: "ops-lead",
		NextAction:       "monitor delivery",
		ReviewBy:         suspendedAt.Add(72 * time.Hour),
	}
}

func inventory(t *testing.T) domain.InTransitInventory {
	t.Helper()
	built, err := domain.TakeInventory([]domain.InventoryEntry{
		entry(t, "request-1"), entry(t, "request-2"),
	}, suspendedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("take inventory: %v", err)
	}
	return built
}

func suspension(t *testing.T) domain.SuspensionDecision {
	t.Helper()
	built, err := domain.RecordSuspension(domain.SuspensionDecisionSpec{
		ID:            mustValue(t, domain.NewSuspensionID, "suspension-1"),
		TriggerSource: "HARD_COMPLIANCE_RISK",
		Basis:         "PAR-GOV-05/rule-3",
		Evidence:      "evidence-pack/suspension-1",
		Scope:         mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v1"),
		ExecutedBy:    "duty-officer",
		OccurredAt:    suspendedAt,
		EffectiveAt:   suspendedAt.Add(10 * time.Minute),
		InTransitNote: "in-transit continues under current authority",
	})
	if err != nil {
		t.Fatalf("record suspension: %v", err)
	}
	return built
}

// Covers: PN-08 治理记录表「暂停决定：触发来源、规则或人工依据、证据、范围、执行
// 身份、发生/生效时间、在途影响」——九件缺一立不起；暂停不取消不迁移在途（类型上
// 没有那些字段）。「在途盘点：逐项六件齐全、对象不重」——缺件与重复都拒。
func TestSuspensionAndInventoryDemandTheirFullShape(t *testing.T) {
	if suspension(t).Evidence() != "evidence-pack/suspension-1" {
		t.Fatal("证据引用没有随决定保全")
	}

	missingEvidence := domain.SuspensionDecisionSpec{
		ID:            mustValue(t, domain.NewSuspensionID, "suspension-2"),
		TriggerSource: "HARD_COMPLIANCE_RISK",
		Basis:         "PAR-GOV-05/rule-3",
		Scope:         mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v1"),
		ExecutedBy:    "duty-officer",
		OccurredAt:    suspendedAt,
		EffectiveAt:   suspendedAt,
		InTransitNote: "note",
	}
	if _, err := domain.RecordSuspension(missingEvidence); !errors.Is(err, domain.ErrInvalidSuspension) {
		t.Fatalf("err = %v; 没有证据的暂停被收下了", err)
	}

	incomplete := entry(t, "request-3")
	incomplete.NextAction = ""
	if _, err := domain.TakeInventory([]domain.InventoryEntry{incomplete}, suspendedAt); !errors.Is(err, domain.ErrInvalidInventory) {
		t.Fatalf("err = %v; 六件不全的盘点条目被收下了", err)
	}
	if _, err := domain.TakeInventory([]domain.InventoryEntry{
		entry(t, "request-1"), entry(t, "request-1"),
	}, suspendedAt); !errors.Is(err, domain.ErrInvalidInventory) {
		t.Fatalf("err = %v; 同一对象两条盘点分不出真假", err)
	}
}

// Covers: PN-08 治理记录表「恢复决定：原暂停决定、原因解除证据、一致性核对、在途
// 盘点、决定方、决定/生效时间；原暂停记录不可修改」——四件核心缺一立不起；恢复只
// 引用暂停标识，没有任何回写入口（不可修改是结构性的）。
func TestResumptionDemandsReleaseEvidenceAndInventory(t *testing.T) {
	resumption, err := domain.RecordResumption(domain.ResumptionDecisionSpec{
		Suspension:       suspension(t).ID(),
		ReleaseEvidence:  "evidence-pack/release-1",
		ConsistencyCheck: "consistency-check/1",
		Inventory:        inventory(t),
		DecidedBy:        "pilot-business-owner",
		DecidedAt:        suspendedAt.Add(48 * time.Hour),
		EffectiveAt:      suspendedAt.Add(49 * time.Hour),
	})
	if err != nil {
		t.Fatalf("record resumption: %v", err)
	}
	if resumption.Suspension().String() != "suspension-1" {
		t.Fatal("恢复没有指回原暂停决定")
	}
	if len(resumption.Inventory().Entries()) != 2 {
		t.Fatal("在途盘点没有随恢复保全")
	}

	withoutInventory := domain.ResumptionDecisionSpec{
		Suspension:       suspension(t).ID(),
		ReleaseEvidence:  "evidence-pack/release-1",
		ConsistencyCheck: "consistency-check/1",
		DecidedBy:        "pilot-business-owner",
		DecidedAt:        suspendedAt.Add(48 * time.Hour),
		EffectiveAt:      suspendedAt.Add(49 * time.Hour),
	}
	if _, err := domain.RecordResumption(withoutInventory); !errors.Is(err, domain.ErrInvalidResumption) {
		t.Fatalf("err = %v; 没有在途盘点的恢复被收下了", err)
	}
}

// Covers: PN-08 失败场景「原权威无法继续且存在未完成外部动作——先停原权威写入，逐
// 对象盘点和受控接管；错误结果是批量迁移、重复指令或覆盖历史」——停止写入证据先行、
// 新权威区间形状完整、在途盘点必备；接管记录不带任何历史改写入口。
func TestATakeoverDemandsStopEvidenceIntervalAndInventory(t *testing.T) {
	interval := domain.AuthorityInterval{
		ObjectScope: "pilot-scope/v1",
		Capability:  "SHIPMENT_ACCEPTANCE",
		FactKind:    "ACCEPTANCE_DECISION",
		Authority:   "successor-system",
		From:        suspendedAt.Add(72 * time.Hour),
	}

	takeover, err := domain.RecordTakeover(domain.TakeoverRecordSpec{
		StopEvidence:     "evidence-pack/authority-stopped",
		Interval:         interval,
		AcceptedFacts:    "facts-index/1",
		PendingExternals: "pending-externals/1",
		ActualControl:    "control-map/1",
		Responsibilities: "funds-and-customs/1",
		NextAction:       "notify partners",
		Inventory:        inventory(t),
		EffectiveAt:      suspendedAt.Add(72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("record takeover: %v", err)
	}
	if takeover.Interval().Authority != "successor-system" {
		t.Fatal("新权威没有随接管记录")
	}

	withoutStop := domain.TakeoverRecordSpec{
		Interval:         interval,
		AcceptedFacts:    "facts-index/1",
		PendingExternals: "pending-externals/1",
		ActualControl:    "control-map/1",
		Responsibilities: "funds-and-customs/1",
		NextAction:       "notify partners",
		Inventory:        inventory(t),
		EffectiveAt:      suspendedAt.Add(72 * time.Hour),
	}
	if _, err := domain.RecordTakeover(withoutStop); !errors.Is(err, domain.ErrInvalidTakeover) {
		t.Fatalf("err = %v; 没有停止写入证据的接管被收下了——先停原权威是硬顺序", err)
	}
}
