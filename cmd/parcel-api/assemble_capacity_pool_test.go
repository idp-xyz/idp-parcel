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

// Covers: 容量池登记口的第二参是真编排（ADR-0201 决定一、三）——在真实 PostgreSQL 上经操作者渠道的登记写面：指向
// 没登过的班次建池答 200 SCHEDULE_NOT_FOUND；带逐维容量声明的周期安排按日期物化登记一班答 201，答案逐池列出两池
// 已建立；在那一班上另登一维答 201、再登答 200 已存在（证首笔事务真的提交了）。测试输入是隔离合成，只记 `S`。
func TestTheWiredCapacityPoolRegistrarChecksTheScheduleAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	schedules, err := buildTransportScheduleRegistration(db)
	if err != nil {
		t.Fatalf("装配班次登记编排：%v", err)
	}
	pools, err := buildCapacityPoolRegistration(db)
	if err != nil {
		t.Fatalf("装配建池编排：%v", err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	templateEndpoint := tfhttp.NewRegisterScheduleTemplateEndpoint(registries.transport, schedules)
	scheduleEndpoint := tfhttp.NewRegisterTransportScheduleEndpoint(registries.transport, schedules)
	poolEndpoint := tfhttp.NewRegisterCapacityPoolEndpoint(registries.transport, pools)

	post := func(handler http.Handler, body string) (int, string, string) {
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
		return response.Code, answer.Outcome, response.Body.String()
	}

	bags := `{"schedule": "SYN-SCHED-SZX-SIN-DAILY/2026-10-21", "dimension": "BAG_POSITIONS", "unit": "BAG", "capacity": 40}`
	if status, outcome, body := post(poolEndpoint, bags); status != http.StatusOK || outcome != "SCHEDULE_NOT_FOUND" {
		t.Fatalf("班次不在册建池 = %d %s", status, body)
	}
	if status, outcome, body := post(templateEndpoint, `{"template": "SYN-SCHED-SZX-SIN-DAILY", "connection": "SYN-CONN-SZX-SIN",
		"fromNode": "SYN-NODE-SZX-GATE", "toNode": "SYN-NODE-SIN-HUB", "weekdays": ["MON","TUE","WED","THU","FRI","SAT","SUN"],
		"departureLocalTime": "10:00", "durationMinutes": 300, "timeZone": "Asia/Shanghai", "serviceScope": "SYN-SCOPE-01",
		"effectiveFrom": "2026-01-01", "capacities": [{"dimension": "WEIGHT", "unit": "KG", "capacity": 5000},
		{"dimension": "PIECES", "unit": "PCS", "capacity": 1000}]}`); status != http.StatusCreated || outcome != "TEMPLATE_REGISTERED" {
		t.Fatalf("登周期安排 = %d %s", status, body)
	}
	status, outcome, body := post(scheduleEndpoint, `{"template": "SYN-SCHED-SZX-SIN-DAILY", "runningDate": "2026-10-21"}`)
	if status != http.StatusCreated || outcome != "SCHEDULE_REGISTERED" ||
		!strings.Contains(body, `"outcome":"POOL_ESTABLISHED","poolId":"SYN-SCHED-SZX-SIN-DAILY/2026-10-21/WEIGHT"`) ||
		!strings.Contains(body, `"outcome":"POOL_ESTABLISHED","poolId":"SYN-SCHED-SZX-SIN-DAILY/2026-10-21/PIECES"`) {
		t.Fatalf("物化登记一班 = %d %s", status, body)
	}
	if status, outcome, body := post(poolEndpoint, bags); status != http.StatusCreated || outcome != "POOL_ESTABLISHED" {
		t.Fatalf("另登一维 = %d %s", status, body)
	}
	if status, outcome, body := post(poolEndpoint, bags); status != http.StatusOK || outcome != "EXISTING_POOL" {
		t.Fatalf("同一维再登 = %d %s——首笔事务没有提交", status, body)
	}
}
