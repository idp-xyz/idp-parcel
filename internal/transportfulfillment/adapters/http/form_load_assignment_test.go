package tfhttp_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 票 tf-face-algebra/13：`分配已形成`回显链头时，链头是撤回版就要让调用方看得出来——否则一个已撤回的分配
// 会被读成仍在生效。夹具全为合成登记（S 级）。

const loadAssignmentPath = "/transport-fulfillment-load-assignment-registrations"

var synLoadAssignedAt = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

func synLoadAssignmentEndpoint(t *testing.T, registry *loadAssignmentRegistryDouble) http.Handler {
	t.Helper()
	return synLoadAssignmentEndpointOver(t, registry, registeredSchedules("SYN-TENANT-01", "SYN-SCHEDULE-1"))
}

func synLoadAssignmentEndpointOver(
	t *testing.T,
	registry *loadAssignmentRegistryDouble,
	schedules *scheduleRegistryDouble,
) http.Handler {
	t.Helper()
	intake := newLoadAssignmentIntake(t)
	intake.tenant = httpValue(t, domain.NewTenantID, "SYN-TENANT-01")
	handler := application.NewFormLoadAssignmentHandler(application.FormLoadAssignmentDeps{
		Assignments: registry,
		Schedules:   schedules,
		Clock:       tfClock{at: synLoadAssignedAt},
	})
	return tfhttp.NewFormLoadAssignmentEndpoint(intake, handler)
}

func synFirstLoadAssignment(t *testing.T) domain.LoadAssignment {
	t.Helper()
	first, err := domain.FormLoadAssignment(domain.LoadAssignmentSpec{
		TenantID:   httpValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Assignment: httpValue(t, domain.NewLoadAssignmentReference, "SYN-LA-0001"),
		Schedule:   httpValue(t, domain.NewScheduleReference, "SYN-SCHEDULE-1"),
		Members: []domain.CarriedObjectReference{
			httpValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-1"),
			httpValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-2"),
		},
		Version:    httpValue(t, domain.NewLoadAssignmentVersion, "SYN-LAV-1"),
		AssignedAt: synLoadAssignedAt,
	})
	if err != nil {
		t.Fatalf("形成首版夹具：%v", err)
	}
	return first
}

func seedLoadAssignment(registry *loadAssignmentRegistryDouble, assignment domain.LoadAssignment, recordedAt time.Time) {
	key := ports.LoadAssignmentKey{
		TenantID:   assignment.TenantID(),
		Assignment: assignment.Assignment(),
		Version:    assignment.Version(),
	}
	registry.records[loadAssignmentKey(key)] = ports.LoadAssignmentRecord{Key: key, Assignment: assignment, RecordedAt: recordedAt}
}

func anotherFirstVersionBody() map[string]any {
	return map[string]any{
		"assignment": "SYN-LA-0001",
		"schedule":   "SYN-SCHEDULE-2",
		"members":    []string{"SYN-PARCEL-9"},
		"version":    "SYN-LAV-9",
		"assignedAt": "2026-09-05T15:00:00Z",
	}
}

func decodeRawAnswer(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v; body = %s", err, recorder.Body.String())
	}
	return raw
}

func answerKeys(raw map[string]any) string {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// Covers: 票 tf-face-algebra/13——链头是撤回版时，LOAD_ASSIGNMENT_ALREADY_FORMED 回显带撤回标记与撤回时刻，调用方据此
// 另起分配而不是去变化入口。撤回时刻按本包答复的登记册精度写（亚微秒尾巴折掉），与其余时刻同一个写法。
func TestFormLoadAssignmentAlreadyFormedShowsAWithdrawnChainHead(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	first := synFirstLoadAssignment(t)
	seedLoadAssignment(registry, first, synLoadAssignedAt.Add(time.Minute))
	withdrawnAt := time.Date(2026, 9, 5, 13, 0, 0, 123456789, time.UTC)
	withdrawn, err := first.Withdraw(httpValue(t, domain.NewLoadAssignmentVersion, "SYN-LAV-2"), withdrawnAt)
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	seedLoadAssignment(registry, withdrawn, withdrawnAt.Add(time.Minute))

	answered := postTo(t, synLoadAssignmentEndpoint(t, registry), loadAssignmentPath, anotherFirstVersionBody())
	if answered.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", answered.Code, answered.Body.String())
	}
	raw := decodeRawAnswer(t, answered)
	if raw["outcome"] != "LOAD_ASSIGNMENT_ALREADY_FORMED" || raw["version"] != "SYN-LAV-2" {
		t.Fatalf("应回显撤回版链头 SYN-LAV-2：%s", answered.Body.String())
	}
	if raw["withdrawn"] != true {
		t.Fatalf("链头是撤回版，答复却没有撤回标记：%s", answered.Body.String())
	}
	if raw["withdrawnAt"] != "2026-09-05T13:00:00.123456Z" {
		t.Fatalf("withdrawnAt = %v, want 登记册精度的 2026-09-05T13:00:00.123456Z", raw["withdrawnAt"])
	}
}

// Covers: 票 tf-face-algebra/13——链头不是撤回版时答复形状不变：首版与变化版作链头，键集都只有原来那几格，
// 撤回两键整键不出现，不以 false 或空串占位。
func TestFormLoadAssignmentAlreadyFormedKeepsItsShapeForALiveChainHead(t *testing.T) {
	const liveShape = "assignedAt,assignment,members,outcome,schedule,version"

	t.Run("the first version is the chain head", func(t *testing.T) {
		registry := newLoadAssignmentRegistry()
		seedLoadAssignment(registry, synFirstLoadAssignment(t), synLoadAssignedAt.Add(time.Minute))

		answered := postTo(t, synLoadAssignmentEndpoint(t, registry), loadAssignmentPath, anotherFirstVersionBody())
		raw := decodeRawAnswer(t, answered)
		if answered.Code != http.StatusOK || raw["outcome"] != "LOAD_ASSIGNMENT_ALREADY_FORMED" || raw["version"] != "SYN-LAV-1" {
			t.Fatalf("status = %d body = %s", answered.Code, answered.Body.String())
		}
		if got := answerKeys(raw); got != liveShape {
			t.Fatalf("键集 = %s, want %s", got, liveShape)
		}
	})

	t.Run("a revised version is the chain head", func(t *testing.T) {
		registry := newLoadAssignmentRegistry()
		first := synFirstLoadAssignment(t)
		seedLoadAssignment(registry, first, synLoadAssignedAt.Add(time.Minute))
		revised, err := first.ReviseMembers(
			[]domain.CarriedObjectReference{httpValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-1")},
			nil,
			httpValue(t, domain.NewLoadAssignmentVersion, "SYN-LAV-2"),
			synLoadAssignedAt.Add(time.Hour),
		)
		if err != nil {
			t.Fatalf("变化：%v", err)
		}
		seedLoadAssignment(registry, revised, synLoadAssignedAt.Add(time.Hour+time.Minute))

		answered := postTo(t, synLoadAssignmentEndpoint(t, registry), loadAssignmentPath, anotherFirstVersionBody())
		raw := decodeRawAnswer(t, answered)
		if answered.Code != http.StatusOK || raw["outcome"] != "LOAD_ASSIGNMENT_ALREADY_FORMED" || raw["version"] != "SYN-LAV-2" {
			t.Fatalf("status = %d body = %s", answered.Code, answered.Body.String())
		}
		if got := answerKeys(raw); got != liveShape {
			t.Fatalf("键集 = %s, want %s", got, liveShape)
		}
	})
}

func synFirstVersionBody(schedule string) map[string]any {
	return map[string]any{
		"assignment": "SYN-LA-0002",
		"schedule":   schedule,
		"members":    []string{"SYN-PARCEL-1"},
		"version":    "SYN-LAV-1",
		"assignedAt": "2026-09-05T12:00:00Z",
	}
}

// Covers: 票 demo-chain-gaps/19（AT-TF-099）——班次不在本租户班次登记里答 200 SCHEDULE_NOT_FOUND，答复只有 outcome 一键：没有形成
// 任何版本可回显，也不带续办引用（原样重投仍是这一格）；只登在他租户名下的同名班次同答这一格，答复不透露它在别处存在。
// 同一分配换成在册班次再投照常 201，证这一格没有留下任何东西挡住它。
func TestFormLoadAssignmentAnswersScheduleNotFound(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	schedules := registeredSchedules("SYN-TENANT-01", "SYN-SCHEDULE-1")
	schedules.registered["SYN-TENANT-02|SYN-SCHEDULE-OTHER-TENANT"] = true
	endpoint := synLoadAssignmentEndpointOver(t, registry, schedules)

	for _, schedule := range []string{"SYN-SCHEDULE-NEVER-REGISTERED", "SYN-SCHEDULE-OTHER-TENANT"} {
		answered := postTo(t, endpoint, loadAssignmentPath, synFirstVersionBody(schedule))
		if answered.Code != http.StatusOK {
			t.Fatalf("%s：status = %d, want 200; body = %s", schedule, answered.Code, answered.Body.String())
		}
		raw := decodeRawAnswer(t, answered)
		if raw["outcome"] != "SCHEDULE_NOT_FOUND" || answerKeys(raw) != "outcome" {
			t.Fatalf("%s：want 只有 outcome 的 SCHEDULE_NOT_FOUND，got %s", schedule, answered.Body.String())
		}
	}
	if len(registry.records) != 0 {
		t.Fatalf("班次不在册却登记了 %d 个版本", len(registry.records))
	}

	formed := postTo(t, endpoint, loadAssignmentPath, synFirstVersionBody("SYN-SCHEDULE-1"))
	if formed.Code != http.StatusCreated || decodeRawAnswer(t, formed)["outcome"] != "LOAD_ASSIGNMENT_FORMED" {
		t.Fatalf("换成在册班次：status = %d body = %s", formed.Code, formed.Body.String())
	}
}

// Covers: 票 demo-chain-gaps/19（AT-TF-099）——班次登记读不动答 200 LOAD_ASSIGNMENT_UNDECIDED 带续办引用，与班次不在册分开：
// 前者原样重试就可能过，后者要去登班次或换班次。
func TestFormLoadAssignmentIsUndecidedWhenTheScheduleRegistryIsUnreadable(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	schedules := registeredSchedules("SYN-TENANT-01", "SYN-SCHEDULE-1")
	schedules.findErr = errors.New("班次登记不可用")

	answered := postTo(t, synLoadAssignmentEndpointOver(t, registry, schedules), loadAssignmentPath, synFirstVersionBody("SYN-SCHEDULE-1"))
	if answered.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", answered.Code, answered.Body.String())
	}
	raw := decodeRawAnswer(t, answered)
	if raw["outcome"] != "LOAD_ASSIGNMENT_UNDECIDED" || raw["continuationReference"] == nil {
		t.Fatalf("want 带续办引用的 LOAD_ASSIGNMENT_UNDECIDED，got %s", answered.Body.String())
	}
	if len(registry.records) != 0 {
		t.Fatalf("班次登记读不动却登记了 %d 个版本", len(registry.records))
	}
}
