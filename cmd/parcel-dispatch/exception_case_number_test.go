package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
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
		`SELECT COALESCE(document_number, ''), COALESCE(document_number_absence, '')
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

// Covers: AT-VE-228 派发半边（撞号那一半，证据 S）——租户登了一份会与已用案件号重复的格式：出发门禁阻断信号经生产路由分诊为
// 自动建案，案件单据号撞号。信封入账（PUBLISHED、失败零次，不烧失败预算、不堵分区），投影照落，案件、发作期都不落，命中记进
// 案件撞号册；同一拍的重驱再撞一次，作为单列一类的错误交给循环记日志（veapplication.ErrCaseNumberingRuleCollision）并推后。
// 租户换了格式、到了重试时刻，下一拍的重驱按原样再分诊、建案取新格式的号，撞号册那一行销掉。
func TestACaseNumberCollisionSettlesTheEnvelopeAndTheRedriveEstablishesTheCaseAfterTheFormatChanges(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoDepartureGateSignalRule(t, fixture)
	registerAutoEstablishingTriage(t, fixture)
	tenant := fixture.identity.TenantID().String()
	numbers, err := buildDocumentNumbers(fixture.db, dispatchSettings{})
	if err != nil {
		t.Fatalf("构造编号组件：%v", err)
	}
	if registered, err := numbers.Registrations.RegisterTimeZone(t.Context(), docnumber.RegisterTimeZoneCommand{
		Tenant: tenant, TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	}); err != nil || registered.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登单据编号时区：%+v %v", registered, err)
	}
	registerCaseRule := func(template string, effectiveFrom time.Time) {
		t.Helper()
		if registered, err := numbers.Registrations.RegisterRule(t.Context(), docnumber.RegisterRuleCommand{
			Tenant: tenant, Type: "EXCEPTION_CASE", Template: template, EffectiveFrom: effectiveFrom, RegisteredBy: "SYN-TEST",
		}); err != nil || registered.Outcome != docnumber.VersionRegistered {
			t.Fatalf("登案件规则 %s：%+v %v", template, registered, err)
		}
	}
	registerCaseRule("SYN-{seq:5}", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	for index := 1; index <= docnumber.MaxSkippedNumbers; index++ {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.exception_case
				(tenant_id, case_id, root_parcel, impact_scope, responsible_team, phase, established_at, document_number)
			 VALUES ($1, $2, 'SYN-PARCEL-OLD', 'SYN-PARCEL-OLD', 'SYN-TEAM-EXC', 'AWAITING_RESPONSE', now(), $3)`,
			tenant, fmt.Sprintf("XCS-OLD-%02d", index), fmt.Sprintf("SYN-%05d", index),
		); err != nil {
			t.Fatalf("摆旧号：%v", err)
		}
	}
	saveDepartureSegment(t, fixture)

	eventID := recordDepartureThroughProduction(t, fixture, "SYN-MOVE-DISPATCH-02")
	published, err := fixture.beat.DispatchOnce(t.Context())
	if published != 1 || !errors.Is(err, veapplication.ErrCaseNumberingRuleCollision) {
		t.Fatalf("撞号那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}
	var status string
	var failures int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT status, failures FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID).Scan(&status, &failures); err != nil ||
		status != "PUBLISHED" || failures != 0 {
		t.Fatalf("移动事实信封 = %s / 失败 %d 次 %v，want 入账、不烧失败预算", status, failures, err)
	}
	onlyProjectionEntry(t, fixture, movementParcelBlocked)
	if n := signalEpisodes(t, fixture, movementParcelBlocked); n != 0 {
		t.Fatalf("撞号那一轮落了 %d 个发作期", n)
	}
	var cases, collisions int
	var template string
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM visibility_exception.exception_case WHERE tenant_id = $1 AND root_parcel = $2),
		        (SELECT count(*) FROM visibility_exception.case_numbering_collision WHERE tenant_id = $1 AND parcel_ref = $2),
		        (SELECT max(template) FROM visibility_exception.case_numbering_collision WHERE tenant_id = $1 AND parcel_ref = $2)`,
		tenant, movementParcelBlocked).Scan(&cases, &collisions, &template); err != nil || cases != 0 || collisions != 1 || template != "SYN-{seq:5}" {
		t.Fatalf("案件 %d 件、撞号册 %d 行（格式 %q）%v，want 没案件、记一行", cases, collisions, template, err)
	}

	// 推后了：格式没换的下一拍不再撞、不出声。
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("推后期间的一拍：%v", err)
	}

	registerCaseRule("SYN-B-{seq:5}", time.Now().Add(-time.Second))
	if _, err := fixture.pool.Exec(t.Context(),
		`UPDATE visibility_exception.case_numbering_collision SET next_attempt_at = now() WHERE tenant_id = $1`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("换格式之后的一拍：%v", err)
	}
	if number, absence := caseNumberOf(t, fixture, movementParcelBlocked); number != "SYN-B-00001" || absence != "" {
		t.Fatalf("重驱建的案件号 = %q（%q），want 新格式的首号", number, absence)
	}
	if n := signalEpisodes(t, fixture, movementParcelBlocked); n != 1 {
		t.Fatalf("重驱之后发作期 %d 个", n)
	}
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.case_numbering_collision WHERE tenant_id = $1`, tenant).Scan(&collisions); err != nil || collisions != 0 {
		t.Fatalf("建成之后撞号册还有 %d 行 %v", collisions, err)
	}
}
