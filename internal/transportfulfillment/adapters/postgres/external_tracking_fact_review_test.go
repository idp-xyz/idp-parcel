package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证外部承运轨迹事实的查阅读口（label-channel/21）：按（租户，轨迹源）
// 上列当前版，待判断过滤只留 PENDING，被回指的旧版不上列，判断版本带依据与前版原样透出。
// 夹具全为合成登记（S 级），不含任何真实轨迹源。

// reviewFactRecord 与 trackingFactRecord 分开：这里要按源分组、按发生时间排序，两维都得可变。
func reviewFactRecord(
	t *testing.T,
	tenant, source, fact, version string,
	occurredAt time.Time,
) ports.ExternalTrackingFactRecord {
	t.Helper()
	spec := domain.ExternalTrackingFactSpec{
		TenantID:    segmentRef(t, domain.NewTenantID, tenant),
		Fact:        segmentRef(t, domain.NewExternalTrackingFactReference, fact),
		Version:     segmentRef(t, domain.NewExternalTrackingFactVersion, version),
		Source:      segmentRef(t, domain.NewTrackingSourceReference, source),
		Credential:  segmentRef(t, domain.NewExternalCarrierCredentialReference, "SYN-CARRIER/"+fact),
		Object:      segmentRef(t, domain.NewCarriedObjectReference, "SYN-PCL-"+fact),
		SourceEvent: segmentRef(t, domain.NewSourceEventReference, "evt-"+fact),
		Status:      segmentRef(t, domain.NewRawStatusReference, "IN_TRANSIT"),
		OccurredAt:  occurredAt,
		ReceivedAt:  occurredAt.Add(90 * time.Second),
		Effective:   domain.PendingEffectiveTime(),
	}
	adopted, err := domain.AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("形成事实夹具：%v", err)
	}
	return ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: spec.TenantID, Fact: spec.Fact, Version: spec.Version},
		Fact:       adopted,
		RecordedAt: spec.ReceivedAt.Add(time.Second),
	}
}

func TestListingCurrentFactsSeparatesPendingFromJudgedAndFollowsTheChain(t *testing.T) {
	repository, transactor, _ := newExternalTrackingFacts(t)
	ctx := t.Context()
	const tenant = "tenant-lc21-review"
	base := time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)

	// 源 A：一条待判断（较晚发生）、一条已由所有者显式判断（首版被回指）；源 B：一条待判断。
	pendingA := reviewFactRecord(t, tenant, "syn-source-a", "RVF-A1", "RVV-A1", base.Add(2*time.Hour))
	judgedFirst := reviewFactRecord(t, tenant, "syn-source-a", "RVF-A2", "RVV-A2a", base)
	pendingB := reviewFactRecord(t, tenant, "syn-source-b", "RVF-B1", "RVV-B1", base.Add(time.Hour))
	for _, record := range []ports.ExternalTrackingFactRecord{pendingA, judgedFirst, pendingB} {
		if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, record); outcome != ports.ExternalTrackingFactSaved {
			t.Fatalf("夹具 %s 落库 outcome = %s", record.Key.Fact, outcome)
		}
	}
	judgment, _ := domain.JudgeEffectiveTimeExplicitly(base.Add(10 * time.Minute))
	judgedFact, err := judgedFirst.Fact.JudgeEffectiveTime(judgment, segmentRef(t, domain.NewExternalTrackingFactVersion, "RVV-A2b"))
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	judgedSecond := ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: judgedFirst.Key.TenantID, Fact: judgedFirst.Key.Fact, Version: judgedFact.Version()},
		Fact:       judgedFact,
		RecordedAt: judgedFirst.RecordedAt.Add(time.Minute),
	}
	if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, judgedSecond); outcome != ports.ExternalTrackingFactSaved {
		t.Fatalf("判断版本落库 outcome = %s", outcome)
	}

	tenantID := segmentRef(t, domain.NewTenantID, tenant)
	sourceA := segmentRef(t, domain.NewTrackingSourceReference, "syn-source-a")

	pending, err := rowsOf(repository.ListCurrentExternalTrackingFacts(ctx, tenantID, sourceA, ports.PendingEffectiveTimeOnly, 50, firstPage(t, ports.ExternalTrackingFactPages)))
	if err != nil {
		t.Fatalf("待判断上列：%v", err)
	}
	if len(pending) != 1 || pending[0].Fact != "RVF-A1" || pending[0].Version != "RVV-A1" {
		t.Fatalf("源 A 的待判断当前版应只有 RVF-A1：%+v", pending)
	}
	row := pending[0]
	if row.EffectiveBasis != "PENDING" || row.EffectiveAt != nil || row.EffectiveRule != "" || row.Supersedes != "" {
		t.Fatalf("待判断行不得带有效时间、规则或前版：%+v", row)
	}
	if row.Source != "syn-source-a" || row.Object != "SYN-PCL-RVF-A1" || row.Credential != "SYN-CARRIER/RVF-A1" ||
		row.SourceEvent != "evt-RVF-A1" || row.Status != "IN_TRANSIT" || row.Origin != "MATERIAL" {
		t.Fatalf("行的身份与源给的内容没有原样透出：%+v", row)
	}
	if !row.OccurredAt.Equal(base.Add(2*time.Hour)) || !row.ReceivedAt.Equal(base.Add(2*time.Hour+90*time.Second)) {
		t.Fatalf("发生/接收时间没有原样透出：%s / %s", row.OccurredAt, row.ReceivedAt)
	}

	current, err := rowsOf(repository.ListCurrentExternalTrackingFacts(ctx, tenantID, sourceA, ports.EveryCurrentVersion, 50, firstPage(t, ports.ExternalTrackingFactPages)))
	if err != nil {
		t.Fatalf("全部当前版上列：%v", err)
	}
	if len(current) != 2 {
		t.Fatalf("源 A 应有两条当前版（一待判断、一已判断），被回指的 RVV-A2a 不上列：%+v", current)
	}
	// 按发生时间先后：RVF-A2（base）在 RVF-A1（base+2h）之前。
	if current[0].Fact != "RVF-A2" || current[0].Version != "RVV-A2b" || current[1].Fact != "RVF-A1" {
		t.Fatalf("排序应按发生时间先后且只列当前版：%+v", current)
	}
	judgedRow := current[0]
	if judgedRow.EffectiveBasis != "JUDGED_EXPLICITLY" || judgedRow.EffectiveAt == nil ||
		!judgedRow.EffectiveAt.Equal(base.Add(10*time.Minute)) || judgedRow.Supersedes != "RVV-A2a" || judgedRow.Origin != "JUDGMENT" {
		t.Fatalf("判断版本应带依据、有效时间、前版与判断来源：%+v", judgedRow)
	}

	sourceB := segmentRef(t, domain.NewTrackingSourceReference, "syn-source-b")
	pendingOfB, err := rowsOf(repository.ListCurrentExternalTrackingFacts(ctx, tenantID, sourceB, ports.PendingEffectiveTimeOnly, 50, firstPage(t, ports.ExternalTrackingFactPages)))
	if err != nil || len(pendingOfB) != 1 || pendingOfB[0].Fact != "RVF-B1" {
		t.Fatalf("源 B 的待判断当前版应只有 RVF-B1：%v %+v", err, pendingOfB)
	}

	limited, err := rowsOf(repository.ListCurrentExternalTrackingFacts(ctx, tenantID, sourceA, ports.EveryCurrentVersion, 1, firstPage(t, ports.ExternalTrackingFactPages)))
	if err != nil || len(limited) != 1 {
		t.Fatalf("页大小应被尊重：%v %+v", err, limited)
	}
}

func TestListingCurrentFactsRefusesMeaninglessInput(t *testing.T) {
	repository, _, _ := newExternalTrackingFacts(t)
	ctx := t.Context()
	tenantID := segmentRef(t, domain.NewTenantID, "tenant-lc21-review")
	source := segmentRef(t, domain.NewTrackingSourceReference, "syn-source-a")

	if _, err := rowsOf(repository.ListCurrentExternalTrackingFacts(ctx, tenantID, source, ports.EveryCurrentVersion, 0, firstPage(t, ports.ExternalTrackingFactPages))); err == nil {
		t.Fatal("非正页大小应被拒")
	}
	if _, err := rowsOf(repository.ListCurrentExternalTrackingFacts(ctx, tenantID, source, ports.EffectiveTimeReviewFilterInvalid, 10, firstPage(t, ports.ExternalTrackingFactPages))); err == nil {
		t.Fatal("封闭集外的过滤词应被拒，不得静默当成某一格")
	}
	empty, err := rowsOf(repository.ListCurrentExternalTrackingFacts(ctx, tenantID,
		segmentRef(t, domain.NewTrackingSourceReference, "syn-source-nobody"), ports.PendingEffectiveTimeOnly, 10, firstPage(t, ports.ExternalTrackingFactPages)))
	if err != nil || len(empty) != 0 {
		t.Fatalf("没有事实的源如实交回空列表：%v %+v", err, empty)
	}
}

// Covers: 人工录入的轨迹事实在读面上带出留痕；判过有效时间另成一版之后留痕仍跟着这条事实；轨迹源送来的整格缺席。
func TestListingCurrentFactsCarriesWhoEnteredAManualFactEvenAfterItWasJudged(t *testing.T) {
	repository, transactor, pool := newExternalTrackingFacts(t)
	ctx := t.Context()
	const tenant = "tenant-manual-provenance"
	base := time.Date(2026, 11, 13, 6, 0, 0, 0, time.UTC)

	manual := reviewFactRecord(t, tenant, "syn-source-m", "MPF-1", "MPV-1a", base)
	fromSource := reviewFactRecord(t, tenant, "syn-source-m", "MPF-2", "MPV-2", base.Add(time.Hour))
	for _, record := range []ports.ExternalTrackingFactRecord{manual, fromSource} {
		if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, record); outcome != ports.ExternalTrackingFactSaved {
			t.Fatalf("夹具 %s 落库 outcome = %s", record.Key.Fact, outcome)
		}
	}
	enteredAt := base.Add(5 * time.Minute)
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.external_tracking_manual_entry
		     (tenant_id, fact_id, version_id, entered_by, original_ref, entered_at)
		 VALUES ($1, 'MPF-1', 'MPV-1a', 'SYN-OP-3', '承运商邮件 11-13.eml', $2)`, tenant, enteredAt); err != nil {
		t.Fatalf("落人工录入留痕：%v", err)
	}
	judgment, _ := domain.JudgeEffectiveTimeExplicitly(base.Add(10 * time.Minute))
	judgedFact, err := manual.Fact.JudgeEffectiveTime(judgment, segmentRef(t, domain.NewExternalTrackingFactVersion, "MPV-1b"))
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if outcome := mustSaveTrackingFact(t, transactor, ctx, repository, ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: manual.Key.TenantID, Fact: manual.Key.Fact, Version: judgedFact.Version()},
		Fact:       judgedFact,
		RecordedAt: manual.RecordedAt.Add(time.Minute),
	}); outcome != ports.ExternalTrackingFactSaved {
		t.Fatalf("判断版本落库 outcome = %s", outcome)
	}

	rows, err := rowsOf(repository.ListCurrentExternalTrackingFacts(ctx, segmentRef(t, domain.NewTenantID, tenant),
		segmentRef(t, domain.NewTrackingSourceReference, "syn-source-m"), ports.EveryCurrentVersion, 50,
		firstPage(t, ports.ExternalTrackingFactPages)))
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列当前版：%v %+v", err, rows)
	}
	judged, plain := rows[0], rows[1]
	if judged.Fact != "MPF-1" || judged.Version != "MPV-1b" || judged.ManualEntry == nil ||
		judged.ManualEntry.EnteredBy != "SYN-OP-3" || judged.ManualEntry.Original != "承运商邮件 11-13.eml" ||
		!judged.ManualEntry.EnteredAt.Equal(enteredAt) {
		t.Fatalf("人工录入的事实判过之后应仍带留痕：%+v %+v", judged, judged.ManualEntry)
	}
	if plain.Fact != "MPF-2" || plain.ManualEntry != nil {
		t.Fatalf("轨迹源送来的事实不得带留痕：%+v", plain)
	}
}
