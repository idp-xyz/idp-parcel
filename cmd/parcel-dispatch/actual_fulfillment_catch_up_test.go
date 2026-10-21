package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证实际履约补形成（ADR-0231）：干线协议各版经 PC 真发布、真收尾（截止与同起点退役，ADR-0233），委托照提交口的
// 形状落册，段由生产交接登记编排立起、参与由生产编排结束，采用走真采用编排（同笔交门铃），换版走真换版编排（同笔交门铃）；派发进程照生产
// 路由表一拍一拍跑。采用晚到、协议后补条件、旧版被截止或退役三种情形各走一遍：先如实留在实际履约未形成册上，换版或采用之后补形成、BUY
// 随之发起，册上那一条了结；重投与重放不多出一项。夹具全部合成，证据只记 `S`。

// publishTrunkVersion 经 PC 发布册发一版干线协议：label 那一版自 startsAt 起生效，charged 为真时声明实际履约按件计，否则不声明任何条件。
func publishTrunkVersion(t *testing.T, db *bentopg.DB, label string, startsAt time.Time, charged bool) {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustPC(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-TRUNK-"+label),
		mustPC(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-TRUNK-"+label), approvedAt)
	if err != nil {
		t.Fatal(err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustPC(t, pcdomain.NewTenantID, formationTenant),
		Kind:          pcdomain.SupplierAgreementObject,
		ObjectID:      mustPC(t, pcdomain.NewCommercialObjectID, trunkAgreementObject),
		Version:       mustPC(t, pcdomain.NewCommercialVersionLabel, label),
		Scope:         mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-01"),
		ContentDigest: mustPC(t, pcdomain.NewCommercialContentDigest, "sha256:trunk-"+label),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   approvedAt,
		EffectiveAt:   approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("干线协议 %s：%v", label, err)
	}
	agreement, err := pcdomain.NewSupplierAgreement(version,
		mustPC(t, pcdomain.NewPartyID, "SYN-PARTY-CARRIER-TRUNK-01"),
		mustPC(t, pcdomain.NewLegalEntityReference, "SYN-LE-01"),
		mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-01"),
		mustPC(t, pcdomain.NewPricingPlanReference, "SYN-PLAN-CN-SG-COST-01"),
		version.Effective())
	if err != nil {
		t.Fatal(err)
	}
	if charged {
		perPiece, err := pcdomain.NewChargedOccurrenceCondition(pcdomain.ActualFulfillmentChargeReason, pcdomain.PerPieceQuantity)
		if err != nil {
			t.Fatal(err)
		}
		conditions, err := pcdomain.NewChargeOccurrenceConditions(perPiece)
		if err != nil {
			t.Fatal(err)
		}
		if agreement, err = agreement.WithChargeConditions(conditions); err != nil {
			t.Fatal(err)
		}
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if outcome, err := publications.SaveVersion(txCtx, version); err != nil || outcome != pcports.PublicationSaved {
			return errors.Join(err, errors.New("save agreement version: "+outcome.String()))
		}
		if outcome, err := publications.SaveSupplierAgreement(txCtx, agreement); err != nil || outcome != pcports.SupplierAgreementSaved {
			return errors.Join(err, errors.New("save agreement body: "+outcome.String()))
		}
		return nil
	})
}

// closeTrunkVersion 经 PC 收尾编排让 v1 让位给 successor：同起点退役，或截止在后继的起点。
func closeTrunkVersion(t *testing.T, db *bentopg.DB, successor string, retire bool) {
	t.Helper()
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatal(err)
	}
	closer := pcapplication.NewCloseCommercialVersionHandler(publications, systemClock{})
	target := pcapplication.VersionClosureTarget{
		Tenant:    mustPC(t, pcdomain.NewTenantID, formationTenant),
		Scope:     mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-01"),
		Kind:      pcdomain.SupplierAgreementObject,
		ObjectID:  mustPC(t, pcdomain.NewCommercialObjectID, trunkAgreementObject),
		Version:   mustPC(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Successor: mustPC(t, pcdomain.NewCommercialVersionLabel, successor),
	}
	var result pcapplication.VersionClosureResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		if retire {
			result, err = closer.RetireAtSameStart(txCtx, pcapplication.RetireCommercialVersionAtSameStartCommand{
				VersionClosureTarget: target, Reference: mustPC(t, pcdomain.NewRetirementReference, "SYN-RETIRE-TRUNK-V1-DCG88"),
			})
		} else {
			result, err = closer.Cut(txCtx, pcapplication.CutCommercialVersionAtSuccessorCommand{
				VersionClosureTarget: target, Reference: mustPC(t, pcdomain.NewValidityCorrectionReference, "SYN-CUT-TRUNK-V1-DCG88"),
				CorrectedAt: formationAt(time.Hour),
			})
		}
		return err
	})
	if result.Outcome() != pcapplication.VersionClosureRegistered {
		t.Fatalf("收尾 v1 = %s %v", result.Outcome(), result.Cause())
	}
}

// reviseTrunkCommission 走真换版编排（同一笔落版、交门铃），把委托改按 successor 自 appliesFrom 起采购。
func reviseTrunkCommission(t *testing.T, db *bentopg.DB, store *outbox.Store, successor string, appliesFrom time.Time) {
	t.Helper()
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		t.Fatal(err)
	}
	agreements, err := tfpartycommercial.NewSupplierAgreementSource(versions)
	if err != nil {
		t.Fatal(err)
	}
	doorbell, err := tfpostgres.NewOutboxCommissionAgreementRevisionHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	reviser := tfapplication.NewReviseCommissionAgreementHandler(tfapplication.ReviseCommissionAgreementDeps{
		Commissions: commissions, Revisions: commissions, Agreements: agreements, Doorbell: doorbell, Clock: systemClock{},
	})
	var result tfapplication.CommissionAgreementRevisionResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = reviser.Revise(txCtx, tfapplication.ReviseCommissionAgreementCommand{
			TenantID:         mustTF(t, tfdomain.NewTenantID, formationTenant),
			DecidedBy:        "https://dex.syn.example#SYN-OPERATOR-CONFIG-01",
			Commission:       formationCommission,
			SuccessorObject:  trunkAgreementObject,
			SuccessorVersion: successor,
			AppliesFrom:      appliesFrom,
			Basis:            "SYN-DECISION-REV-TRUNK-" + successor,
		})
		return err
	})
	if result.Outcome() != tfapplication.CommissionAgreementRevised {
		t.Fatalf("换版到 %s = %s/%s", successor, result.Outcome(), result.UndecidedReason())
	}
}

// dispatchBeats 跑 n 拍，一拍失败就红。
func dispatchBeats(t *testing.T, beat Beat, n int) {
	t.Helper()
	for range n {
		if _, err := beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
	}
}

// outboxEventsOf 交回某一类信封的事件 ID 与状态。
func outboxEventsOf(t *testing.T, db *bentopg.DB, eventType string) map[string]string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := querier.Query(t.Context(), `SELECT event_id FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, eventType)
	if err != nil {
		t.Fatalf("读信封：%v", err)
	}
	defer rows.Close()
	ids := map[string]string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[id] = ""
	}
	for id := range ids {
		ids[id] = outboxStatus(t, db, id)
	}
	return ids
}

func unformedEntryOf(t *testing.T, db *bentopg.DB, parcel string) (tfports.ActualFulfillmentUnformedRecord, bool) {
	t.Helper()
	register, err := tfpostgres.NewActualFulfillmentUnformeds(db)
	if err != nil {
		t.Fatal(err)
	}
	record, found, err := register.FindByKey(t.Context(), tfports.ActualFulfillmentUnformedKey{
		TenantID: mustTF(t, tfdomain.NewTenantID, formationTenant),
		Segment:  mustTF(t, tfdomain.NewFulfillmentSegmentReference, formationSegment),
		Object:   mustTF(t, tfdomain.NewCarriedObjectReference, parcel),
	})
	if err != nil {
		t.Fatalf("读未形成册：%v", err)
	}
	return record, found
}

// listedUnformed 照查阅页那一格列一个租户未形成的那几件。
func listedUnformed(t *testing.T, db *bentopg.DB, tenant string) []tfports.ActualFulfillmentUnformedRecord {
	t.Helper()
	catalogue, err := tfpostgres.NewReviewCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := catalogue.ListActualFulfillmentUnformed(t.Context(), mustTF(t, tfdomain.NewTenantID, tenant), 50)
	if err != nil {
		t.Fatalf("查阅未形成：%v", err)
	}
	return listed
}

func buyRequests(t *testing.T, db *bentopg.DB) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.evaluation_request WHERE tenant_id = $1`, formationTenant).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	return requests
}

// Covers: AT-TF-300（S，ADR-0231 决定一、二）——采用晚于派发处理：SIN 交接那一封先跑，段还没采用委托，入账、不形成、不进未形成册；随后采用，
// 采用那一笔交出门铃，派发一拍把已结束的那一条参与补形成（成员是这件包裹、旅程 `JRN/<包裹>`、登记者是系统身份、按 v1 按件计），下一拍
// BUY 请求发出。门铃重投（inbox 认出）、补形成再跑一遍（答已存在）、SIN 那一封重放，都不多出一项。
func TestALateAdoptionFormsTheParticipationThatHadAlreadyEnded(t *testing.T) {
	const parcel = "SYN-PARCEL-DCG88-01"
	beat, db, store := wiredBeat(t)
	registerTrunkAgreement(t, db, formationTenant)
	registerBuyTriggers(t, db, formationTenant)
	saveTrunkCommission(t, db, parcel)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	sin := handOverAtSIN(t, register, db, parcel)
	dispatchUntilPublished(t, beat, db, sin)
	if n := len(formationOccurrences(t, db)); n != 0 {
		t.Fatalf("采用之前形成了 %d 项", n)
	}
	if _, found := unformedEntryOf(t, db, parcel); found {
		t.Fatal("没有采用连线的段进了未形成册")
	}

	adoptTrunkCommission(t, db)
	doorbells := outboxEventsOf(t, db, tfpostgres.SegmentCommissionAdoptionRegisteredEventType)
	if len(doorbells) != 1 || tfinbox.SegmentCommissionAdoptionRegisteredEventType != tfpostgres.SegmentCommissionAdoptionRegisteredEventType {
		t.Fatalf("采用门铃 = %v", doorbells)
	}
	dispatchBeats(t, beat, 3)
	for id, status := range outboxEventsOf(t, db, tfpostgres.SegmentCommissionAdoptionRegisteredEventType) {
		if status != "PUBLISHED" {
			t.Fatalf("门铃 %s = %s（%s）", id, status, recordedFailureCode(t, db, id))
		}
	}
	occurrences := formationOccurrences(t, db)
	if len(occurrences) != 1 || occurrences[0].journey != "JRN/"+parcel ||
		occurrences[0].registeredBy != tfapplication.ActualFulfillmentFormationRegistrant ||
		occurrences[0].agreement != trunkAgreementObject+"/v1" || len(occurrences[0].members) != 1 || occurrences[0].members[0] != parcel {
		t.Fatalf("补形成的发生项 = %+v", occurrences)
	}
	if requests := buyRequests(t, db); requests != 1 {
		t.Fatalf("BUY 请求 %d 份, want 1", requests)
	}

	former, err := productionActualFulfillmentFormer(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	var again tfapplication.CatchUpActualFulfillmentResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		again, err = former.CatchUpOnAdoption(txCtx, tfapplication.CatchUpActualFulfillmentOnAdoptionCommand{
			TenantID: mustTF(t, tfdomain.NewTenantID, formationTenant), Segment: formationSegment,
		})
		return err
	})
	if again.Disposition() != tfapplication.FormationSettled || len(again.Formations()) != 1 ||
		again.Formations()[0].Registration().Outcome() != tfapplication.ChargeOccurrenceExisting {
		t.Fatalf("补形成再跑一遍 = %s %+v", again.Disposition(), again.Formations())
	}
	lane, _ := formationLane(t, db, store)
	if err := lane.Consume(t.Context(), formationHandoverEnvelope(parcel)); err != nil {
		t.Fatalf("SIN 那一封重放：%v", err)
	}
	if n := len(formationOccurrences(t, db)); n != 1 {
		t.Fatalf("重放之后 %d 项, want 仍 1", n)
	}
}

// Covers: AT-TF-301、AT-TF-304（S，ADR-0231 决定三、四、六）——协议后补条件：委托指名的 v1 没声明实际履约的条件，v2 已同起点另发、声明了；
// 自动形成照旧按 v1 答未配置——不随 PC 新版自动改指——册上立一条（第 1 版、问的是 v1），查阅页列出它、别的租户看不到。委托换版到 v2、
// 自提交起用，换版门铃一拍补形成（按 v2 的快照），册上那一条了结、查阅页不再列，BUY 随之发出。
func TestACommissionRevisionFormsWhatTheAgreementLaterDeclared(t *testing.T) {
	const parcel = "SYN-PARCEL-DCG88-02"
	beat, db, store := wiredBeat(t)
	publishTrunkVersion(t, db, "v1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false)
	publishTrunkVersion(t, db, "v2", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true)
	registerBuyTriggers(t, db, formationTenant)
	saveTrunkCommission(t, db, parcel)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	adoptTrunkCommission(t, db)
	sin := handOverAtSIN(t, register, db, parcel)
	dispatchUntilPublished(t, beat, db, sin)

	if n := len(formationOccurrences(t, db)); n != 0 {
		t.Fatalf("v1 没声明条件却形成了 %d 项", n)
	}
	entry, found := unformedEntryOf(t, db, parcel)
	if !found || entry.State != tfports.ActualFulfillmentUnformed || entry.Entry.Reason != tfports.UnformedNotConfigured ||
		entry.Entry.Revision != 1 || entry.Entry.Agreement.Version() != "v1" || entry.Entry.Commission.String() != formationCommission {
		t.Fatalf("未形成那一条 = %+v %v", entry, found)
	}
	if listed := listedUnformed(t, db, formationTenant); len(listed) != 1 || listed[0].Entry.Key.Object.String() != parcel {
		t.Fatalf("查阅页 = %+v", listed)
	}
	if listed := listedUnformed(t, db, "SYN-TENANT-02"); len(listed) != 0 {
		t.Fatalf("别的租户看到了 %+v", listed)
	}

	reviseTrunkCommission(t, db, store, "v2", formationAt(0))
	dispatchBeats(t, beat, 3)
	for id, status := range outboxEventsOf(t, db, tfpostgres.CommissionAgreementRevisedEventType) {
		if status != "PUBLISHED" {
			t.Fatalf("换版门铃 %s = %s（%s）", id, status, recordedFailureCode(t, db, id))
		}
	}
	occurrences := formationOccurrences(t, db)
	if len(occurrences) != 1 || occurrences[0].agreement != trunkAgreementObject+"/v2" || occurrences[0].unit != "PIECE" {
		t.Fatalf("换版之后补形成 = %+v", occurrences)
	}
	if entry, _ := unformedEntryOf(t, db, parcel); entry.State != tfports.ActualFulfillmentUnformedFormed {
		t.Fatalf("册上那一条 = %s", entry.State)
	}
	if listed := listedUnformed(t, db, formationTenant); len(listed) != 0 {
		t.Fatalf("了结之后查阅页仍列 %+v", listed)
	}
	if requests := buyRequests(t, db); requests != 1 {
		t.Fatalf("BUY 请求 %d 份, want 1", requests)
	}
}

// Covers: AT-TF-302、AT-TF-305（S，ADR-0231 决定五、六；ADR-0233 越权风险点 1）——委托指名的 v1 被收尾之后结束的参与：同起点退役答已终止、
// 截止在后继起点答未生效，两格都留在未形成册上并记下那一格；委托换版到后继（退役时自提交起用，截止时自后继起点起用）之后补形成，册上了结。
func TestAClosedAgreementVersionIsTracedAndRecoveredByRevision(t *testing.T) {
	for name, retire := range map[string]bool{"同起点退役": true, "截止": false} {
		t.Run(name, func(t *testing.T) {
			parcel := "SYN-PARCEL-DCG88-RETIRED"
			successorStart, appliesFrom, want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), formationAt(0), tfports.UnformedAgreementTerminated
			if !retire {
				parcel = "SYN-PARCEL-DCG88-CUT"
				successorStart, appliesFrom, want = formationAt(2*time.Hour), formationAt(2*time.Hour), tfports.UnformedAgreementNotEffective
			}
			beat, db, store := wiredBeat(t)
			publishTrunkVersion(t, db, "v1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true)
			publishTrunkVersion(t, db, "v2", successorStart, true)
			closeTrunkVersion(t, db, "v2", retire)
			saveTrunkCommission(t, db, parcel)
			register := newHandoverRegistration(t, db, store)
			handOverAtSZX(t, register, db, parcel)
			adoptTrunkCommission(t, db)
			sin := handOverAtSIN(t, register, db, parcel)
			dispatchUntilPublished(t, beat, db, sin)

			entry, found := unformedEntryOf(t, db, parcel)
			if !found || entry.State != tfports.ActualFulfillmentUnformed || entry.Entry.Reason != want || entry.Entry.Agreement.Version() != "v1" {
				t.Fatalf("收尾之后 = %+v %v, want 未形成 %s", entry, found, want)
			}
			if n := len(formationOccurrences(t, db)); n != 0 {
				t.Fatalf("收尾之后形成了 %d 项", n)
			}

			reviseTrunkCommission(t, db, store, "v2", appliesFrom)
			dispatchBeats(t, beat, 3)
			occurrences := formationOccurrences(t, db)
			if len(occurrences) != 1 || occurrences[0].agreement != trunkAgreementObject+"/v2" {
				t.Fatalf("换版到后继之后 = %+v", occurrences)
			}
			if entry, _ := unformedEntryOf(t, db, parcel); entry.State != tfports.ActualFulfillmentUnformedFormed {
				t.Fatalf("册上那一条 = %s", entry.State)
			}
		})
	}
}

// saveAmbiguousJourneys 在 TF 旅程册上给 parcel 登两趟同一刻开始的退运旅程：那一件的发生项旅程定不下来。
func saveAmbiguousJourneys(t *testing.T, db *bentopg.DB, parcel string) {
	t.Helper()
	journeys, err := tfpostgres.NewAlternateJourneys(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, formationTenant)
	for _, name := range []string{"A", "B"} {
		basis := mustTF(t, tfdomain.NewDispositionBasisReference, "SYN-DISPOSITION-RETURN-DCG88-"+name)
		original := mustTF(t, tfdomain.NewJourneyReference, "JRN/"+parcel)
		journey, err := tfdomain.FormAlternateJourney(tfdomain.AlternateJourneySpec{
			TenantID: tenant, Journey: mustTF(t, tfdomain.NewJourneyReference, "SYN-JRN-RETURN-DCG88-"+name),
			Purpose: tfdomain.ReturnJourneyPurpose, OriginalJourney: original,
			BasisKind: tfdomain.ServiceDispositionDecision, Basis: basis,
			Members:   []tfdomain.CarriedObjectReference{mustTF(t, tfdomain.NewCarriedObjectReference, parcel)},
			StartedAt: formationAt(30 * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
			outcome, err := journeys.Save(txCtx, tfports.AlternateJourneyRecord{
				Key:           tfports.AlternateJourneyKey{TenantID: tenant, Original: original, Purpose: tfdomain.ReturnJourneyPurpose, Basis: basis},
				ContentDigest: "SYN-JOURNEY-DIGEST-" + name,
				Journey:       journey,
				RecordedAt:    formationAt(30 * time.Minute),
			})
			if err == nil && outcome != tfports.AlternateJourneySaved {
				err = errors.New("alternate journey was not saved fresh")
			}
			return err
		})
	}
}

// Covers: AT-TF-303（S，ADR-0231 决定二）——采用晚到的那一段上两件包裹，其中一件的旅程在旅程册上平局：采用门铃 PUBLISHED，另一件照常补
// 形成，平局那一件记进未形成册（FORMATION_DEFECT，类别码 AMBIGUOUS_JOURNEY 连同完整错误、根因在尾部）、查阅页列出——不因一件缺陷整封烧完失败预算、其余各件静默缺失。
func TestOneDefectiveParticipationDoesNotSinkTheLateAdoptionCatchUp(t *testing.T) {
	const parcel, tied = "SYN-PARCEL-DCG88-31", "SYN-PARCEL-DCG88-32"
	beat, db, store := wiredBeat(t)
	registerTrunkAgreement(t, db, formationTenant)
	saveTrunkCommission(t, db, parcel, tied)
	saveAmbiguousJourneys(t, db, tied)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	handOverAtSZX(t, register, db, tied)
	dispatchUntilPublished(t, beat, db, handOverAtSIN(t, register, db, parcel))
	dispatchUntilPublished(t, beat, db, handOverAtSIN(t, register, db, tied))

	adoptTrunkCommission(t, db)
	dispatchBeats(t, beat, 3)
	for id, status := range outboxEventsOf(t, db, tfpostgres.SegmentCommissionAdoptionRegisteredEventType) {
		if status != "PUBLISHED" {
			t.Fatalf("门铃 %s = %s（%s）", id, status, recordedFailureCode(t, db, id))
		}
	}
	occurrences := formationOccurrences(t, db)
	if len(occurrences) != 1 || occurrences[0].members[0] != parcel {
		t.Fatalf("补形成 = %+v, want 只有旅程定得下来的那一件", occurrences)
	}
	entry, found := unformedEntryOf(t, db, tied)
	if !found || entry.Entry.Reason != tfports.UnformedFormationDefect || !strings.HasPrefix(entry.Entry.Defect, "AMBIGUOUS_JOURNEY: ") ||
		!strings.HasSuffix(entry.Entry.Defect, tfdomain.ErrAmbiguousJourney.Error()) {
		t.Fatalf("平局那一件 = %+v %v", entry, found)
	}
	if listed := listedUnformed(t, db, formationTenant); len(listed) != 1 || listed[0].Entry.Key.Object.String() != tied {
		t.Fatalf("查阅页 = %+v", listed)
	}
}
