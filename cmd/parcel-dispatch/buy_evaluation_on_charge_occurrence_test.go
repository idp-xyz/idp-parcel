package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	satransport "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/transportfulfillment"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉路由表的「TF 发生项已形成 → SA 发起 BUY 评价」一条（ADR-0208；票 demo-chain-gaps/26 完成判据「真库测试用手插
// 的 TF 发生项行跑到 BUY 请求 FIRED」）。钉的是消费这一侧：发生项行经 TF 真登记册手插，信封按本消费者认的载荷形状手工
// 入队，所以本文件不证生产方交出的类型串与载荷和这里相等（见 sainbox.ChargeOccurrenceFormedEventType 头注）。PC 登一份
// 干线协议版本连同正文，SA 触发册登两行：实际履约带费用项目、取消不带。一拍之后：
//   - 正：实际履约发生项发起恰一份 BUY 请求，费用项目取触发册、主要范围取协议的商业范围、发生项与协议取自发生项，
//     请求信封随之入队——触发编排答的正是 FIRED。
//   - 入账、零请求：没登的原因（失败尝试）、登了却没登费用项目（取消）。
//   - 发生项上的协议 PC 不认：两侧矛盾，落 dispatch.publish_failed。
//   - 信封指着还没有的发生项：可见性未决，落 dispatch.consumer_undecided。
// 证据只记 S。

const (
	occurrenceFormedConsumer = "settlement-accounting/request-buy-evaluation-on-occurrence-formed"
	trunkAgreementObject     = "SYN-SUPPLIER-TRUNK-CN-SG-01"
	trunkFeeItem             = "SYN-FEE-ITEM-TRUNK-COST-01"
)

// registerTrunkAgreement 登一份已生效的干线协议版本与正文：商业范围 SYN-SCOPE-01，与演示成本卡同一个计价范围。
func registerTrunkAgreement(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	version := sellChainVersion(t, tenant, pcdomain.SupplierAgreementObject, trunkAgreementObject)
	agreement, err := pcdomain.NewSupplierAgreement(
		version,
		mustPC(t, pcdomain.NewPartyID, "SYN-PARTY-CARRIER-TRUNK-01"),
		mustPC(t, pcdomain.NewLegalEntityReference, "SYN-LE-01"),
		mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-01"),
		mustPC(t, pcdomain.NewPricingPlanReference, "SYN-PLAN-CN-SG-COST-01"),
		version.Effective(),
	)
	if err != nil {
		t.Fatalf("干线协议正文：%v", err)
	}
	// 实际履约按件计（ADR-0209）：经真登记用例登记的那一条要按协议声明判，协议没声明就答未配置、到不了信封。
	perPiece, err := pcdomain.NewChargedOccurrenceCondition(pcdomain.ActualFulfillmentChargeReason, pcdomain.PerPieceQuantity)
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := pcdomain.NewChargeOccurrenceConditions(perPiece)
	if err != nil {
		t.Fatal(err)
	}
	if agreement, err = agreement.WithChargeConditions(conditions); err != nil {
		t.Fatalf("干线协议条件：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("PC 发布登记册：%v", err)
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

// registerBuyTriggers 登两行：实际履约带费用项目，取消不带。失败尝试不登。
func registerBuyTriggers(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	triggers, err := sapostgres.NewBuyEvaluationTriggers(db)
	if err != nil {
		t.Fatalf("SA 触发册：%v", err)
	}
	fulfillment, err := sadomain.NewBuyEvaluationTriggerRegistration(
		saValue(t, sadomain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"), sadomain.BuyEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatal(err)
	}
	if fulfillment, err = fulfillment.WithFeeItem(saValue(t, sadomain.NewFeeItemReference, trunkFeeItem)); err != nil {
		t.Fatal(err)
	}
	cancellation, err := sadomain.NewBuyEvaluationTriggerRegistration(
		saValue(t, sadomain.NewOccurrenceReasonReference, "CANCELLATION"), sadomain.BuyEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		for _, registration := range []sadomain.BuyEvaluationTriggerRegistration{fulfillment, cancellation} {
			if effect, err := triggers.SaveBuyEvaluationTrigger(txCtx, saValue(t, sadomain.NewTenantID, tenant), registration, beatInstant()); err != nil || effect != saports.CatalogueRegistered {
				return errors.Join(err, errors.New("save buy evaluation trigger"))
			}
		}
		return nil
	})
}

// insertChargeOccurrence 经 TF 真登记册手插一份首版发生项：干线段上一件包裹的一次。
func insertChargeOccurrence(t *testing.T, db *bentopg.DB, tenant, id string, reason tfdomain.ChargeOccurrenceReason, agreement string) {
	t.Helper()
	spec := tfdomain.TransportChargeOccurrenceSpec{Reason: reason, Quantity: 1, OccurredAt: time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)}
	var err error
	if spec.TenantID, err = tfdomain.NewTenantID(tenant); err != nil {
		t.Fatal(err)
	}
	if spec.Occurrence, err = tfdomain.NewChargeOccurrenceReference(id); err != nil {
		t.Fatal(err)
	}
	if spec.Journey, err = tfdomain.NewJourneyReference("SYN-JRN-SYN-PARCEL-01"); err != nil {
		t.Fatal(err)
	}
	if spec.LegalEntity, err = tfdomain.NewProcurementLegalEntityReference("SYN-LE-01"); err != nil {
		t.Fatal(err)
	}
	if spec.Provider, err = tfdomain.NewServiceProviderReference("SYN-PARTY-CARRIER-TRUNK-01"); err != nil {
		t.Fatal(err)
	}
	if spec.Agreement, err = tfdomain.NewAgreementSnapshotReference(agreement); err != nil {
		t.Fatal(err)
	}
	if spec.FactBasis, err = tfdomain.NewOccurrenceBasisReference("SYN-BASIS-" + id); err != nil {
		t.Fatal(err)
	}
	if spec.Scope, err = tfdomain.NewOccurrenceScopeReference("SYN-SEGMENT-SZX-SIN-01"); err != nil {
		t.Fatal(err)
	}
	member, err := tfdomain.NewCarriedObjectReference("SYN-PARCEL-01")
	if err != nil {
		t.Fatal(err)
	}
	spec.Members = []tfdomain.CarriedObjectReference{member}
	if spec.Unit, err = tfdomain.NewQuantityUnitReference("SEGMENT"); err != nil {
		t.Fatal(err)
	}
	if spec.Validity, err = tfdomain.NewOccurrenceValidityVersion("V1"); err != nil {
		t.Fatal(err)
	}
	registrant, err := tfdomain.NewRegistrantReference("https://dex.syn.example#SYN-OPERATOR-CONFIG-01")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Condition, err = tfdomain.NewAssertedOccurrenceCondition(registrant, "SYN 断言："+agreement+" 的发生条件已满足"); err != nil {
		t.Fatal(err)
	}
	occurrence, err := tfdomain.FormTransportChargeOccurrence(spec)
	if err != nil {
		t.Fatalf("发生项 %s：%v", id, err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		t.Fatalf("TF 发生项登记册：%v", err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		outcome, err := occurrences.Save(txCtx, tfports.ChargeOccurrenceRecord{
			Key:        tfports.ChargeOccurrenceKey{TenantID: spec.TenantID, Occurrence: spec.Occurrence, Validity: spec.Validity},
			Occurrence: occurrence,
			RecordedAt: beatInstant(),
		})
		if err == nil && outcome != tfports.ChargeOccurrenceSaved {
			err = errors.New("charge occurrence was not saved fresh")
		}
		return err
	})
}

// Covers: 路由先登（ADR-0049）；一份实际履约发生项经生产路由表发起恰一份 BUY 请求（FIRED）；没登原因、没登费用项目
// 答未配置并入账；协议 PC 不认是 publish_failed；发生项读不回是 consumer_undecided。
func TestAFormedChargeOccurrenceFiresABuyEvaluationRequestThroughTheRouteTable(t *testing.T) {
	const tenant = "tenant-a"
	observed := map[string]error{}
	beat, db, store := wiredBeat(t, dispatch.WithDeliveryFailureObserver(func(delivery eventing.Delivery, _ eventing.FailureCode, err error) {
		observed[string(delivery.Envelope.ID)] = err
	}))
	registerTrunkAgreement(t, db, tenant)
	registerBuyTriggers(t, db, tenant)
	insertChargeOccurrence(t, db, tenant, "SYN-OCC-TRUNK-01", tfdomain.ActualFulfillmentOccurrence, trunkAgreementObject+"/v1")
	insertChargeOccurrence(t, db, tenant, "SYN-OCC-ATTEMPT-01", tfdomain.FailedAttemptOccurrence, trunkAgreementObject+"/v1")
	insertChargeOccurrence(t, db, tenant, "SYN-OCC-CANCEL-01", tfdomain.CancellationOccurrence, trunkAgreementObject+"/v1")
	insertChargeOccurrence(t, db, tenant, "SYN-OCC-STRANGER-01", tfdomain.ActualFulfillmentOccurrence, "SYN-SUPPLIER-UNKNOWN/v1")

	payload := func(occurrence string) string {
		return `{"tenantId":"` + tenant + `","occurrence":"` + occurrence + `","version":"V1"}`
	}
	for id, occurrence := range map[string]string{
		"occ-fired":    "SYN-OCC-TRUNK-01",
		"occ-attempt":  "SYN-OCC-ATTEMPT-01",
		"occ-cancel":   "SYN-OCC-CANCEL-01",
		"occ-stranger": "SYN-OCC-STRANGER-01",
		"occ-missing":  "SYN-OCC-MISSING-01",
	} {
		enqueueForBeat(t, db, store, id, sainbox.ChargeOccurrenceFormedEventType, payload(occurrence))
	}

	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍：%v", err)
	}
	for _, id := range []string{"occ-fired", "occ-attempt", "occ-cancel"} {
		if status := outboxStatus(t, db, id); status != "PUBLISHED" {
			t.Fatalf("%s status = %q, want PUBLISHED；失败码 = %q 原文 = %v", id, status, recordedFailureCode(t, db, id), observed[id])
		}
		if n := inboxRowsOf(t, db, occurrenceFormedConsumer, id); n != 1 {
			t.Fatalf("%s inbox 行数 = %d, want 1", id, n)
		}
	}
	if got := recordedFailureCode(t, db, "occ-stranger"); got != "dispatch.publish_failed" {
		t.Fatalf("协议不认：failure_code = %q, want dispatch.publish_failed；原文 = %v", got, observed["occ-stranger"])
	}
	if !errors.Is(observed["occ-stranger"], saapplication.ErrOccurrenceAgreementNotRegistered) {
		t.Fatalf("协议不认的原文 = %v", observed["occ-stranger"])
	}
	if got := recordedFailureCode(t, db, "occ-missing"); got != "dispatch.consumer_undecided" {
		t.Fatalf("读不回：failure_code = %q, want dispatch.consumer_undecided；原文 = %v", got, observed["occ-missing"])
	}
	if !errors.Is(observed["occ-missing"], satransport.ErrChargeOccurrenceNotVisible) {
		t.Fatalf("读不回的原文 = %v", observed["occ-missing"])
	}
	for _, id := range []string{"occ-stranger", "occ-missing"} {
		if n := inboxRowsOf(t, db, occurrenceFormedConsumer, id); n != 0 {
			t.Fatalf("%s 必须回滚：inbox 行数 = %d", id, n)
		}
	}

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(), `
		SELECT scope_ref, occurrence_id, occurrence_reason, occurrence_version, fee_item, agreement_ref, requested_by
		  FROM settlement_accounting.evaluation_request
		 WHERE tenant_id = $1`, tenant)
	if err != nil {
		t.Fatalf("读 BUY 请求册：%v", err)
	}
	defer rows.Close()
	var requests [][7]string
	for rows.Next() {
		var row [7]string
		if err := rows.Scan(&row[0], &row[1], &row[2], &row[3], &row[4], &row[5], &row[6]); err != nil {
			t.Fatalf("读请求行：%v", err)
		}
		requests = append(requests, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读请求行：%v", err)
	}
	want := [7]string{"SYN-SCOPE-01", "SYN-OCC-TRUNK-01", "ACTUAL_FULFILLMENT", "V1", trunkFeeItem,
		trunkAgreementObject + "/v1", "settlement-accounting/buy-evaluation-on-occurrence-formed"}
	if len(requests) != 1 || requests[0] != want {
		t.Fatalf("BUY 请求册 = %v, want 恰一份 %v", requests, want)
	}

	var envelopes int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		"settlement-accounting.evaluation-request.submitted",
	).Scan(&envelopes); err != nil {
		t.Fatalf("数请求信封：%v", err)
	}
	if envelopes != 1 {
		t.Fatalf("BUY 请求信封 %d 封, want 1（与请求同一笔）", envelopes)
	}
}
