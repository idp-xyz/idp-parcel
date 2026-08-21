package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证暂停/恢复/接管三库：写入代数、盘点 jsonb 往返、
// 恢复外键指回暂停、接管按区间身份幂等、形状 CHECK、事务纪律。断言一律在事务闭包外。

var incidentAt = time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)

type incidentFixture struct {
	suspensions *adapter.Suspensions
	resumptions *adapter.Resumptions
	takeovers   *adapter.Takeovers
	transactor  bentoapp.Transactor
}

func newIncidentFixture(t *testing.T) *incidentFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	suspensions, err := adapter.NewSuspensions(db)
	if err != nil {
		t.Fatalf("构造暂停库：%v", err)
	}
	resumptions, err := adapter.NewResumptions(db)
	if err != nil {
		t.Fatalf("构造恢复库：%v", err)
	}
	takeovers, err := adapter.NewTakeovers(db)
	if err != nil {
		t.Fatalf("构造接管库：%v", err)
	}
	return &incidentFixture{
		suspensions: suspensions,
		resumptions: resumptions,
		takeovers:   takeovers,
		transactor:  db.Transactor(),
	}
}

func (fixture *incidentFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func incidentInventory(t *testing.T, identity string) domain.InTransitInventory {
	t.Helper()
	inventory, err := domain.TakeInventory([]domain.InventoryEntry{{
		ObjectIdentity:   identity,
		CurrentFacts:     "accepted-fact/" + identity,
		CurrentAuthority: "parcel-product",
		ResponsibleParty: "ops-owner-1",
		NextAction:       "hold-until-resume",
		ReviewBy:         incidentAt.Add(24 * time.Hour),
	}}, incidentAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("盘点：%v", err)
	}
	return inventory
}

func incidentSuspension(t *testing.T, id string) domain.SuspensionDecision {
	t.Helper()
	decision, err := domain.RecordSuspension(domain.SuspensionDecisionSpec{
		ID:            govRef(t, domain.NewSuspensionID, id),
		TriggerSource: "stage-no-go",
		Basis:         "limited-production-blocked",
		Evidence:      "evidence-pack/r8",
		Scope:         govRef(t, domain.NewScopeVersionReference, "pilot-scope/v3"),
		ExecutedBy:    "pilot-business-owner",
		OccurredAt:    incidentAt,
		EffectiveAt:   incidentAt.Add(time.Hour),
		InTransitNote: "in-transit objects stay with current authority",
	})
	if err != nil {
		t.Fatalf("形成暂停：%v", err)
	}
	return decision
}

func incidentResumption(t *testing.T, suspensionID string) domain.ResumptionDecision {
	t.Helper()
	decision, err := domain.RecordResumption(domain.ResumptionDecisionSpec{
		Suspension:       govRef(t, domain.NewSuspensionID, suspensionID),
		ReleaseEvidence:  "cause-cleared/r2",
		ConsistencyCheck: "inventory-matches-current-facts",
		Inventory:        incidentInventory(t, "parcel-held-1"),
		DecidedBy:        "pilot-business-owner",
		DecidedAt:        incidentAt.Add(2 * time.Hour),
		EffectiveAt:      incidentAt.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成恢复：%v", err)
	}
	return decision
}

func incidentInterval(open bool) domain.AuthorityInterval {
	interval := domain.AuthorityInterval{
		ObjectScope: "lane-1-parcels",
		Capability:  "shipment-intake",
		FactKind:    "acceptance-decision",
		Authority:   "ops-takeover",
		From:        incidentAt,
	}
	if !open {
		interval.To = incidentAt.Add(48 * time.Hour)
	}
	return interval
}

func incidentTakeover(t *testing.T, interval domain.AuthorityInterval) domain.TakeoverRecord {
	t.Helper()
	record, err := domain.RecordTakeover(domain.TakeoverRecordSpec{
		StopEvidence:     "legacy-writer-stopped/r1",
		Interval:         interval,
		AcceptedFacts:    "accepted-facts-cutover/r1",
		PendingExternals: "none-pending",
		ActualControl:    "ops-desk-owns-writes",
		Responsibilities: "ops-owner-1",
		NextAction:       "shadow-then-limited-production",
		Inventory:        incidentInventory(t, "parcel-cutover-1"),
		EffectiveAt:      incidentAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("形成接管：%v", err)
	}
	return record
}

func sameInventory(left, right domain.InTransitInventory) bool {
	if !left.TakenAt().Equal(right.TakenAt()) {
		return false
	}
	leftEntries, rightEntries := left.Entries(), right.Entries()
	if len(leftEntries) != len(rightEntries) {
		return false
	}
	for i := range leftEntries {
		if leftEntries[i].ObjectIdentity != rightEntries[i].ObjectIdentity ||
			leftEntries[i].CurrentFacts != rightEntries[i].CurrentFacts ||
			leftEntries[i].CurrentAuthority != rightEntries[i].CurrentAuthority ||
			leftEntries[i].ResponsibleParty != rightEntries[i].ResponsibleParty ||
			leftEntries[i].NextAction != rightEntries[i].NextAction ||
			!leftEntries[i].ReviewBy.Equal(rightEntries[i].ReviewBy) {
			return false
		}
	}
	return true
}

func TestSuspensionRoundTripsAndSecondSaveKeepsTheTxUsable(t *testing.T) {
	fixture := newIncidentFixture(t)
	ctx := t.Context()
	first := incidentSuspension(t, "suspend-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.suspensions.Save(txCtx, first)
		if err != nil {
			return err
		}
		if saved != ports.GovernanceSaved {
			return fmt.Errorf("首写结果 = %d，应为 SAVED", saved)
		}
		return nil
	})

	found, exists, err := fixture.suspensions.FindByID(ctx, first.ID())
	if err != nil || !exists {
		t.Fatalf("取回暂停：%v exists=%v", err, exists)
	}
	if found != first {
		t.Errorf("暂停读回变形：%+v，应为 %+v", found, first)
	}

	mutated, err := domain.RecordSuspension(domain.SuspensionDecisionSpec{
		ID:            first.ID(),
		TriggerSource: "manual-ops",
		Basis:         "different-basis",
		Evidence:      "evidence-pack/r9",
		Scope:         govRef(t, domain.NewScopeVersionReference, "pilot-scope/v4"),
		ExecutedBy:    "someone-else",
		OccurredAt:    incidentAt.Add(4 * time.Hour),
		EffectiveAt:   incidentAt.Add(5 * time.Hour),
		InTransitNote: "would overwrite if allowed",
	})
	if err != nil {
		t.Fatalf("形成第二份暂停：%v", err)
	}

	var outcome ports.GovernanceSaveOutcome
	var winner domain.SuspensionDecision
	var winnerFound bool
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.suspensions.Save(txCtx, mutated)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = fixture.suspensions.FindByID(txCtx, first.ID())
		return err
	})
	if outcome != ports.GovernanceAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %d，应为 ALREADY_RECORDED", outcome)
	}
	if !winnerFound || winner != first {
		t.Fatalf("同事务读回赢家失败：found=%v winner=%+v", winnerFound, winner)
	}
}

func TestResumptionRoundTripsAndRequiresTheSuspension(t *testing.T) {
	fixture := newIncidentFixture(t)
	ctx := t.Context()
	suspension := incidentSuspension(t, "suspend-2")
	resumption := incidentResumption(t, "suspend-2")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.resumptions.Save(txCtx, resumption)
		return err
	}); err == nil {
		t.Fatal("没有暂停的恢复被库接受了")
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.suspensions.Save(txCtx, suspension); err != nil {
			return err
		}
		saved, err := fixture.resumptions.Save(txCtx, resumption)
		if err != nil {
			return err
		}
		if saved != ports.GovernanceSaved {
			return fmt.Errorf("恢复首写结果 = %d，应为 SAVED", saved)
		}
		return nil
	})

	found, exists, err := fixture.resumptions.FindBySuspension(ctx, resumption.Suspension())
	if err != nil || !exists {
		t.Fatalf("取回恢复：%v exists=%v", err, exists)
	}
	if found.ReleaseEvidence() != resumption.ReleaseEvidence() ||
		found.ConsistencyCheck() != resumption.ConsistencyCheck() ||
		found.DecidedBy() != resumption.DecidedBy() ||
		!found.DecidedAt().Equal(resumption.DecidedAt()) ||
		!found.EffectiveAt().Equal(resumption.EffectiveAt()) ||
		!sameInventory(found.Inventory(), resumption.Inventory()) {
		t.Errorf("恢复读回变形：%+v", found)
	}

	mutated, err := domain.RecordResumption(domain.ResumptionDecisionSpec{
		Suspension:       resumption.Suspension(),
		ReleaseEvidence:  "other-evidence",
		ConsistencyCheck: "other-check",
		Inventory:        incidentInventory(t, "parcel-held-9"),
		DecidedBy:        "someone-else",
		DecidedAt:        incidentAt.Add(6 * time.Hour),
		EffectiveAt:      incidentAt.Add(7 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成第二份恢复：%v", err)
	}
	var outcome ports.GovernanceSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.resumptions.Save(txCtx, mutated)
		outcome = saved
		return err
	})
	if outcome != ports.GovernanceAlreadyRecorded {
		t.Fatalf("第二份恢复结果 = %d，应为 ALREADY_RECORDED", outcome)
	}
	winner, exists, err := fixture.resumptions.FindBySuspension(ctx, resumption.Suspension())
	if err != nil || !exists || winner.ReleaseEvidence() != "cause-cleared/r2" {
		t.Fatalf("先到者被改写：err=%v exists=%v evidence=%s", err, exists, winner.ReleaseEvidence())
	}
}

func TestTakeoverRoundTripsOpenAndClosedIntervalsSeparately(t *testing.T) {
	fixture := newIncidentFixture(t)
	ctx := t.Context()
	open := incidentTakeover(t, incidentInterval(true))
	closed := incidentTakeover(t, incidentInterval(false))

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if saved, err := fixture.takeovers.Save(txCtx, open); err != nil || saved != ports.GovernanceSaved {
			return fmt.Errorf("开放区间接管写入：saved=%d err=%v", saved, err)
		}
		if saved, err := fixture.takeovers.Save(txCtx, closed); err != nil || saved != ports.GovernanceSaved {
			return fmt.Errorf("封闭区间接管写入：saved=%d err=%v", saved, err)
		}
		return nil
	})

	foundOpen, exists, err := fixture.takeovers.FindByInterval(ctx, open.Interval())
	if err != nil || !exists {
		t.Fatalf("取回开放接管：%v exists=%v", err, exists)
	}
	if foundOpen.StopEvidence() != open.StopEvidence() ||
		foundOpen.Interval() != open.Interval() ||
		foundOpen.AcceptedFacts() != open.AcceptedFacts() ||
		foundOpen.PendingExternals() != open.PendingExternals() ||
		foundOpen.ActualControl() != open.ActualControl() ||
		foundOpen.Responsibilities() != open.Responsibilities() ||
		foundOpen.NextAction() != open.NextAction() ||
		!foundOpen.EffectiveAt().Equal(open.EffectiveAt()) ||
		!sameInventory(foundOpen.Inventory(), open.Inventory()) {
		t.Errorf("开放接管读回变形：%+v", foundOpen)
	}

	foundClosed, exists, err := fixture.takeovers.FindByInterval(ctx, closed.Interval())
	if err != nil || !exists || foundClosed.Interval().To.IsZero() {
		t.Fatalf("取回封闭接管：err=%v exists=%v to=%v", err, exists, foundClosed.Interval().To)
	}

	var outcome ports.GovernanceSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		mutated, err := domain.RecordTakeover(domain.TakeoverRecordSpec{
			StopEvidence:     "would-overwrite",
			Interval:         open.Interval(),
			AcceptedFacts:    "other-facts",
			PendingExternals: "other-pending",
			ActualControl:    "other-control",
			Responsibilities: "other-party",
			NextAction:       "other-action",
			Inventory:        incidentInventory(t, "parcel-cutover-9"),
			EffectiveAt:      incidentAt.Add(9 * time.Hour),
		})
		if err != nil {
			return err
		}
		saved, err := fixture.takeovers.Save(txCtx, mutated)
		outcome = saved
		return err
	})
	if outcome != ports.GovernanceAlreadyRecorded {
		t.Fatalf("同开放区间第二份结果 = %d，应为 ALREADY_RECORDED", outcome)
	}
}

func TestIncidentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	fixture := newIncidentFixture(t)
	ctx := t.Context()

	if _, err := fixture.suspensions.Save(ctx, incidentSuspension(t, "suspend-x")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存暂停应拒，实得：%v", err)
	}
	if _, err := fixture.resumptions.Save(ctx, incidentResumption(t, "suspend-x")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存恢复应拒，实得：%v", err)
	}
	if _, err := fixture.takeovers.Save(ctx, incidentTakeover(t, incidentInterval(true))); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存接管应拒，实得：%v", err)
	}
}

// scopedSuspension 与 liftingResumption 把范围版本与生效时刻交给用例摆布：
// 「时点 T 上还拦不拦」这条查询的三处边界全落在这两样上，固定夹具试不出来。
func scopedSuspension(t *testing.T, id, scope string, effectiveAt time.Time) domain.SuspensionDecision {
	t.Helper()
	decision, err := domain.RecordSuspension(domain.SuspensionDecisionSpec{
		ID:            govRef(t, domain.NewSuspensionID, id),
		TriggerSource: "ops-standby",
		Basis:         "hard-risk-rule/v1",
		Evidence:      "evidence-pack/r9",
		Scope:         govRef(t, domain.NewScopeVersionReference, scope),
		ExecutedBy:    "pilot-business-owner",
		OccurredAt:    effectiveAt.Add(-time.Hour),
		EffectiveAt:   effectiveAt,
		InTransitNote: "in-transit objects stay with current authority",
	})
	if err != nil {
		t.Fatalf("形成暂停：%v", err)
	}
	return decision
}

func liftingResumption(t *testing.T, suspensionID string, effectiveAt time.Time) domain.ResumptionDecision {
	t.Helper()
	decision, err := domain.RecordResumption(domain.ResumptionDecisionSpec{
		Suspension:       govRef(t, domain.NewSuspensionID, suspensionID),
		ReleaseEvidence:  "cause-cleared/r3",
		ConsistencyCheck: "inventory-matches-current-facts",
		Inventory:        incidentInventory(t, "parcel-held-2"),
		DecidedBy:        "pilot-business-owner",
		DecidedAt:        effectiveAt.Add(-time.Hour),
		EffectiveAt:      effectiveAt,
	})
	if err != nil {
		t.Fatalf("形成恢复：%v", err)
	}
	return decision
}

// Covers: 生效时刻起算、恢复时刻起解除。两端都取半开，与本仓权威区间 `[From, To)` 同向；
// 恢复必须是明确决定，所以没有恢复记录之前一直拦着。
func TestAnUnresumedSuspensionBlocksFromItsEffectiveMomentUntilTheResumptionTakesEffect(t *testing.T) {
	fixture := newIncidentFixture(t)
	ctx := t.Context()
	scope := govRef(t, domain.NewScopeVersionReference, "pilot-scope/v9")
	suspendedAt := incidentAt.Add(time.Hour)
	resumedAt := incidentAt.Add(5 * time.Hour)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.suspensions.Save(txCtx, scopedSuspension(t, "suspend-window", "pilot-scope/v9", suspendedAt))
		return err
	})

	for name, probe := range map[string]struct {
		at    time.Time
		wants bool
	}{
		"生效前一刻":  {at: suspendedAt.Add(-time.Second), wants: false},
		"恰在生效时刻": {at: suspendedAt, wants: true},
		"生效之后":   {at: suspendedAt.Add(time.Hour), wants: true},
	} {
		t.Run("未恢复·"+name, func(t *testing.T) {
			_, found, err := fixture.suspensions.FindUnresumedSuspension(ctx, scope, probe.at)
			if err != nil {
				t.Fatalf("取尚未恢复的暂停：%v", err)
			}
			if found != probe.wants {
				t.Fatalf("found = %v, want %v", found, probe.wants)
			}
		})
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.resumptions.Save(txCtx, liftingResumption(t, "suspend-window", resumedAt))
		return err
	})

	for name, probe := range map[string]struct {
		at    time.Time
		wants bool
	}{
		"恢复前一刻":  {at: resumedAt.Add(-time.Second), wants: true},
		"恰在恢复时刻": {at: resumedAt, wants: false},
		"恢复之后":   {at: resumedAt.Add(time.Hour), wants: false},
	} {
		t.Run("已恢复·"+name, func(t *testing.T) {
			_, found, err := fixture.suspensions.FindUnresumedSuspension(ctx, scope, probe.at)
			if err != nil {
				t.Fatalf("取尚未恢复的暂停：%v", err)
			}
			if found != probe.wants {
				t.Fatalf("found = %v, want %v；恢复只在其生效时刻之后解除", found, probe.wants)
			}
		})
	}
}

// Covers: 范围版本按字面相等匹配。暂停范围变动时形成的是带新依据与生效时间的**新版本**，
// 此前判断不被覆盖，因此一条暂停只为它写明的那一版说话；版本之间的先后关系在
// ScopeVersionReference 这个不透明串上读不出来，也就无从写出跨版本匹配。
func TestASuspensionAnswersOnlyForTheScopeVersionItNames(t *testing.T) {
	fixture := newIncidentFixture(t)
	ctx := t.Context()
	suspendedAt := incidentAt.Add(time.Hour)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.suspensions.Save(txCtx, scopedSuspension(t, "suspend-v1", "pilot-scope/v1", suspendedAt))
		return err
	})

	found, exists, err := fixture.suspensions.FindUnresumedSuspension(
		ctx, govRef(t, domain.NewScopeVersionReference, "pilot-scope/v1"), suspendedAt)
	if err != nil || !exists {
		t.Fatalf("同版本应取到：%v exists=%v", err, exists)
	}
	if found.Scope().String() != "pilot-scope/v1" {
		t.Fatalf("取回范围 = %q", found.Scope().String())
	}

	if _, exists, err = fixture.suspensions.FindUnresumedSuspension(
		ctx, govRef(t, domain.NewScopeVersionReference, "pilot-scope/v2"), suspendedAt); err != nil {
		t.Fatalf("取尚未恢复的暂停：%v", err)
	} else if exists {
		t.Fatal("v1 的暂停被当成了 v2 的")
	}
}

// Covers: 同一范围多条暂停各自独立解除——恢复记录逐条引用一个暂停标识，所以解除其中一条
// 之后，只要还有一条已生效且未恢复，范围就仍在暂停中。
func TestAScopeStaysSuspendedWhileAnyUnresumedSuspensionRemains(t *testing.T) {
	fixture := newIncidentFixture(t)
	ctx := t.Context()
	scope := govRef(t, domain.NewScopeVersionReference, "pilot-scope/v7")
	earlier := incidentAt.Add(time.Hour)
	later := incidentAt.Add(2 * time.Hour)
	askedAt := incidentAt.Add(6 * time.Hour)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.suspensions.Save(txCtx, scopedSuspension(t, "suspend-early", "pilot-scope/v7", earlier)); err != nil {
			return err
		}
		_, err := fixture.suspensions.Save(txCtx, scopedSuspension(t, "suspend-late", "pilot-scope/v7", later))
		return err
	})

	// 定序取最早那条：同一份登记册每次问都得到同一条，调用方才不会每问一次就换一个引用。
	found, exists, err := fixture.suspensions.FindUnresumedSuspension(ctx, scope, askedAt)
	if err != nil || !exists {
		t.Fatalf("两条未恢复时应取到：%v exists=%v", err, exists)
	}
	if found.ID().String() != "suspend-early" {
		t.Fatalf("交回 = %q, want suspend-early", found.ID().String())
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.resumptions.Save(txCtx, liftingResumption(t, "suspend-early", incidentAt.Add(3*time.Hour)))
		return err
	})

	found, exists, err = fixture.suspensions.FindUnresumedSuspension(ctx, scope, askedAt)
	if err != nil || !exists {
		t.Fatalf("还剩一条未恢复时仍应取到：%v exists=%v", err, exists)
	}
	if found.ID().String() != "suspend-late" {
		t.Fatalf("交回 = %q, want suspend-late；解除一条不等于范围已恢复", found.ID().String())
	}
}

func TestIncidentShapeChecksRejectBlankAndEmptyInventory(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO pilot_governance.suspension_decision
			(suspension_id, trigger_source, basis, evidence, scope, executed_by,
			 occurred_at, effective_at, in_transit_note)
		 VALUES (' ', 'src', 'basis', 'evidence', 'scope', 'owner', now(), now(), 'note')`); err == nil {
		t.Fatal("空白暂停标识被库接受了")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO pilot_governance.suspension_decision
			(suspension_id, trigger_source, basis, evidence, scope, executed_by,
			 occurred_at, effective_at, in_transit_note)
		 VALUES ('suspend-check', 'src', 'basis', 'evidence', 'scope', 'owner', now(), now(), 'note')`); err != nil {
		t.Fatalf("落暂停行：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO pilot_governance.resumption_decision
			(suspension_id, release_evidence, consistency_check, inventory,
			 inventory_taken_at, decided_by, decided_at, effective_at)
		 VALUES ('suspend-check', 'release', 'check', '[]'::jsonb, now(), 'owner', now(), now())`); err == nil {
		t.Fatal("空盘点数组被库接受了")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO pilot_governance.takeover_record
			(object_scope, capability, fact_kind, authority, from_at, to_at,
			 stop_evidence, accepted_facts, pending_externals, actual_control,
			 responsibilities, next_action, inventory, inventory_taken_at, effective_at)
		 VALUES ('scope', 'cap', 'fact', 'auth', now(), now() - interval '1 hour',
		         'stop', 'facts', 'pending', 'control', 'duty', 'next',
		         '[{"objectIdentity":"o"}]'::jsonb, now(), now())`); err == nil {
		t.Fatal("to_at 早于 from_at 的接管被库接受了")
	}
}
