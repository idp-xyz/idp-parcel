package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证节点作业的受控登记口（ADR-0085 决定一；ADR-0212 决定二末项）：演示种子那一行照 seed.sh 原样登进真库；退出码按恢复动作分格；
// 与 parcel-api 的登记写面是同一个编排、同一本册。夹具全是 SYN 合成值（S 级）。

var demoWindowSeed = filepath.Join("..", "..", "scripts", "demo-seeds", "data", "node-operations", "01-pre-advice-window-syn-tenant-01.json")

func newRegistrar(t *testing.T) (registrar, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	built, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	return built, db
}

// Covers: 演示判据——种子里演示租户那一行时窗照 seed.sh 原样登进真库答已登记、重灌答已有结果（退出码都是 0，seed.sh 重跑不断）；
// 登进去的那一行按时点读得回，提前量与宽限量照文件。证据层级只记 S。
func TestTheDemoWindowSeedRegistersOnARealDatabase(t *testing.T) {
	built, db := newRegistrar(t)
	raw, err := os.ReadFile(demoWindowSeed)
	if err != nil {
		t.Fatalf("读演示种子：%v", err)
	}
	ctx := t.Context()
	if message, code := execute(ctx, kindPreAdviceWindow, raw, built); code != exitRegistered || !strings.Contains(message, "PRE_ADVICE_WINDOW_REGISTERED") {
		t.Fatalf("首灌答 %q（%d）", message, code)
	}
	if message, code := execute(ctx, kindPreAdviceWindow, raw, built); code != exitRegistered || !strings.Contains(message, "EXISTING_PRE_ADVICE_WINDOW") {
		t.Fatalf("重灌答 %q（%d）", message, code)
	}

	var seed struct {
		TenantID      string    `json:"tenantId"`
		LeadMinutes   int64     `json:"leadMinutes"`
		GraceMinutes  int64     `json:"graceMinutes"`
		EffectiveFrom time.Time `json:"effectiveFrom"`
	}
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatal(err)
	}
	if seed.TenantID != "SYN-TENANT-01" {
		t.Fatalf("演示种子的租户是 %q，要演示租户 SYN-TENANT-01", seed.TenantID)
	}
	windows, err := adapter.NewPreAdviceWindows(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := domain.NewTenantID(seed.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	window, found, err := windows.FindPreAdviceWindowAt(ctx, tenant, seed.EffectiveFrom)
	if err != nil || !found || window.Lead() != time.Duration(seed.LeadMinutes)*time.Minute || window.Grace() != time.Duration(seed.GraceMinutes)*time.Minute {
		t.Fatalf("登进去的那一行 = %v/%v %v %v", window.Lead(), window.Grace(), found, err)
	}
}

// Covers: 退出码按恢复动作分格（ADR-0029 判据）——区间与在册行重叠答 2（原行不被顶替，要人看过再定）；未知字段、缺租户、缺提前量、
// 宽限量为负、未知族都答 1，不进事务。
func TestTheRegisterAnswersEachRecoveryActionWithItsOwnExitCode(t *testing.T) {
	built, _ := newRegistrar(t)
	ctx := t.Context()
	first := `{"tenantId": "SYN-TENANT-PSXM08", "leadMinutes": 1440, "graceMinutes": 360, "effectiveFrom": "2026-01-01T00:00:00Z", "basis": "SYN-B-01"}`
	if message, code := execute(ctx, kindPreAdviceWindow, []byte(first), built); code != exitRegistered {
		t.Fatalf("首登答 %q（%d）", message, code)
	}
	overlapping := `{"tenantId": "SYN-TENANT-PSXM08", "leadMinutes": 60, "graceMinutes": 60, "effectiveFrom": "2026-03-01T00:00:00Z", "basis": "SYN-B-02"}`
	if message, code := execute(ctx, kindPreAdviceWindow, []byte(overlapping), built); code != exitOverlap || !strings.Contains(message, "PRE_ADVICE_WINDOW_OVERLAPS") {
		t.Fatalf("重叠答 %q（%d）", message, code)
	}
	for name, input := range map[string]struct {
		kind string
		raw  string
	}{
		"未知字段":  {kindPreAdviceWindow, `{"tenantId": "SYN-TENANT-PSXM08", "leadMinutes": 1, "graceMinutes": 1, "effectiveFrom": "2027-01-01T00:00:00Z", "basis": "SYN-B", "node": "SYN-NODE"}`},
		"缺租户":   {kindPreAdviceWindow, `{"leadMinutes": 1, "graceMinutes": 1, "effectiveFrom": "2027-01-01T00:00:00Z", "basis": "SYN-B"}`},
		"缺提前量":  {kindPreAdviceWindow, `{"tenantId": "SYN-TENANT-PSXM08", "graceMinutes": 1, "effectiveFrom": "2027-01-01T00:00:00Z", "basis": "SYN-B"}`},
		"宽限量为负": {kindPreAdviceWindow, `{"tenantId": "SYN-TENANT-PSXM08", "leadMinutes": 1, "graceMinutes": -1, "effectiveFrom": "2027-01-01T00:00:00Z", "basis": "SYN-B"}`},
		"未知族":   {"handover-note", first},
	} {
		if message, code := execute(ctx, input.kind, []byte(input.raw), built); code != exitUsage {
			t.Fatalf("%s: 答 %q（%d），要 %d", name, message, code, exitUsage)
		}
	}
}

// transactionalRegistrar 把本口装好的编排包进一笔事务交给在线口，与 parcel-api 的装配同一个包法：在线口按框架合同不带事务。
type transactionalRegistrar struct{ registrar registrar }

func (wrapper transactionalRegistrar) Handle(ctx context.Context, command nodeopsapp.RegisterPreAdviceWindowCommand) (nodeopsapp.PreAdviceWindowRegistrationResult, error) {
	var result nodeopsapp.PreAdviceWindowRegistrationResult
	err := wrapper.registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := wrapper.registrar.handler.Handle(txCtx, command)
		result = handled
		return err
	})
	return result, err
}

// registryWriterStub 是在线口认证方的替身：令牌一律认到 SYN-TENANT-PSXM08。认证本身由 parcel-api 的装配用例证。
type registryWriterStub struct{}

func (registryWriterStub) AuthenticateRegistryWrite(context.Context, string) (domain.TenantID, error) {
	return domain.NewTenantID("SYN-TENANT-PSXM08")
}

// Covers: ADR-0085 决定一「登记写面与受控批量口同一用例」——本口登下的一行，经 parcel-api 的在线写面原样再登答已有结果；本口再登在线
// 口登下的另一行同样答已有结果。两个入口落的是同一本册、判的是同一套重放。
func TestTheRegisterAndTheOnlineFaceShareOneRegistration(t *testing.T) {
	built, _ := newRegistrar(t)
	ctx := t.Context()
	intake, err := nodeopshttp.NewOperatorRegistryIntake(registryWriterStub{})
	if err != nil {
		t.Fatal(err)
	}
	online := nodeopshttp.NewRegisterPreAdviceWindowEndpoint(intake, transactionalRegistrar{built})
	post := func(body string) (int, string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/node-operations-pre-advice-window-registrations", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		online.ServeHTTP(response, request)
		var answer struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("decode %s: %v", response.Body.String(), err)
		}
		return response.Code, answer.Outcome
	}

	viaRegister := `{"leadMinutes": 1440, "graceMinutes": 360, "effectiveFrom": "2026-01-01T00:00:00Z", "effectiveUntil": "2026-07-01T00:00:00Z", "basis": "SYN-B-01"}`
	if message, code := execute(ctx, kindPreAdviceWindow, []byte(`{"tenantId": "SYN-TENANT-PSXM08", `+viaRegister[1:]), built); code != exitRegistered {
		t.Fatalf("本口首登答 %q（%d）", message, code)
	}
	if status, outcome := post(viaRegister); status != http.StatusOK || outcome != "EXISTING_PRE_ADVICE_WINDOW" {
		t.Fatalf("在线口再登本口那一行答 %d %s", status, outcome)
	}

	viaOnline := `{"leadMinutes": 60, "graceMinutes": 30, "effectiveFrom": "2026-07-01T00:00:00Z", "basis": "SYN-B-02"}`
	if status, outcome := post(viaOnline); status != http.StatusCreated || outcome != "PRE_ADVICE_WINDOW_REGISTERED" {
		t.Fatalf("在线口首登答 %d %s", status, outcome)
	}
	if message, code := execute(ctx, kindPreAdviceWindow, []byte(`{"tenantId": "SYN-TENANT-PSXM08", `+viaOnline[1:]), built); code != exitRegistered ||
		!strings.Contains(message, "EXISTING_PRE_ADVICE_WINDOW") {
		t.Fatalf("本口再登在线口那一行答 %q（%d）", message, code)
	}
}
