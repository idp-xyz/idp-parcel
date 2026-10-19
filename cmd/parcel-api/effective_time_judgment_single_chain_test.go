package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件在生产装配 buildEffectiveTimeJudgment 与真库上证票 tf-face-algebra/06：同一事实的并发判断只成一条版本链——
// 输家撞「一版至多被回指一次」后读回赢家，同值答`已按同值判过`回显赢家，异值在赢家之上改判。测试输入是隔离合成，
// 只记 `S`。

type judgmentChainFixture struct {
	pool   *pgxpool.Pool
	db     *bentopg.DB
	judge  tfhttp.EffectiveTimeJudge
	facts  *tfpostgres.ExternalTrackingFacts
	tenant tfdomain.TenantID
	source tfdomain.TrackingSourceReference
}

func newJudgmentChainFixture(t *testing.T) judgmentChainFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	judge, err := buildEffectiveTimeJudgment(db)
	if err != nil {
		t.Fatalf("装配判断编排：%v", err)
	}
	facts, err := tfpostgres.NewExternalTrackingFacts(db)
	if err != nil {
		t.Fatalf("构造事实登记册：%v", err)
	}
	return judgmentChainFixture{
		pool:   pool,
		db:     db,
		judge:  judge,
		facts:  facts,
		tenant: mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-TFA06"),
		source: mustValue(t, tfdomain.NewTrackingSourceReference, "SYN-CARRIER-TFA06"),
	}
}

// seedPending 经登记册落一条待判断的事实，返回那一版。
func (fixture judgmentChainFixture) seedPending(t *testing.T, fact string) tfports.ExternalTrackingFactRecord {
	t.Helper()
	pending := pendingExternalTrackingFact(t, fixture.tenant, fixture.source, fact, fact+"/v1")
	var saved tfports.ExternalTrackingFactSaveOutcome
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		saved, saveErr = fixture.facts.Save(txCtx, pending)
		return saveErr
	}); err != nil || saved != tfports.ExternalTrackingFactSaved {
		t.Fatalf("落待判断事实 %s：outcome=%s err=%v", fact, saved, err)
	}
	return pending
}

func (fixture judgmentChainFixture) command(fact string, at time.Time) tfapp.JudgeEffectiveTimeCommand {
	return tfapp.JudgeEffectiveTimeCommand{TenantID: fixture.tenant, Fact: fact, EffectiveAt: at}
}

// currentVersions 取当前版读面上这条事实的全部当前版——分叉的链在这里就是不止一条。
func (fixture judgmentChainFixture) currentVersions(t *testing.T, fact string) []string {
	t.Helper()
	views, err := fixture.facts.ListCurrentExternalTrackingFacts(t.Context(), fixture.tenant, fixture.source, tfports.EveryCurrentVersion, 100)
	if err != nil {
		t.Fatalf("读当前版：%v", err)
	}
	var versions []string
	for _, view := range views {
		if view.Fact == fact {
			versions = append(versions, view.Version)
		}
	}
	return versions
}

func (fixture judgmentChainFixture) successorsOf(t *testing.T, fact, version string) int {
	t.Helper()
	var rows int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.external_carrier_tracking_fact
		  WHERE tenant_id = $1 AND fact_ref = $2 AND supersedes_version = $3`,
		fixture.tenant.String(), fact, version).Scan(&rows); err != nil {
		t.Fatalf("数后继：%v", err)
	}
	return rows
}

// raceAgainstAHeldJudgment 摆出确定的交错：另一方的判断版本在事务里落下未提交，本口此刻读当前版仍是待判断那一版、
// 照常插入，在回指唯一索引上等那一方；那一方提交后本口撞上它，读回赢家再判。修复前本口不等任何人，直接答已判断。
func raceAgainstAHeldJudgment(
	t *testing.T,
	fixture judgmentChainFixture,
	fact string,
	winnerAt, loserAt time.Time,
) (tfapp.JudgeEffectiveTimeResult, tfports.ExternalTrackingFactRecord) {
	t.Helper()
	pending := fixture.seedPending(t, fact)
	judgment, err := tfdomain.JudgeEffectiveTimeExplicitly(winnerAt)
	if err != nil {
		t.Fatalf("赢家的判断：%v", err)
	}
	winnerVersion := mustValue(t, tfdomain.NewExternalTrackingFactVersion, fact+"/winner")
	judged, err := pending.Fact.JudgeEffectiveTime(judgment, winnerVersion)
	if err != nil {
		t.Fatalf("赢家换版本：%v", err)
	}
	winner := tfports.ExternalTrackingFactRecord{
		Key:        tfports.ExternalTrackingFactKey{TenantID: fixture.tenant, Fact: pending.Key.Fact, Version: winnerVersion},
		Fact:       judged,
		RecordedAt: pending.RecordedAt.Add(time.Minute),
	}

	observer, err := pgx.Connect(t.Context(), pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })
	database := fixture.pool.Config().ConnConfig.Database

	held, release, committed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	// 放行登记进清理：放行之前任何一处 Fatal，赢家都会一直占着连接，连接池清理挂到 go test 超时，报不出干净的红。
	var releaseOnce sync.Once
	releaseWinner := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseWinner)
	go func() {
		committed <- fixture.db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			saved, saveErr := fixture.facts.Save(txCtx, winner)
			if saveErr == nil && saved != tfports.ExternalTrackingFactSaved {
				saveErr = fmt.Errorf("赢家 outcome = %s", saved)
			}
			close(held)
			<-release
			return saveErr
		})
	}()
	<-held

	type answer struct {
		result tfapp.JudgeEffectiveTimeResult
		err    error
	}
	answered := make(chan answer, 1)
	go func() {
		result, err := fixture.judge.Judge(context.Background(), fixture.command(fact, loserAt))
		answered <- answer{result, err}
	}()

	var early *answer
	for deadline := time.Now().Add(10 * time.Second); early == nil; {
		select {
		case got := <-answered:
			early = &got
			continue
		default:
		}
		var waiting int
		if err := observer.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND wait_event_type = 'Lock'`, database).Scan(&waiting); err != nil {
			t.Fatalf("看锁等待：%v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("本口既没答也没挂在锁上")
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseWinner()
	if err := <-committed; err != nil {
		t.Fatalf("赢家提交：%v", err)
	}
	got := early
	if got == nil {
		received := <-answered
		got = &received
	}
	if got.err != nil {
		t.Fatalf("本口判断：%v", got.err)
	}
	return got.result, winner
}

// Covers: 同值输家确定落在回指唯一索引上——答`已按同值判过`回显赢家，库里待判断那一版只有一个后继，当前版读面只有赢家。
func TestALosingConcurrentJudgmentWithTheSameTimeEchoesTheWinnerAgainstARealDatabase(t *testing.T) {
	fixture := newJudgmentChainFixture(t)
	const fact = "SYN-EXTF-TFA06-SAME"
	at := effectiveTimeJudgmentAnchor.Add(5 * time.Minute)

	result, winner := raceAgainstAHeldJudgment(t, fixture, fact, at, at)
	if result.Outcome() != tfapp.EffectiveTimeAlreadyJudgedAsGiven {
		t.Fatalf("同值输家 outcome = %s reason=%s, want ALREADY_JUDGED_AS_GIVEN", result.Outcome(), result.UndecidedReason())
	}
	if record, has := result.Record(); !has || record.Key.Version != winner.Key.Version {
		t.Fatalf("应回显赢家 %s：%q has=%v", winner.Key.Version, record.Key.Version, has)
	}
	if successors := fixture.successorsOf(t, fact, fact+"/v1"); successors != 1 {
		t.Fatalf("待判断那一版有 %d 个后继, want 1", successors)
	}
	if current := fixture.currentVersions(t, fact); len(current) != 1 || current[0] != winner.Key.Version.String() {
		t.Fatalf("当前版读面 %v, want 只有赢家", current)
	}
}

// Covers: 异值输家确定落在回指唯一索引上——在赢家之上改判，链线性，当前版读面只有改判那一版。
func TestALosingConcurrentJudgmentWithAnotherTimeIsJudgedOverTheWinnerAgainstARealDatabase(t *testing.T) {
	fixture := newJudgmentChainFixture(t)
	const fact = "SYN-EXTF-TFA06-DIFF"
	winnerAt := effectiveTimeJudgmentAnchor.Add(5 * time.Minute)
	loserAt := effectiveTimeJudgmentAnchor.Add(7 * time.Minute)

	result, winner := raceAgainstAHeldJudgment(t, fixture, fact, winnerAt, loserAt)
	if result.Outcome() != tfapp.EffectiveTimeJudged {
		t.Fatalf("异值输家 outcome = %s reason=%s, want EFFECTIVE_TIME_JUDGED", result.Outcome(), result.UndecidedReason())
	}
	record, _ := result.Record()
	if prior, has := record.Fact.Supersedes(); !has || prior != winner.Key.Version {
		t.Fatalf("改判应回指赢家 %s：%q has=%v", winner.Key.Version, prior, has)
	}
	if successors := fixture.successorsOf(t, fact, fact+"/v1"); successors != 1 {
		t.Fatalf("待判断那一版有 %d 个后继, want 1", successors)
	}
	if current := fixture.currentVersions(t, fact); len(current) != 1 || current[0] != record.Key.Version.String() {
		t.Fatalf("当前版读面 %v, want 只有改判那一版 %s", current, record.Key.Version)
	}
}

// Covers: 同一载荷经生产装配并发投递多份——每轮恰一份`已判断`，其余答`已按同值判过`回显赢家；当前版读面只有一条，
// 判断意图只有一封。交错由调度决定，确定落在索引上的那条路见上两个用例。
func TestConcurrentIdenticalJudgmentsFormExactlyOneJudgedVersionAgainstARealDatabase(t *testing.T) {
	fixture := newJudgmentChainFixture(t)
	at := effectiveTimeJudgmentAnchor.Add(5 * time.Minute)
	const racers = 6
	facts := make([]string, 0, 4)

	for round := range 4 {
		fact := fmt.Sprintf("SYN-EXTF-TFA06-BURST-%d", round)
		facts = append(facts, fact)
		fixture.seedPending(t, fact)
		results := make([]tfapp.JudgeEffectiveTimeResult, racers)
		start := make(chan struct{})
		var wait sync.WaitGroup
		for racer := range racers {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				result, err := fixture.judge.Judge(context.Background(), fixture.command(fact, at))
				if err != nil {
					t.Errorf("%s 第 %d 份判断：%v", fact, racer, err)
					return
				}
				results[racer] = result
			}()
		}
		close(start)
		wait.Wait()

		var winner string
		judged := 0
		for _, result := range results {
			if result.Outcome() == tfapp.EffectiveTimeJudged {
				judged++
				record, _ := result.Record()
				winner = record.Key.Version.String()
			}
		}
		if judged != 1 {
			t.Fatalf("%s 并发 %d 份同值判断，答已判断的有 %d 份, want 1", fact, racers, judged)
		}
		for racer, result := range results {
			if result.Outcome() == tfapp.EffectiveTimeJudged {
				continue
			}
			record, has := result.Record()
			if result.Outcome() != tfapp.EffectiveTimeAlreadyJudgedAsGiven || !has || record.Key.Version.String() != winner {
				t.Fatalf("%s 第 %d 份答 %s（%q），want ALREADY_JUDGED_AS_GIVEN 回显赢家 %s",
					fact, racer, result.Outcome(), record.Key.Version, winner)
			}
		}
		if current := fixture.currentVersions(t, fact); len(current) != 1 || current[0] != winner {
			t.Fatalf("%s 当前版读面 %v, want 只有赢家 %s", fact, current, winner)
		}
	}

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{
		Now:         time.Now().UTC().Add(time.Hour),
		Limit:       100,
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("认领待发信封：%v", err)
	}
	for _, fact := range facts {
		envelopes := 0
		for _, delivery := range deliveries {
			if string(delivery.Envelope.Type) == "transport-fulfillment.external-carrier-tracking.judged" &&
				strings.HasPrefix(delivery.Envelope.Subject, fact+"/") {
				envelopes++
			}
		}
		if envelopes != 1 {
			t.Fatalf("%s 的判断意图 %d 封, want 1", fact, envelopes)
		}
	}
}
