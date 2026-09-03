package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 票 tf-segment-lifecycle-closure/07：四个 admin 写面（关段、建派送任务、装载分配、明确终止参与）。
// 它们是运营决定不是承运方回传口（ADR-0085 登记写面的形状）。

// ---- 端口替身 ----

type dispatchTaskRegistryDouble struct {
	records map[string]ports.DispatchTaskRecord
	findErr error
}

func newDispatchTaskRegistry() *dispatchTaskRegistryDouble {
	return &dispatchTaskRegistryDouble{records: map[string]ports.DispatchTaskRecord{}}
}

func (double *dispatchTaskRegistryDouble) FindByKey(
	_ context.Context,
	key ports.DispatchTaskKey,
) (ports.DispatchTaskRecord, bool, error) {
	if double.findErr != nil {
		return ports.DispatchTaskRecord{}, false, double.findErr
	}
	record, found := double.records[key.TenantID.String()+"|"+key.Task.String()]
	return record, found, nil
}

func (double *dispatchTaskRegistryDouble) Save(
	_ context.Context,
	record ports.DispatchTaskRecord,
) (ports.DispatchTaskSaveOutcome, error) {
	storeKey := record.Key.TenantID.String() + "|" + record.Key.Task.String()
	if _, exists := double.records[storeKey]; exists {
		return ports.DispatchTaskAlreadyOpen, nil
	}
	double.records[storeKey] = record
	return ports.DispatchTaskSaved, nil
}

type loadAssignmentRegistryDouble struct {
	records map[string]ports.LoadAssignmentRecord
}

func newLoadAssignmentRegistry() *loadAssignmentRegistryDouble {
	return &loadAssignmentRegistryDouble{records: map[string]ports.LoadAssignmentRecord{}}
}

func loadAssignmentKey(key ports.LoadAssignmentKey) string {
	return key.TenantID.String() + "|" + key.Assignment.String() + "|" + key.Version.String()
}

func (double *loadAssignmentRegistryDouble) FindByKey(
	_ context.Context,
	key ports.LoadAssignmentKey,
) (ports.LoadAssignmentRecord, bool, error) {
	record, found := double.records[loadAssignmentKey(key)]
	return record, found, nil
}

func (double *loadAssignmentRegistryDouble) Save(
	_ context.Context,
	record ports.LoadAssignmentRecord,
) (ports.LoadAssignmentSaveOutcome, error) {
	if _, exists := double.records[loadAssignmentKey(record.Key)]; exists {
		return ports.LoadAssignmentVersionAlreadyRegistered, nil
	}
	double.records[loadAssignmentKey(record.Key)] = record
	return ports.LoadAssignmentSaved, nil
}

// ---- 通用 intake 替身：JSON 体 → 命令，租户从信封给 ----

type jsonIntake[C any] struct {
	tenant domain.TenantID
	err    error
	build  func(tenant domain.TenantID, raw map[string]any) (C, error)
}

func (intake *jsonIntake[C]) intake(request *http.Request) (C, error) {
	var zero C
	if intake.err != nil {
		return zero, intake.err
	}
	var raw map[string]any
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		return zero, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return intake.build(intake.tenant, raw)
}

func stringField(raw map[string]any, name string) string {
	value, _ := raw[name].(string)
	return value
}

func stringsField(raw map[string]any, name string) []string {
	items, _ := raw[name].([]any)
	var values []string
	for _, item := range items {
		if text, ok := item.(string); ok {
			values = append(values, text)
		}
	}
	return values
}

func timeField(raw map[string]any, name string) (time.Time, error) {
	text := stringField(raw, name)
	if text == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return parsed, nil
}

type closureIntakeDouble struct {
	jsonIntake[application.CloseFulfillmentSegmentCommand]
}

func (intake *closureIntakeDouble) IntakeSegmentClosure(_ context.Context, request *http.Request) (application.CloseFulfillmentSegmentCommand, error) {
	return intake.intake(request)
}

type dispatchTaskIntakeDouble struct {
	jsonIntake[application.OpenDispatchTaskCommand]
}

func (intake *dispatchTaskIntakeDouble) IntakeDispatchTask(_ context.Context, request *http.Request) (application.OpenDispatchTaskCommand, error) {
	return intake.intake(request)
}

type loadAssignmentIntakeDouble struct {
	jsonIntake[application.FormLoadAssignmentCommand]
}

func (intake *loadAssignmentIntakeDouble) IntakeLoadAssignment(_ context.Context, request *http.Request) (application.FormLoadAssignmentCommand, error) {
	return intake.intake(request)
}

type terminationIntakeDouble struct {
	jsonIntake[tfhttp.ParticipationTermination]
}

func (intake *terminationIntakeDouble) IntakeParticipationTermination(_ context.Context, request *http.Request) (tfhttp.ParticipationTermination, error) {
	return intake.intake(request)
}

func tenantOne(t *testing.T) domain.TenantID {
	t.Helper()
	return httpValue(t, domain.NewTenantID, "tenant-1")
}

func newClosureIntake(t *testing.T) *closureIntakeDouble {
	t.Helper()
	return &closureIntakeDouble{jsonIntake[application.CloseFulfillmentSegmentCommand]{
		tenant: tenantOne(t),
		build: func(tenant domain.TenantID, raw map[string]any) (application.CloseFulfillmentSegmentCommand, error) {
			closedAt, err := timeField(raw, "closedAt")
			return application.CloseFulfillmentSegmentCommand{TenantID: tenant, Segment: stringField(raw, "segment"), ClosedAt: closedAt}, err
		},
	}}
}

func newDispatchTaskIntake(t *testing.T) *dispatchTaskIntakeDouble {
	t.Helper()
	return &dispatchTaskIntakeDouble{jsonIntake[application.OpenDispatchTaskCommand]{
		tenant: tenantOne(t),
		build: func(tenant domain.TenantID, raw map[string]any) (application.OpenDispatchTaskCommand, error) {
			kind := domain.DispatchTaskKindInvalid
			switch stringField(raw, "kind") {
			case "PICKUP":
				kind = domain.PickupDispatch
			case "DELIVERY":
				kind = domain.DeliveryDispatch
			}
			command := application.OpenDispatchTaskCommand{
				TenantID:   tenant,
				Task:       stringField(raw, "task"),
				Kind:       kind,
				Objects:    stringsField(raw, "objects"),
				Place:      stringField(raw, "place"),
				Conditions: stringField(raw, "conditions"),
			}
			var err error
			if command.WindowFrom, err = timeField(raw, "windowFrom"); err != nil {
				return command, err
			}
			if command.WindowTo, err = timeField(raw, "windowTo"); err != nil {
				return command, err
			}
			command.OpenedAt, err = timeField(raw, "openedAt")
			return command, err
		},
	}}
}

func newLoadAssignmentIntake(t *testing.T) *loadAssignmentIntakeDouble {
	t.Helper()
	return &loadAssignmentIntakeDouble{jsonIntake[application.FormLoadAssignmentCommand]{
		tenant: tenantOne(t),
		build: func(tenant domain.TenantID, raw map[string]any) (application.FormLoadAssignmentCommand, error) {
			assignedAt, err := timeField(raw, "assignedAt")
			return application.FormLoadAssignmentCommand{
				TenantID:   tenant,
				Assignment: stringField(raw, "assignment"),
				Schedule:   stringField(raw, "schedule"),
				Members:    stringsField(raw, "members"),
				Version:    stringField(raw, "version"),
				AssignedAt: assignedAt,
			}, err
		},
	}}
}

func newTerminationIntake(t *testing.T) *terminationIntakeDouble {
	t.Helper()
	return &terminationIntakeDouble{jsonIntake[tfhttp.ParticipationTermination]{
		tenant: tenantOne(t),
		build: func(tenant domain.TenantID, raw map[string]any) (tfhttp.ParticipationTermination, error) {
			endedAt, err := timeField(raw, "endedAt")
			return tfhttp.ParticipationTermination{
				TenantID: tenant,
				Segment:  stringField(raw, "segment"),
				Object:   stringField(raw, "object"),
				Basis:    stringField(raw, "basis"),
				EndedAt:  endedAt,
			}, err
		},
	}}
}

// ---- 夹具：一个由交接立起来的段，供关段与终止两口用 ----

type segmentOperationsFixture struct {
	segments  *segmentRegistryDouble
	handovers *handoverRegistryDouble
	close     http.Handler
	terminate http.Handler
}

func newSegmentOperationsFixture(t *testing.T, objects ...string) *segmentOperationsFixture {
	t.Helper()
	handover := newHandoverFixture(t)
	for index, object := range objects {
		body := defaultHandoverBody()
		body.Object = object
		body.Version = "handover-result/" + object + "/v1"
		body.JudgedAt = time.Date(2026, 9, 5, 10, index, 0, 0, time.UTC).Format(time.RFC3339)
		body.Segment = "segment-1"
		if seed := postTo(t, handover.register, "/transport-fulfillment/handovers", body); seed.Code != http.StatusCreated {
			t.Fatalf("seed %s: status = %d body = %s", object, seed.Code, seed.Body.String())
		}
	}
	fixture := &segmentOperationsFixture{segments: handover.segments, handovers: handover.registry}
	closer := application.NewCloseFulfillmentSegmentHandler(application.CloseFulfillmentSegmentDeps{Segments: fixture.segments})
	fixture.close = tfhttp.NewCloseFulfillmentSegmentEndpoint(newClosureIntake(t), closer)
	ender := application.NewEndFulfillmentParticipationHandler(application.EndFulfillmentParticipationDeps{
		Segments:   fixture.segments,
		Handovers:  fixture.handovers,
		Deliveries: newDeliveryStore(),
		Clock:      tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.terminate = tfhttp.NewTerminateFulfillmentParticipationEndpoint(newTerminationIntake(t), ender)
	return fixture
}

type operationView struct {
	Outcome               string   `json:"outcome"`
	Segment               string   `json:"segment"`
	Object                string   `json:"object"`
	Task                  string   `json:"task"`
	Kind                  string   `json:"kind"`
	Objects               []string `json:"objects"`
	Assignment            string   `json:"assignment"`
	Schedule              string   `json:"schedule"`
	Members               []string `json:"members"`
	Version               string   `json:"version"`
	ContinuationReference string   `json:"continuationReference"`
}

func decodeOperation(t *testing.T, recorder *httptest.ResponseRecorder) operationView {
	t.Helper()
	var view operationView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: CONTEXT 生命周期④在写面上的整条序列——仍有在场参与关不上（SEGMENT_STILL_ACTIVE）；全部参与
// 结束后声明关段 201 SEGMENT_CLOSED；重放是自己的答案 SEGMENT_ALREADY_CLOSED；不在册的段 SEGMENT_NOT_FOUND。
// 四格各自成格，续办动作两两不同，传输层不合并。**不自动关段**：这个口是让人做那个决定的口。
func TestSegmentClosureRunsTheLifecycleSequence(t *testing.T) {
	fixture := newSegmentOperationsFixture(t, "parcel-1", "parcel-2")
	body := map[string]any{"segment": "segment-1", "closedAt": "2026-09-05T13:00:00Z"}

	active := postTo(t, fixture.close, "/transport-fulfillment-segment-closures", body)
	if active.Code != http.StatusOK || decodeOperation(t, active).Outcome != "SEGMENT_STILL_ACTIVE" {
		t.Fatalf("status = %d body = %s, want 200 SEGMENT_STILL_ACTIVE", active.Code, active.Body.String())
	}

	fixture.segments.endAllParticipations(t, "tenant-1", "segment-1", time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC))
	closed := postTo(t, fixture.close, "/transport-fulfillment-segment-closures", body)
	if closed.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", closed.Code, closed.Body.String())
	}
	if view := decodeOperation(t, closed); view.Outcome != "SEGMENT_CLOSED" || view.Segment != "segment-1" {
		t.Fatalf("view = %+v", view)
	}

	replay := postTo(t, fixture.close, "/transport-fulfillment-segment-closures", body)
	if replay.Code != http.StatusOK || decodeOperation(t, replay).Outcome != "SEGMENT_ALREADY_CLOSED" {
		t.Fatalf("status = %d body = %s, want 200 SEGMENT_ALREADY_CLOSED", replay.Code, replay.Body.String())
	}

	absent := postTo(t, fixture.close, "/transport-fulfillment-segment-closures", map[string]any{"segment": "segment-9", "closedAt": "2026-09-05T13:00:00Z"})
	if absent.Code != http.StatusOK || decodeOperation(t, absent).Outcome != "SEGMENT_NOT_FOUND" {
		t.Fatalf("status = %d body = %s, want 200 SEGMENT_NOT_FOUND", absent.Code, absent.Body.String())
	}
}

// Covers: 明确终止参与是三来源里唯一走写面的一路——201 PARTICIPATION_ENDED；重放 PARTICIPATION_ALREADY_ENDED；
// 对象不在段内 OBJECT_NOT_IN_SEGMENT。
func TestParticipationTerminationEndsOneObjectOnly(t *testing.T) {
	fixture := newSegmentOperationsFixture(t, "parcel-1", "parcel-2")
	body := map[string]any{"segment": "segment-1", "object": "parcel-1", "basis": "control-termination/case-1", "endedAt": "2026-09-05T12:30:00Z"}

	ended := postTo(t, fixture.terminate, "/transport-fulfillment-participation-terminations", body)
	if ended.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", ended.Code, ended.Body.String())
	}
	if view := decodeOperation(t, ended); view.Outcome != "PARTICIPATION_ENDED" || view.Segment != "segment-1" || view.Object != "parcel-1" {
		t.Fatalf("view = %+v", view)
	}
	if fixture.segments.participationOf(t, "tenant-1", "segment-1", "parcel-1").Active() {
		t.Fatal("parcel-1 的参与没有结束")
	}
	if !fixture.segments.participationOf(t, "tenant-1", "segment-1", "parcel-2").Active() {
		t.Fatal("parcel-2 的参与被一起结束了——整段结果覆盖了成员差异")
	}

	replay := postTo(t, fixture.terminate, "/transport-fulfillment-participation-terminations", body)
	if replay.Code != http.StatusOK || decodeOperation(t, replay).Outcome != "PARTICIPATION_ALREADY_ENDED" {
		t.Fatalf("status = %d body = %s", replay.Code, replay.Body.String())
	}

	stranger := postTo(t, fixture.terminate, "/transport-fulfillment-participation-terminations", map[string]any{"segment": "segment-1", "object": "parcel-9", "basis": "b", "endedAt": "2026-09-05T12:30:00Z"})
	if stranger.Code != http.StatusOK || decodeOperation(t, stranger).Outcome != "OBJECT_NOT_IN_SEGMENT" {
		t.Fatalf("status = %d body = %s", stranger.Code, stranger.Body.String())
	}
}

// Covers: 票 07「终止口的 Intake 只能铸终止那一路」——端点交给编排的命令 Source 恒为明确终止、不带
// 下一段与交接/交付键；交付与交接两路是内部触发（票 06），从这个口进来就是让接入方替 TF 做步骤 7。
// 用捕获替身看编排实际收到的命令，而不是看请求形状。
func TestParticipationTerminationEndpointOnlyMintsTheTerminationSource(t *testing.T) {
	capture := &capturingParticipationEnder{}
	endpoint := tfhttp.NewTerminateFulfillmentParticipationEndpoint(newTerminationIntake(t), capture)

	postTo(t, endpoint, "/transport-fulfillment-participation-terminations", map[string]any{
		"segment": "segment-1", "object": "parcel-1", "basis": "control-termination/case-1", "endedAt": "2026-09-05T12:30:00Z",
		"nextSegment": "segment-2", "scope": "scope-1", "version": "v9", "attempt": "attempt-1",
	})

	if capture.last == nil {
		t.Fatal("编排没有被调到")
	}
	command := *capture.last
	if command.Source != application.ParticipationEndedByTermination {
		t.Fatalf("source = %v, want termination", command.Source)
	}
	if command.NextSegment != "" || command.NextPlannedSegment != "" || command.Scope != "" || command.Version != "" || command.Attempt != "" {
		t.Fatalf("终止口不该收下一段或交接/交付键：%+v", command)
	}
	if command.Segment != "segment-1" || command.Object != "parcel-1" || command.Basis != "control-termination/case-1" {
		t.Fatalf("command = %+v", command)
	}
}

// Covers: 授权角色建立派送任务——201 DISPATCH_TASK_OPENED 带任务、种类与对象范围；重投交回原任务
// DISPATCH_TASK_ALREADY_OPEN；缺工作范围 INPUT_NOT_ACCEPTED。
func TestOpenDispatchTaskRegistersTheWorkScope(t *testing.T) {
	registry := newDispatchTaskRegistry()
	handler := application.NewOpenDispatchTaskHandler(application.OpenDispatchTaskDeps{Tasks: registry, Clock: tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)}})
	endpoint := tfhttp.NewOpenDispatchTaskEndpoint(newDispatchTaskIntake(t), handler)
	body := map[string]any{
		"task": "dispatch-task-1", "kind": "PICKUP", "objects": []string{"parcel-1", "parcel-2"}, "place": "customer-warehouse-1",
		"windowFrom": "2026-09-06T09:00:00Z", "windowTo": "2026-09-06T12:00:00Z", "conditions": "service-condition/v1", "openedAt": "2026-09-05T12:00:00Z",
	}

	opened := postTo(t, endpoint, "/transport-fulfillment-dispatch-task-registrations", body)
	if opened.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", opened.Code, opened.Body.String())
	}
	view := decodeOperation(t, opened)
	if view.Outcome != "DISPATCH_TASK_OPENED" || view.Task != "dispatch-task-1" || view.Kind != "PICKUP" || len(view.Objects) != 2 {
		t.Fatalf("view = %+v", view)
	}

	replay := postTo(t, endpoint, "/transport-fulfillment-dispatch-task-registrations", body)
	if replay.Code != http.StatusOK || decodeOperation(t, replay).Outcome != "DISPATCH_TASK_ALREADY_OPEN" {
		t.Fatalf("status = %d body = %s", replay.Code, replay.Body.String())
	}

	body["objects"] = []string{}
	body["task"] = "dispatch-task-2"
	rejected := postTo(t, endpoint, "/transport-fulfillment-dispatch-task-registrations", body)
	if rejected.Code != http.StatusOK || decodeOperation(t, rejected).Outcome != "INPUT_NOT_ACCEPTED" {
		t.Fatalf("status = %d body = %s", rejected.Code, rejected.Body.String())
	}
}

// Covers: 运输运营形成装载分配——201 LOAD_ASSIGNMENT_FORMED 带分配、班次、版本与成员；同版本重投交回原版本
// LOAD_ASSIGNMENT_VERSION_EXISTS。分配是执行意图不是已发生的装载：响应里没有任何「已装载」字段。
func TestFormLoadAssignmentRegistersTheVersion(t *testing.T) {
	handler := application.NewFormLoadAssignmentHandler(application.FormLoadAssignmentDeps{Assignments: newLoadAssignmentRegistry(), Clock: tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)}})
	endpoint := tfhttp.NewFormLoadAssignmentEndpoint(newLoadAssignmentIntake(t), handler)
	body := map[string]any{"assignment": "load-assignment-1", "schedule": "schedule-1", "members": []string{"parcel-1", "parcel-2"}, "version": "LAV-000000000001", "assignedAt": "2026-09-05T12:00:00Z"}

	formed := postTo(t, endpoint, "/transport-fulfillment-load-assignment-registrations", body)
	if formed.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", formed.Code, formed.Body.String())
	}
	view := decodeOperation(t, formed)
	if view.Outcome != "LOAD_ASSIGNMENT_FORMED" || view.Assignment != "load-assignment-1" || view.Schedule != "schedule-1" || view.Version != "LAV-000000000001" || len(view.Members) != 2 {
		t.Fatalf("view = %+v", view)
	}

	replay := postTo(t, endpoint, "/transport-fulfillment-load-assignment-registrations", body)
	if replay.Code != http.StatusOK || decodeOperation(t, replay).Outcome != "LOAD_ASSIGNMENT_VERSION_EXISTS" {
		t.Fatalf("status = %d body = %s", replay.Code, replay.Body.String())
	}
}

// Covers: ADR-0022 在四口上的错误支——编排 error 5xx 无 outcome；未决 outcome 200 带续办引用（以关段口为例，
// 其余三口走同一个 commandEndpoint）。
func TestSegmentOperationsKeepTheAnswerFormedDistinction(t *testing.T) {
	t.Run("an orchestration error forms no answer", func(t *testing.T) {
		endpoint := tfhttp.NewCloseFulfillmentSegmentEndpoint(newClosureIntake(t), failingSegmentCloser{})
		response := postTo(t, endpoint, "/transport-fulfillment-segment-closures", map[string]any{"segment": "segment-1", "closedAt": "2026-09-05T13:00:00Z"})
		if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	})

	t.Run("a registry failure is an undecided answer", func(t *testing.T) {
		fixture := newSegmentOperationsFixture(t, "parcel-1")
		fixture.segments.findErr = errors.New("registry down")
		response := postTo(t, fixture.close, "/transport-fulfillment-segment-closures", map[string]any{"segment": "segment-1", "closedAt": "2026-09-05T13:00:00Z"})
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
		}
		if view := decodeOperation(t, response); view.Outcome != "SEGMENT_CLOSE_UNDECIDED" || view.ContinuationReference == "" {
			t.Fatalf("view = %+v", view)
		}
	})
}

// Covers: ADR-0085 决定二——写准入不另立形；四口装字面量 UnconfiguredIntake 时 403、不读体、不构造命令。
func TestUnconfiguredSegmentOperationIntakesRefuseWithoutReadingTheBody(t *testing.T) {
	endpoints := map[string]http.Handler{
		"/transport-fulfillment-segment-closures":              tfhttp.NewCloseFulfillmentSegmentEndpoint(tfhttp.UnconfiguredIntake{}, unreachableSegmentCloser{t: t}),
		"/transport-fulfillment-dispatch-task-registrations":   tfhttp.NewOpenDispatchTaskEndpoint(tfhttp.UnconfiguredIntake{}, unreachableDispatchTaskOpener{t: t}),
		"/transport-fulfillment-load-assignment-registrations": tfhttp.NewFormLoadAssignmentEndpoint(tfhttp.UnconfiguredIntake{}, unreachableLoadAssigner{t: t}),
		"/transport-fulfillment-participation-terminations":    tfhttp.NewTerminateFulfillmentParticipationEndpoint(tfhttp.UnconfiguredIntake{}, unreachableParticipationEnder{t: t}),
	}
	for path, endpoint := range endpoints {
		t.Run(path, func(t *testing.T) {
			probe := &readProbe{}
			request := httptest.NewRequest(http.MethodPost, path, probe)
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			assertNoOutcome(t, response)
			if probe.read {
				t.Fatal("an unconfigured intake read the business content")
			}
		})
	}
}

// ---- 编排替身 ----

type capturingParticipationEnder struct {
	last *application.EndFulfillmentParticipationCommand
}

func (capture *capturingParticipationEnder) End(
	_ context.Context,
	command application.EndFulfillmentParticipationCommand,
) (application.EndFulfillmentParticipationResult, error) {
	capture.last = &command
	return application.EndFulfillmentParticipationResult{}, errors.New("capture only")
}

type failingSegmentCloser struct{}

func (failingSegmentCloser) Close(context.Context, application.CloseFulfillmentSegmentCommand) (application.CloseFulfillmentSegmentResult, error) {
	return application.CloseFulfillmentSegmentResult{}, errors.New("orchestration exploded")
}

type unreachableSegmentCloser struct{ t *testing.T }

func (handler unreachableSegmentCloser) Close(context.Context, application.CloseFulfillmentSegmentCommand) (application.CloseFulfillmentSegmentResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.CloseFulfillmentSegmentResult{}, nil
}

type unreachableDispatchTaskOpener struct{ t *testing.T }

func (handler unreachableDispatchTaskOpener) Open(context.Context, application.OpenDispatchTaskCommand) (application.OpenDispatchTaskResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.OpenDispatchTaskResult{}, nil
}

type unreachableLoadAssigner struct{ t *testing.T }

func (handler unreachableLoadAssigner) Form(context.Context, application.FormLoadAssignmentCommand) (application.FormLoadAssignmentResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.FormLoadAssignmentResult{}, nil
}

type unreachableParticipationEnder struct{ t *testing.T }

func (handler unreachableParticipationEnder) End(context.Context, application.EndFulfillmentParticipationCommand) (application.EndFulfillmentParticipationResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.EndFulfillmentParticipationResult{}, nil
}
