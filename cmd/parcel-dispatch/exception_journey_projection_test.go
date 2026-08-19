package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 CONS-PROJ-EJ-B：TF 替代/退运旅程启动只投 VE 投影，不 FanOut 给 PS。
// 一封信带全体成员，消费侧按成员循环拆分（ADR-0066）；映射目录未配置必须未归类
// 入账，整封仍定稿。EJ 一个信封型对两个事实类型（exception-journey-alternate/-return），
// 登记仍按信封型一行——类型分岔已在适配器用例钉住，这里证装配一拍。
// 不种 PAR-VIS-01，不登记 tracking-projection.derived。

const (
	deriveExceptionJourneyConsumerName = "visibility-exception/derive-projection-from-exception-journey"
	exceptionJourneyOriginalRef        = "SYN-JOURNEY-01"
	exceptionJourneyNewRef             = "SYN-JOURNEY-02"
	exceptionJourneyBasisRef           = "SYN-DISPOSITION-01"
	exceptionJourneyMemberOne          = "SYN-PARCEL-01"
	exceptionJourneyMemberTwo          = "SYN-PARCEL-02"
)

// Covers: 生产 wireDispatcher 一拍——旅程信封定稿 published==1，两个成员各得一条
// 未归类投影（成员维进引用不进类型，ADR-0066）。不要学揽收 FanOut 去要 published==0。
func TestARecordedExceptionJourneyDerivesAProjectionPerMemberAndPublishes(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	eventID := recordRecordedExceptionJourney(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("旅程只投 VE 却没定稿：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}

	assertUnclassifiedJourneyProjection(t, fixture, exceptionJourneyMemberOne)
	assertUnclassifiedJourneyProjection(t, fixture, exceptionJourneyMemberTwo)
	if n := fixture.countInbox(t, deriveExceptionJourneyConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
}

// recordRecordedExceptionJourney 站在 TF 侧落一条双成员替代旅程并把意图入队——信封
// 只带旅程幂等键四维，成员清单由消费方按键重读本体取回。
func recordRecordedExceptionJourney(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	journeys, err := tfpostgres.NewAlternateJourneys(fixture.db)
	if err != nil {
		t.Fatalf("构造替代旅程库：%v", err)
	}
	handoff, err := tfpostgres.NewOutboxExceptionJourneyHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造旅程交接：%v", err)
	}

	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	startedAt := time.Now().UTC().Add(-2 * time.Minute)
	journey, err := tfdomain.FormAlternateJourney(tfdomain.AlternateJourneySpec{
		TenantID:        tenant,
		Journey:         mustTF(t, tfdomain.NewJourneyReference, exceptionJourneyNewRef),
		Purpose:         tfdomain.AlternateJourneyPurpose,
		OriginalJourney: mustTF(t, tfdomain.NewJourneyReference, exceptionJourneyOriginalRef),
		BasisKind:       tfdomain.ServiceDispositionDecision,
		Basis:           mustTF(t, tfdomain.NewDispositionBasisReference, exceptionJourneyBasisRef),
		Members: []tfdomain.CarriedObjectReference{
			mustTF(t, tfdomain.NewCarriedObjectReference, exceptionJourneyMemberOne),
			mustTF(t, tfdomain.NewCarriedObjectReference, exceptionJourneyMemberTwo),
		},
		StartedAt: startedAt,
	})
	if err != nil {
		t.Fatalf("形成替代旅程：%v", err)
	}

	record := tfports.AlternateJourneyRecord{
		Key: tfports.AlternateJourneyKey{
			TenantID: journey.TenantID(),
			Original: journey.OriginalJourney(),
			Purpose:  journey.Purpose(),
			Basis:    journey.Basis(),
		},
		ContentDigest: "SYN-JOURNEY-DIGEST-01",
		Journey:       journey,
		RecordedAt:    startedAt.Add(time.Second),
	}
	var outcome tfports.AlternateJourneySaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = journeys.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffExceptionJourney(txCtx, tfports.AlternateJourneyIntent{Record: record})
	})
	if outcome != tfports.AlternateJourneySaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	// 信封 ID 取旅程幂等键加类型段，与 OutboxExceptionJourneyHandoff 的 ADR-0043 认领一致。
	return tenant.String() + "/" + exceptionJourneyOriginalRef + "/" +
		tfdomain.AlternateJourneyPurpose.String() + "/" + exceptionJourneyBasisRef + "/exception-journey"
}

func assertUnclassifiedJourneyProjection(t *testing.T, fixture *synVerticalFixture, member string) {
	t.Helper()

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, member)

	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(), tenant, parcel)
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatalf("成员 %s 没有当前投影——逐成员拆分没走到它", member)
	}
	if len(projection.Entries()) != 1 {
		t.Fatalf("成员 %s entries = %d, want 1", member, len(projection.Entries()))
	}
	entry := projection.Entries()[0]
	if _, classified := entry.Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明里程碑")
	}
	if entry.MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q, want MAPPING_NOT_CONFIGURED", entry.MappingVersion())
	}
	if entry.Fact().Kind().String() != "exception-journey-alternate" {
		t.Fatalf("kind = %q, want exception-journey-alternate（目的分格进类型，ADR-0066）", entry.Fact().Kind())
	}
	wantFact := "exception-journey/" + member + "/" + exceptionJourneyOriginalRef + "/" +
		tfdomain.AlternateJourneyPurpose.String() + "/" + exceptionJourneyBasisRef
	if entry.Fact().Fact().String() != wantFact {
		t.Fatalf("fact = %q, want %s（成员维与键三维进引用）", entry.Fact().Fact(), wantFact)
	}
}
