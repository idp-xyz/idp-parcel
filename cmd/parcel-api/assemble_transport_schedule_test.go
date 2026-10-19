package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// Covers: 周期安排与具体班次两口的第二参是真编排（ADR-0201 决定一）——两本册子在真实 PostgreSQL 上装得起来，经操作者
// 渠道的登记写面登一份周期安排答 201、按运行日期显式登一个班次答 201、同一日期再登答 200 已存在（证首笔事务真的
// 提交了）。测试输入是隔离合成，只记 `S`。
func TestTheWiredTransportScheduleRegistrarMaterializesAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildTransportScheduleRegistration(db)
	if err != nil {
		t.Fatalf("装配班次登记编排：%v", err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	templates := tfhttp.NewRegisterScheduleTemplateEndpoint(registries.transport, registrar)
	schedules := tfhttp.NewRegisterTransportScheduleEndpoint(registries.transport, registrar)

	post := func(handler http.Handler, body string) (int, string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var answer struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("decode %s: %v", response.Body.String(), err)
		}
		return response.Code, answer.Outcome
	}

	if status, outcome := post(templates, `{"template": "SYN-SCHED-SZX-SIN-DAILY", "connection": "SYN-CONN-SZX-SIN",
		"fromNode": "SYN-NODE-SZX-GATE", "toNode": "SYN-NODE-SIN-HUB", "weekdays": ["MON","TUE","WED","THU","FRI","SAT","SUN"],
		"departureLocalTime": "08:00", "durationMinutes": 300, "timeZone": "Asia/Shanghai", "serviceScope": "SYN-SCOPE-01",
		"effectiveFrom": "2026-01-01"}`); status != http.StatusCreated || outcome != "TEMPLATE_REGISTERED" {
		t.Fatalf("登周期安排 = %d %s", status, outcome)
	}
	materialize := `{"template": "SYN-SCHED-SZX-SIN-DAILY", "runningDate": "2026-10-21"}`
	if status, outcome := post(schedules, materialize); status != http.StatusCreated || outcome != "SCHEDULE_REGISTERED" {
		t.Fatalf("显式登一个班次 = %d %s", status, outcome)
	}
	if status, outcome := post(schedules, materialize); status != http.StatusOK || outcome != "EXISTING_SCHEDULE" {
		t.Fatalf("同一日期再登 = %d %s——首笔事务没有提交", status, outcome)
	}
}
