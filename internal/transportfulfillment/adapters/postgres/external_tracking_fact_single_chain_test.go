package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真库证票 tf-face-algebra/06 的收编更正那一半：源声明的更正与所有者的判断接在同一个当前版上，库面只让
// 一方回指它；后到的更正撞键后按声明重新解析到赢家之上再落，不分叉。测试输入是隔离合成，只记 `S`。

type resolvedCredential struct{ object string }

func (credential resolvedCredential) ResolveCredential(
	context.Context,
	domain.TenantID,
	domain.ExternalCarrierCredentialReference,
) (domain.CarriedObjectReference, ports.CredentialResolution, error) {
	object, err := domain.NewCarriedObjectReference(credential.object)
	return object, ports.CredentialResolved, err
}

type fixedTrackingClock struct{ at time.Time }

func (clock fixedTrackingClock) Now() time.Time { return clock.at }

// syntheticTrackingRecord 形成一条待判断的合成事实版本；维度全取 SYN- 值。
func syntheticTrackingRecord(t *testing.T, fact, version, event string) ports.ExternalTrackingFactRecord {
	t.Helper()
	spec := domain.ExternalTrackingFactSpec{
		TenantID:    segmentRef(t, domain.NewTenantID, "SYN-TENANT-TFA06"),
		Fact:        segmentRef(t, domain.NewExternalTrackingFactReference, fact),
		Version:     segmentRef(t, domain.NewExternalTrackingFactVersion, version),
		Source:      segmentRef(t, domain.NewTrackingSourceReference, "SYN-SOURCE-TFA06"),
		Credential:  segmentRef(t, domain.NewExternalCarrierCredentialReference, "SYN-CREDENTIAL-TFA06"),
		Object:      segmentRef(t, domain.NewCarriedObjectReference, "SYN-PARCEL-TFA06"),
		SourceEvent: segmentRef(t, domain.NewSourceEventReference, event),
		Status:      segmentRef(t, domain.NewRawStatusReference, "IN_TRANSIT"),
		OccurredAt:  trackingOccurredAtDB,
		ReceivedAt:  trackingReceivedAtDB,
		Effective:   domain.PendingEffectiveTime(),
	}
	adopted, err := domain.AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("形成事实夹具：%v", err)
	}
	return ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: spec.TenantID, Fact: spec.Fact, Version: spec.Version},
		Fact:       adopted,
		RecordedAt: trackingReceivedAtDB.Add(time.Second),
	}
}

// Covers: 判断版本在事务里落下未提交，收编执行器此刻按更正声明解析到待判断那一版、照常插入，在回指唯一索引上等；
// 判断提交后更正撞键，重新解析到判断版本并回指它。修复前更正不等任何人，与判断各自回指同一前版。
func TestACorrectionLosingToAHeldJudgmentLandsOnTheWinnerAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := adapter.NewExternalTrackingFacts(db)
	if err != nil {
		t.Fatalf("事实登记册：%v", err)
	}
	identities, err := adapter.NewResultVersions(db)
	if err != nil {
		t.Fatalf("身份签发：%v", err)
	}
	transactor := db.Transactor()
	ctx := t.Context()
	executor := application.NewAdoptTrackingMaterialHandler(application.AdoptTrackingMaterialDeps{
		Facts:       facts,
		Identities:  identities,
		Credentials: resolvedCredential{object: "SYN-PARCEL-TFA06"},
		Rules:       absentRules{},
		Ledger:      facts,
		Downstream:  noHandoff{},
		Clock:       fixedTrackingClock{at: trackingReceivedAtDB.Add(time.Hour)},
	})

	original := syntheticTrackingRecord(t, "SYN-EXTF-TFA06", "SYN-EXTV-TFA06-a", "SYN-EVT-TFA06-1")
	mustSaveTrackingFact(t, transactor, ctx, facts, original)
	judgment, err := domain.JudgeEffectiveTimeExplicitly(trackingReceivedAtDB)
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	judged, err := original.Fact.JudgeEffectiveTime(judgment, segmentRef(t, domain.NewExternalTrackingFactVersion, "SYN-EXTV-TFA06-b"))
	if err != nil {
		t.Fatalf("判断换版本：%v", err)
	}
	winner := ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: original.Key.TenantID, Fact: original.Key.Fact, Version: judged.Version()},
		Fact:       judged,
		RecordedAt: original.RecordedAt.Add(time.Minute),
	}

	observer, err := pgx.Connect(ctx, pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })
	database := pool.Config().ConnConfig.Database

	held, release, committed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	// 放行登记进清理：放行之前任何一处 Fatal，赢家都会一直占着连接，连接池清理挂到 go test 超时，报不出干净的红。
	var releaseOnce sync.Once
	releaseWinner := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseWinner)
	go func() {
		committed <- transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			saved, saveErr := facts.Save(txCtx, winner)
			if saveErr == nil && saved != ports.ExternalTrackingFactSaved {
				saveErr = fmt.Errorf("判断版本 outcome = %s", saved)
			}
			close(held)
			<-release
			return saveErr
		})
	}()
	<-held

	correction := ports.TrackingMaterial{
		Source:          "SYN-SOURCE-TFA06",
		Subject:         ports.TrackingSubject{Tenant: original.Key.TenantID, CredentialReference: "SYN-CREDENTIAL-TFA06"},
		SourceEventID:   "SYN-EVT-TFA06-2",
		CorrectionOf:    "SYN-EVT-TFA06-1",
		OccurredAt:      ports.SourceTime{Given: true, At: trackingOccurredAtDB.Add(time.Hour)},
		ReceivedAt:      trackingReceivedAtDB.Add(time.Hour),
		StatusReference: "DELIVERED",
		PayloadDigest:   "sha256:SYN-TFA06-2",
	}
	type answer struct {
		result application.AdoptTrackingMaterialResult
		err    error
	}
	answered := make(chan answer, 1)
	go func() {
		var result application.AdoptTrackingMaterialResult
		err := transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			var adoptErr error
			result, adoptErr = executor.Adopt(txCtx, correction)
			return adoptErr
		})
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
		if err := observer.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND wait_event_type = 'Lock'`, database).Scan(&waiting); err != nil {
			t.Fatalf("看锁等待：%v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("收编既没答也没挂在锁上")
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseWinner()
	if err := <-committed; err != nil {
		t.Fatalf("判断版本提交：%v", err)
	}
	got := early
	if got == nil {
		received := <-answered
		got = &received
	}
	if got.err != nil {
		t.Fatalf("收编更正：%v", got.err)
	}
	if got.result.Outcome() != application.TrackingFactAdoptedPendingJudgment {
		t.Fatalf("撞键后应读回赢家再落：%s reason=%s", got.result.Outcome(), got.result.UndecidedReason())
	}
	record, _ := got.result.Record()
	if prior, has := record.Fact.Supersedes(); !has || prior != winner.Key.Version {
		t.Fatalf("更正应回指判断版本 %s：%q has=%v", winner.Key.Version, prior, has)
	}

	var successors int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.external_carrier_tracking_fact
		  WHERE tenant_id = $1 AND fact_ref = $2 AND supersedes_version = $3`,
		original.Key.TenantID.String(), original.Key.Fact.String(), original.Key.Version.String()).Scan(&successors); err != nil {
		t.Fatalf("数后继：%v", err)
	}
	if successors != 1 {
		t.Fatalf("首版有 %d 个后继, want 1", successors)
	}
	current, found, err := facts.FindCurrent(ctx, original.Key.TenantID, original.Key.Fact)
	if err != nil || !found || current.Key.Version != record.Key.Version {
		t.Fatalf("当前版应是更正那一版 %s：%q found=%v err=%v", record.Key.Version, current.Key.Version, found, err)
	}
}

// Covers: 库面那条部分唯一索引本身——同一前版上第二个后继由 Save 折成`已登记`，不落行。
func TestASecondSuccessorOfOneVersionIsRefusedByTheRegistry(t *testing.T) {
	repository, transactor, _ := newExternalTrackingFacts(t)
	ctx := t.Context()
	first := syntheticTrackingRecord(t, "SYN-EXTF-TFA06-IDX", "SYN-EXTV-TFA06-IDX-a", "SYN-EVT-TFA06-IDX")
	mustSaveTrackingFact(t, transactor, ctx, repository, first)

	successor := func(version string, at time.Time) ports.ExternalTrackingFactRecord {
		judgment, err := domain.JudgeEffectiveTimeExplicitly(at)
		if err != nil {
			t.Fatalf("判断：%v", err)
		}
		judged, err := first.Fact.JudgeEffectiveTime(judgment, segmentRef(t, domain.NewExternalTrackingFactVersion, version))
		if err != nil {
			t.Fatalf("判断换版本：%v", err)
		}
		return ports.ExternalTrackingFactRecord{
			Key:        ports.ExternalTrackingFactKey{TenantID: first.Key.TenantID, Fact: first.Key.Fact, Version: judged.Version()},
			Fact:       judged,
			RecordedAt: first.RecordedAt.Add(time.Minute),
		}
	}
	if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, successor("SYN-EXTV-TFA06-IDX-b", trackingReceivedAtDB)); outcome != ports.ExternalTrackingFactSaved {
		t.Fatalf("第一个后继：%s", outcome)
	}
	if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, successor("SYN-EXTV-TFA06-IDX-c", trackingReceivedAtDB.Add(time.Minute))); outcome != ports.ExternalTrackingFactAlreadyRegistered {
		t.Fatalf("同一前版的第二个后继应答已登记：%s", outcome)
	}
	if _, found, err := repository.FindByKey(ctx, ports.ExternalTrackingFactKey{
		TenantID: first.Key.TenantID, Fact: first.Key.Fact, Version: segmentRef(t, domain.NewExternalTrackingFactVersion, "SYN-EXTV-TFA06-IDX-c"),
	}); err != nil || found {
		t.Fatalf("第二个后继不得落行：found=%v err=%v", found, err)
	}
}
