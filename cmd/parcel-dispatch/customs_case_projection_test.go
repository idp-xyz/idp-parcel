package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 CONS-PROJ-CC-CASE-B：CC 关务案件建立只投 VE 投影，不 FanOut 给 PS。
// 一封信带全体成员关联，消费侧按成员循环拆分（ADR-0066）；映射目录未配置必须未归类
// 入账，整封仍定稿。成员维与案件维一并进事实引用；建立无语义分支，单一事实类型。
// 不种 PAR-VIS-01，不登记 tracking-projection.derived。

const (
	deriveCustomsCaseConsumerName = "visibility-exception/derive-projection-from-customs-case"
	customsCaseSYNID              = "SYN-CASE-01"
	customsCaseJurisdiction       = "SYN-JURIS-01"
	customsCaseProcedure          = "SYN-PROCEDURE-01"
	customsCaseObligation         = "SYN-OBLIGATION-01"
	customsCaseMemberOne          = "SYN-PARCEL-01"
	customsCaseMemberTwo          = "SYN-PARCEL-02"
)

// Covers: 生产 wireDispatcher 一拍——案件信封定稿 published==1，两个关联包裹各得一条
// 未归类投影（成员维与案件维进引用，ADR-0066）。不要学揽收 FanOut 去要 published==0。
func TestAnEstablishedCustomsCaseDerivesAProjectionPerParcelAndPublishes(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	eventID := recordEstablishedCustomsCase(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("案件只投 VE 却没定稿：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}

	assertUnclassifiedCaseProjection(t, fixture, customsCaseMemberOne)
	assertUnclassifiedCaseProjection(t, fixture, customsCaseMemberTwo)
	if n := fixture.countInbox(t, deriveCustomsCaseConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
}

// recordEstablishedCustomsCase 站在 CC 侧建一份双包裹案件并把意图入队——信封只带
// 案件身份键五维，成员关联（含客户归属）由消费方按键重读本体取回。
func recordEstablishedCustomsCase(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	cases, err := ccpostgres.NewCustomsCases(fixture.db)
	if err != nil {
		t.Fatalf("构造案件库：%v", err)
	}
	handoff, err := ccpostgres.NewOutboxCustomsCaseHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造案件交接：%v", err)
	}

	tenant := mustCC(t, ccdomain.NewTenantID, fixture.identity.TenantID().String())
	establishedAt := time.Now().UTC().Add(-2 * time.Minute)
	customsCase, err := ccdomain.EstablishCustomsCase(ccdomain.CustomsCaseSpec{
		ID:           mustCC(t, ccdomain.NewCustomsCaseID, customsCaseSYNID),
		Jurisdiction: mustCC(t, ccdomain.NewRegulatoryJurisdictionReference, customsCaseJurisdiction),
		Direction:    ccdomain.ImportManifest,
		Procedure:    mustCC(t, ccdomain.NewCustomsProcedureReference, customsCaseProcedure),
		Obligation:   mustCC(t, ccdomain.NewObligationScopeReference, customsCaseObligation),
		Parcels: []ccdomain.CaseParcelAssociation{
			{
				Parcel:    customsCaseMemberOne,
				Customer:  "SYN-CUSTOMER-01",
				SourceRef: "shipment-request/" + customsCaseMemberOne,
			},
			{
				Parcel:    customsCaseMemberTwo,
				Customer:  "SYN-CUSTOMER-01",
				SourceRef: "shipment-request/" + customsCaseMemberTwo,
			},
		},
		EstablishedAt: establishedAt,
	})
	if err != nil {
		t.Fatalf("建立关务案件：%v", err)
	}

	key := ccports.CustomsCaseKey{
		TenantID:     tenant,
		Jurisdiction: customsCase.Jurisdiction(),
		Direction:    customsCase.Direction(),
		Procedure:    customsCase.Procedure(),
		Obligation:   customsCase.Obligation(),
	}
	var outcome ccports.CustomsCaseSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = cases.Save(txCtx, key, customsCase); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffCase(txCtx, ccports.CustomsCaseHandoffIntent{Key: key, Case: customsCase})
	})
	if outcome != ccports.CustomsCaseSaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	// 信封 ID 取案件身份键五维，与 OutboxCustomsCaseHandoff 的 ADR-0043 认领一致。
	return tenant.String() + "/" + customsCaseJurisdiction + "/" +
		ccdomain.ImportManifest.String() + "/" + customsCaseProcedure + "/" + customsCaseObligation
}

func assertUnclassifiedCaseProjection(t *testing.T, fixture *synVerticalFixture, member string) {
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
	if entry.Fact().Kind().String() != "customs-case-established" {
		t.Fatalf("kind = %q, want customs-case-established（建立无语义分支，单一类型）", entry.Fact().Kind())
	}
	wantFact := "customs-case/" + member + "/" + customsCaseSYNID
	if entry.Fact().Fact().String() != wantFact {
		t.Fatalf("fact = %q, want %s（成员维与案件维进引用）", entry.Fact().Fact(), wantFact)
	}
}

func mustCC[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
