package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var effectiveTimeJudgmentAnchor = time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)

// Covers: 判断口的第二参是真编排（票 label-channel/21）——事实登记册、版本签发与 Outbox 意图交付在真实
// PostgreSQL 上装得起来，事务边界成立（同值重放答已按同值判过，证首笔真的提交了）；判断版本回指被判断的那一版
// 并把意图入队（Outbox 里认领得到那一封，VE 那一半由派发进程接走）；同一只事实登记册作读口，待判断视图随之
// 清空、全部当前版视图带出依据。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredEffectiveTimeJudgeAnswersHonestlyAgainstARealDatabase(t *testing.T) {
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

	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-LC21")
	source := mustValue(t, tfdomain.NewTrackingSourceReference, "SYN-CARRIER-Y")
	// 一条已认领、有效时间待判断的事实——源未登规则时收编执行器留下的那种版本；直接经登记册落库，
	// 收编那一段由 label-channel/16 的用例另证。
	pending := pendingExternalTrackingFact(t, tenant, source, "SYN-EXTF-LC21-1", "SYN-EXTV-LC21-1")
	// 闭包只做 IO 并回 error，断言留在闭包外：在事务回调里 t.Fatalf 会 Goexit，提交与回滚两条分支都被跳过。
	var saved tfports.ExternalTrackingFactSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		saved, saveErr = facts.Save(txCtx, pending)
		return saveErr
	}); err != nil {
		t.Fatalf("落待判断事实：%v", err)
	}
	if saved != tfports.ExternalTrackingFactSaved {
		t.Fatalf("夹具落库 outcome = %s", saved)
	}

	before, err := facts.ListCurrentExternalTrackingFacts(t.Context(), tenant, source, tfports.PendingEffectiveTimeOnly, 10)
	if err != nil || len(before) != 1 || before[0].Fact != "SYN-EXTF-LC21-1" {
		t.Fatalf("判断前待判断视图应恰有这一条：%v %+v", err, before)
	}

	command := tfapp.JudgeEffectiveTimeCommand{
		TenantID:    tenant,
		Fact:        "SYN-EXTF-LC21-1",
		EffectiveAt: effectiveTimeJudgmentAnchor.Add(5 * time.Minute),
	}
	judged, err := judge.Judge(t.Context(), command)
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if got := judged.Outcome(); got != tfapp.EffectiveTimeJudged {
		t.Fatalf("outcome = %v, want EFFECTIVE_TIME_JUDGED", got)
	}
	record, has := judged.Record()
	if !has {
		t.Fatal("判断成功却没带回记录")
	}
	if prior, present := record.Fact.Supersedes(); !present || prior.String() != "SYN-EXTV-LC21-1" {
		t.Fatalf("supersedes = (%q, %v)，判断版本没有回指被判断的那一版", prior.String(), present)
	}
	if judged.HandoffReference() != "" {
		t.Fatalf("意图应已与登记同笔入队，却留了续办引用 %q", judged.HandoffReference())
	}

	replay, err := judge.Judge(t.Context(), command)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.EffectiveTimeAlreadyJudgedAsGiven {
		t.Fatalf("outcome = %v, want ALREADY_JUDGED_AS_GIVEN——重放没读到判断版本，首笔事务没有提交", got)
	}

	after, err := facts.ListCurrentExternalTrackingFacts(t.Context(), tenant, source, tfports.PendingEffectiveTimeOnly, 10)
	if err != nil || len(after) != 0 {
		t.Fatalf("判断后待判断视图应为空：%v %+v", err, after)
	}
	current, err := facts.ListCurrentExternalTrackingFacts(t.Context(), tenant, source, tfports.EveryCurrentVersion, 10)
	if err != nil || len(current) != 1 || current[0].Version != record.Key.Version.String() ||
		current[0].EffectiveBasis != "JUDGED_EXPLICITLY" || current[0].Supersedes != "SYN-EXTV-LC21-1" {
		t.Fatalf("全部当前版视图应只剩判断版本并带依据与前版：%v %+v", err, current)
	}

	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	envelope := claimExternalTrackingFactEnvelope(t, store, record.Key.Fact.String()+"/"+record.Key.Version.String())
	if envelope.Scope != tenant.String() {
		t.Fatalf("意图信封的作用域应是判断所在租户：%q", envelope.Scope)
	}
}

// pendingExternalTrackingFact 形成一条待判断的事实版本：三个时间里只有源给的发生时间与本上下文铸的接收时间，
// 有效时间是如实的「待判断」——不是零值，是显式的一格（ADR-0102 决定三）。
func pendingExternalTrackingFact(
	t *testing.T,
	tenant tfdomain.TenantID,
	source tfdomain.TrackingSourceReference,
	fact, version string,
) tfports.ExternalTrackingFactRecord {
	t.Helper()
	spec := tfdomain.ExternalTrackingFactSpec{
		TenantID:    tenant,
		Fact:        mustValue(t, tfdomain.NewExternalTrackingFactReference, fact),
		Version:     mustValue(t, tfdomain.NewExternalTrackingFactVersion, version),
		Source:      source,
		Credential:  mustValue(t, tfdomain.NewExternalCarrierCredentialReference, "SYN-CARRIER-Y/"+fact),
		Object:      mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-"+fact),
		SourceEvent: mustValue(t, tfdomain.NewSourceEventReference, "SYN-EVT-"+fact),
		Status:      mustValue(t, tfdomain.NewRawStatusReference, "DELIVERED"),
		OccurredAt:  effectiveTimeJudgmentAnchor,
		ReceivedAt:  effectiveTimeJudgmentAnchor.Add(90 * time.Second),
		Effective:   tfdomain.PendingEffectiveTime(),
	}
	adopted, err := tfdomain.AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("形成待判断事实：%v", err)
	}
	return tfports.ExternalTrackingFactRecord{
		Key:        tfports.ExternalTrackingFactKey{TenantID: tenant, Fact: spec.Fact, Version: spec.Version},
		Fact:       adopted,
		RecordedAt: spec.ReceivedAt.Add(time.Second),
	}
}

// claimExternalTrackingFactEnvelope 按派发一拍的同一条认领路径把信封取回来（判据同 claimSubmittedEnvelope）：
// 要证的正是「库里那一封」。按主题认——同一库里可能躺着别的用例入队的同类型信封。
func claimExternalTrackingFactEnvelope(t *testing.T, store *outbox.Store, subject string) eventing.Envelope {
	t.Helper()
	deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{
		Now:         time.Now().UTC().Add(time.Hour),
		Limit:       100,
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("认领待发信封：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.Type) == "transport-fulfillment.external-carrier-tracking.judged" &&
			delivery.Envelope.Subject == subject {
			return delivery.Envelope
		}
	}
	t.Fatalf("认领到 %d 封，其中没有主题为 %s 的「外部承运轨迹已判断」", len(deliveries), subject)
	return eventing.Envelope{}
}
