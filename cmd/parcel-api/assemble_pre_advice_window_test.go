package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// unwiredPreAdviceWindowRegistration 是端点表装配测试里预报有效时窗那一格的占位：未配置与渠道换口的用例都钉在编排之前，走到它就是
// Intake 漏了一道。
type unwiredPreAdviceWindowRegistration struct{}

func (unwiredPreAdviceWindowRegistration) Handle(context.Context, nodeopsapp.RegisterPreAdviceWindowCommand) (nodeopsapp.PreAdviceWindowRegistrationResult, error) {
	return nodeopsapp.PreAdviceWindowRegistrationResult{}, errOrchestrationNotWired
}

// Covers: 预报有效时窗登记口的第二参是真编排（ADR-0212 决定二末项；ADR-0085 决定一）——时窗册在真实 PostgreSQL 上装得起来，经
// 操作者渠道的登记写面首登答 201、原样再登答 200 已有结果（证首笔事务真的提交了）、区间重叠答 200 重叠并回显在册那一行。测试输入
// 是隔离合成，只记 `S`。
func TestTheWiredPreAdviceWindowRegistrarRegistersAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildPreAdviceWindowRegistration(db)
	if err != nil {
		t.Fatalf("装配时窗登记编排：%v", err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := nodeopshttp.NewRegisterPreAdviceWindowEndpoint(registries.nodeOperations, registrar)

	post := func(body string) (int, string, int64) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/node-operations-pre-advice-window-registrations", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var answer struct {
			Outcome string `json:"outcome"`
			Window  struct {
				GraceMinutes int64 `json:"graceMinutes"`
			} `json:"window"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("decode %s: %v", response.Body.String(), err)
		}
		return response.Code, answer.Outcome, answer.Window.GraceMinutes
	}

	first := `{"leadMinutes": 1440, "graceMinutes": 720, "effectiveFrom": "2026-01-01T00:00:00Z", "effectiveUntil": "2027-01-01T00:00:00Z",
		"basis": "SYN-BASIS-PSXM08-WINDOW"}`
	if status, outcome, _ := post(first); status != http.StatusCreated || outcome != "PRE_ADVICE_WINDOW_REGISTERED" {
		t.Fatalf("首登 = %d %s", status, outcome)
	}
	if status, outcome, _ := post(first); status != http.StatusOK || outcome != "EXISTING_PRE_ADVICE_WINDOW" {
		t.Fatalf("原样再登 = %d %s——首笔事务没有提交", status, outcome)
	}
	overlapping := `{"leadMinutes": 60, "graceMinutes": 30, "effectiveFrom": "2026-06-01T00:00:00Z", "basis": "SYN-BASIS-PSXM08-OTHER"}`
	if status, outcome, grace := post(overlapping); status != http.StatusOK || outcome != "PRE_ADVICE_WINDOW_OVERLAPS" || grace != 720 {
		t.Fatalf("区间重叠 = %d %s grace=%d, want the registered row echoed", status, outcome, grace)
	}
}

// unwiredPreAdviceWindowClosure 是端点表装配测试里时窗收口那一格的占位，理由同 unwiredPreAdviceWindowRegistration。
type unwiredPreAdviceWindowClosure struct{}

func (unwiredPreAdviceWindowClosure) Handle(context.Context, nodeopsapp.ClosePreAdviceWindowCommand) (nodeopsapp.PreAdviceWindowClosureResult, error) {
	return nodeopsapp.PreAdviceWindowClosureResult{}, errOrchestrationNotWired
}

// Covers: 时窗收口口的第二参是真编排（票 ps-external-mark-relations/14）——经操作者渠道的登记写面，开放终点的一行登下之后后继答
// 重叠；收口口收口答 201、原样再收答 200 已有结果（证首笔事务真的提交了）；收口之后，起点恰在收口时刻的后继经登记口答 201；
// 收口时刻早于起点答坏载荷。测试输入是隔离合成，只记 `S`。
func TestTheWiredPreAdviceWindowCloserClosesAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildPreAdviceWindowRegistration(db)
	if err != nil {
		t.Fatalf("装配时窗登记编排：%v", err)
	}
	closer, err := buildPreAdviceWindowClosure(db)
	if err != nil {
		t.Fatalf("装配时窗收口编排：%v", err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	registration := nodeopshttp.NewRegisterPreAdviceWindowEndpoint(registries.nodeOperations, registrar)
	closure := nodeopshttp.NewClosePreAdviceWindowEndpoint(registries.nodeOperations, closer)

	post := func(endpoint http.Handler, path, body string) (int, string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var answer struct {
			Outcome string `json:"outcome"`
			Error   struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("decode %s: %v", response.Body.String(), err)
		}
		if answer.Outcome == "" {
			return response.Code, answer.Error.Code
		}
		return response.Code, answer.Outcome
	}
	const registrations = "/node-operations-pre-advice-window-registrations"
	const closures = "/node-operations-pre-advice-window-closures"

	open := `{"leadMinutes": 1440, "graceMinutes": 720, "effectiveFrom": "2026-01-01T00:00:00Z", "basis": "SYN-BASIS-PSXM14-WINDOW"}`
	if status, outcome := post(registration, registrations, open); status != http.StatusCreated || outcome != "PRE_ADVICE_WINDOW_REGISTERED" {
		t.Fatalf("开放终点那一行 = %d %s", status, outcome)
	}
	successor := `{"leadMinutes": 60, "graceMinutes": 30, "effectiveFrom": "2026-07-01T00:00:00Z", "basis": "SYN-BASIS-PSXM14-SUCCESSOR"}`
	if status, outcome := post(registration, registrations, successor); status != http.StatusOK || outcome != "PRE_ADVICE_WINDOW_OVERLAPS" {
		t.Fatalf("收口之前的后继 = %d %s", status, outcome)
	}
	closing := `{"windowEffectiveFrom": "2026-01-01T00:00:00Z", "closedAt": "2026-07-01T00:00:00Z", "basis": "SYN-BASIS-PSXM14-CLOSURE"}`
	if status, outcome := post(closure, closures, closing); status != http.StatusCreated || outcome != "PRE_ADVICE_WINDOW_CLOSED" {
		t.Fatalf("收口 = %d %s", status, outcome)
	}
	if status, outcome := post(closure, closures, closing); status != http.StatusOK || outcome != "EXISTING_PRE_ADVICE_WINDOW_CLOSURE" {
		t.Fatalf("原样再收 = %d %s——首笔事务没有提交", status, outcome)
	}
	if status, outcome := post(registration, registrations, successor); status != http.StatusCreated || outcome != "PRE_ADVICE_WINDOW_REGISTERED" {
		t.Fatalf("收口之后的后继 = %d %s", status, outcome)
	}
	early := `{"windowEffectiveFrom": "2026-07-01T00:00:00Z", "closedAt": "2026-06-30T00:00:00Z", "basis": "SYN-BASIS-PSXM14-CLOSURE"}`
	if status, code := post(closure, closures, early); status != http.StatusBadRequest || code != "MALFORMED_REQUEST" {
		t.Fatalf("收口时刻早于起点 = %d %s", status, code)
	}
}
