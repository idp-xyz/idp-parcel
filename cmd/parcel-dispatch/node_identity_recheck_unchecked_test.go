package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 16 证落未核对行时自投的重核门铃经生产装配续得上（ADR-0199 决定三第三个触发、决定五，票
// no-reception-source/05）：路由表认得出这一类、投给 NO 同一个重核消费者；门铃指名的那一件在 PS 恢复后重核落下关联，PS 仍读
// 不动时交回未决、册上不追加。夹具全为合成（S 级）。

const (
	uncheckedDoorbellType = "node-operations.unchecked-pending-identity.recorded"
	uncheckedMark         = "SYN-CUSTOMER-PARCEL-NRS05"
	uncheckedSource       = "SYN-DELIVERY-NRS05"
	uncheckedUnit         = "SYN-UNIT-NRS05"
)

// Covers: 路由表认得出自投门铃这一类，投给重核消费者。手法同 PS 门铃那一条：毒丸载荷让消费门显式拒收入账并交回 nil，于是这一条
// 定稿；漏挂或挂错撞的是无订阅者。
func TestAnUncheckedPendingIdentityDoorbellReachesTheRecheckConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "unchecked-doorbell-1", noinbox.UncheckedPendingIdentityRecordedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把自投门铃投给重核消费者",
			published, recordedFailureCode(t, db, "unchecked-doorbell-1"))
	}
}

// Covers: NO 消费者自写的门铃类型串与 NO outbox 适配器写出的那一个相等。两串分属同一上下文的两个适配器包，照 PS 门铃那一条的
// 手法各写一份、在组合根钉住。
func TestTheRecheckConsumerAndTheUncheckedDoorbellAgreeOnTheEventType(t *testing.T) {
	if string(noinbox.UncheckedPendingIdentityRecordedEventType) != nopostgres.UncheckedPendingIdentityRecordedEventType ||
		nopostgres.UncheckedPendingIdentityRecordedEventType != uncheckedDoorbellType {
		t.Fatalf("NO 消费者认 %q，NO 门铃写出 %q", noinbox.UncheckedPendingIdentityRecordedEventType, nopostgres.UncheckedPendingIdentityRecordedEventType)
	}
}

// Covers: 票 no-reception-source/05 真库用例二（票 01 点名）——「关联早于读不动形成」那一格由自投门铃续上，不经人工：委托接受、
// 号码的关联形成（PS 门铃那一拍投过去时册上还没有这件实物，什么也不做）→ PS 外部标识关联变更册挪走，这件实物经生产适配器
// 收寄落成未核对的待识别，同一笔入队门铃一封 → 一拍：门铃投给重核消费者，身份核对仍读不动，记未决、inbox 无账、册上不追加、
// 意图为零 → 挪回之后门铃重投：追加一版`节点收寄已形成`（前版是未核对那一版、来源标识由它派生、关联那件包裹），业务时间是原
// 收寄时刻、控制不另起，收寄意图一封、指回追加版。
func TestTheWiredDispatchRechecksAnUncheckedPendingUnitByItsOwnDoorbell(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
	receivedAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)

	submitWithCustomerParcelNumber(t, fixture, uncheckedMark)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("PS 门铃那一拍：%v", err)
	}

	restore := moveRelationChanges(t, fixture.pool)
	unchecked := receiveUncheckedForRecheck(t, fixture.db, tenant, receivedAt)
	doorbells := fixture.outboxIDsOfType(t, uncheckedDoorbellType)
	if len(doorbells) != 1 {
		t.Fatalf("未核对行落下后门铃 %d 封, want 1", len(doorbells))
	}
	rows := fixture.countSQL(t, `SELECT count(*) FROM node_operations.reception`)

	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("PS 仍读不动那一拍：%v", err)
	}
	if got := recordedFailureCode(t, fixture.db, doorbells[0]); got != "dispatch.consumer_undecided" {
		t.Fatalf("门铃失败码 = %q, want dispatch.consumer_undecided——身份核对仍读不动是在等依赖", got)
	}
	if n := fixture.countInbox(t, recheckConsumerName, doorbells[0]); n != 0 {
		t.Fatalf("未决的门铃留下了 %d 行 inbox", n)
	}
	if after := fixture.countSQL(t, `SELECT count(*) FROM node_operations.reception`); after != rows {
		t.Fatalf("PS 仍读不动，册上 %d → %d 行, want 不追加", rows, after)
	}
	if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != 0 {
		t.Fatalf("PS 仍读不动时交了 %d 封收寄意图", n)
	}

	restore()
	for attempt := 0; fixture.countInbox(t, recheckConsumerName, doorbells[0]) == 0; attempt++ {
		if attempt == 20 {
			t.Fatalf("挪回之后门铃 %d 拍仍未处理，失败码 %q", attempt, recordedFailureCode(t, fixture.db, doorbells[0]))
		}
		time.Sleep(5 * time.Millisecond)
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("PS 恢复后那一拍：%v", err)
		}
	}

	receptions, err := nopostgres.NewReceptions(fixture.db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	derived := noports.ReceptionKey{TenantID: tenant, SourceID: "IDENTITY-RECHECK/" + unchecked.JudgmentVersion.String()}
	formed, found, err := receptions.FindByKey(ctx, derived)
	if err != nil || !found {
		t.Fatalf("追加版没落册：found = %v err = %v", found, err)
	}
	association, associated := formed.Intake.Association()
	switch {
	case formed.Kind != noports.RecordIntakeFormed || !associated || association.String() != "SYN-PARCEL-01":
		t.Fatalf("追加版 = %+v, want 节点收寄已形成、关联 SYN-PARCEL-01", formed)
	case formed.Predecessor != unchecked.JudgmentVersion:
		t.Fatalf("追加版前版 = %s, want 未核对那一版 %s", formed.Predecessor, unchecked.JudgmentVersion)
	case !formed.Intake.ReceivedAt().Equal(receivedAt) || formed.Control.Basis() != unchecked.Control.Basis():
		t.Fatalf("追加版接收 %s 控制 %s, want 照原收寄 %s、%s", formed.Intake.ReceivedAt(), formed.Control.Basis(), receivedAt, unchecked.Control.Basis())
	}
	intents := intakeIntentSources(t, fixture.db)
	if len(intents) != 1 || intents[0] != derived.SourceID {
		t.Fatalf("收寄意图 = %v, want 恰一封、指回追加版 %s", intents, derived.SourceID)
	}
	original, found, err := receptions.FindByKey(ctx, unchecked.Key)
	if err != nil || !found || original.IdentityCheck != noports.IdentityUnchecked {
		t.Fatalf("未核对的原行没有原样留着：%+v（found %v err %v）", original, found, err)
	}
}

// moveRelationChanges 把 PS 外部标识关联变更册挪到别的名字下，身份核对读口于是报 SQL 级错误——真库上能造出的最接近「PS 读
// 不动」的形状，手法同 parcel-api 的 moveTable。
func moveRelationChanges(t *testing.T, pool *pgxpool.Pool) (restore func()) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `ALTER TABLE parcel_shipment.external_identifier_relation_change RENAME TO external_identifier_relation_change_moved_nrs05`); err != nil {
		t.Fatalf("挪走关联变更册：%v", err)
	}
	return func() {
		t.Helper()
		if _, err := pool.Exec(t.Context(), `ALTER TABLE parcel_shipment.external_identifier_relation_change_moved_nrs05 RENAME TO external_identifier_relation_change`); err != nil {
			t.Fatalf("挪回关联变更册：%v", err)
		}
	}
}

// receiveUncheckedForRecheck 经收寄编排（生产适配器，身份读口照生产包一层 savepoint）在身份核对读不动时收一件实物，落成未核对的
// 待识别并自投门铃。
func receiveUncheckedForRecheck(t *testing.T, db *bentopg.DB, tenant nodomain.TenantID, receivedAt time.Time) noports.ReceptionRecord {
	t.Helper()
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		t.Fatalf("构造版本签发：%v", err)
	}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造收寄交接：%v", err)
	}
	doorbell, err := nopostgres.NewOutboxIdentityRecheckDoorbell(db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造重核门铃：%v", err)
	}
	identifiers, err := pspostgres.NewExternalIdentifiers(db)
	if err != nil {
		t.Fatalf("构造关联册读面：%v", err)
	}
	view, err := noparcel.NewParcelIdentityView(identifiers)
	if err != nil {
		t.Fatalf("构造身份核对读口：%v", err)
	}
	identity, err := nopostgres.NewSavepointedParcelIdentityView(db, view)
	if err != nil {
		t.Fatalf("构造身份读口 savepoint：%v", err)
	}
	services, err := nodeIdentityRecheckServiceResults(db)
	if err != nil {
		t.Fatalf("构造服务结果读口：%v", err)
	}
	handler, err := noapplication.NewReceiveDeliveredUnitHandler(noapplication.ReceiveDeliveredUnitDeps{
		Identity: identity, ServiceResults: services, Receptions: receptions, Versions: versions,
		Downstream: downstream, Doorbell: doorbell, Clock: systemClock{},
	})
	if err != nil {
		t.Fatalf("构造收寄编排：%v", err)
	}
	var result noapplication.ReceiveDeliveredUnitResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = handler.Handle(txCtx, noapplication.ReceiveDeliveredUnitCommand{
			TenantID:    tenant,
			SourceID:    uncheckedSource,
			Node:        mustNO(t, nodomain.NewNodeReference, "SYN-NODE-01"),
			DeliveredBy: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-CUSTOMER-01"),
			Unit:        mustNO(t, nodomain.NewHandlingUnitID, uncheckedUnit),
			Mark:        noports.ExternalMarkObservation{Mark: uncheckedMark},
			Claim:       noapplication.ExplicitReception,
			Evidence:    mustNO(t, nodomain.NewReceptionEvidenceReference, "SYN-SIGN-NRS05"),
			OccurredAt:  receivedAt,
		})
		return handleErr
	})
	record, recorded := result.Record()
	if result.Outcome() != noapplication.UnitPendingIdentification || !recorded || record.IdentityCheck != noports.IdentityUnchecked {
		t.Fatalf("身份读不动时收寄 outcome = %v 核对态 %q, want 未核对的待识别", result.Outcome(), record.IdentityCheck)
	}
	return record
}
