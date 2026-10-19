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
