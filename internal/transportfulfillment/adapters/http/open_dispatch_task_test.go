package tfhttp_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 tf-face-algebra/10——201 `DISPATCH_TASK_OPENED` 与 200 `DISPATCH_TASK_ALREADY_OPEN` 两种答复同形，都回显在册任务的
// conditions；同一任务换 conditions 重投，回显的是在册那一份，调用方据此比得出差异。整个答复体逐字节比对，顺带钉住整秒
// 时刻的写法不变。夹具全为合成登记（S 级）。
func TestOpenDispatchTaskAnswersEchoTheRegisteredConditions(t *testing.T) {
	handler := application.NewOpenDispatchTaskHandler(application.OpenDispatchTaskDeps{
		Tasks: newDispatchTaskRegistry(),
		Clock: tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	intake := newDispatchTaskIntake(t)
	intake.tenant = httpValue(t, domain.NewTenantID, "SYN-TENANT-01")
	endpoint := tfhttp.NewOpenDispatchTaskEndpoint(intake, handler)
	body := map[string]any{
		"task": "SYN-DT-0001", "kind": "DELIVERY", "objects": []string{"SYN-PARCEL-1", "SYN-PARCEL-2"}, "place": "SYN-PLACE-1",
		"windowFrom": "2026-09-06T09:00:00Z", "windowTo": "2026-09-06T12:00:00Z", "conditions": "SYN-CONDITION/A", "openedAt": "2026-09-05T12:00:00Z",
	}
	const registered = `"task":"SYN-DT-0001","kind":"DELIVERY","objects":["SYN-PARCEL-1","SYN-PARCEL-2"],"place":"SYN-PLACE-1",` +
		`"windowFrom":"2026-09-06T09:00:00Z","windowTo":"2026-09-06T12:00:00Z","conditions":"SYN-CONDITION/A","openedAt":"2026-09-05T12:00:00Z"}`

	opened := postTo(t, endpoint, "/transport-fulfillment-dispatch-task-registrations", body)
	if want := `{"outcome":"DISPATCH_TASK_OPENED",` + registered; opened.Code != http.StatusCreated || strings.TrimSpace(opened.Body.String()) != want {
		t.Fatalf("首答 %d\n got %s\nwant %s", opened.Code, strings.TrimSpace(opened.Body.String()), want)
	}

	body["conditions"] = "SYN-CONDITION/B"
	replay := postTo(t, endpoint, "/transport-fulfillment-dispatch-task-registrations", body)
	if want := `{"outcome":"DISPATCH_TASK_ALREADY_OPEN",` + registered; replay.Code != http.StatusOK || strings.TrimSpace(replay.Body.String()) != want {
		t.Fatalf("换 conditions 重投 %d\n got %s\nwant %s", replay.Code, strings.TrimSpace(replay.Body.String()), want)
	}
}
