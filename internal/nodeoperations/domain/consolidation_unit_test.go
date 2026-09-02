package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

var sealedAt = time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)

// workSource 造一份完整的来源表达。业务时间由调用方给，好让每次作业各带各的时刻——
// 这正是 Clock.Now() 换掉之后现场该有的样子。
func workSource(t *testing.T, sourceID string, at time.Time) domain.WorkFactSource {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		sourceID,
		mustValue(t, domain.NewPerformingPartyReference, "packer-1"),
		mustValue(t, domain.NewExecutionEvidenceReference, "WORK-EVIDENCE/"+sourceID),
		at,
	)
	if err != nil {
		t.Fatalf("work fact source %q: %v", sourceID, err)
	}
	return source
}

func openUnit(t *testing.T) *domain.ConsolidationUnit {
	t.Helper()
	unit, err := domain.OpenConsolidationUnit(
		mustValue(t, domain.NewConsolidationUnitID, "bag-instance-1"),
		mustValue(t, domain.NewCarrierAssetReference, "bag-asset-7"),
		workSource(t, "src-open", sealedAt.Add(-time.Hour)),
	)
	if err != nil {
		t.Fatalf("open consolidation unit: %v", err)
	}
	return unit
}

func addMember(t *testing.T, unit *domain.ConsolidationUnit, id string) {
	t.Helper()
	if err := unit.AddMember(mustValue(t, domain.NewHandlingUnitID, id)); err != nil {
		t.Fatalf("add member %q: %v", id, err)
	}
}

// Covers: NO CONTEXT「封装冻结当时成员快照。封装后的成员变化必须依次保留开封、成员
// 变化和重新封装事实，并形成新封签记录；任何历史快照和封签都不得被新版本覆盖」——
// 封装态加减成员拒（必须先开封）；开封-变更-重封产生第二份快照，第一份原样在列；
// 空单元封不出快照。
func TestSealingFreezesSnapshotsAndNeverOverwritesThem(t *testing.T) {
	unit := openUnit(t)
	addMember(t, unit, "unit-1")
	addMember(t, unit, "unit-2")

	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-1"),
		mustValue(t, domain.NewWorkBasisReference, "PACK-TASK/7"),
		workSource(t, "src-seal-1", sealedAt),
	); err != nil {
		t.Fatalf("seal: %v", err)
	}

	if err := unit.AddMember(mustValue(t, domain.NewHandlingUnitID, "unit-3")); !errors.Is(err, domain.ErrUnitSealed) {
		t.Fatalf("err = %v; 封装后改变成员必须先开封", err)
	}

	if err := unit.Unseal(mustValue(t, domain.NewWorkBasisReference, "UNPACK-TASK/8"), sealedAt.Add(time.Hour)); err != nil {
		t.Fatalf("unseal: %v", err)
	}
	if err := unit.RemoveMember(mustValue(t, domain.NewHandlingUnitID, "unit-2")); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-2"),
		mustValue(t, domain.NewWorkBasisReference, "REPACK-TASK/9"),
		workSource(t, "src-seal-2", sealedAt.Add(2*time.Hour)),
	); err != nil {
		t.Fatalf("reseal: %v", err)
	}

	snapshots := unit.Snapshots()
	if len(snapshots) != 2 {
		t.Fatalf("snapshots = %d, want 2；重新封装创建新快照不覆盖原快照", len(snapshots))
	}
	if len(snapshots[0].Members()) != 2 || snapshots[0].Seal().String() != "seal-1" {
		t.Fatalf("first snapshot = %#v; 原快照被改写了", snapshots[0])
	}
	if len(snapshots[1].Members()) != 1 || snapshots[1].Seal().String() != "seal-2" {
		t.Fatalf("second snapshot = %#v", snapshots[1])
	}

	empty := openUnit(t)
	if err := empty.Seal(
		mustValue(t, domain.NewSealReference, "seal-9"),
		mustValue(t, domain.NewWorkBasisReference, "PACK-TASK/9"),
		workSource(t, "src-seal-empty", sealedAt),
	); !errors.Is(err, domain.ErrInvalidConsolidation) {
		t.Fatalf("err = %v; 空单元冻结不出任何关系", err)
	}
}

// Covers: UC-NO-003 结果契约「作业事实已形成」必须保存`执行方、来源和证据`与`发生
// 时间`，以及 CONTEXT「封签记录」的`施封依据`——开启与封装两口都拒绝说不出来源的
// 作业，快照把这几格原样留住，封装时刻取自现场自带的业务时间而不是服务端时钟
// （ADR-0023）。
func TestConsolidationRefusesWorkThatCannotNameItsSource(t *testing.T) {
	id := mustValue(t, domain.NewConsolidationUnitID, "bag-instance-2")
	asset := mustValue(t, domain.NewCarrierAssetReference, "bag-asset-8")

	if _, err := domain.OpenConsolidationUnit(id, asset, domain.WorkFactSource{}); !errors.Is(err, domain.ErrInvalidWorkFactSource) {
		t.Fatalf("err = %v; 一个说不出谁开的实例被开出来了", err)
	}

	// 四格逐个抽掉都不成立，且都不给兜底——尤其业务时间，缺它不得回退到系统时钟。
	party := mustValue(t, domain.NewPerformingPartyReference, "packer-1")
	evidence := mustValue(t, domain.NewExecutionEvidenceReference, "WORK-EVIDENCE/1")
	for name, attempt := range map[string]struct {
		sourceID    string
		performedBy domain.PerformingPartyReference
		evidence    domain.ExecutionEvidenceReference
		occurredAt  time.Time
	}{
		"缺来源身份": {"   ", party, evidence, sealedAt},
		"缺执行方":  {"src-1", domain.PerformingPartyReference{}, evidence, sealedAt},
		"缺证据":   {"src-1", party, domain.ExecutionEvidenceReference{}, sealedAt},
		"缺业务时间": {"src-1", party, evidence, time.Time{}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewWorkFactSource(attempt.sourceID, attempt.performedBy, attempt.evidence, attempt.occurredAt)
			if !errors.Is(err, domain.ErrInvalidWorkFactSource) {
				t.Fatalf("err = %v", err)
			}
		})
	}

	unit := openUnit(t)
	addMember(t, unit, "unit-1")
	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-1"),
		mustValue(t, domain.NewWorkBasisReference, "PACK/1"),
		domain.WorkFactSource{},
	); !errors.Is(err, domain.ErrInvalidWorkFactSource) {
		t.Fatalf("err = %v; 一次说不出执行方的封装被记下来了", err)
	}

	sealSource := workSource(t, "src-seal-kept", sealedAt)
	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-1"),
		mustValue(t, domain.NewWorkBasisReference, "PACK/1"),
		sealSource,
	); err != nil {
		t.Fatalf("seal: %v", err)
	}
	snapshot := unit.Snapshots()[0]
	if snapshot.Source().SourceID() != "src-seal-kept" ||
		snapshot.Source().PerformedBy() != sealSource.PerformedBy() ||
		snapshot.Source().Evidence() != sealSource.Evidence() {
		t.Fatalf("快照没留住来源：%+v", snapshot.Source())
	}
	if !snapshot.SealedAt().Equal(sealedAt) {
		t.Fatalf("sealedAt = %v, want %v；封装时刻必须来自现场自带的业务时间", snapshot.SealedAt(), sealedAt)
	}
	if unit.OpenedBy().SourceID() != "src-open" {
		t.Fatalf("实例没留住开启来源：%+v", unit.OpenedBy())
	}
}

// Covers: NO CONTEXT「全部成员已经移出，或剩余成员已通过明确处置转移后，实例才能
// 显式终局关闭。已关闭集运单元实例永久终局，不得重开、清空后复用或承载新成员」——
// 带成员且无处置转移关不了；关闭后一切操作拒；封装态先开封再关。
func TestClosureIsExplicitAndPermanent(t *testing.T) {
	unit := openUnit(t)
	addMember(t, unit, "unit-1")

	if err := unit.Close(domain.WorkBasisReference{}, sealedAt); !errors.Is(err, domain.ErrMembersStillContained) {
		t.Fatalf("err = %v; 带成员且无处置转移关掉了", err)
	}
	if err := unit.Close(
		mustValue(t, domain.NewWorkBasisReference, "DISPOSITION-TRANSFER/3"),
		sealedAt,
	); err != nil {
		t.Fatalf("close with disposition: %v", err)
	}
	if !unit.Closed() || unit.ClosedAt().IsZero() {
		t.Fatal("关闭没有落到实例上")
	}

	if err := unit.AddMember(mustValue(t, domain.NewHandlingUnitID, "unit-9")); !errors.Is(err, domain.ErrUnitClosed) {
		t.Fatalf("err = %v; 已关闭实例承载了新成员", err)
	}
	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-9"),
		mustValue(t, domain.NewWorkBasisReference, "PACK/9"),
		workSource(t, "src-seal-closed", sealedAt.Add(time.Hour)),
	); !errors.Is(err, domain.ErrUnitClosed) {
		t.Fatalf("err = %v; 已关闭实例被重新封装了", err)
	}
	if err := unit.Close(
		mustValue(t, domain.NewWorkBasisReference, "DISPOSITION/again"),
		sealedAt.Add(time.Hour),
	); !errors.Is(err, domain.ErrUnitClosed) {
		t.Fatalf("err = %v; 终局关了两次", err)
	}

	emptied := openUnit(t)
	addMember(t, emptied, "unit-1")
	if err := emptied.RemoveMember(mustValue(t, domain.NewHandlingUnitID, "unit-1")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := emptied.Close(domain.WorkBasisReference{}, sealedAt); err != nil {
		t.Fatalf("close emptied: %v", err)
	}
}

// Covers: 成员纪律——重复移入拒（本单元侧的单父级防线；跨单元唯一性由收纳编排凭
// 仓储视野核对）；移出不在册的成员拒；封装态移出拒。
func TestMembershipStaysDisciplined(t *testing.T) {
	unit := openUnit(t)
	addMember(t, unit, "unit-1")

	if err := unit.AddMember(mustValue(t, domain.NewHandlingUnitID, "unit-1")); !errors.Is(err, domain.ErrInvalidConsolidation) {
		t.Fatalf("err = %v; 同一实物移入了两次", err)
	}
	if err := unit.RemoveMember(mustValue(t, domain.NewHandlingUnitID, "unit-9")); !errors.Is(err, domain.ErrInvalidConsolidation) {
		t.Fatalf("err = %v; 移出了不在册的成员", err)
	}

	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-1"),
		mustValue(t, domain.NewWorkBasisReference, "PACK/1"),
		workSource(t, "src-seal-membership", sealedAt),
	); err != nil {
		t.Fatalf("seal: %v", err)
	}
	if err := unit.RemoveMember(mustValue(t, domain.NewHandlingUnitID, "unit-1")); !errors.Is(err, domain.ErrUnitSealed) {
		t.Fatalf("err = %v; 封装态移出了成员", err)
	}
	if err := unit.Unseal(mustValue(t, domain.NewWorkBasisReference, "UNPACK/2"), sealedAt.Add(time.Hour)); err != nil {
		t.Fatalf("unseal: %v", err)
	}
	unsealed := openUnit(t)
	if err := unsealed.Unseal(mustValue(t, domain.NewWorkBasisReference, "UNPACK/3"), sealedAt); !errors.Is(err, domain.ErrUnitNotSealed) {
		t.Fatalf("err = %v; 未封装开了封", err)
	}
}
