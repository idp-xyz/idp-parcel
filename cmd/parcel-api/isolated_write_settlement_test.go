package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

// Covers: 票 operator-channel/08 完成判据「隔离环境里上列各口对合成写答业务结果而不是 ACCESS_CHANNEL_NOT_CONFIGURED」——
// 外部资金事实采用口。Intake 走进程自己那道门，编排走生产装配（buildSettlementRegistrationOrchestration），中间是生产的端点
// 构造函数：三件都是 main 交给路由的那一份。带 tenantId 的载荷在 Intake 就拒、不带 outcome。测试输入是隔离合成，只记 `S`。
func TestIsolatedExternalFundsFactAdoptionAnswersABusinessOutcomeAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildSettlementRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配结算登记编排：%v", err)
	}
	endpoint := settlementhttp.NewRegisterExternalFundsFactEndpoint(isolatedWriteAdmissionForTest(t).settlementIntake(),
		registration.externalFundsFact)
	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/settlement-external-funds-fact-registrations", strings.NewReader(body)))
		return recorder
	}
	body := `{"factRef":"SYN-FUNDS-FACT-0801","sourceRef":"SYN-FUNDS-SOURCE/bank-08","payerRef":"SYN-PAYER/consignee-08",` +
		`"kind":"RECEIPT_CONFIRMED","currency":"SGD","amountMinor":12345,"version":"v1","occurredAt":"2026-09-24T15:00:00+08:00"}`

	adopted := post(body)
	if adopted.Code != http.StatusCreated || registrationOutcome(t, adopted) != "FUNDS_FACT_ADOPTED" {
		t.Fatalf("合成载荷答 %d %s，want 201 FUNDS_FACT_ADOPTED", adopted.Code, adopted.Body.String())
	}

	selfReported := post(`{"tenantId":"SYN-TENANT-01",` + strings.TrimPrefix(body, "{"))
	if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
		t.Fatalf("带 tenantId 的载荷答 %d %s，want 400 MALFORMED_REQUEST", selfReported.Code, selfReported.Body.String())
	}
	assertNoOutcome(t, selfReported, "/settlement-external-funds-fact-registrations")
}
