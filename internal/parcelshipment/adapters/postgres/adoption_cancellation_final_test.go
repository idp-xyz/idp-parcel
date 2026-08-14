package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证采认、取消与终局三库的行为：整行往返后经领域构造门
// 复验、责任起点唯一由部分唯一索引裁决并发（AT-PS-049）、终局重派生翻旧插新且历史
// 保留（AT-PS-063）、三走向/两面形状与 NULL 缝由库内 CHECK 钉住、租户隔离、无事务
// 拒、回滚无痕。

var (
	intakeOccurredAt  = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	adoptionRecordAt  = time.Date(2026, 10, 5, 9, 5, 0, 0, time.UTC)
	cancelRequestedAt = time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	finalOccurredAt   = time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)
)

func TestAnAdoptionRoundTripsWithItsCommitment(t *testing.T) {
	adoptions, _, _, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()

	record := adoptedRecord(t, "tenant-1", "parcel-1", "SRV-1", "digest-1")
	mustSaveAdoption(t, transactor, ctx, adoptions, record)

	found, present, err := adoptions.FindByKey(ctx, record.Key)
	if err != nil || !present {
		t.Fatalf("按键读回：present=%v err=%v", present, err)
	}
	if !found.Adopted || found.ContentDigest != "digest-1" ||
		found.CustomerAccountID.String() != "customer-a" ||
		found.ShipmentRequestID.String() != "REQ-1" {
		t.Fatalf("记录头部往返变形：%+v", found)
	}
	source := found.Intake.Source()
	if source.Object().String() != "handover-unit-9" ||
		source.Place().String() != "node-A" ||
		source.Control().String() != "custody-basis-3" ||
		!source.OccurredAt().Equal(intakeOccurredAt) {
		t.Fatal("来源五件没有随采用往返")
	}
	if found.Intake.Baseline().String() != "SUB-V1" ||
		!found.Intake.ResponsibilityStart().Equal(intakeOccurredAt) {
		t.Fatal("基线锚或责任起点往返变形")
	}
	if found.Commitment.Version().String() != "CMT-0001" ||
		found.Commitment.Expected().String() != "resolution-1@2026-10-01T00:00:00Z" ||
		!found.Commitment.EffectiveAt().Equal(intakeOccurredAt) {
		t.Fatal("正式承诺没有随采用往返")
	}

	start, started, err := adoptions.FindResponsibilityStart(ctx, psTenant(t, "tenant-1"), record.Key.Parcel)
	if err != nil || !started || start.Key != record.Key {
		t.Fatalf("责任起点读口没有指回采用行：started=%v err=%v", started, err)
	}
}

// TestOneParcelCannotStartResponsibilityTwice 证 AT-PS-049 的库面：第二个采用（另一
// 来源类型、另一版本，绕过编排预检直接写）撞责任起点索引答`已有记录`，先合法形成者
// 保留；不采用记录不占责任起点。
func TestOneParcelCannotStartResponsibilityTwice(t *testing.T) {
	adoptions, _, _, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()

	mustSaveAdoption(t, transactor, ctx, adoptions, adoptedRecord(t, "tenant-1", "parcel-1", "SRV-1", "digest-1"))

	second := adoptedRecord(t, "tenant-1", "parcel-1", "SRV-2", "digest-2")
	second.Key.Kind = domain.OffsitePickupSource
	second = reshapeAdoptedSource(t, second)
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := adoptions.Save(txCtx, second)
		if err != nil {
			return err
		}
		if outcome != ports.IntakeAdoptionAlreadyRecorded {
			t.Fatalf("second adoption outcome = %d, want ALREADY_RECORDED", outcome)
		}
		return nil
	})

	refusal := refusedAdoption(t, "tenant-1", "parcel-1", "SRV-3", "digest-3")
	mustSaveAdoption(t, transactor, ctx, adoptions, refusal)

	start, started, err := adoptions.FindResponsibilityStart(ctx, psTenant(t, "tenant-1"), refusal.Key.Parcel)
	if err != nil || !started {
		t.Fatalf("责任起点读口：started=%v err=%v", started, err)
	}
	if start.Key.Version.String() != "SRV-1" {
		t.Fatal("后到的采用顶掉了先合法形成的责任起点")
	}
}

func TestAdoptionScopesAreInvisibleToEachOther(t *testing.T) {
	adoptions, _, _, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()

	record := adoptedRecord(t, "tenant-1", "parcel-1", "SRV-1", "digest-1")
	mustSaveAdoption(t, transactor, ctx, adoptions, record)

	otherKey := record.Key
	otherKey.TenantID = psTenant(t, "tenant-b")
	if _, present, err := adoptions.FindByKey(ctx, otherKey); err != nil || present {
		t.Errorf("他租户按同名键读到了本租户的采用：present=%v err=%v", present, err)
	}
	if _, started, err := adoptions.FindResponsibilityStart(ctx, psTenant(t, "tenant-b"), record.Key.Parcel); err != nil || started {
		t.Errorf("他租户读到了本租户的责任起点：started=%v err=%v", started, err)
	}
}

// TestCancellationWalksItsThreeKinds 证取消库三走向整行往返：取消成立带决定五件、
// 待处置带越过的收寄版本、拒绝带规则依据；成立的决定经 FindCancellation 读口按包裹
// 读回（采用编排核对边界用的正是它）。
func TestCancellationWalksItsThreeKinds(t *testing.T) {
	_, cancellations, _, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()

	cancelled := cancelledRecord(t, "tenant-1", "req-1", "parcel-1", "digest-1")
	mustSaveCancellation(t, transactor, ctx, cancellations, cancelled)
	mustSaveCancellation(t, transactor, ctx, cancellations, ports.CancellationRecord{
		Key:           cancellationKey(t, "tenant-1", "req-2", "parcel-2"),
		ContentDigest: "digest-2",
		Kind:          ports.RecordDispositionPending,
		IntakeVersion: mustBuild(t, domain.NewSourceResultVersion, "SRV-9"),
		DecidedAt:     adoptionRecordAt,
	})
	mustSaveCancellation(t, transactor, ctx, cancellations, ports.CancellationRecord{
		Key:           cancellationKey(t, "tenant-1", "req-3", "parcel-3"),
		ContentDigest: "digest-3",
		Kind:          ports.RecordCancellationRefused,
		RefusalBasis:  mustBuild(t, domain.NewCheckReason, "AUTHORITY_DENIED/rule-7"),
		DecidedAt:     adoptionRecordAt,
	})

	found, present, err := cancellations.FindByKey(ctx, cancelled.Key)
	if err != nil || !present || found.Kind != ports.RecordParcelCancelled {
		t.Fatalf("取消成立读回：present=%v kind=%v err=%v", present, found.Kind, err)
	}
	if found.Cancellation.ID().String() != "CAN-0001" ||
		found.Cancellation.Authority().String() != "cancel-rule-3" ||
		!found.Cancellation.RequestedAt().Equal(cancelRequestedAt) {
		t.Fatal("取消决定往返变形")
	}

	pending, present, err := cancellations.FindByKey(ctx, cancellationKey(t, "tenant-1", "req-2", "parcel-2"))
	if err != nil || !present || pending.Kind != ports.RecordDispositionPending ||
		pending.IntakeVersion.String() != "SRV-9" {
		t.Fatal("待处置记录（越过的收寄版本）往返变形")
	}

	refused, present, err := cancellations.FindByKey(ctx, cancellationKey(t, "tenant-1", "req-3", "parcel-3"))
	if err != nil || !present || refused.Kind != ports.RecordCancellationRefused ||
		refused.RefusalBasis.String() != "AUTHORITY_DENIED/rule-7" {
		t.Fatal("拒绝记录往返变形")
	}

	decision, cancelledFound, err := cancellations.FindCancellation(ctx, psTenant(t, "tenant-1"), cancelled.Key.Parcel)
	if err != nil || !cancelledFound || decision.ID().String() != "CAN-0001" {
		t.Fatalf("按包裹读取消决定：found=%v err=%v", cancelledFound, err)
	}
	if _, otherFound, err := cancellations.FindCancellation(ctx, psTenant(t, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-2")); err != nil || otherFound {
		t.Error("待处置记录被当成了已成立的取消决定")
	}
}

// TestCancellationReplayKeepsTheFirstDecision 证同请求键第二份（内容相反）答`已有
// 记录`且不覆盖，撞键后同事务读回赢家。
func TestCancellationReplayKeepsTheFirstDecision(t *testing.T) {
	_, cancellations, _, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()

	first := cancelledRecord(t, "tenant-1", "req-1", "parcel-1", "digest-1")
	mustSaveCancellation(t, transactor, ctx, cancellations, first)

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		forged := ports.CancellationRecord{
			Key:           first.Key,
			ContentDigest: "digest-forged",
			Kind:          ports.RecordCancellationRefused,
			RefusalBasis:  mustBuild(t, domain.NewCheckReason, "AUTHORITY_DENIED/rule-7"),
			DecidedAt:     adoptionRecordAt,
		}
		outcome, err := cancellations.Save(txCtx, forged)
		if err != nil {
			return err
		}
		if outcome != ports.CancellationAlreadyRecorded {
			t.Fatalf("replay outcome = %d, want ALREADY_RECORDED", outcome)
		}
		winner, present, err := cancellations.FindByKey(txCtx, first.Key)
		if err != nil || !present {
			t.Fatalf("撞键后同事务读回失败：present=%v err=%v", present, err)
		}
		if winner.Kind != ports.RecordParcelCancelled {
			t.Fatal("迟到的拒绝覆盖了先到的取消")
		}
		return nil
	})
}

func TestAFinalOutcomeRoundTripsAndRederivesInPlace(t *testing.T) {
	_, _, finals, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()

	first := firstFinalRecord(t, "tenant-1", "parcel-1", "ORV-1", "digest-1")
	mustSaveFinal(t, transactor, ctx, finals, first)

	current, present, err := finals.FindCurrentFinal(ctx, psTenant(t, "tenant-1"), first.Key.Parcel)
	if err != nil || !present || !current.Finalized {
		t.Fatalf("当前终局读口：present=%v err=%v", present, err)
	}
	if current.Final.Version().String() != "FIN-0001" ||
		current.Final.Kind().String() != "DELIVERED_FINAL" ||
		current.Final.RuleVersion().String() != "final-rule-v2" ||
		!current.Final.EffectiveAt().Equal(finalOccurredAt) {
		t.Fatalf("终局往返变形：%+v", current.Final)
	}
	if _, rederived := current.Final.PriorVersion(); rederived {
		t.Fatal("首派生长出了前版")
	}

	// 同源新版本重派生：新行接过当前位、prior 指回前版、原版本行保留（AT-PS-063）。
	rederived := rederivedFinalRecord(t, current, "ORV-2", "digest-2")
	mustSaveFinal(t, transactor, ctx, finals, rederived)

	next, present, err := finals.FindCurrentFinal(ctx, psTenant(t, "tenant-1"), first.Key.Parcel)
	if err != nil || !present {
		t.Fatalf("重派生后的当前终局：present=%v err=%v", present, err)
	}
	if next.Final.Version().String() != "FIN-0002" || next.Key.Version.String() != "ORV-2" {
		t.Fatalf("当前终局没有换到新版本：%+v", next.Final)
	}
	prior, has := next.Final.PriorVersion()
	if !has || prior.String() != "FIN-0001" {
		t.Fatal("重派生版本没有指回前版")
	}
	if reason, has := next.Final.RederivationBasis(); !has || reason.String() != "SOURCE_REVISED/ORV-2" {
		t.Fatal("重派生原因没有随版本往返")
	}

	original, present, err := finals.FindByKey(ctx, first.Key)
	if err != nil || !present || original.Final.Version().String() != "FIN-0001" {
		t.Fatal("原终局历史没有保留在原版本行上")
	}

	// 从已失效锚重放重派生：翻旧零行答`已有记录`，不再动链。
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		stale := rederivedFinalRecord(t, current, "ORV-3", "digest-3")
		outcome, err := finals.Save(txCtx, stale)
		if err != nil {
			return err
		}
		if outcome != ports.FinalOutcomeAlreadyRecorded {
			t.Fatalf("stale rederive outcome = %d, want ALREADY_RECORDED", outcome)
		}
		return nil
	})
}

// TestARefusedFinalNeverTakesTheCurrentSlot 证不采用记录整行往返且从不占当前位。
func TestARefusedFinalNeverTakesTheCurrentSlot(t *testing.T) {
	_, _, finals, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()

	refusal := ports.FinalOutcomeRecord{
		Key:           finalKey(t, "tenant-1", "parcel-1", domain.ReturnCompletedOutcome, "ORV-9"),
		ContentDigest: "digest-9",
		RefusalBasis:  mustBuild(t, domain.NewCheckReason, "FINAL_ALREADY_FORMED/FIN-0001"),
		AdoptedAt:     adoptionRecordAt,
	}
	mustSaveFinal(t, transactor, ctx, finals, refusal)

	found, present, err := finals.FindByKey(ctx, refusal.Key)
	if err != nil || !present || found.Finalized ||
		found.RefusalBasis.String() != "FINAL_ALREADY_FORMED/FIN-0001" {
		t.Fatalf("不采用记录往返变形：present=%v err=%v", present, err)
	}
	if _, present, err := finals.FindCurrentFinal(ctx, psTenant(t, "tenant-1"), refusal.Key.Parcel); err != nil || present {
		t.Errorf("不采用记录占了当前终局位：present=%v err=%v", present, err)
	}
}

func TestFinalScopesAreInvisibleToEachOther(t *testing.T) {
	_, _, finals, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()

	first := firstFinalRecord(t, "tenant-1", "parcel-1", "ORV-1", "digest-1")
	mustSaveFinal(t, transactor, ctx, finals, first)

	if _, present, err := finals.FindCurrentFinal(ctx, psTenant(t, "tenant-b"), first.Key.Parcel); err != nil || present {
		t.Errorf("他租户读到了本租户的当前终局：present=%v err=%v", present, err)
	}
}

// TestJudgmentShapesArePinnedInTheDatabase 证形状矩阵的库面，四支裸写探针全部走
// NULL 缝（f822e1d 入册的三值逻辑纪律）：采用无承诺、待处置无收寄版本、终局有前版
// 无原因、不采用占当前位。
func TestJudgmentShapesArePinnedInTheDatabase(t *testing.T) {
	_, _, _, _, pool := newJudgmentStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.intake_adoption
			(tenant_id, parcel_id, source_kind, source_version,
			 customer_account_id, shipment_request_id, content_digest, adopted,
			 source_object, source_place, source_control, occurred_at,
			 baseline_version, commitment_version, expected_commitment, adopted_at)
		 VALUES ('tenant-1', 'parcel-x', 'NODE_INTAKE', 'SRV-1',
		         'customer-a', 'REQ-1', 'digest-x', true,
		         'obj', 'place', 'ctl', now(),
		         'SUB-V1', NULL, NULL, now())`); err == nil {
		t.Fatal("一行「采用却没有承诺」按 NULL 溜进了采认库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.parcel_cancellation
			(tenant_id, request_key, parcel_id, content_digest, kind,
			 crossed_intake_version, decided_at)
		 VALUES ('tenant-1', 'req-x', 'parcel-x', 'digest-x',
		         'DISPOSITION_PENDING', NULL, now())`); err == nil {
		t.Fatal("一行「待处置却没有越过的收寄版本」按 NULL 溜进了取消库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.final_outcome
			(tenant_id, parcel_id, outcome_kind, outcome_version,
			 content_digest, finalized, is_current,
			 final_version, final_kind, rule_version,
			 decision_ref, execution_ref, occurred_at,
			 prior_version, rederivation_reason, adopted_at)
		 VALUES ('tenant-1', 'parcel-x', 'EFFECTIVE_DELIVERY', 'ORV-1',
		         'digest-x', true, true,
		         'FIN-0002', 'DELIVERED_FINAL', 'rule-v1',
		         'decision-1', 'evidence-1', now(),
		         'FIN-0001', NULL, now())`); err == nil {
		t.Fatal("一行「有前版没原因」按 NULL 溜进了终局库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.final_outcome
			(tenant_id, parcel_id, outcome_kind, outcome_version,
			 content_digest, finalized, is_current, refusal_basis, adopted_at)
		 VALUES ('tenant-1', 'parcel-x', 'EFFECTIVE_DELIVERY', 'ORV-2',
		         'digest-x', false, true, 'basis-1', now())`); err == nil {
		t.Fatal("一行不采用记录占了当前终局位")
	}
}

func TestJudgmentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	adoptions, cancellations, finals, _, _ := newJudgmentStores(t)
	ctx := t.Context()

	if _, err := adoptions.Save(ctx, adoptedRecord(t, "tenant-1", "parcel-1", "SRV-1", "digest-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存采用应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := cancellations.Save(ctx, cancelledRecord(t, "tenant-1", "req-1", "parcel-1", "digest-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存取消应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := finals.Save(ctx, firstFinalRecord(t, "tenant-1", "parcel-1", "ORV-1", "digest-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存终局应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestJudgmentRollbackLeavesNothingBehind(t *testing.T) {
	adoptions, cancellations, finals, transactor, _ := newJudgmentStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	adoption := adoptedRecord(t, "tenant-1", "parcel-1", "SRV-1", "digest-1")
	cancellation := cancelledRecord(t, "tenant-1", "req-1", "parcel-2", "digest-2")
	final := firstFinalRecord(t, "tenant-1", "parcel-3", "ORV-1", "digest-3")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := adoptions.Save(txCtx, adoption); err != nil {
			return err
		}
		if _, err := cancellations.Save(txCtx, cancellation); err != nil {
			return err
		}
		if _, err := finals.Save(txCtx, final); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, present, err := adoptions.FindByKey(ctx, adoption.Key); err != nil || present {
		t.Errorf("回滚后采用仍在：present=%v err=%v", present, err)
	}
	if _, present, err := cancellations.FindByKey(ctx, cancellation.Key); err != nil || present {
		t.Errorf("回滚后取消仍在：present=%v err=%v", present, err)
	}
	if _, present, err := finals.FindByKey(ctx, final.Key); err != nil || present {
		t.Errorf("回滚后终局仍在：present=%v err=%v", present, err)
	}
}

// ---- 夹具 ----

func newJudgmentStores(t *testing.T) (
	*adapter.IntakeAdoptions,
	*adapter.ParcelCancellations,
	*adapter.FinalOutcomes,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	adoptions, err := adapter.NewIntakeAdoptions(db)
	if err != nil {
		t.Fatalf("构造采认库：%v", err)
	}
	cancellations, err := adapter.NewParcelCancellations(db)
	if err != nil {
		t.Fatalf("构造取消库：%v", err)
	}
	finals, err := adapter.NewFinalOutcomes(db)
	if err != nil {
		t.Fatalf("构造终局库：%v", err)
	}
	return adoptions, cancellations, finals, db.Transactor(), pool
}

func psTenant(t *testing.T, tenant string) domain.TenantID {
	t.Helper()
	return mustBuild(t, domain.NewTenantID, tenant)
}

func mustSaveAdoption(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	adoptions *adapter.IntakeAdoptions,
	record ports.IntakeAdoptionRecord,
) {
	t.Helper()
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := adoptions.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.IntakeAdoptionSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}

func mustSaveCancellation(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	cancellations *adapter.ParcelCancellations,
	record ports.CancellationRecord,
) {
	t.Helper()
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := cancellations.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.CancellationSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}

func mustSaveFinal(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	finals *adapter.FinalOutcomes,
	record ports.FinalOutcomeRecord,
) {
	t.Helper()
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := finals.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.FinalOutcomeSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}

// adoptedRecord 造一份节点收寄的采用记录：来源五件+基线锚+首版承诺。收寄与承诺经
// 领域构造门形成——门的校验正是「什么算合法」的单一权威。
func adoptedRecord(t *testing.T, tenant, parcel, version, digest string) ports.IntakeAdoptionRecord {
	t.Helper()

	key := ports.IntakeAdoptionKey{
		TenantID: psTenant(t, tenant),
		Parcel:   mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Kind:     domain.NodeIntakeSource,
		Version:  mustBuild(t, domain.NewSourceResultVersion, version),
	}
	record := ports.IntakeAdoptionRecord{
		Key:               key,
		CustomerAccountID: mustBuild(t, domain.NewCustomerAccountID, "customer-a"),
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "REQ-1"),
		ContentDigest:     digest,
		Adopted:           true,
		AdoptedAt:         adoptionRecordAt,
	}
	return fillAdoptedSource(t, record)
}

// fillAdoptedSource 按记录键组收寄与承诺（键改动后重组用 reshapeAdoptedSource）。
func fillAdoptedSource(t *testing.T, record ports.IntakeAdoptionRecord) ports.IntakeAdoptionRecord {
	t.Helper()

	source, err := domain.NewIntakeSource(domain.IntakeSourceSpec{
		Kind:       record.Key.Kind,
		Object:     mustBuild(t, domain.NewSourceObjectReference, "handover-unit-9"),
		Parcel:     record.Key.Parcel,
		Place:      mustBuild(t, domain.NewIntakePlaceReference, "node-A"),
		Control:    mustBuild(t, domain.NewIntakeControlReference, "custody-basis-3"),
		Version:    record.Key.Version,
		OccurredAt: intakeOccurredAt,
	})
	if err != nil {
		t.Fatalf("构造来源：%v", err)
	}
	intake, err := domain.AdoptNetworkIntake(source, mustBuild(t, domain.NewSubmissionVersionID, "SUB-V1"))
	if err != nil {
		t.Fatalf("采用来源：%v", err)
	}
	commitment, err := domain.FormFormalCommitment(
		mustBuild(t, domain.NewCommitmentVersionID, "CMT-0001"),
		intake,
		mustBuild(t, domain.NewExpectedCommitmentReference, "resolution-1@2026-10-01T00:00:00Z"),
	)
	if err != nil {
		t.Fatalf("形成承诺：%v", err)
	}
	record.Intake = intake
	record.Commitment = commitment
	return record
}

func reshapeAdoptedSource(t *testing.T, record ports.IntakeAdoptionRecord) ports.IntakeAdoptionRecord {
	t.Helper()
	return fillAdoptedSource(t, record)
}

func refusedAdoption(t *testing.T, tenant, parcel, version, digest string) ports.IntakeAdoptionRecord {
	t.Helper()
	return ports.IntakeAdoptionRecord{
		Key: ports.IntakeAdoptionKey{
			TenantID: psTenant(t, tenant),
			Parcel:   mustBuild(t, domain.NewDeclaredParcelID, parcel),
			Kind:     domain.NodeIntakeSource,
			Version:  mustBuild(t, domain.NewSourceResultVersion, version),
		},
		CustomerAccountID: mustBuild(t, domain.NewCustomerAccountID, "customer-a"),
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "REQ-1"),
		ContentDigest:     digest,
		RefusalBasis:      mustBuild(t, domain.NewCheckReason, "RESPONSIBILITY_ALREADY_STARTED/NODE_INTAKE/SRV-1"),
		AdoptedAt:         adoptionRecordAt,
	}
}

func cancellationKey(t *testing.T, tenant, requestKey, parcel string) ports.CancellationRequestKey {
	t.Helper()
	return ports.CancellationRequestKey{
		TenantID:   psTenant(t, tenant),
		RequestKey: mustBuild(t, domain.NewSourceRequestKey, requestKey),
		Parcel:     mustBuild(t, domain.NewDeclaredParcelID, parcel),
	}
}

func cancelledRecord(t *testing.T, tenant, requestKey, parcel, digest string) ports.CancellationRecord {
	t.Helper()

	key := cancellationKey(t, tenant, requestKey, parcel)
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustBuild(t, domain.NewParcelCancellationID, "CAN-0001"),
		Parcel:      key.Parcel,
		Requester:   mustBuild(t, domain.NewCancellationRequesterReference, "customer-a"),
		Authority:   mustBuild(t, domain.NewCancellationAuthorityReference, "cancel-rule-3"),
		Reason:      mustBuild(t, domain.NewCancellationReasonReference, "order-withdrawn"),
		RequestedAt: cancelRequestedAt,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("形成取消决定：%v", err)
	}
	return ports.CancellationRecord{
		Key:           key,
		ContentDigest: digest,
		Kind:          ports.RecordParcelCancelled,
		Cancellation:  cancellation,
		DecidedAt:     adoptionRecordAt,
	}
}

func finalKey(
	t *testing.T,
	tenant, parcel string,
	kind domain.ResponsibilityOutcomeKind,
	version string,
) ports.FinalAdoptionKey {
	t.Helper()
	return ports.FinalAdoptionKey{
		TenantID: psTenant(t, tenant),
		Parcel:   mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Kind:     kind,
		Version:  mustBuild(t, domain.NewResponsibilityOutcomeVersion, version),
	}
}

func responsibilityOutcome(t *testing.T, key ports.FinalAdoptionKey) domain.ResponsibilityOutcome {
	t.Helper()
	outcome, err := domain.NewResponsibilityOutcome(domain.ResponsibilityOutcomeSpec{
		Kind:       key.Kind,
		Parcel:     key.Parcel,
		Decision:   mustBuild(t, domain.NewResponsibilityDecisionReference, "delivery-judgment-5"),
		Execution:  mustBuild(t, domain.NewExecutionEvidenceReference, "pod-evidence-5"),
		Version:    key.Version,
		OccurredAt: finalOccurredAt,
	})
	if err != nil {
		t.Fatalf("构造责任结果：%v", err)
	}
	return outcome
}

func firstFinalRecord(t *testing.T, tenant, parcel, version, digest string) ports.FinalOutcomeRecord {
	t.Helper()

	key := finalKey(t, tenant, parcel, domain.EffectiveDeliveryOutcome, version)
	final, err := domain.FormParcelFinalOutcome(domain.ParcelFinalOutcomeSpec{
		Version:     mustBuild(t, domain.NewFinalOutcomeVersionID, "FIN-0001"),
		Parcel:      key.Parcel,
		Kind:        mustBuild(t, domain.NewFinalKindReference, "DELIVERED_FINAL"),
		Source:      responsibilityOutcome(t, key),
		RuleVersion: mustBuild(t, domain.NewFinalRuleVersionReference, "final-rule-v2"),
	})
	if err != nil {
		t.Fatalf("形成终局：%v", err)
	}
	return ports.FinalOutcomeRecord{
		Key:           key,
		ContentDigest: digest,
		Finalized:     true,
		Final:         final,
		AdoptedAt:     adoptionRecordAt,
	}
}

// rederivedFinalRecord 从当前终局经 Rederive 造重派生记录（AT-PS-063 的领域走法）。
func rederivedFinalRecord(
	t *testing.T,
	current ports.FinalOutcomeRecord,
	newSourceVersion, digest string,
) ports.FinalOutcomeRecord {
	t.Helper()

	key := ports.FinalAdoptionKey{
		TenantID: current.Key.TenantID,
		Parcel:   current.Key.Parcel,
		Kind:     current.Key.Kind,
		Version:  mustBuild(t, domain.NewResponsibilityOutcomeVersion, newSourceVersion),
	}
	rederived, err := current.Final.Rederive(
		mustBuild(t, domain.NewFinalOutcomeVersionID, "FIN-0002"),
		responsibilityOutcome(t, key),
		mustBuild(t, domain.NewRederivationReason, "SOURCE_REVISED/"+newSourceVersion),
	)
	if err != nil {
		t.Fatalf("重派生终局：%v", err)
	}
	return ports.FinalOutcomeRecord{
		Key:           key,
		ContentDigest: digest,
		Finalized:     true,
		Final:         rederived,
		AdoptedAt:     adoptionRecordAt,
	}
}
