package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 票 no-reception-identity/03 的真库并发回归：两件实物贴同一个号，在两条连接的两笔事务里送站。A 读完别的实物关联、落册
// 之前挂住，B 此时送站；B 要么等在那件包裹的锁上、A 提交后读到 A 的关联而答身份冲突，要么（修前）径直走完、两件都关联
// 成立。挂住 A 用钩子，B 走生产装配；看 B 等不等锁走一条池外的独立连接；等待全部带超时。夹具全为合成（S 级）。

// pausingReceptions 是生产收寄库外面一层：按包裹读完别的实物关联之后先跑 afterFindOthers，再把读到的交回编排。
type pausingReceptions struct {
	noports.ReceptionStore
	afterFindOthers func()
}

func (store pausingReceptions) FindOtherUnitIntakes(
	ctx context.Context,
	tenant nodomain.TenantID,
	parcel nodomain.ParcelAssociationReference,
	unit nodomain.HandlingUnitID,
) ([]noports.ReceptionRecord, error) {
	records, err := store.ReceptionStore.FindOtherUnitIntakes(ctx, tenant, parcel, unit)
	if err == nil {
		store.afterFindOthers()
	}
	return records, err
}

// awaitAdvisoryLockWaitOrDone 等到有后端在咨询锁上等，或另一方已经走完（没被挡住），交回是不是前者；两者都不发生即失败。
// 观察走池外的独立连接：测试池只有两条连接，挂住的一方与等锁的一方各占一条。只认咨询锁，免得别的锁等待冒充包裹锁。
func awaitAdvisoryLockWaitOrDone(t *testing.T, pool *pgxpool.Pool, done <-chan struct{}) bool {
	t.Helper()
	observer, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Errorf("开观察连接：%v", err)
		return false
	}
	defer observer.Close(context.Background())
	deadline := time.Now().Add(lockWaitDeadline)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			t.Logf("另一方没被挡住，已走完")
			return false
		default:
		}
		var waiting int
		if err := observer.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity
			  WHERE datname = current_database() AND wait_event_type = 'Lock' AND wait_event = 'advisory'`,
		).Scan(&waiting); err != nil {
			t.Errorf("查等锁：%v", err)
			return false
		}
		if waiting > 0 {
			t.Logf("另一方等在包裹锁上，放行")
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("另一方既没走完也没在等锁")
	return false
}

// Covers: `AT-NO-019`「同一外部条码出现在两件实物——两件实物保持独立，形成身份冲突并暂停方向性作业」的并发半边（票
// no-reception-identity/03，ADR-0196 越权风险点「并发未串行」）。号码在关联册里只标识一件包裹；A 经挂住的装配读完别的
// 实物关联（空）后挂起，B 经生产装配送站，等在那件包裹的锁上；A 提交后 B 答待识别、记身份冲突、候选是那件包裹，A 答
// 收寄已形成且原判断原样，A 那件被追加一版身份冲突判断——方向依据的身份读口读这本册，两件都含身份冲突。
func TestConcurrentDeliveriesOfOneMarkSerializeOnTheParcel(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-NRI03"))
	production, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	deliver := func(sourceID, unit string) nodeopsapp.ReceiveDeliveredUnitCommand {
		command := receptionCommand(t, sourceID, nodeopsapp.ExplicitReception)
		command.Unit = mustValue(t, nodomain.NewHandlingUnitID, unit)
		command.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-NRI03"}
		return command
	}
	first := deliver("SYN-DELIVERY-NRI03-A", "SYN-UNIT-NRI03-A")
	second := deliver("SYN-DELIVERY-NRI03-B", "SYN-UNIT-NRI03-B")

	var (
		secondResult nodeopsapp.ReceiveDeliveredUnitResult
		secondErr    error
		secondDone   chan struct{}
		waited       bool
	)
	deps, err := buildReceptionDeps(db)
	if err != nil {
		t.Fatalf("装配收寄依赖：%v", err)
	}
	deps.Receptions = pausingReceptions{ReceptionStore: deps.Receptions, afterFindOthers: func() {
		if secondDone != nil {
			return
		}
		secondDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			secondResult, secondErr = production.Handle(t.Context(), second)
		}(secondDone)
		waited = awaitAdvisoryLockWaitOrDone(t, pool, secondDone)
	}}
	paused, err := receptionWithin(db, deps)
	if err != nil {
		t.Fatalf("装配挂住的收寄编排：%v", err)
	}

	firstResult, firstErr := paused.Handle(t.Context(), first)
	if secondDone == nil {
		t.Fatal("A 没走到按包裹读别的实物关联那一步——号码没唯一识别出包裹")
	}
	awaitDone(t, secondDone)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("A err = %v，B err = %v", firstErr, secondErr)
	}
	if firstResult.Outcome() != nodeopsapp.NodeIntakeFormed || secondResult.Outcome() != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("两件 outcome = %v / %v, want INTAKE_FORMED / PENDING_IDENTIFICATION——两笔事务互相看不到对方的关联",
			firstResult.Outcome(), secondResult.Outcome())
	}
	if !waited {
		t.Fatal("B 没在包裹锁上等就走完了")
	}
	late, _ := secondResult.Record()
	if !late.IdentityConflict || !late.Control.Active() || !slices.Equal(candidatesOf(late), []string{"syn-parcel-1"}) {
		t.Fatalf("B = %+v, want 记身份冲突、控制成立、候选是那件包裹", late)
	}

	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	original, _ := firstResult.Record()
	kept, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: first.TenantID, SourceID: first.SourceID})
	if err != nil || !found || kept.Kind != noports.RecordIntakeFormed || kept.IdentityConflict ||
		kept.Intake.Version() != original.Intake.Version() {
		t.Fatalf("A 的原判断 = %+v（found %v err %v），want 原样", kept, found, err)
	}
	identities, err := nopostgres.NewIntakeIdentities(db)
	if err != nil {
		t.Fatalf("构造身份读口：%v", err)
	}
	for _, unit := range []string{"SYN-UNIT-NRI03-A", "SYN-UNIT-NRI03-B"} {
		observations, err := identities.FindIntakeIdentities(t.Context(), first.TenantID, mustValue(t, nodomain.NewHandlingUnitID, unit))
		if err != nil {
			t.Fatalf("读 %s 的收寄身份：%v", unit, err)
		}
		basis, err := nodomain.JudgeIdentityBasis(observations)
		if err != nil {
			t.Fatalf("判 %s 的身份：%v", unit, err)
		}
		if reasons := basis.Cell().Reasons(); !slices.Contains(reasons, nodomain.ReasonIdentityConflict) {
			t.Fatalf("%s 的身份一格 = %v（%d 行）, want 含身份冲突", unit, reasons, len(observations))
		}
	}
}
