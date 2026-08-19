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

// 本文件证 CONS-PROJ-HANDOVER-B：权威交接登记只投 VE 投影，不 FanOut 给 PS。
// 映射目录未配置必须未归类入账；本路只有 VE，映射未配置仍是 PROJECTION_DERIVED，
// 整封可以定稿。不种 PAR-VIS-01，不登记 tracking-projection.derived。

const (
	deriveHandoverConsumerName = "visibility-exception/derive-projection-from-transport-handover"
	transportHandoverObject    = "SYN-PARCEL-01"
	transportHandoverScope     = "SYN-SCOPE-01"
	transportHandoverVersion   = "SYN-HANDOVER-V1"
	handoverFactRef            = "transport-handover/" + transportHandoverObject + "/" + transportHandoverScope
)

// Covers: 生产 wireDispatcher 一拍——VE 投影未归类入账，交接信封定稿 published==1。
// 不要学揽收 FanOut 去要 published==0。派生交接与交接同分区，第一拍定稿后 derived
// 会占头；不得为测试去登记 derived。
func TestARegisteredTransportHandoverDerivesAnUnclassifiedProjectionAndPublishes(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	eventID := recordRegisteredTransportHandover(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("交接只投 VE 却没定稿：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}

	assertUnclassifiedHandoverProjection(t, fixture)
	if n := fixture.countInbox(t, deriveHandoverConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
}

func recordRegisteredTransportHandover(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handovers, err := tfpostgres.NewTransportHandovers(fixture.db)
	if err != nil {
		t.Fatalf("构造交接登记库：%v", err)
	}
	handoff, err := tfpostgres.NewOutboxTransportHandoverRegistrationHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造交接登记交接：%v", err)
	}

	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	judgedAt := time.Now().UTC().Add(-2 * time.Minute)
	handover, err := tfdomain.FormTransportHandover(tfdomain.TransportHandoverSpec{
		TenantID:          tenant,
		Object:            mustTF(t, tfdomain.NewCarriedObjectReference, transportHandoverObject),
		Scope:             mustTF(t, tfdomain.NewHandoverScopeReference, transportHandoverScope),
		ReleasedBy:        mustTF(t, tfdomain.NewHandoverPartyReference, "SYN-NODE-01"),
		ReceivedBy:        mustTF(t, tfdomain.NewHandoverPartyReference, "SYN-CARRIER-01"),
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: mustTF(t, tfdomain.NewHandoverEvidenceReference, "evidence/SYN-RELEASE-01"),
		ReceivingEvidence: mustTF(t, tfdomain.NewHandoverEvidenceReference, "evidence/SYN-RECEIVE-01"),
		Rule:              mustTF(t, tfdomain.NewHandoverRuleReference, "SYN-HANDOVER-RULE/v1"),
		Version:           mustTF(t, tfdomain.NewHandoverResultVersion, transportHandoverVersion),
		JudgedAt:          judgedAt,
	})
	if err != nil {
		t.Fatalf("形成对象级交接：%v", err)
	}

	record := tfports.TransportHandoverRecord{
		Key: tfports.TransportHandoverKey{
			TenantID: handover.TenantID(),
			Object:   handover.Object(),
			Scope:    handover.Scope(),
			Version:  handover.Version(),
		},
		ContentDigest: "SYN-HANDOVER-DIGEST-01",
		Handover:      handover,
		RecordedAt:    judgedAt.Add(time.Second),
	}
	var outcome tfports.HandoverSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = handovers.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffTransportHandover(txCtx, tfports.TransportHandoverRegistrationIntent{Record: record})
	})
	if outcome != tfports.HandoverSaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	return tenant.String() + "/" + transportHandoverObject + "/" + transportHandoverScope + "/" + transportHandoverVersion
}

func assertUnclassifiedHandoverProjection(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, transportHandoverObject)

	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(), tenant, parcel)
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatal("没有当前投影")
	}
	if len(projection.Entries()) != 1 {
		t.Fatalf("entries = %d, want 1", len(projection.Entries()))
	}
	entry := projection.Entries()[0]
	if _, classified := entry.Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明里程碑")
	}
	if entry.MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q, want MAPPING_NOT_CONFIGURED", entry.MappingVersion())
	}
	if entry.Fact().Kind().String() != "handover-handed-over" {
		t.Fatalf("kind = %q, want handover-handed-over", entry.Fact().Kind())
	}
	if entry.Fact().Fact().String() != handoverFactRef {
		t.Fatalf("fact = %q, want %s", entry.Fact().Fact(), handoverFactRef)
	}
}
