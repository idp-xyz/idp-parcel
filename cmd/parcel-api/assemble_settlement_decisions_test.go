package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证运营确认口与截单发布口的生产装配（ADR-0339 决定一、二；票 product-strategy-boundary/22 第 2 笔）：
// 端点行按特性登记处那一份 build 与 mount 装上，编排是 buildSettlementOrchestrations 装的真确认、真截单；操作者 Intake 是生产
// 那只铸造器经真操作者册铸的。准入范围取放行一切的替身，理由同手工外部事实那一份。一笔预估客户费用、它的七项确认事实与一份
// 已到的确认依据直写进册，对账安排与归属日形态照受控登记口的载荷登。证据只记 `S`。

const (
	decisionsTenant    = "SYN-TENANT-01"
	decisionsClerk     = "SYN-CLERK-PSB22"
	decisionsPublisher = "SYN-PUBLISHER-PSB22"
	decisionsAccount   = "SYN-SA-PSB22"
	decisionsCharge    = "SYN-CHARGE-PSB22-1"
	decisionsFeeItem   = "SYN-FEE-ITEM-FREIGHT"

	chargeConfirmationFace   = "/settlement-charge-confirmations"
	statementPublicationFace = "/settlement-statement-publications"
)

type settlementDecisionsFixture struct {
	db        *bentopg.DB
	tenant    sadomain.TenantID
	endpoints map[string]http.Handler
}

func newSettlementDecisionsFixture(t *testing.T) *settlementDecisionsFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	registerOperatorIn(t, db, operators, decisionsTenant, decisionsClerk, interval, accessidentity.DecisionChargeConfirmation)
	registerOperatorIn(t, db, operators, decisionsTenant, decisionsPublisher, interval, accessidentity.DecisionStatementPublication)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	endpoints := map[string]http.Handler{}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*settlementDecisionsWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(endpointSources{db: db}, endpointChannels{operatorDecisions: decisions})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, endpoint := range mounted {
			endpoints[endpoint.Pattern] = endpoint.Handler
		}
	}
	for _, pattern := range []string{chargeConfirmationFace, statementPublicationFace} {
		if endpoints[pattern] == nil {
			t.Fatalf("%s 没按登记处挂上", pattern)
		}
	}
	fixture := &settlementDecisionsFixture{db: db, tenant: mustSettlementValue(t, sadomain.NewTenantID, decisionsTenant), endpoints: endpoints}
	fixture.seedCharge(t)
	return fixture
}

func mustSettlementValue[T any](t *testing.T, parse func(string) (T, error), value string) T {
	t.Helper()
	parsed, err := parse(value)
	if err != nil {
		t.Fatalf("%q：%v", value, err)
	}
	return parsed
}

// seedCharge 登一户账户、一笔预估客户费用、它的七项确认事实、费用项目的确认条件与一份已到的依据，以及对账安排与按确认日
// 归属的形态——补拍确认能走到`已确认`并形成归属日，截单能把它收进本期。确认与截单两个时点不在这里登，由用例自己登。
func (fixture *settlementDecisionsFixture) seedCharge(t *testing.T) {
	t.Helper()
	db, tenant := fixture.db, fixture.tenant
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		t.Fatal(err)
	}
	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	catalogues, err := sapostgres.NewSettlementCatalogues(db)
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := sapostgres.NewChargeConfirmationConditions(db)
	if err != nil {
		t.Fatal(err)
	}
	arrangements, err := sapostgres.NewStatementArrangements(db)
	if err != nil {
		t.Fatal(err)
	}
	attributions, err := sapostgres.NewChargeAttributions(db)
	if err != nil {
		t.Fatal(err)
	}
	key, err := sadomain.NewSettlementAccountKey(
		mustSettlementValue(t, sadomain.NewLegalEntityReference, "SYN-LE-01"),
		mustSettlementValue(t, sadomain.NewSettlementCounterpartyReference, "SYN-PARTY-SHIPPER-01"),
		sadomain.ChargeReceivable,
		mustSettlementValue(t, sadomain.NewCurrencyCode, "CNY"),
		mustSettlementValue(t, sadomain.NewSettlementPolicyReference, "SYN-SETTLEMENT-PREPAID-01"))
	if err != nil {
		t.Fatal(err)
	}
	account, err := sadomain.NewSettlementAccount(mustSettlementValue(t, sadomain.NewSettlementAccountID, decisionsAccount), key,
		sadomain.SettlementCounterpartyReference{}, false, mustSettlementValue(t, sadomain.NewResponsibilityBasis, "SYN-CONTRACT-01"))
	if err != nil {
		t.Fatal(err)
	}
	cny := mustSettlementValue(t, sadomain.NewCurrencyCode, "CNY")
	formedAt := time.Now().UTC().Add(-2 * time.Hour)
	charge, err := sadomain.FormCustomerCharge(sadomain.CustomerChargeSpec{
		ID:                 mustSettlementValue(t, sadomain.NewCustomerChargeID, decisionsCharge),
		FeeItem:            mustSettlementValue(t, sadomain.NewFeeItemReference, decisionsFeeItem),
		Evaluation:         mustSettlementValue(t, sadomain.NewSellEvaluationReference, "SYN-EVAL-PSB22-1"),
		OriginalCurrency:   cny,
		OriginalMinor:      9600,
		SettlementCurrency: cny,
		SettlementMinor:    9600,
		Stage:              sadomain.ChargeEstimated,
		FormedAt:           formedAt,
		SourceOccurredAt:   formedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	facts, err := sadomain.NewChargeConfirmationFactRegistration(charge.ID(), sadomain.ConfirmedChargeFacts{
		ResponsibleEntity:    key.LegalEntity(),
		Counterparty:         key.Counterparty(),
		Direction:            key.Direction(),
		SettlementAccount:    account.ID(),
		ContractBasis:        mustSettlementValue(t, sadomain.NewContractBasisReference, "SYN-CONTRACT-01"),
		PrimaryChargingScope: mustSettlementValue(t, sadomain.NewChargingScopeReference, "SYN-SCOPE-01"),
		SourceFact:           mustSettlementValue(t, sadomain.NewSourceFactReference, "SYN-INTAKE-PSB22-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	basis := mustSettlementValue(t, sadomain.NewConfirmationBasisReference, "final-outcome/SYN-PARCEL-PSB22/EFFECTIVE_DELIVERY/V1")
	_, arrangement, err := registrationjson.StatementArrangementFromJSON([]byte(
		`{"tenantId":"` + decisionsTenant + `","accountId":"` + decisionsAccount + `","timeZone":"Asia/Shanghai","cutoffMinute":0}`))
	if err != nil {
		t.Fatal(err)
	}
	_, form, err := registrationjson.ChargeAttributionFromJSON([]byte(
		`{"tenantId":"` + decisionsTenant + `","feeItem":"` + decisionsFeeItem + `","form":"CHARGE_CONFIRMED"}`))
	if err != nil {
		t.Fatal(err)
	}
	at := formedAt
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := accounts.Save(ctx, tenant, account, at); err != nil {
			return err
		}
		if _, err := charges.SaveEstimated(ctx, tenant, charge); err != nil {
			return err
		}
		if _, err := catalogues.SaveChargeConfirmationFacts(ctx, tenant, facts, at); err != nil {
			return err
		}
		if _, err := conditions.SaveConfirmationCondition(ctx, tenant, charge.FeeItem(), sadomain.EffectiveDeliveryFinalizedBasis(), at); err != nil {
			return err
		}
		if _, err := conditions.RecordConfirmationBasis(ctx, tenant, charge.ID(), charge.FeeItem(), sadomain.EffectiveDeliveryFinalizedBasis(), basis, at); err != nil {
			return err
		}
		if _, err := arrangements.SaveStatementArrangement(ctx, tenant, arrangement, at); err != nil {
			return err
		}
		_, err := attributions.SaveChargeAttribution(ctx, tenant, form, at)
		return err
	}); err != nil {
		t.Fatalf("登夹具：%v", err)
	}
}

func (fixture *settlementDecisionsFixture) adoptMoment(t *testing.T, moment sadomain.SettlementMoment) {
	t.Helper()
	moments, err := sapostgres.NewSettlementMoments(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := moments.SaveSettlementMoment(ctx, fixture.tenant, moment, time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (fixture *settlementDecisionsFixture) post(t *testing.T, pattern, subject, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+subject)
	response := httptest.NewRecorder()
	fixture.endpoints[pattern].ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%s 答复不是 JSON：%s", pattern, response.Body.String())
	}
	return response.Code, answer
}

func (fixture *settlementDecisionsFixture) text(t *testing.T, query string, args ...any) string {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var value *string
	if err := querier.QueryRow(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("%s：%v", query, err)
	}
	if value == nil {
		return ""
	}
	return *value
}

const chargeStageQuery = `SELECT stage FROM settlement_accounting.customer_charge WHERE tenant_id = $1 AND charge_id = $2`

// Covers: AT-SA-221（ADR-0339 决定一）——持确认授予的人点名一笔费用：确认时点没登答未配置、不调编排；登了之后确认、答 201，重放答
// 已确认；没登确认条件的费用答 200 带未决原因；持发布授予的人打这一口答未授予。
func TestTheChargeConfirmationFaceConfirmsThroughTheSettlementMoment(t *testing.T) {
	fixture := newSettlementDecisionsFixture(t)
	body := `{"charge":"` + decisionsCharge + `"}`

	if status, answer := fixture.post(t, chargeConfirmationFace, decisionsPublisher, body); status != http.StatusForbidden {
		t.Fatalf("持发布授予的人确认 = %d %v，want 403", status, answer)
	}
	if status, answer := fixture.post(t, chargeConfirmationFace, decisionsClerk, body); status != http.StatusOK ||
		answer["outcome"] != "SETTLEMENT_MOMENT_UNCONFIGURED" {
		t.Fatalf("没登确认时点 = %d %v", status, answer)
	}
	if stage := fixture.text(t, chargeStageQuery, decisionsTenant, decisionsCharge); stage != "ESTIMATED" {
		t.Fatalf("没登时点却动了费用：%s", stage)
	}

	fixture.adoptMoment(t, sadomain.SettlementMomentConfirm)
	if status, answer := fixture.post(t, chargeConfirmationFace, decisionsClerk, body); status != http.StatusCreated ||
		answer["outcome"] != "CHARGE_CONFIRMED" {
		t.Fatalf("确认 = %d %v", status, answer)
	}
	if stage := fixture.text(t, chargeStageQuery, decisionsTenant, decisionsCharge); stage != "CONFIRMED" {
		t.Fatalf("费用阶段 = %s", stage)
	}
	if status, answer := fixture.post(t, chargeConfirmationFace, decisionsClerk, body); status != http.StatusOK ||
		answer["outcome"] != "ALREADY_CONFIRMED" {
		t.Fatalf("重放 = %d %v", status, answer)
	}
	if status, _ := fixture.post(t, chargeConfirmationFace, decisionsClerk, `{"charge":"`+decisionsCharge+`","tenantId":"SYN-TENANT-02"}`); status != http.StatusBadRequest {
		t.Fatalf("自报租户 = %d，want 400", status)
	}

	// 另一个费用项目没登确认条件：租户取值缺席是答案，200 带未决原因，不是没形成答案。
	fixture.seedUnconditionedCharge(t)
	if status, answer := fixture.post(t, chargeConfirmationFace, decisionsClerk, `{"charge":"SYN-CHARGE-PSB22-NOCOND"}`); status != http.StatusOK ||
		answer["outcome"] != "CONFIRM_UNDECIDED" || answer["undecidedReason"] != "CONDITION_UNCONFIGURED" {
		t.Fatalf("没登确认条件 = %d %v", status, answer)
	}
}

func (fixture *settlementDecisionsFixture) seedUnconditionedCharge(t *testing.T) {
	t.Helper()
	charges, err := sapostgres.NewCustomerCharges(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	cny := mustSettlementValue(t, sadomain.NewCurrencyCode, "CNY")
	formedAt := time.Now().UTC().Add(-time.Hour)
	charge, err := sadomain.FormCustomerCharge(sadomain.CustomerChargeSpec{
		ID:                 mustSettlementValue(t, sadomain.NewCustomerChargeID, "SYN-CHARGE-PSB22-NOCOND"),
		FeeItem:            mustSettlementValue(t, sadomain.NewFeeItemReference, "SYN-FEE-ITEM-UNCONDITIONED"),
		Evaluation:         mustSettlementValue(t, sadomain.NewSellEvaluationReference, "SYN-EVAL-PSB22-2"),
		OriginalCurrency:   cny,
		OriginalMinor:      500,
		SettlementCurrency: cny,
		SettlementMinor:    500,
		Stage:              sadomain.ChargeEstimated,
		FormedAt:           formedAt,
		SourceOccurredAt:   formedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := charges.SaveEstimated(ctx, fixture.tenant, charge)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// Covers: AT-SA-222、AT-SA-223（ADR-0339 决定二、三）——持发布授予的人列一期费用、报合计发布：截单时点没登答未配置；登了之后合计不平答勾稽不平、不发；
// 平了发布、答 201 带单号与入本期的费用，客户费用读口随之透出归属日与所在对账单；同一份重放答已存在。
func TestTheStatementPublicationFacePublishesAListedPeriod(t *testing.T) {
	fixture := newSettlementDecisionsFixture(t)
	fixture.adoptMoment(t, sadomain.SettlementMomentConfirm)
	if status, answer := fixture.post(t, chargeConfirmationFace, decisionsClerk, `{"charge":"`+decisionsCharge+`"}`); status != http.StatusCreated {
		t.Fatalf("先确认 = %d %v", status, answer)
	}
	// 对账安排的截单时刻登成 0 点：本地时刻一到截单时刻就归下一日，确认那一刻的归属日因此是上海的明天。
	last := time.Now().In(time.FixedZone("CST", 8*3600)).AddDate(0, 0, 1).Format("2006-01-02")
	cutOffAt := time.Now().UTC().Format(time.RFC3339)
	publication := func(total string) string {
		return `{"number":"SYN-STMT-PSB22-1","account":"` + decisionsAccount + `","period":"SYN-PERIOD-2026-10","version":"v1",` +
			`"currency":"CNY","charges":["` + decisionsCharge + `"],"lastAttributionDate":"` + last + `","declaredTotal":"` + total + `",` +
			`"cutOffAt":"` + cutOffAt + `"}`
	}

	if status, answer := fixture.post(t, statementPublicationFace, decisionsClerk, publication("9600")); status != http.StatusForbidden {
		t.Fatalf("持确认授予的人发布 = %d %v，want 403", status, answer)
	}
	if status, answer := fixture.post(t, statementPublicationFace, decisionsPublisher, publication("9600")); status != http.StatusOK ||
		answer["outcome"] != "SETTLEMENT_MOMENT_UNCONFIGURED" {
		t.Fatalf("没登截单时点 = %d %v", status, answer)
	}

	fixture.adoptMoment(t, sadomain.SettlementMomentCutOff)
	if status, answer := fixture.post(t, statementPublicationFace, decisionsPublisher, publication("9500")); status != http.StatusOK ||
		answer["outcome"] != "STATEMENT_IMBALANCE" {
		t.Fatalf("合计不平 = %d %v", status, answer)
	}
	status, answer := fixture.post(t, statementPublicationFace, decisionsPublisher, publication("9600"))
	if status != http.StatusCreated || answer["outcome"] != "STATEMENT_PUBLISHED" || answer["statement"] != "SYN-STMT-PSB22-1" ||
		answer["total"] != "9600" {
		t.Fatalf("发布 = %d %v", status, answer)
	}
	if lines, _ := answer["lines"].([]any); len(lines) != 1 || lines[0] != decisionsCharge {
		t.Fatalf("入本期 = %v", answer["lines"])
	}
	// 客户费用读口透出归属日与所在对账单，运营据此列下一期的清单（ADR-0339 决定三）。
	catalogue, err := sapostgres.NewChargeCatalogue(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := catalogue.ListCustomerCharges(t.Context(), fixture.tenant, 10)
	if err != nil || len(rows) != 1 || rows[0].AttributionDate != last || rows[0].BilledOn != "SYN-STMT-PSB22-1" {
		t.Fatalf("读口 = %+v err = %v，want 归属日 %s、所在单 SYN-STMT-PSB22-1", rows, err, last)
	}
	if status, answer := fixture.post(t, statementPublicationFace, decisionsPublisher, publication("9600")); status != http.StatusOK ||
		answer["outcome"] != "EXISTING_STATEMENT" {
		t.Fatalf("重放 = %d %v", status, answer)
	}
	if status, _ := fixture.post(t, statementPublicationFace, decisionsPublisher, strings.Replace(publication("9600"), `"9600"`, `9600`, 1)); status != http.StatusBadRequest {
		t.Fatalf("合计写成数字 = %d，want 400", status)
	}
}
