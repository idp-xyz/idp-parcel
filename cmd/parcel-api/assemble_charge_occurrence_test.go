package main

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// unwiredChargeOccurrenceRegistration 是端点表装配测试里发生项登记那一格的占位：未配置与渠道换口的用例都钉在编排
// 之前，走到它就是 Intake 漏了一道。
type unwiredChargeOccurrenceRegistration struct{}

func (unwiredChargeOccurrenceRegistration) Register(
	context.Context,
	tfapp.RegisterChargeOccurrenceCommand,
) (tfapp.RegisterChargeOccurrenceResult, error) {
	return tfapp.RegisterChargeOccurrenceResult{}, errOrchestrationNotWired
}

const (
	chargeTenant    = "SYN-TENANT-01"
	chargeSegment   = "SYN-SEG-SZX-SIN-ASSEMBLY"
	chargeParcel    = "SYN-PARCEL-ASSEMBLY"
	chargeAgreement = "SYN-SUPPLIER-TRUNK-CN-SG-01"
	chargeProvider  = "SYN-PARTY-CARRIER-TRUNK-01"

	// 干线协议的三版（ADR-0209）：v1 没声明收费发生条件，v2 实际履约按件计，v3 实际履约按段计。
	chargeUndeclaredVersion = "v1"
	chargePerPieceVersion   = "v2"
	chargePerSegmentVersion = "v3"
)

func chargeValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// seedTrunkAgreementAndEndedSegment 登三版已生效的干线协议（见 chargePerPieceVersion 一组常量），再登一个参与已以下
// 一次交接结束的干线段。
func seedTrunkAgreementAndEndedSegment(t *testing.T, db *bentopg.DB) {
	t.Helper()
	seedTrunkAgreementAndEndedSegmentOf(t, db, chargeParcel)
}

// seedTrunkAgreementAndEndedSegmentOf 同上，段里每个包裹各一条已以下一次交接结束的参与。
func seedTrunkAgreementAndEndedSegmentOf(t *testing.T, db *bentopg.DB, parcels ...string) {
	t.Helper()
	ctx := t.Context()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := pcdomain.NewApprovalBasis(chargeValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-TRUNK"),
		chargeValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-TRUNK"), approvedAt)
	if err != nil {
		t.Fatal(err)
	}
	type trunkVersion struct {
		version   pcdomain.CommercialVersion
		agreement pcdomain.SupplierAgreement
	}
	var versions []trunkVersion
	for label, form := range map[string]pcdomain.ChargeQuantityForm{
		chargeUndeclaredVersion: pcdomain.ChargeQuantityFormInvalid,
		chargePerPieceVersion:   pcdomain.PerPieceQuantity,
		chargePerSegmentVersion: pcdomain.PerSegmentQuantity,
	} {
		version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
			TenantID:      chargeValue(t, pcdomain.NewTenantID, chargeTenant),
			Kind:          pcdomain.SupplierAgreementObject,
			ObjectID:      chargeValue(t, pcdomain.NewCommercialObjectID, chargeAgreement),
			Version:       chargeValue(t, pcdomain.NewCommercialVersionLabel, label),
			Scope:         chargeValue(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-01"),
			ContentDigest: chargeValue(t, pcdomain.NewCommercialContentDigest, "sha256:"+chargeAgreement+"-"+label),
			Effective:     interval,
			Status:        pcdomain.CommercialVersionEffective,
			Approval:      approval,
			PublishedAt:   approvedAt,
			EffectiveAt:   approvedAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("协议版本 %s：%v", label, err)
		}
		agreement, err := pcdomain.NewSupplierAgreement(version,
			chargeValue(t, pcdomain.NewPartyID, chargeProvider),
			chargeValue(t, pcdomain.NewLegalEntityReference, "SYN-LE-01"),
			chargeValue(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-01"),
			chargeValue(t, pcdomain.NewPricingPlanReference, "SYN-PLAN-CN-SG-COST-01"),
			version.Effective())
		if err != nil {
			t.Fatalf("协议正文 %s：%v", label, err)
		}
		if form != pcdomain.ChargeQuantityFormInvalid {
			condition, err := pcdomain.NewChargedOccurrenceCondition(pcdomain.ActualFulfillmentChargeReason, form)
			if err != nil {
				t.Fatal(err)
			}
			conditions, err := pcdomain.NewChargeOccurrenceConditions(condition)
			if err != nil {
				t.Fatal(err)
			}
			if agreement, err = agreement.WithChargeConditions(conditions); err != nil {
				t.Fatal(err)
			}
		}
		versions = append(versions, trunkVersion{version: version, agreement: agreement})
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatal(err)
	}
	segment, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       chargeValue(t, tfdomain.NewTenantID, chargeTenant),
		Segment:        chargeValue(t, tfdomain.NewFulfillmentSegmentReference, chargeSegment),
		Participations: endedTrunkParticipations(t, parcels),
	})
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		for _, trunk := range versions {
			if outcome, err := publications.SaveVersion(txCtx, trunk.version); err != nil || outcome != pcports.PublicationSaved {
				return errors.Join(err, errors.New("save agreement version: "+outcome.String()))
			}
			if outcome, err := publications.SaveSupplierAgreement(txCtx, trunk.agreement); err != nil || outcome != pcports.SupplierAgreementSaved {
				return errors.Join(err, errors.New("save agreement body: "+outcome.String()))
			}
		}
		if outcome, err := segments.Save(txCtx, tfports.FulfillmentSegmentRecord{
			Key:        tfports.FulfillmentSegmentKey{TenantID: segment.TenantID(), Segment: segment.Segment()},
			Segment:    segment,
			RecordedAt: time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC),
		}); err != nil || outcome != tfports.SegmentSaved {
			return errors.Join(err, errors.New("save segment"))
		}
		return nil
	}); err != nil {
		t.Fatalf("夹具：%v", err)
	}
}

// endedTrunkParticipations 给每个包裹一条凭 SZX 交接入场、以 SIN 交接离场的参与。第一个包裹的交接依据沿用单包裹
// 夹具原有的那两个串。
func endedTrunkParticipations(t *testing.T, parcels []string) []tfdomain.RehydrateParticipationSpec {
	t.Helper()
	rows := make([]tfdomain.RehydrateParticipationSpec, 0, len(parcels))
	for index, parcel := range parcels {
		suffix := "ASSEMBLY"
		if index > 0 {
			suffix = parcel
		}
		rows = append(rows, tfdomain.RehydrateParticipationSpec{
			Object:     chargeValue(t, tfdomain.NewCarriedObjectReference, parcel),
			EntryKind:  tfdomain.EnteredByTransportHandover,
			EntryBasis: chargeValue(t, tfdomain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX-"+suffix),
			EnteredAt:  time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC),
			EndKind:    tfdomain.EndedByNextHandover,
			EndBasis:   chargeValue(t, tfdomain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SIN-"+suffix),
			EndedAt:    time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC),
		})
	}
	return rows
}

func chargeCommandFor(t *testing.T, agreementVersion string) tfapp.RegisterChargeOccurrenceCommand {
	t.Helper()
	return tfapp.RegisterChargeOccurrenceCommand{
		TenantID:         chargeValue(t, tfdomain.NewTenantID, chargeTenant),
		Registrant:       "https://dex.syn.example#SYN-OPERATOR-CONFIG-01",
		Reason:           tfdomain.ActualFulfillmentOccurrence,
		Segment:          chargeSegment,
		Objects:          []string{chargeParcel},
		Journey:          "SYN-JRN-" + chargeParcel,
		Provider:         chargeProvider,
		AgreementObject:  chargeAgreement,
		AgreementVersion: agreementVersion,
	}
}

func countChargeRows(t *testing.T, db *bentopg.DB) (occurrences, envelopes int) {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.transport_charge_occurrence`).Scan(&occurrences); err != nil {
		t.Fatalf("数发生项：%v", err)
	}
	if err := querier.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		tfpostgres.ChargeOccurrenceFormedEventType).Scan(&envelopes); err != nil {
		t.Fatalf("数信封：%v", err)
	}
	return occurrences, envelopes
}

// 生产装配对真库走通（隔离合成 S）：登记在自己的事务里把发生项与已形成信封同笔落地，重放答已存在且不多出第二封；协议
// 否定格与未配置什么都不写。
// Covers: AT-TF-120、AT-TF-124、AT-TF-166、AT-TF-168 的生产装配半边。
func TestChargeOccurrenceRegistrationRunsOnRealPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seedTrunkAgreementAndEndedSegment(t, db)
	registrar, err := buildChargeOccurrenceRegistration(db)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}

	undeclared, err := registrar.Register(t.Context(), chargeCommandFor(t, chargeUndeclaredVersion))
	if err != nil || undeclared.Outcome() != tfapp.ChargeOccurrenceNotConfigured || undeclared.Agreement() != chargeAgreement+"/v1" {
		t.Fatalf("没声明条件的 v1：%q %q %v", undeclared.Outcome(), undeclared.Agreement(), err)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 0 || envelopes != 0 {
		t.Fatalf("未配置之后发生项 %d 行、信封 %d 封，want 0 / 0", occurrences, envelopes)
	}

	first, err := registrar.Register(t.Context(), chargeCommandFor(t, chargePerPieceVersion))
	if err != nil || first.Outcome() != tfapp.ChargeOccurrenceRegistered {
		t.Fatalf("首登：%q %q %v", first.Outcome(), first.Detail(), err)
	}
	record, _ := first.Record()
	if record.Occurrence.Agreement().String() != chargeAgreement+"/v2" || record.Occurrence.LegalEntity().String() != "SYN-LE-01" {
		t.Fatalf("协议快照 / 法人 = %q / %q", record.Occurrence.Agreement(), record.Occurrence.LegalEntity())
	}
	if quantity, unit := record.Occurrence.Quantity(); quantity != 1 || unit.String() != "PIECE" ||
		record.Occurrence.Condition().Basis() != tfdomain.ConditionDeclaredByAgreement {
		t.Fatalf("数量/单位/依据 = %d/%q/%q", quantity, unit, record.Occurrence.Condition().Basis())
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 1 || envelopes != 1 {
		t.Fatalf("首登后发生项 %d 行、信封 %d 封，want 1 / 1", occurrences, envelopes)
	}

	replay, err := registrar.Register(t.Context(), chargeCommandFor(t, chargePerPieceVersion))
	if err != nil || replay.Outcome() != tfapp.ChargeOccurrenceExisting {
		t.Fatalf("重放：%q %v", replay.Outcome(), err)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 1 || envelopes != 1 {
		t.Fatalf("重放后发生项 %d 行、信封 %d 封，want 1 / 1", occurrences, envelopes)
	}

	unknown, err := registrar.Register(t.Context(), chargeCommandFor(t, "v9"))
	if err != nil || unknown.Outcome() != tfapp.ChargeOccurrenceAgreementNotRegistered {
		t.Fatalf("没有这一版协议：%q %v", unknown.Outcome(), err)
	}
}

// failingChargeHandoff 是交不出意图的 Outbox：证未决那一格整笔回滚，不留一份落了册而信封缺席的发生项。
type failingChargeHandoff struct{}

func (failingChargeHandoff) HandOffChargeOccurrence(context.Context, tfports.ChargeOccurrenceIntent) error {
	return errors.New("outbox unavailable")
}

// Covers: ADR-0207 决定五——编排答未决时装配点回滚整笔：发生项先落、意图交不出，提交它就是一份结算永远等不到的成本来源。
func TestAnUndecidedChargeOccurrenceRollsBackTheWholeTransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seedTrunkAgreementAndEndedSegment(t, db)
	segments, _ := tfpostgres.NewFulfillmentSegments(db)
	judgments, _ := tfpostgres.NewActualCarrierJudgments(db)
	adoptions, _ := tfpostgres.NewSegmentCommissionAdoptions(db)
	occurrences, _ := tfpostgres.NewChargeOccurrences(db)
	versions, _ := pcpostgres.NewSupplierAgreementContents(db)
	agreements, err := tfpartycommercial.NewSupplierAgreementSource(versions)
	if err != nil {
		t.Fatal(err)
	}
	registrar := transactionalChargeOccurrenceRegistrar{
		transactor: db.Transactor(),
		inner: tfapp.NewRegisterChargeOccurrenceHandler(tfapp.RegisterChargeOccurrenceDeps{
			Segments:         segments,
			CarrierJudgments: judgments,
			Adoptions:        adoptions,
			Agreements:       agreements,
			Occurrences:      occurrences,
			Handoff:          failingChargeHandoff{},
			Clock:            systemClock{},
		}),
	}
	result, err := registrar.Register(t.Context(), chargeCommandFor(t, chargePerPieceVersion))
	if err != nil || result.Outcome() != tfapp.ChargeOccurrenceUndecided || result.Detail() != tfapp.OccurrenceHandoffUnavailable {
		t.Fatalf("交意图失败：%q %q %v", result.Outcome(), result.Detail(), err)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 0 || envelopes != 0 {
		t.Fatalf("未决之后留下发生项 %d 行、信封 %d 封——应整笔回滚", occurrences, envelopes)
	}
}
