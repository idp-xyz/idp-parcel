package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veregistration "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// registerAutoEstablishingTriage 给出发门禁阻断信号登一条自动建案的分诊规则（合成取值，只记 S）：演示种子里这一类进人工复核，
// 本文件要看的是自动建案那一步。
func registerAutoEstablishingTriage(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	command, err := veregistration.TriageRulesFromJSON([]byte(`{
	  "tenantId": "` + fixture.identity.TenantID().String() + `",
	  "version": "SYN-VE-TRIAGE-AUTO-DEPARTURE-V1",
	  "approvedBy": "SYN-OPS-TRACKING-LEAD",
	  "effectiveFrom": "2026-01-01T00:00:00Z",
	  "entries": [
	    { "kind": "DEPARTURE_GATE_BLOCKED", "confidence": "SOURCE_CONFIRMED", "outcome": "AUTO_ESTABLISH", "team": "SYN-TEAM-EXC" }
	  ]
	}`))
	if err != nil {
		t.Fatalf("翻译分诊规则：%v", err)
	}
	registrar, err := vepostgres.NewCatalogRegistrar(fixture.db)
	if err != nil {
		t.Fatalf("构造目录写入方：%v", err)
	}
	catalogs, err := veapplication.NewCatalogRegistration(registrar)
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	var result veapplication.RegisterCatalogResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var registerErr error
		result, registerErr = catalogs.RegisterTriageRules(txCtx, command)
		return registerErr
	})
	if result.Outcome() != veapplication.CatalogRegistered {
		t.Fatalf("登记分诊规则 outcome = %q，拒绝理由 = %q", result.Outcome(), result.RefusalReason())
	}
}

// caseNumberOf 读那一件包裹上自动建立的案件的单据号两格。
func caseNumberOf(t *testing.T, fixture *synVerticalFixture, parcel string) (string, string) {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("读执行器：%v", err)
	}
	var number, reason string
	if err := querier.QueryRow(t.Context(),
		`SELECT COALESCE(document_number, ''), COALESCE(numberless_reason, '')
		   FROM visibility_exception.exception_case
		  WHERE tenant_id = $1 AND root_parcel = $2`,
		fixture.identity.TenantID().String(), parcel,
	).Scan(&number, &reason); err != nil {
		t.Fatalf("读自动建立的案件：%v", err)
	}
	return number, reason
}

// Covers: AT-VE-227 派发半边、CONTEXT「异常案件」（document-numbering/06，证据 S）——分诊自动建案经生产路由在派生事务里取案件单据号：
// 租户登了单据编号时区，案件按产品默认模板 `EX-{yyyy}{mm}{dd}-{seq:4}`、以该时区下的服务端日期出号；取号编排与派生事务是
// 本进程同一个库上装的那一套。
func TestAnAutoEstablishedCaseTakesItsNumberThroughTheProductionRoute(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoDepartureGateSignalRule(t, fixture)
	registerAutoEstablishingTriage(t, fixture)
	numbers, err := buildDocumentNumbers(fixture.db, dispatchSettings{})
	if err != nil {
		t.Fatalf("构造编号组件：%v", err)
	}
	if registered, err := numbers.Registrations.RegisterTimeZone(t.Context(), docnumber.RegisterTimeZoneCommand{
		Tenant: fixture.identity.TenantID().String(), TimeZone: "Asia/Shanghai",
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	}); err != nil || registered.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登单据编号时区：%+v %v", registered, err)
	}
	saveDepartureSegment(t, fixture)

	before := time.Now()
	eventID := recordDepartureThroughProduction(t, fixture, "SYN-MOVE-DISPATCH-02")
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("第一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}
	after := time.Now()

	number, reason := caseNumberOf(t, fixture, movementParcelBlocked)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	// 跨午夜那一瞬两头都认：号取的是落单那一刻的服务端时钟，测试只知道它落在 before 与 after 之间。
	wanted := map[string]bool{
		"EX-" + before.In(shanghai).Format("20060102") + "-0001": true,
		"EX-" + after.In(shanghai).Format("20060102") + "-0001":  true,
	}
	if !wanted[number] || reason != "" {
		t.Fatalf("案件单据号 = %q 无号原因 = %q，want %v", number, reason, wanted)
	}
}

// Covers: AT-VE-228 派发半边、ADR-0355 决定七——租户没登单据编号时区：分诊照常自动建案，案件不带单据号，记「单据编号时区未配置」。
func TestAnAutoEstablishedCaseWithoutANumberingTimeZoneIsEstablishedWithoutANumber(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoDepartureGateSignalRule(t, fixture)
	registerAutoEstablishingTriage(t, fixture)
	saveDepartureSegment(t, fixture)

	eventID := recordDepartureThroughProduction(t, fixture, "SYN-MOVE-DISPATCH-02")
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("第一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}
	if number, reason := caseNumberOf(t, fixture, movementParcelBlocked); number != "" || reason != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("案件单据号 = %q 无号原因 = %q", number, reason)
	}
}
