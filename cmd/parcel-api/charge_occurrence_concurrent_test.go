package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 运输收费发生项登记的真库并发回归（ADR-0207 决定四「查与随后的落册在同一事务里按（租户、原因、事实依据）串行化」）。
// A 落册之后、交意图之前挂住——此刻握着事实依据锁、行还没提交；B 经生产装配登同一段，等在那把锁上。A 提交后 B 必须
// 读到 A 那一行再作答。看 B 等没等锁走池外的独立连接，按锁空间认；等待全部带超时。夹具全为合成（S 级）。

// chargeOccurrenceLockSpace 是发生项册取事实依据锁时用的锁空间名，与 PG 适配器的同一个串：观察方按它认出 B 等的是哪一把。
const chargeOccurrenceLockSpace = "transport_fulfillment.charge_occurrence.fact_basis"

// pausingChargeHandoff 是生产交接口外面一层：落册之后、交意图之前先跑 beforeHandOff。
type pausingChargeHandoff struct {
	tfports.ChargeOccurrenceHandoff
	beforeHandOff func()
}

func (handoff pausingChargeHandoff) HandOffChargeOccurrence(ctx context.Context, intent tfports.ChargeOccurrenceIntent) error {
	handoff.beforeHandOff()
	return handoff.ChargeOccurrenceHandoff.HandOffChargeOccurrence(ctx, intent)
}

// raceChargeRegistrations 让 first 在挂住的那一笔里登记，挂住时经生产装配起 second，等 second 落到事实依据锁上再放行
// first。交回两方的答复与 second 是否真的等过锁。
func raceChargeRegistrations(
	t *testing.T,
	pool *pgxpool.Pool,
	db *bentopg.DB,
	first, second tfapp.RegisterChargeOccurrenceCommand,
) (firstResult, secondResult tfapp.RegisterChargeOccurrenceResult, waited bool) {
	t.Helper()
	production, err := buildChargeOccurrenceRegistration(db)
	if err != nil {
		t.Fatalf("装配登记编排：%v", err)
	}
	deps, err := chargeOccurrenceDeps(db)
	if err != nil {
		t.Fatalf("装配登记依赖：%v", err)
	}
	var (
		secondErr  error
		secondDone chan struct{}
	)
	deps.Handoff = pausingChargeHandoff{ChargeOccurrenceHandoff: deps.Handoff, beforeHandOff: func() {
		if secondDone != nil {
			return
		}
		secondDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			secondResult, secondErr = production.Register(t.Context(), second)
		}(secondDone)
		waited = awaitAdvisoryWaiters(t, pool, map[string]int{chargeOccurrenceLockSpace: 1}, secondDone)
	}}
	paused := transactionalChargeOccurrenceRegistrar{transactor: db.Transactor(), inner: tfapp.NewRegisterChargeOccurrenceHandler(deps)}

	firstResult, firstErr := paused.Register(t.Context(), first)
	if secondDone == nil {
		t.Fatalf("A 没走到交意图那一步：%q/%q %v", firstResult.Outcome(), firstResult.Detail(), firstErr)
	}
	awaitDone(t, secondDone)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("A err = %v，B err = %v", firstErr, secondErr)
	}
	return firstResult, secondResult, waited
}

func concurrentChargeDB(t *testing.T, parcels ...string) (*pgxpool.Pool, *bentopg.DB) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seedTrunkAgreementAndEndedSegmentOf(t, db, parcels...)
	return pool, db
}

// Covers: ADR-0207 决定四的并发半边——同一段上两笔并发登记、成员集合有交集（A 登一个包裹，B 登它和另一个）。身份按
// 成员集合派生，两者是两个身份，主键挡不住；B 等在事实依据锁上，A 提交后 B 读到 A 那一项，答冲突、细格
// MEMBERS_ALREADY_COVERED、回显 A。册上一项、信封一封。
func TestConcurrentRegistrationsWithOverlappingMembersSerializeOnTheFact(t *testing.T) {
	const otherParcel = "SYN-PARCEL-ASSEMBLY-02"
	pool, db := concurrentChargeDB(t, chargeParcel, otherParcel)
	wider := chargeCommandFor(t, "v1")
	wider.Objects = []string{chargeParcel, otherParcel}

	first, second, waited := raceChargeRegistrations(t, pool, db, chargeCommandFor(t, "v1"), wider)
	if !waited {
		t.Fatal("B 没在事实依据锁上等就走完了")
	}
	if first.Outcome() != tfapp.ChargeOccurrenceRegistered {
		t.Fatalf("A：%q/%q, want REGISTERED", first.Outcome(), first.Detail())
	}
	formed, _ := first.Record()
	echoed, present := second.Record()
	if second.Outcome() != tfapp.ChargeOccurrenceConflict || second.Detail() != tfapp.MembersAlreadyCovered ||
		!present || echoed.Key != formed.Key {
		t.Fatalf("B：%q/%q 回显 %v, want CONFLICT/MEMBERS_ALREADY_COVERED 回显 A 那一项 %v",
			second.Outcome(), second.Detail(), echoed.Key, formed.Key)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 1 || envelopes != 1 {
		t.Fatalf("发生项 %d 行、信封 %d 封，want 1 / 1——两笔事务互相看不见对方", occurrences, envelopes)
	}
}

// Covers: ADR-0207 决定二「两格都先问册」的并发半边——A 正在登记一项外部发生项（已落册、未提交），B 同时声明同一段同一
// 成员自营。B 等在事实依据锁上，A 提交后 B 按身份读到 A 那一项，答冲突并回显它，不答不适用：同一件事不能既有外部成本
// 又不适用。
func TestASelfOperatedDeclarationRacingAnExternalRegistrationIsAConflict(t *testing.T) {
	pool, db := concurrentChargeDB(t, chargeParcel)
	declared := chargeCommandFor(t, "")
	declared.Provider, declared.AgreementObject, declared.ConditionAssertion = "", "", ""
	declared.SelfOperated = true

	first, second, waited := raceChargeRegistrations(t, pool, db, chargeCommandFor(t, "v1"), declared)
	if !waited {
		t.Fatal("B 没在事实依据锁上等就走完了")
	}
	if first.Outcome() != tfapp.ChargeOccurrenceRegistered {
		t.Fatalf("A：%q/%q, want REGISTERED", first.Outcome(), first.Detail())
	}
	formed, _ := first.Record()
	echoed, present := second.Record()
	if second.Outcome() != tfapp.ChargeOccurrenceConflict || !present || echoed.Key != formed.Key {
		t.Fatalf("B：%q/%q 回显 %v, want CONFLICT 回显 A 那一项 %v", second.Outcome(), second.Detail(), echoed.Key, formed.Key)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 1 || envelopes != 1 {
		t.Fatalf("发生项 %d 行、信封 %d 封，want 1 / 1", occurrences, envelopes)
	}
}
