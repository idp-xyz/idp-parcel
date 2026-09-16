package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	settlementapp "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// Covers: `/settlement-external-funds-fact-registrations` 与 `/settlement-external-funds-fact-correction-registrations`
// 的第二参是真编排（票 sa-cc/31 判据 4）——buildSettlementRegistrationOrchestration 在真实 PostgreSQL 上装得起来
// （每一口都接真、构造门一口不漏），且两个事务壳**确实提交**并把版本行与向 CC 交的采用信封罩在同一笔里：
// 经端点体 + 真 Registrar 采用一条 → 版本行 + 一封 `settlement-accounting.external-funds-fact.adopted`；重放同载荷
// → 已存在、无第二封；更正回指链头 → 第二行第二封。判据与 cmd/parcel-settlement-register 的 vertical_test 同一条、
// 不同入口（ADR-0085 决定一：CLI 与端点消费同一登记用例）。
//
// 接没接对不在本文件证：两个 Registrar 契约的命令类型互不相同、又按用例方法名分成两个契约，接错编译期就红。
// 测试输入是隔离合成，只记 `S`，不进生产装配；币种取测试码 XTS，不预填任何真实币种。

// translatingFundsIntake 把请求体经 registrationjson 译成命令——本用例要证的是端点体之后那半条链，Intake 这一格
// 在生产上还是未配置（`PAR-INT-01`），这里用译装那一份代替、不另写解析（红线「译装只用 registrationjson 那一份」）。
type translatingFundsIntake struct{}

func (translatingFundsIntake) IntakeExternalFundsFactRegistration(
	_ context.Context, request *http.Request,
) (settlementapp.AdoptFundsFactCommand, error) {
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		return settlementapp.AdoptFundsFactCommand{}, err
	}
	return registrationjson.ExternalFundsFactFromJSON(raw)
}

func (translatingFundsIntake) IntakeExternalFundsFactCorrectionRegistration(
	_ context.Context, request *http.Request,
) (settlementapp.CorrectFundsFactCommand, error) {
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		return settlementapp.CorrectFundsFactCommand{}, err
	}
	return registrationjson.ExternalFundsFactCorrectionFromJSON(raw)
}

const apiFundsFactPartition = "SYN-TENANT-API-SA31/funds-fact/SYN-API-FACT-1"

func apiAdoptDocument(version, amountMinor string) string {
	return `{"tenantId":"SYN-TENANT-API-SA31","factRef":"SYN-API-FACT-1","sourceRef":"SYN-API-SOURCE-BANK-1",` +
		`"payerRef":"SYN-API-PAYER-1","kind":"RECEIPT_CONFIRMED","currency":"XTS","amountMinor":` + amountMinor + `,` +
		`"version":"` + version + `","occurredAt":"2026-09-01T08:00:00Z"}`
}

func apiCorrectionDocument(corrects, version, amountMinor string) string {
	return `{"tenantId":"SYN-TENANT-API-SA31","factRef":"SYN-API-FACT-1","corrects":"` + corrects + `",` +
		`"version":"` + version + `","amountMinor":` + amountMinor + `,"correctedAt":"2026-09-02T08:00:00Z"}`
}

func apiCountRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("数行：%v", err)
	}
	return count
}

func TestTheWiredExternalFundsFactRegistrationsRecordAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()
	registration, err := buildSettlementRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配结算登记编排：%v", err)
	}
	adopt := settlementhttp.NewRegisterExternalFundsFactEndpoint(translatingFundsIntake{}, registration.externalFundsFact)
	correct := settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(translatingFundsIntake{}, registration.externalFundsFactCorrection)

	post := func(endpoint http.Handler, document string, wantStatus int, wantOutcome string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(document)))
		if recorder.Code != wantStatus {
			t.Fatalf("答 %d（%s），要 %d", recorder.Code, recorder.Body.String(), wantStatus)
		}
		var body struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("解响应 %s：%v", recorder.Body.Bytes(), err)
		}
		if body.Outcome != wantOutcome {
			t.Fatalf("outcome = %q，要 %s", body.Outcome, wantOutcome)
		}
	}
	versionRows := func() int {
		return apiCountRows(t, pool,
			`SELECT count(*) FROM settlement_accounting.external_funds_fact_version WHERE tenant_id = $1 AND fact_id = $2`,
			"SYN-TENANT-API-SA31", "SYN-API-FACT-1")
	}
	envelopes := func() int {
		return apiCountRows(t, pool,
			`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE partition_key = $1`, apiFundsFactPartition)
	}

	// 正：首版采用 → 201 已采用、版本行 + 一封；事件类型是 sa-cc/02 钉的那个形，信封 ID 是带版本维的定长指纹
	// （票 sa-cc/32 裁决 1），按生产同一公式重算来查。
	post(adopt, apiAdoptDocument("SYN-API-FACT-1/v1", "8000"), http.StatusCreated, "FUNDS_FACT_ADOPTED")
	if versionRows() != 1 || envelopes() != 1 {
		t.Fatalf("首版后版本行 = %d、信封 = %d，各要 1——读不到行或封说明事务壳没提交", versionRows(), envelopes())
	}
	var eventType string
	if err := pool.QueryRow(ctx,
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		string(outboxintent.FingerprintEventID("funds-fact", "SYN-TENANT-API-SA31", "SYN-API-FACT-1", "SYN-API-FACT-1/v1")),
	).Scan(&eventType); err != nil {
		t.Fatalf("按带版本维重算的信封 ID 读不到那一封：%v", err)
	}
	if eventType != "settlement-accounting.external-funds-fact.adopted" {
		t.Fatalf("事件类型 = %q", eventType)
	}

	// 重放同载荷 → 200 已存在，无第二封（同 ID 被 Outbox 认领键吞掉）；重放要读回首行才答得出，重放即提交证据。
	post(adopt, apiAdoptDocument("SYN-API-FACT-1/v1", "8000"), http.StatusOK, "EXISTING_FUNDS_FACT")
	if versionRows() != 1 || envelopes() != 1 {
		t.Fatalf("重放后版本行 = %d、信封 = %d，各要仍是 1", versionRows(), envelopes())
	}

	// 反：同版本字面换金额 → 200 内容冲突，绝不覆盖、无新封。
	post(adopt, apiAdoptDocument("SYN-API-FACT-1/v1", "8001"), http.StatusOK, "FUNDS_FACT_CONFLICT")
	if versionRows() != 1 || envelopes() != 1 {
		t.Fatalf("冲突后版本行 = %d、信封 = %d，各要仍是 1", versionRows(), envelopes())
	}

	// 正：更正回指链头 → 201 已采用、第二行第二封（更正口接的是同一只编排的另一个方法，事务壳是另一个）。
	post(correct, apiCorrectionDocument("SYN-API-FACT-1/v1", "SYN-API-FACT-1/v2", "9000"), http.StatusCreated, "FUNDS_FACT_ADOPTED")
	if versionRows() != 2 || envelopes() != 2 {
		t.Fatalf("更正后版本行 = %d、信封 = %d，要 2 / 2", versionRows(), envelopes())
	}

	// 反：回指非链头顺序到达 → 200 未受理，零新行零新封——事务壳没有替它落半行。
	post(correct, apiCorrectionDocument("SYN-API-FACT-1/v1", "SYN-API-FACT-1/v3", "9500"), http.StatusOK, "SOURCE_NOT_ACCEPTED")
	if versionRows() != 2 || envelopes() != 2 {
		t.Fatalf("未受理后版本行 = %d、信封 = %d，各要仍是 2", versionRows(), envelopes())
	}
}
