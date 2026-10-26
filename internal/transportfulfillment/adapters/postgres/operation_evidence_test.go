package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ADR-0303 的存储半边（迁移 0051）：七个运营决定口的操作证据列落得进、读得回；同一个
// 决定换一位操作者再写（撞键、窄口写空）不顶替在册的那一位；没有操作者的行（存量，或不经运营决定口形成的）读回是
// 「没记下」而不是空串；库面 CHECK 挡空白，挡操作者出现在不能属于的行上。夹具全为合成登记（S 级）。

const (
	operatorADB = "https://id.syn.example/dex#SYN-OPERATOR-A"
	operatorBDB = "https://id.syn.example/dex#SYN-OPERATOR-B"
)

func operatorDB(t *testing.T, raw string) domain.RegistrantReference {
	t.Helper()
	return segmentRef(t, domain.NewRegistrantReference, raw)
}

// readColumn 读一格可空文本：nil 即库面 NULL。
func readColumn(t *testing.T, pool *pgxpool.Pool, query string, args ...any) *string {
	t.Helper()
	var value *string
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("读 %q：%v", query, err)
	}
	return value
}

func wantColumn(t *testing.T, got *string, want string, what string) {
	t.Helper()
	switch {
	case want == "" && got != nil:
		t.Fatalf("%s = %q，want NULL", what, *got)
	case want != "" && (got == nil || *got != want):
		t.Fatalf("%s = %v，want %q", what, got, want)
	}
}

// wantCheckViolation 断言一条写被 0051 立的那道 CHECK 拒下。
func wantCheckViolation(t *testing.T, err error, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != constraint {
		t.Fatalf("err = %v，want 23514 / %s", err, constraint)
	}
}

// Covers: `AT-TF-337`、`AT-TF-338` 存储半边——关段人与终止人经窄口落列、随段读回；另一位操作者再关、再终止写中 0 行，在册
// 的不被顶替；以交付结束的参与终止人是 NULL；来源更正长出的替代版本按锁后的在册终点承继终止人（与终点三件同一次读）；
// CHECK 挡关段人落在没关的段上、终止人落在不是明确终止的参与上、两列写空白。
func TestTheSegmentRegistryKeepsTheClosingAndTerminatingOperators(t *testing.T) {
	repository, transactor, pool := newSegmentRegistry(t)
	ctx := t.Context()
	key := segmentKeyFixture(t, "tenant-1", "SEG-OE1")
	root := activeMember(t, "parcel-1")
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-OE1", root, deliveredMember(t, "parcel-2")))
	staleTail := rebuiltChainTail(t, "SEG-OE1", root, supersedingMember(t, root, "v2", segmentEnteredAtFixture.Add(time.Hour)))

	terminated := root
	terminated.EndKind = domain.EndedByControlTermination
	terminated.EndBasis = segmentRef(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/case-OE1")
	terminated.EndedAt = segmentEnteredAtFixture.Add(3 * time.Hour)
	end := func(by string) ports.ParticipationEndOutcome {
		member := terminated
		member.TerminatedBy = operatorDB(t, by)
		var outcome ports.ParticipationEndOutcome
		mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = repository.EndParticipation(txCtx, key, rebuiltParticipation(t, "SEG-OE1", member))
			return err
		})
		return outcome
	}
	if outcome := end(operatorADB); outcome != ports.ParticipationEnded {
		t.Fatalf("终止 outcome = %d", outcome)
	}
	if outcome := end(operatorBDB); outcome != ports.ParticipationAlreadyEnded {
		t.Fatalf("另一位操作者再终止 outcome = %d，want ParticipationAlreadyEnded", outcome)
	}
	participationColumn := `SELECT terminated_by FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-OE1' AND object_ref = $1 AND entry_basis = $2`
	wantColumn(t, readColumn(t, pool, participationColumn, "parcel-1", root.EntryBasis.String()), operatorADB, "终止人")
	wantColumn(t, readColumn(t, pool, participationColumn, "parcel-2", "OFFSITE-PICKUP/parcel-2"), "", "交付结束的终止人")

	var superseded ports.ParticipationSupersedeOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		superseded, err = repository.Supersede(txCtx, key, staleTail, segmentEnteredAtFixture.Add(4*time.Hour))
		return err
	})
	if superseded != ports.ParticipationSuperseded {
		t.Fatalf("替代 outcome = %d", superseded)
	}
	wantColumn(t, readColumn(t, pool, participationColumn, "parcel-1", staleTail.EntryBasis().String()), operatorADB, "替代版本承继的终止人")

	closeSegment := func(by string) ports.SegmentCloseOutcome {
		var outcome ports.SegmentCloseOutcome
		mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = repository.CloseSegment(txCtx, key, segmentEnteredAtFixture.Add(9*time.Hour), operatorDB(t, by))
			return err
		})
		return outcome
	}
	if outcome := closeSegment(operatorADB); outcome != ports.SegmentClosed {
		t.Fatalf("关段 outcome = %d", outcome)
	}
	if outcome := closeSegment(operatorBDB); outcome != ports.SegmentAlreadyClosed {
		t.Fatalf("另一位操作者再关 outcome = %d，want SegmentAlreadyClosed", outcome)
	}

	found, _, err := repository.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	if by, recorded := found.Segment.ClosedBy(); !recorded || by.String() != operatorADB {
		t.Fatalf("读回关段人 = %q (%v)，want %q", by.String(), recorded, operatorADB)
	}
	tail, _ := found.Segment.ParticipationFor(root.Object)
	if by, recorded := tail.TerminatedBy(); !recorded || by.String() != operatorADB {
		t.Fatalf("读回链尾终止人 = %q (%v)，want %q", by.String(), recorded, operatorADB)
	}
	delivered, _ := found.Segment.ParticipationFor(segmentRef(t, domain.NewCarriedObjectReference, "parcel-2"))
	if _, recorded := delivered.TerminatedBy(); recorded {
		t.Fatal("交付结束的参与读回不该有终止人")
	}

	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-OE2", activeMember(t, "parcel-3")))
	_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.actual_fulfillment_segment SET closed_by = $1
		 WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-OE2'`, operatorADB)
	wantCheckViolation(t, err, "actual_fulfillment_segment_closed_by_only_when_closed")
	_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.actual_fulfillment_segment SET closed_by = '  '
		 WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-OE1'`)
	wantCheckViolation(t, err, "actual_fulfillment_segment_closed_by_only_when_closed")
	for name, object := range map[string]string{"仍在场": "parcel-3", "交付结束": "parcel-2"} {
		_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.fulfillment_participation SET terminated_by = $1
			 WHERE tenant_id = 'tenant-1' AND object_ref = $2`, operatorADB, object)
		if err == nil {
			t.Fatalf("%s的参与收下了终止人", name)
		}
		wantCheckViolation(t, err, "fulfillment_participation_terminated_by_only_when_terminated")
	}
}

// Covers: `AT-TF-341`、`AT-TF-336`、`AT-TF-335` 存储半边——建任务人、分配人、提交人随首登落列、读回；同键第二笔写（另一位
// 操作者）撞键不改；不带操作者的行（执行器形成的任务、规则生效之前的行）落 NULL、读回没记下；取消委托的 Replace 不碰提交人；
// 三列都拒空白。
func TestTheRegistryRecordsKeepTheirDecidingOperator(t *testing.T) {
	t.Run("dispatch task", func(t *testing.T) {
		repository, transactor, pool := newDispatchTasks(t)
		ctx := t.Context()
		record := dispatchTaskRecord(t, "DT-OE1", "parcel-1")
		record.OpenedBy = operatorDB(t, operatorADB)
		mustSaveDispatchTask(t, transactor, ctx, repository, record)
		replay := record
		replay.OpenedBy = operatorDB(t, operatorBDB)
		mustSaveDispatchTask(t, transactor, ctx, repository, replay)
		found, _, err := repository.FindByKey(ctx, record.Key)
		if err != nil || found.OpenedBy.String() != operatorADB {
			t.Fatalf("读回建任务人 = %q err=%v，want %q", found.OpenedBy.String(), err, operatorADB)
		}

		unattributed := dispatchTaskRecord(t, "DT-OE2", "parcel-2")
		mustSaveDispatchTask(t, transactor, ctx, repository, unattributed)
		wantColumn(t, readColumn(t, pool, `SELECT opened_by FROM transport_fulfillment.dispatch_task WHERE task_ref = 'DT-OE2'`), "", "不带操作者的建任务人")
		if found, _, err := repository.FindByKey(ctx, unattributed.Key); err != nil || found.OpenedBy.Present() {
			t.Fatalf("不带操作者读回 = %q err=%v", found.OpenedBy.String(), err)
		}
		_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.dispatch_task SET opened_by = ' ' WHERE task_ref = 'DT-OE2'`)
		wantCheckViolation(t, err, "dispatch_task_opened_by_not_blank")
	})

	t.Run("load assignment", func(t *testing.T) {
		repository, transactor, pool := newLoadAssignments(t)
		ctx := t.Context()
		record := loadAssignmentRecord(t, "LA-OE1", "LAV-OE1", "parcel-1")
		record.DecidedBy = operatorDB(t, operatorADB)
		mustSaveLoadAssignment(t, transactor, ctx, repository, record)
		replay := record
		replay.DecidedBy = operatorDB(t, operatorBDB)
		mustSaveLoadAssignment(t, transactor, ctx, repository, replay)
		found, _, err := repository.FindByKey(ctx, record.Key)
		if err != nil || found.DecidedBy.String() != operatorADB {
			t.Fatalf("读回分配人 = %q err=%v，want %q", found.DecidedBy.String(), err, operatorADB)
		}

		unattributed := loadAssignmentRecord(t, "LA-OE2", "LAV-OE2", "parcel-2")
		mustSaveLoadAssignment(t, transactor, ctx, repository, unattributed)
		wantColumn(t, readColumn(t, pool, `SELECT decided_by FROM transport_fulfillment.load_assignment WHERE assignment_ref = 'LA-OE2'`), "", "不带操作者的分配人")
		_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.load_assignment SET decided_by = '' WHERE assignment_ref = 'LA-OE2'`)
		wantCheckViolation(t, err, "load_assignment_decided_by_not_blank")
	})

	t.Run("transport commission", func(t *testing.T) {
		commissions, _, _, transactor, pool := newCommissionStores(t)
		ctx := t.Context()
		record := submittedCommission(t, "tenant-a", "commission-OE1")
		record.SubmittedBy = operatorDB(t, operatorADB)
		save := func(record ports.TransportCommissionRecord) ports.CommissionSaveOutcome {
			var outcome ports.CommissionSaveOutcome
			mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
				var err error
				outcome, err = commissions.Save(txCtx, record)
				return err
			})
			return outcome
		}
		if outcome := save(record); outcome != ports.CommissionSaved {
			t.Fatalf("提交 outcome = %d", outcome)
		}
		replay := record
		replay.SubmittedBy = operatorDB(t, operatorBDB)
		if outcome := save(replay); outcome != ports.CommissionAlreadyRegistered {
			t.Fatalf("另一位操作者再交 outcome = %d", outcome)
		}

		found, _, err := commissions.FindByKey(ctx, record.Key)
		if err != nil || found.SubmittedBy.String() != operatorADB {
			t.Fatalf("读回提交人 = %q err=%v，want %q", found.SubmittedBy.String(), err, operatorADB)
		}
		cancelled, err := found.Commission.Cancel(commissionAt.Add(time.Hour))
		if err != nil {
			t.Fatalf("cancel：%v", err)
		}
		replaced := found
		replaced.Commission = cancelled
		replaced.SubmittedBy = operatorDB(t, operatorBDB)
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			_, err := commissions.Replace(txCtx, replaced)
			return err
		})
		wantColumn(t, readColumn(t, pool, `SELECT submitted_by FROM transport_fulfillment.transport_commission WHERE commission_id = 'commission-OE1'`),
			operatorADB, "取消之后的提交人")

		if outcome := save(submittedCommission(t, "tenant-a", "commission-OE2")); outcome != ports.CommissionSaved {
			t.Fatalf("不带操作者提交 outcome = %d", outcome)
		}
		wantColumn(t, readColumn(t, pool, `SELECT submitted_by FROM transport_fulfillment.transport_commission WHERE commission_id = 'commission-OE2'`), "", "不带操作者的提交人")
		_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.transport_commission SET submitted_by = ' ' WHERE commission_id = 'commission-OE2'`)
		wantCheckViolation(t, err, "transport_commission_submitted_by_not_blank")
	})
}

// Covers: `AT-TF-340`、`AT-TF-339` 存储半边——显式判断那一版的判断人与收寄判断每一版的判断人落列、经按键读、当前版读与查阅
// 读面读回；素材到达形成的版本判断人是 NULL，CHECK 挡判断人落在不是显式判断的版本上；同版本第二笔写不顶替；两列都拒空白。
func TestTheJudgmentRecordsKeepTheirJudgeAndTheReviewFacesShowIt(t *testing.T) {
	t.Run("effective time judgment", func(t *testing.T) {
		repository, transactor, pool := newExternalTrackingFacts(t)
		ctx := t.Context()
		pending := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-OE1", version: "EXTV-OE1", event: "evt-OE1"})
		mustSaveTrackingFact(t, transactor, ctx, repository, pending)
		judgment, err := domain.JudgeEffectiveTimeExplicitly(trackingReceivedAtDB)
		if err != nil {
			t.Fatal(err)
		}
		judgedFact, err := pending.Fact.JudgeEffectiveTime(judgment, segmentRef(t, domain.NewExternalTrackingFactVersion, "EXTV-OE2"))
		if err != nil {
			t.Fatal(err)
		}
		judged := ports.ExternalTrackingFactRecord{
			Key:               ports.ExternalTrackingFactKey{TenantID: pending.Key.TenantID, Fact: pending.Key.Fact, Version: judgedFact.Version()},
			Fact:              judgedFact,
			RecordedAt:        pending.RecordedAt.Add(time.Minute),
			EffectiveJudgedBy: operatorDB(t, operatorADB),
		}
		if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, judged); outcome != ports.ExternalTrackingFactSaved {
			t.Fatalf("判断版本 outcome = %d", outcome)
		}
		replay := judged
		replay.EffectiveJudgedBy = operatorDB(t, operatorBDB)
		if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, replay); outcome != ports.ExternalTrackingFactAlreadyRegistered {
			t.Fatalf("另一位操作者同版本再写 outcome = %d", outcome)
		}

		current, found, err := repository.FindCurrent(ctx, pending.Key.TenantID, pending.Key.Fact)
		if err != nil || !found || current.EffectiveJudgedBy.String() != operatorADB {
			t.Fatalf("当前版读回判断人 = %q err=%v，want %q", current.EffectiveJudgedBy.String(), err, operatorADB)
		}
		material, _, err := repository.FindByKey(ctx, pending.Key)
		if err != nil || material.EffectiveJudgedBy.Present() {
			t.Fatalf("素材版本读回判断人 = %q err=%v，want 没记下", material.EffectiveJudgedBy.String(), err)
		}
		rows, err := repository.ListCurrentExternalTrackingFacts(ctx, pending.Key.TenantID,
			segmentRef(t, domain.NewTrackingSourceReference, "aggregator-a"), ports.EveryCurrentVersion, 10)
		if err != nil || len(rows) != 1 || rows[0].EffectiveJudgedBy != operatorADB {
			t.Fatalf("查阅读面：%v %+v，want 一行、判断人 %q", err, rows, operatorADB)
		}

		_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.external_carrier_tracking_fact SET effective_judged_by = $1
			 WHERE fact_ref = 'EXTF-OE1' AND version = 'EXTV-OE1'`, operatorADB)
		wantCheckViolation(t, err, "external_carrier_tracking_fact_judged_by_only_when_explicit")
		_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.external_carrier_tracking_fact SET effective_judged_by = ' '
			 WHERE fact_ref = 'EXTF-OE1' AND version = 'EXTV-OE2'`)
		wantCheckViolation(t, err, "external_carrier_tracking_fact_judged_by_only_when_explicit")
	})

	t.Run("carrier first effective pickup judgment", func(t *testing.T) {
		repository, transactor, _, pool := newCarrierPickups(t)
		ctx := t.Context()
		record := carrierPickupRecord(formedCarrierPickupDB(t, "CFEP-OE1", "CFEV-OE1", "PCL-OE1"), carrierPickupJudgedAtDB)
		record.JudgedBy = operatorDB(t, operatorADB)
		if outcome := mustSaveCarrierPickup(t, transactor, ctx, repository, record); outcome != ports.CarrierPickupSaved {
			t.Fatalf("判断 outcome = %d", outcome)
		}
		replay := record
		replay.JudgedBy = operatorDB(t, operatorBDB)
		if outcome := mustSaveCarrierPickup(t, transactor, ctx, repository, replay); outcome != ports.CarrierPickupAlreadyRegistered {
			t.Fatalf("另一位操作者同版本再写 outcome = %d", outcome)
		}
		chain, err := repository.ListByObject(ctx, record.Key.TenantID, segmentRef(t, domain.NewCarriedObjectReference, "PCL-OE1"))
		if err != nil || len(chain) != 1 || chain[0].JudgedBy.String() != operatorADB {
			t.Fatalf("链读回：%v %+v，want 一版、判断人 %q", err, chain, operatorADB)
		}

		unattributed := carrierPickupRecord(formedCarrierPickupDB(t, "CFEP-OE2", "CFEV-OE2", "PCL-OE2"), carrierPickupJudgedAtDB)
		mustSaveCarrierPickup(t, transactor, ctx, repository, unattributed)
		wantColumn(t, readColumn(t, pool, `SELECT judged_by FROM transport_fulfillment.carrier_first_effective_pickup WHERE fact_ref = 'CFEP-OE2'`), "", "不带操作者的判断人")
		if current, _, err := repository.FindCurrentByObject(ctx, unattributed.Key.TenantID, unattributed.Pickup.Object()); err != nil || current.JudgedBy.Present() {
			t.Fatalf("不带操作者读回 = %q err=%v", current.JudgedBy.String(), err)
		}
		_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.carrier_first_effective_pickup SET judged_by = '' WHERE fact_ref = 'CFEP-OE2'`)
		wantCheckViolation(t, err, "carrier_first_effective_pickup_judged_by_not_blank")
	})
}
