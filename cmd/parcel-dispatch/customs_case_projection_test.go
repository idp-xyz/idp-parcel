package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 CONS-PROJ-CC-CASE-B：CC 关务案件建立只投 VE 投影，不 FanOut 给 PS。
// 一封信带全体成员关联，消费侧按成员循环拆分（ADR-0066）；映射目录未配置必须未归类
// 入账，整封仍定稿。成员维与案件维一并进事实引用；建立无语义分支，单一事实类型。
// 不种 PAR-VIS-01；派生信封 tracking-projection.derived 由生产 wireDispatcher 登记进客户视图链
// （WIRE-CUSTOMER-VIEW），本文件不为测试另装订阅者——它们在下一拍定稿，数拍内 published 时要把它们算进去。

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

// Covers: 票 sa-cc/34 裁决 5 / 判据 (4)——同一案件事实以两个不同的信封 ID 各投一封（换形前后各一份：指纹形与
// `760332c7` 上的五维串接形），Inbox 门按 ID 放行第二封，VE 的幂等靠 ports.FactKey + 内容摘要而不靠信封 ID：
// 两封各自 PUBLISHED、两行 inbox，成员投影仍各一条，派生信封仍只有首封那两封。第二封在 DeriveProjectionHandler 里
// 只剩 FactExistingResult 一条路——派生了会多一条 entry 并多入队一封派生信封，同键异摘要才是冲突而两封内容同源，
// 未接受与首封矛盾——所以「两封都定稿、派生信封不增」就是它的可观测证据。
func TestTheSameCustomsCaseUnderTwoEnvelopeIDsDerivesOnce(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fingerprintID := recordEstablishedCustomsCase(t, fixture)
	tenant := fixture.identity.TenantID().String()
	concatenatedID := tenant + "/" + customsCaseJurisdiction + "/" +
		ccdomain.ImportManifest.String() + "/" + customsCaseProcedure + "/" + customsCaseObligation
	reissueUnderAnotherEnvelopeID(t, fixture, fingerprintID, concatenatedID)

	// 两封同分区、分区一次只放一个头，所以要两拍。第 1 拍定稿指纹形那封，VE 派生两条成员投影并各入队一封派生
	// 信封；第 2 拍定稿的是重发的串接形那封加那两封派生信封（客户视图链接住它们，WIRE-CUSTOMER-VIEW）。所以拍内
	// published 不是本票的判据——它把派生信封与要证的两封混在一起数；每拍只弱断「定稿了东西」，定稿与否按两封各自的
	// status 断。
	for beat := 1; beat <= 2; beat++ {
		published, err := fixture.beat.DispatchOnce(ctx)
		if err != nil {
			t.Fatalf("第 %d 拍：%v", beat, err)
		}
		if published < 1 {
			t.Fatalf("第 %d 拍一封都没定稿；失败码 = %q / %q", beat,
				recordedFailureCode(t, fixture.db, fingerprintID), recordedFailureCode(t, fixture.db, concatenatedID))
		}
	}
	for _, id := range []string{fingerprintID, concatenatedID} {
		if status := outboxStatus(t, fixture.db, id); status != "PUBLISHED" {
			t.Fatalf("两封都该定稿：%s 的 status = %q, want PUBLISHED；失败码 = %q",
				id, status, recordedFailureCode(t, fixture.db, id))
		}
		if n := fixture.countInbox(t, deriveCustomsCaseConsumerName, id); n != 1 {
			t.Fatalf("Inbox 门该按 ID 放行每一封：%s 的 inbox 行数 = %d, want 1", id, n)
		}
	}
	// 派生信封只有首封每成员一封：第二封若没答 FactExistingResult 而重新派生，这里会多出两封。
	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != 2 {
		t.Fatalf("派生信封 = %d, want 2（首封每成员一封；第二封同键同摘要不得再派生）", n)
	}
	assertUnclassifiedCaseProjection(t, fixture, customsCaseMemberOne)
	assertUnclassifiedCaseProjection(t, fixture, customsCaseMemberTwo)
}

// outboxStatus 读回一封信在 outbox 里的定稿状态。本票的两封与它们引出的派生信封同拍定稿，拍内 published 数不出
// 「哪一封」，只有按 event_id 查 status 才分得开——与 recordedFailureCode 同一读法，一个看成、一个看败。
func outboxStatus(t *testing.T, db *bentopg.DB, eventID string) string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var status string
	if err := querier.QueryRow(t.Context(),
		`SELECT status FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID,
	).Scan(&status); err != nil {
		t.Fatalf("读回 %s 的 status：%v", eventID, err)
	}
	return status
}

// reissueUnderAnotherEnvelopeID 把 outbox 里某封信原样再入队一份、只换信封 ID——模拟换形前后同一事实各发一封。
// 从库里读回而不在测试里重拼载荷：证的是「同一份内容」，拼一份第二形的载荷只会证测试自己。
func reissueUnderAnotherEnvelopeID(t *testing.T, fixture *synVerticalFixture, eventID, otherID string) {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	envelope := eventing.Envelope{ID: eventing.EventID(otherID)}
	var (
		eventType string
		version   int64
		payload   []byte
	)
	if err := querier.QueryRow(t.Context(),
		`SELECT source, spec_version, event_type, event_version, scope, subject, partition_key,
		        occurred_at, recorded_at, content_type, payload
		   FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID,
	).Scan(&envelope.Source, &envelope.SpecVersion, &eventType, &version, &envelope.Scope, &envelope.Subject,
		&envelope.PartitionKey, &envelope.OccurredAt, &envelope.RecordedAt, &envelope.ContentType, &payload,
	); err != nil {
		t.Fatalf("读回信封 %s：%v", eventID, err)
	}
	envelope.Type = eventing.EventType(eventType)
	envelope.Version = eventing.EventVersion(version)
	envelope.Payload = payload
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		return store.Enqueue(txCtx, envelope)
	})
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
	// 信封 ID 由案件身份键五维按生产同一公式重算（票 sa-cc/34 裁决 3：口名 + 五维全进哈希），与
	// OutboxCustomsCaseHandoff 的 ADR-0043 认领一致；这里不再写字面串接——那串如今只是分区键。
	return string(outboxintent.FingerprintEventID("customs-case",
		tenant.String(), customsCaseJurisdiction, ccdomain.ImportManifest.String(), customsCaseProcedure, customsCaseObligation))
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
