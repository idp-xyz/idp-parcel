package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证外部承运轨迹事实登记册：三时间与判断依据原样往返、（源，源事件）
// 幂等锚拦第二条、当前版按回指派生、留痕只追加，以及库内 CHECK 挡住领域造不出的行。夹具全为
// 合成登记（S 级），不含任何真实轨迹源。

var (
	trackingOccurredAtDB = time.Date(2026, 9, 3, 8, 30, 0, 0, time.UTC)
	trackingReceivedAtDB = time.Date(2026, 9, 3, 8, 31, 12, 0, time.UTC)
)

func newExternalTrackingFacts(t *testing.T) (*adapter.ExternalTrackingFacts, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewExternalTrackingFacts(db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	return repository, db.Transactor(), pool
}

type trackingFixtureOptions struct {
	fact, version, event, correctionOf, supersedes string
	effective                                      domain.EffectiveTimeJudgment
}

func trackingFactRecord(t *testing.T, options trackingFixtureOptions) ports.ExternalTrackingFactRecord {
	t.Helper()
	spec := domain.ExternalTrackingFactSpec{
		TenantID:   segmentRef(t, domain.NewTenantID, "tenant-1"),
		Fact:       segmentRef(t, domain.NewExternalTrackingFactReference, options.fact),
		Version:    segmentRef(t, domain.NewExternalTrackingFactVersion, options.version),
		Source:     segmentRef(t, domain.NewTrackingSourceReference, "aggregator-a"),
		Credential: segmentRef(t, domain.NewExternalCarrierCredentialReference, "carrier-x/1Z999"),
		Object:     segmentRef(t, domain.NewCarriedObjectReference, "PCL-1"),
		Status:     segmentRef(t, domain.NewRawStatusReference, "IN_TRANSIT"),
		OccurredAt: trackingOccurredAtDB,
		ReceivedAt: trackingReceivedAtDB,
		Effective:  options.effective,
	}
	if spec.Effective.Basis() == domain.EffectiveTimeBasisInvalid {
		spec.Effective = domain.PendingEffectiveTime()
	}
	if options.event != "" {
		spec.SourceEvent = segmentRef(t, domain.NewSourceEventReference, options.event)
	}
	if options.correctionOf != "" {
		spec.CorrectionOf = segmentRef(t, domain.NewSourceEventReference, options.correctionOf)
	}
	if options.supersedes != "" {
		spec.Supersedes = segmentRef(t, domain.NewExternalTrackingFactVersion, options.supersedes)
	}
	fact, err := domain.AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("形成事实夹具：%v", err)
	}
	return ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: spec.TenantID, Fact: spec.Fact, Version: spec.Version},
		Fact:       fact,
		RecordedAt: trackingReceivedAtDB.Add(time.Second),
	}
}

func mustSaveTrackingFact(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.ExternalTrackingFacts,
	record ports.ExternalTrackingFactRecord,
) ports.ExternalTrackingFactSaveOutcome {
	t.Helper()
	var outcome ports.ExternalTrackingFactSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	return outcome
}

func TestARuleJudgedTrackingFactRoundTripsWithAllThreeTimes(t *testing.T) {
	repository, transactor, _ := newExternalTrackingFacts(t)
	ctx := t.Context()
	rule := segmentRef2(t, domain.NewEffectiveTimeRuleReference, "aggregator-a/effective-time", "v3")
	judgment, err := domain.JudgeEffectiveTimeByRule(rule, trackingReceivedAtDB)
	if err != nil {
		t.Fatalf("规则判断：%v", err)
	}
	record := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-1", version: "EXTV-1", event: "evt-1", effective: judgment})
	if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, record); outcome != ports.ExternalTrackingFactSaved {
		t.Fatalf("首登 outcome = %s", outcome)
	}

	found, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("取回：%v exists=%v", err, exists)
	}
	if !found.Fact.OccurredAt().Equal(trackingOccurredAtDB) || !found.Fact.ReceivedAt().Equal(trackingReceivedAtDB) {
		t.Fatalf("发生/接收时间没有原样带回：%s / %s", found.Fact.OccurredAt(), found.Fact.ReceivedAt())
	}
	at, judged := found.Fact.EffectiveAt()
	if !judged || !at.Equal(trackingReceivedAtDB) {
		t.Fatalf("有效时间没有原样带回：%s judged=%v", at, judged)
	}
	applied, byRule := found.Fact.Effective().Rule()
	if !byRule || applied.Rule() != "aggregator-a/effective-time" || applied.Version() != "v3" {
		t.Fatalf("采用的规则版本没有原样带回：%q/%q", applied.Rule(), applied.Version())
	}
	if event, given := found.Fact.SourceEvent(); !given || event.String() != "evt-1" {
		t.Fatalf("源事件标识没有原样带回：%q given=%v", event, given)
	}
	if found.Fact.Status().String() != "IN_TRANSIT" || found.Fact.Object().String() != "PCL-1" {
		t.Fatalf("状态词或对象没有原样带回：%q / %q", found.Fact.Status(), found.Fact.Object())
	}
}

func TestAPendingTrackingFactReadsBackPendingAndWithoutASourceEvent(t *testing.T) {
	repository, transactor, _ := newExternalTrackingFacts(t)
	ctx := t.Context()
	record := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-2", version: "EXTV-2"})
	mustSaveTrackingFact(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("取回：%v exists=%v", err, exists)
	}
	if _, judged := found.Fact.EffectiveAt(); judged {
		t.Fatal("待判断的行装回来却成了判断过")
	}
	if _, given := found.Fact.SourceEvent(); given {
		t.Fatal("源未给事件标识，装回来却有了一个")
	}
}

func TestTheSourceEventAnchorRefusesASecondFactForTheSameEvent(t *testing.T) {
	repository, transactor, _ := newExternalTrackingFacts(t)
	ctx := t.Context()
	first := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-3", version: "EXTV-3", event: "evt-dup"})
	mustSaveTrackingFact(t, transactor, ctx, repository, first)

	other := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-3b", version: "EXTV-3b", event: "evt-dup"})
	if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, other); outcome != ports.ExternalTrackingFactAlreadyRegistered {
		t.Fatalf("同（源，源事件）第二条应答已登记：%s", outcome)
	}
	found, exists, err := repository.FindBySourceEvent(ctx, first.Key.TenantID, first.Fact.Source(),
		segmentRef(t, domain.NewSourceEventReference, "evt-dup"))
	if err != nil || !exists || found.Key != first.Key {
		t.Fatalf("按源事件应读回赢家：%v exists=%v key=%+v", err, exists, found.Key)
	}
}

func TestTwoFactsWithoutSourceEventsCoexist(t *testing.T) {
	repository, transactor, _ := newExternalTrackingFacts(t)
	ctx := t.Context()
	mustSaveTrackingFact(t, transactor, ctx, repository, trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-4", version: "EXTV-4"}))
	if outcome := mustSaveTrackingFact(t, transactor, ctx, repository,
		trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-4b", version: "EXTV-4b"})); outcome != ports.ExternalTrackingFactSaved {
		t.Fatalf("源未给事件标识的素材不判重，第二条也该落库：%s", outcome)
	}
}

func TestFindCurrentFollowsTheVersionChain(t *testing.T) {
	repository, transactor, _ := newExternalTrackingFacts(t)
	ctx := t.Context()
	first := trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-5", version: "EXTV-5a", event: "evt-5"})
	mustSaveTrackingFact(t, transactor, ctx, repository, first)

	judgment, _ := domain.JudgeEffectiveTimeExplicitly(trackingReceivedAtDB)
	judged, err := first.Fact.JudgeEffectiveTime(judgment, segmentRef(t, domain.NewExternalTrackingFactVersion, "EXTV-5b"))
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	second := ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: first.Key.TenantID, Fact: first.Key.Fact, Version: judged.Version()},
		Fact:       judged,
		RecordedAt: first.RecordedAt.Add(time.Minute),
	}
	if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, second); outcome != ports.ExternalTrackingFactSaved {
		t.Fatalf("判断版本应落库——它与首版携带同一个源事件，但幂等锚是（源，源事件）对**事实**而非对版本说话：%s", outcome)
	}

	current, exists, err := repository.FindCurrent(ctx, first.Key.TenantID, first.Key.Fact)
	if err != nil || !exists {
		t.Fatalf("当前版：%v exists=%v", err, exists)
	}
	if current.Key.Version != judged.Version() {
		t.Fatalf("当前版应是未被回指的那一版：%q", current.Key.Version)
	}
	if prior, has := current.Fact.Supersedes(); !has || prior != first.Key.Version {
		t.Fatalf("当前版应回指首版：%q has=%v", prior, has)
	}
	original, exists, err := repository.FindByKey(ctx, first.Key)
	if err != nil || !exists {
		t.Fatalf("原版本应保留：%v exists=%v", err, exists)
	}
	if _, judged := original.Fact.EffectiveAt(); judged {
		t.Fatal("原版本被回写成判断过——只插不改被破了")
	}
}

func TestUnadoptedMaterialIsAppendedAsGiven(t *testing.T) {
	repository, transactor, pool := newExternalTrackingFacts(t)
	ctx := t.Context()
	entry := ports.UnadoptedTrackingMaterial{
		TenantID:      segmentRef(t, domain.NewTenantID, "tenant-1"),
		Source:        "aggregator-a",
		Credential:    "carrier-x/1Z999",
		Status:        "DELIVERED",
		ReceivedAt:    trackingReceivedAtDB,
		PayloadDigest: "sha256:abc",
		Reason:        ports.UnadoptedOccurredAtNotGiven,
		RecordedAt:    trackingReceivedAtDB.Add(time.Second),
	}
	for range 2 {
		mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			return repository.RecordUnadopted(txCtx, entry)
		})
	}
	var count int
	var sourceEvent *string
	if err := pool.QueryRow(ctx,
		`SELECT count(*), max(source_event) FROM transport_fulfillment.unadopted_tracking_material WHERE reason = $1`,
		"OCCURRED_AT_NOT_GIVEN_BY_SOURCE").Scan(&count, &sourceEvent); err != nil {
		t.Fatalf("数留痕：%v", err)
	}
	if count != 2 {
		t.Fatalf("同一素材拉到两次就如实留两条——留痕不判重：%d", count)
	}
	if sourceEvent != nil {
		t.Fatal("源未给事件标识应存为 NULL，不是空串")
	}
}

func TestExternalTrackingWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newExternalTrackingFacts(t)
	_, err := repository.Save(t.Context(), trackingFactRecord(t, trackingFixtureOptions{fact: "EXTF-6", version: "EXTV-6"}))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := repository.RecordUnadopted(t.Context(), ports.UnadoptedTrackingMaterial{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务留痕应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestExternalTrackingCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门。
// 「待判断却带有效时间」那一格尤其要有：那正是「默认等于发生时间」在库面上的样子。
func TestExternalTrackingCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	_, _, pool := newExternalTrackingFacts(t)
	ctx := t.Context()
	base := `INSERT INTO transport_fulfillment.external_carrier_tracking_fact
	    (tenant_id, fact_ref, version, source_ref, credential_ref, object_ref, source_event, status_ref,
	     occurred_at, received_at, effective_basis, effective_at, effective_rule, effective_rule_version,
	     correction_of, supersedes_version, version_origin, recorded_at) VALUES `
	for name, values := range map[string]string{
		"待判断却带有效时间":   `('t','BAD-1','v1','s','c','o',NULL,'X',now(),now(),'PENDING',now(),NULL,NULL,NULL,NULL,'MATERIAL',now())`,
		"判过了却没有时间":    `('t','BAD-2','v1','s','c','o',NULL,'X',now(),now(),'JUDGED_EXPLICITLY',NULL,NULL,NULL,NULL,NULL,'MATERIAL',now())`,
		"按规则判却没有规则版本": `('t','BAD-3','v1','s','c','o',NULL,'X',now(),now(),'JUDGED_BY_RULE',now(),'r',NULL,NULL,NULL,'MATERIAL',now())`,
		"显式判却带着规则":    `('t','BAD-4','v1','s','c','o',NULL,'X',now(),now(),'JUDGED_EXPLICITLY',now(),'r','v1',NULL,NULL,'MATERIAL',now())`,
		"依据不在封闭集合内":   `('t','BAD-5','v1','s','c','o',NULL,'X',now(),now(),'DEFAULTED',now(),NULL,NULL,NULL,NULL,'MATERIAL',now())`,
		"素材版本回指却无源声明": `('t','BAD-6','v2','s','c','o',NULL,'X',now(),now(),'PENDING',NULL,NULL,NULL,NULL,'v1','MATERIAL',now())`,
		"判断版本却不回指":    `('t','BAD-6b','v2','s','c','o',NULL,'X',now(),now(),'JUDGED_EXPLICITLY',now(),NULL,NULL,NULL,NULL,'JUDGMENT',now())`,
		"判断版本却仍待判断":   `('t','BAD-6c','v2','s','c','o',NULL,'X',now(),now(),'PENDING',NULL,NULL,NULL,NULL,'v1','JUDGMENT',now())`,
		"版本来路不在封闭集合内": `('t','BAD-6d','v1','s','c','o',NULL,'X',now(),now(),'PENDING',NULL,NULL,NULL,NULL,NULL,'INFERRED',now())`,
		"前版指向自己":      `('t','BAD-7','v2','s','c','o',NULL,'X',now(),now(),'PENDING',NULL,NULL,NULL,'evt-0','v2','MATERIAL',now())`,
		"空串源事件":       `('t','BAD-8','v1','s','c','o','','X',now(),now(),'PENDING',NULL,NULL,NULL,NULL,NULL,'MATERIAL',now())`,
		"缺状态词":        `('t','BAD-9','v1','s','c','o',NULL,'',now(),now(),'PENDING',NULL,NULL,NULL,NULL,NULL,'MATERIAL',now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}
	if _, err := pool.Exec(ctx, `INSERT INTO transport_fulfillment.unadopted_tracking_material
	    (tenant_id, source_ref, source_event, credential_ref, status_ref, received_at, payload_digest, reason, recorded_at)
	    VALUES ('t','s',NULL,'c','X',now(),'d','REJECTED',now())`); err == nil {
		t.Fatal("留痕理由集外的行落进去了")
	}
}

func segmentRef2[T any](t *testing.T, construct func(string, string) (T, error), first, second string) T {
	t.Helper()
	value, err := construct(first, second)
	if err != nil {
		t.Fatalf("construct %q/%q: %v", first, second, err)
	}
	return value
}
