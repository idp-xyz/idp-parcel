package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证解除履约段采用（迁移 TF 0048，ADR-0232 决定一至三）：解除编排接真采用册、真解除册、真委托册与真发生项册；
// 唯一一段解除后委托撤回到已提交，还有别的段时按其余段最早的首个参与重新派生（可以后移）、解除的不是开始所依的那一段时开始不动；原采用
// 与解除都留在册上；同段可以再采用，是下一代。夹具全为合成登记（S 级）。

const withdrawingOperator = "https://id.syn.example/dex#SYN-OPERATOR-CORRECTION-01"

type withdrawalRig struct {
	stores  adoptionStores
	adopter *application.AdoptSegmentCommissionHandler
	handler *application.WithdrawSegmentCommissionAdoptionHandler
	tenant  domain.TenantID
}

func newWithdrawalRig(t *testing.T) withdrawalRig {
	t.Helper()
	stores := newAdoptionStores(t)
	stores.seed(t, t.Context())
	occurrences, err := adapter.NewChargeOccurrences(stores.db)
	if err != nil {
		t.Fatal(err)
	}
	clock := fixedAdoptionClock{at: segmentEnteredAtFixture.Add(24 * time.Hour)}
	return withdrawalRig{
		stores: stores,
		adopter: application.NewAdoptSegmentCommissionHandler(application.AdoptSegmentCommissionDeps{
			Adoptions: stores.adoptions, Segments: stores.segments, Commissions: stores.commissions,
			StartAdvancement: stores.commissions, CommissionHold: stores.commissions, Withdrawals: stores.adoptions,
			Doorbell: stores.doorbell, Clock: clock,
		}),
		handler: application.NewWithdrawSegmentCommissionAdoptionHandler(application.WithdrawSegmentCommissionAdoptionDeps{
			Adoptions: stores.adoptions, Withdrawals: stores.adoptions, CommissionLinks: stores.adoptions,
			Occurrences: occurrences, OccurrenceScope: occurrences,
			Commissions: stores.commissions, CommissionHold: stores.commissions, StartCorrection: stores.commissions,
			Clock: fixedAdoptionClock{at: segmentEnteredAtFixture.Add(48 * time.Hour)},
		}),
		tenant: deliveryValue(t, domain.NewTenantID, "tenant-1"),
	}
}

// adopt 采用 commission-1；after 是新一代接在哪一代之后，首次采用为零。
func (rig withdrawalRig) adopt(t *testing.T, segment string, after int) application.CommissionAdoptionOutcome {
	t.Helper()
	var result application.CommissionAdoptionResult
	mustWithinDeliveryTransaction(t, rig.stores.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = rig.adopter.Adopt(txCtx, application.AdoptSegmentCommissionCommand{
			TenantID: rig.tenant, Segment: segment, Commission: "commission-1", Provider: "partner-1", AfterGeneration: after,
		})
		return err
	})
	return result.Outcome()
}

func (rig withdrawalRig) withdraw(t *testing.T, segment string, generation int) application.AdoptionWithdrawalResult {
	t.Helper()
	var result application.AdoptionWithdrawalResult
	mustWithinDeliveryTransaction(t, rig.stores.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = rig.handler.Withdraw(txCtx, application.WithdrawSegmentCommissionAdoptionCommand{
			TenantID: rig.tenant, Operator: withdrawingOperator, Segment: segment, Commission: "commission-1", Generation: generation,
		})
		return err
	})
	return result
}

// commissionStart 读委托行此刻的开始；未开始答空串与零值。
func (rig withdrawalRig) commissionStart(t *testing.T) (string, time.Time) {
	t.Helper()
	var basis *string
	var at *time.Time
	if err := rig.stores.pool.QueryRow(t.Context(),
		`SELECT started_basis, started_at FROM transport_fulfillment.transport_commission
		  WHERE tenant_id = 'tenant-1' AND commission_id = 'commission-1'`).Scan(&basis, &at); err != nil {
		t.Fatalf("读委托行：%v", err)
	}
	if basis == nil || at == nil {
		return "", time.Time{}
	}
	return *basis, at.UTC()
}

func (rig withdrawalRig) count(t *testing.T, table string) int {
	t.Helper()
	var rows int
	if err := rig.stores.pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.`+table).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// Covers: AT-TF-306、AT-TF-307、AT-TF-310 的真库面——委托只被这一段采用：解除成立，委托撤回到已提交（两列清空），解除行记下之前的开始
// 与变法 WITHDRAWN；采用行原样留着。原样再交答 EXISTING_WITHDRAWAL、不多一行；同段再采用落成第二代，委托重新开始。
func TestWithdrawingTheOnlyAdoptionWithdrawsTheCommissionStartAndKeepsHistory(t *testing.T) {
	rig := newWithdrawalRig(t)
	if outcome := rig.adopt(t, "SEG-ADOPT", 0); outcome != application.CommissionAdopted {
		t.Fatalf("采用 = %q", outcome)
	}
	result := rig.withdraw(t, "SEG-ADOPT", 1)
	if result.Outcome() != application.AdoptionWithdrawn {
		t.Fatalf("解除 = %q/%q", result.Outcome(), result.UndecidedReason())
	}
	if basis, _ := rig.commissionStart(t); basis != "" {
		t.Fatalf("委托仍是已开始：%s", basis)
	}
	record, _ := result.Withdrawal()
	beforeBasis, beforeAt := record.Withdrawal.StartBefore()
	if record.Withdrawal.Generation() != 1 || record.Withdrawal.StartChange() != domain.CommissionStartWithdrawn ||
		beforeBasis.String() != "OFFSITE-PICKUP/parcel-1" || !beforeAt.Equal(segmentEnteredAtFixture) ||
		record.Withdrawal.WithdrawnBy().String() != withdrawingOperator {
		t.Fatalf("解除 = %+v", record.Withdrawal)
	}
	if rig.count(t, "segment_commission_adoption") != 1 || rig.count(t, "segment_commission_adoption_withdrawal") != 1 {
		t.Fatal("原采用或解除没留在册上")
	}
	key := ports.SegmentCommissionAdoptionKey{TenantID: rig.tenant, Segment: deliveryValue(t, domain.NewFulfillmentSegmentReference, "SEG-ADOPT")}
	if _, found, err := rig.stores.adoptions.FindBySegment(t.Context(), key); err != nil || found {
		t.Fatalf("解除之后仍读得到当前采用：%v %v", found, err)
	}
	if replay := rig.withdraw(t, "SEG-ADOPT", 1); replay.Outcome() != application.AdoptionWithdrawalExisting ||
		rig.count(t, "segment_commission_adoption_withdrawal") != 1 {
		t.Fatalf("重放 = %q", replay.Outcome())
	}

	if outcome := rig.adopt(t, "SEG-ADOPT", 0); outcome != application.CommissionAdoptionAlreadyWithdrawn {
		t.Fatalf("解除之前那一次采用原样重交 = %q, want ADOPTION_ALREADY_WITHDRAWN", outcome)
	}
	if outcome := rig.adopt(t, "SEG-ADOPT", 1); outcome != application.CommissionAdopted {
		t.Fatalf("再采用 = %q", outcome)
	}
	again, found, err := rig.stores.adoptions.FindBySegment(t.Context(), key)
	if err != nil || !found || again.Generation != 2 {
		t.Fatalf("再采用不是第二代：%+v %v", again, err)
	}
	if basis, _ := rig.commissionStart(t); basis != "OFFSITE-PICKUP/parcel-1" {
		t.Fatalf("再采用没有让委托重新开始：%q", basis)
	}
	// 再采用之后，第一代那一次解除原样重交：答重放，不解除第二代、不撤开始。
	if replay := rig.withdraw(t, "SEG-ADOPT", 1); replay.Outcome() != application.AdoptionWithdrawalExisting ||
		rig.count(t, "segment_commission_adoption_withdrawal") != 1 {
		t.Fatalf("再采用之后重放第一代那一次解除 = %q", replay.Outcome())
	}
	if basis, _ := rig.commissionStart(t); basis != "OFFSITE-PICKUP/parcel-1" {
		t.Fatalf("旧解除重放撤了新一代的开始：%q", basis)
	}
}

// Covers: AT-TF-308 的真库面——委托还被另一段采用：解除开始所依的那一段，开始改按其余段最早的首个参与重新派生（比原来晚，ADR-0204
// 决定二「只前移」在这里部分停用）；再解除不是开始所依的段之外的另一段……只剩一段时解除它撤回到已提交。解除不是开始所依的那一段，开始不动。
func TestWithdrawingOneOfSeveralAdoptionsRederivesTheCommissionStart(t *testing.T) {
	rig := newWithdrawalRig(t)
	late := activeMember(t, "parcel-2")
	late.EnteredAt = segmentEnteredAtFixture.Add(2 * time.Hour)
	mustSaveSegment(t, rig.stores.transactor, t.Context(), rig.stores.segments, segmentRecord(t, "SEG-LATE", late))
	if rig.adopt(t, "SEG-ADOPT", 0) != application.CommissionAdopted || rig.adopt(t, "SEG-LATE", 0) != application.CommissionAdopted {
		t.Fatal("两段没都采用上")
	}

	rederived := rig.withdraw(t, "SEG-ADOPT", 1)
	record, _ := rederived.Withdrawal()
	afterBasis, afterAt, present := record.Withdrawal.StartAfter()
	if rederived.Outcome() != application.AdoptionWithdrawn || record.Withdrawal.StartChange() != domain.CommissionStartRederived ||
		!present || afterBasis.String() != "OFFSITE-PICKUP/parcel-2" || !afterAt.Equal(late.EnteredAt) {
		t.Fatalf("重新派生 = %q %+v", rederived.Outcome(), record.Withdrawal)
	}
	if basis, at := rig.commissionStart(t); basis != "OFFSITE-PICKUP/parcel-2" || !at.Equal(late.EnteredAt) {
		t.Fatalf("委托的开始没有后移到剩下那一段：(%s, %s)", basis, at)
	}

	// 再采用 SEG-ADOPT：开始前移回它；然后解除不是开始所依的 SEG-LATE，开始不动。
	if rig.adopt(t, "SEG-ADOPT", 1) != application.CommissionAdopted {
		t.Fatal("再采用 SEG-ADOPT 不成")
	}
	kept := rig.withdraw(t, "SEG-LATE", 1)
	keptRecord, _ := kept.Withdrawal()
	if kept.Outcome() != application.AdoptionWithdrawn || keptRecord.Withdrawal.StartChange() != domain.CommissionStartUnchanged {
		t.Fatalf("解除不是开始所依的段 = %q %+v", kept.Outcome(), keptRecord.Withdrawal)
	}
	if basis, at := rig.commissionStart(t); basis != "OFFSITE-PICKUP/parcel-1" || !at.Equal(segmentEnteredAtFixture) {
		t.Fatalf("开始被动了：(%s, %s)", basis, at)
	}
	if last := rig.withdraw(t, "SEG-ADOPT", 2); last.Outcome() != application.AdoptionWithdrawn {
		t.Fatalf("解除最后一段 = %q", last.Outcome())
	}
	if basis, _ := rig.commissionStart(t); basis != "" {
		t.Fatal("一段不剩时委托没撤回到已提交")
	}
}

// 解除册与采用册的库面纪律：第二代必须接在已解除的上一代后面——绕过编排直接插第二代，外键挡住。
func TestASecondGenerationNeedsTheFirstWithdrawn(t *testing.T) {
	rig := newWithdrawalRig(t)
	if rig.adopt(t, "SEG-ADOPT", 0) != application.CommissionAdopted {
		t.Fatal("采用不成")
	}
	_, err := rig.stores.pool.Exec(t.Context(),
		`INSERT INTO transport_fulfillment.segment_commission_adoption
		     (tenant_id, segment_ref, generation, previous_generation, commission_id, provider_ref, start_basis, started_at,
		      content_digest, recorded_at)
		 SELECT tenant_id, segment_ref, 2, 1, commission_id, provider_ref, start_basis, started_at, content_digest, recorded_at
		   FROM transport_fulfillment.segment_commission_adoption WHERE segment_ref = 'SEG-ADOPT'`)
	if err == nil {
		t.Fatal("上一代没解除就插进了第二代")
	}
}

func TestWithdrawalWritesRefuseToRunOutsideATransaction(t *testing.T) {
	stores := newAdoptionStores(t)
	ctx := t.Context()
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")
	commissionKey := ports.TransportCommissionKey{TenantID: tenant, Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1")}
	// 锁随事务释放：事务外取的锁在语句结束即放，等于没取。
	if err := stores.commissions.HoldCommissionStart(ctx, commissionKey); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务取开始锁应返回 ErrTransactionRequired，实得：%v", err)
	}
	record := submittedCommission(t, "tenant-1", "commission-1")
	if _, err := stores.commissions.CorrectStart(ctx, record,
		deliveryValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/parcel-1"), segmentEnteredAtFixture); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务改开始应返回 ErrTransactionRequired，实得：%v", err)
	}
	withdrawal, err := domain.RehydrateSegmentCommissionAdoptionWithdrawal(domain.RehydrateSegmentCommissionAdoptionWithdrawalSpec{
		TenantID: tenant, Segment: deliveryValue(t, domain.NewFulfillmentSegmentReference, "SEG-ADOPT"), Generation: 1,
		Commission: commissionKey.Commission, WithdrawnBy: deliveryValue(t, domain.NewRegistrantReference, withdrawingOperator),
		StartBeforeBasis: deliveryValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/parcel-1"),
		StartBeforeAt:    segmentEnteredAtFixture, StartChange: domain.CommissionStartWithdrawn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores.adoptions.SaveWithdrawal(ctx, ports.SegmentCommissionAdoptionWithdrawalRecord{
		Key:        ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: withdrawal.Segment()},
		Withdrawal: withdrawal, RecordedAt: segmentEnteredAtFixture,
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务落解除应返回 ErrTransactionRequired，实得：%v", err)
	}
}
