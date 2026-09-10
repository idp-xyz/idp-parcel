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

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfparcelshipment "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 票 tf-segment-lifecycle-closure/12「生产入口」：末端派送任务内部触发执行器的端点。本文件证传输层——未配置 403 不读体、
// 方法门、编排 error 5xx 无 outcome、结果代数逐格透出（不是触发事实带 refusal / 未决带续办引用 / 已形成 201 带任务且
// Place 是 PS 引用串逐字）。执行器自己的规则在 application 测试守。

// ---- intake 替身：JSON 体 → 触发命令，租户从信封给 ----

type triggerIntakeDouble struct {
	jsonIntake[application.TriggerDeliveryDispatchCommand]
}

func (intake *triggerIntakeDouble) IntakeDeliveryDispatchTrigger(_ context.Context, request *http.Request) (application.TriggerDeliveryDispatchCommand, error) {
	return intake.intake(request)
}

func newTriggerIntake(t *testing.T) *triggerIntakeDouble {
	t.Helper()
	return &triggerIntakeDouble{jsonIntake[application.TriggerDeliveryDispatchCommand]{
		tenant: tenantOne(t),
		build: func(tenant domain.TenantID, raw map[string]any) (application.TriggerDeliveryDispatchCommand, error) {
			occurredAt, err := timeField(raw, "occurredAt")
			if err != nil {
				return application.TriggerDeliveryDispatchCommand{}, err
			}
			return application.TriggerDeliveryDispatchCommand{
				TenantID:   tenant,
				Segment:    stringField(raw, "segment"),
				Object:     stringField(raw, "object"),
				OccurredAt: occurredAt,
			}, nil
		},
	}}
}

// ---- 段登记册：让替身里的段成为派送段 ----

// deliverySegmentRegistry 包一层 segmentRegistryDouble：读回时把段的服务动作钉成末端派送。替身本尊不带服务动作
// （它为关段与终止参与的用例造），而触发执行器只认派送段。
type deliverySegmentRegistry struct {
	*segmentRegistryDouble
}

func (registry deliverySegmentRegistry) FindByKey(ctx context.Context, key ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error) {
	record, found, err := registry.segmentRegistryDouble.FindByKey(ctx, key)
	if err != nil || !found {
		return record, found, err
	}
	var participations []domain.RehydrateParticipationSpec
	for _, participation := range record.Segment.Participations() {
		participations = append(participations, participationSpecOf(participation))
	}
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       key.TenantID,
		Segment:        key.Segment,
		Participations: participations,
		ServiceAction:  domain.SegmentServesFinalDelivery,
	})
	if err != nil {
		return ports.FulfillmentSegmentRecord{}, false, err
	}
	return ports.FulfillmentSegmentRecord{Key: key, Segment: segment, RecordedAt: record.RecordedAt}, true, nil
}

// seedHandoverParticipation 往替身里放一条凭`已交接`在场的参与：对象在 segment-1 里，链尾未离场、未失效。
func seedHandoverParticipation(t *testing.T, registry *segmentRegistryDouble, object string) {
	t.Helper()
	body := defaultHandoverBody()
	body.Object = object
	body.Version = "handover-result/" + object + "/v1"
	body.Segment = "segment-1"
	handover := newHandoverFixture(t)
	if seed := postTo(t, handover.register, "/transport-fulfillment/handovers", body); seed.Code != http.StatusCreated {
		t.Fatalf("seed %s: status = %d body = %s", object, seed.Code, seed.Body.String())
	}
	*registry = *handover.segments
}

// ---- 派送要求端口替身（时间窗与条件；地点走真适配器） ----

type resolvedWindow struct{ from, to time.Time }

func (window resolvedWindow) LoadDeliveryWindow(context.Context, domain.TenantID, domain.PlannedSegmentReference, bool) (time.Time, time.Time, ports.RequirementResolution, error) {
	return window.from, window.to, ports.RequirementResolved, nil
}

type resolvedConditions struct{ reference string }

func (conditions resolvedConditions) LoadDeliveryConditions(context.Context, domain.TenantID, domain.CarriedObjectReference) (string, ports.RequirementResolution, error) {
	return conditions.reference, ports.RequirementResolved, nil
}

// parcelShipmentReferenceStub 扮 PS 读口：request-1 的成员答基线锚引用，其余答没有。
type parcelShipmentReferenceStub struct {
	members   map[string]bool
	reference psdomain.DeliveryPlaceReference
}

func (stub parcelShipmentReferenceStub) LoadDeliveryPlaceReference(_ context.Context, _ psdomain.TenantID, parcel psdomain.DeclaredParcelID) (psdomain.DeliveryPlaceResolution, error) {
	if !stub.members[parcel.String()] {
		return psdomain.NoDeliveryPlaceResolution(), nil
	}
	return psdomain.DeliveryPlaceReferenced(stub.reference)
}

func baselineReference(t *testing.T) psdomain.DeliveryPlaceReference {
	t.Helper()
	requestID, err := psdomain.NewShipmentRequestID("request-1")
	if err != nil {
		t.Fatalf("request id: %v", err)
	}
	scope, err := psdomain.NewShipmentScopedSourceData(requestID, psdomain.DeliveryPlaceDataGroup())
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	tenant, err := psdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	reference, err := psdomain.NewDeliveryPlaceReference(tenant, scope, psdomain.NewAcceptanceBaselineAnchor())
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	return reference
}

type triggerFixture struct {
	segments  *segmentRegistryDouble
	tasks     *dispatchTaskRegistryDouble
	reference psdomain.DeliveryPlaceReference
	deps      application.TriggerDeliveryDispatchDeps
}

func newTriggerFixture(t *testing.T, members ...string) *triggerFixture {
	t.Helper()
	fixture := &triggerFixture{segments: newSegmentRegistry(), tasks: newDispatchTaskRegistry(), reference: baselineReference(t)}
	known := map[string]bool{}
	for _, member := range members {
		known[member] = true
	}
	places, err := tfparcelshipment.NewDeliveryPlaceSource(parcelShipmentReferenceStub{members: known, reference: fixture.reference})
	if err != nil {
		t.Fatalf("delivery place source: %v", err)
	}
	fixture.deps = application.TriggerDeliveryDispatchDeps{
		Segments:   deliverySegmentRegistry{fixture.segments},
		Places:     places,
		Windows:    resolvedWindow{from: time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC), to: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)},
		Conditions: resolvedConditions{reference: "delivery-conditions/contract-1/v3"},
		Dispatch:   application.NewOpenDispatchTaskHandler(application.OpenDispatchTaskDeps{Tasks: fixture.tasks, Clock: tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)}}),
	}
	return fixture
}

func (fixture *triggerFixture) endpoint(t *testing.T) http.Handler {
	t.Helper()
	return tfhttp.NewTriggerDeliveryDispatchEndpoint(newTriggerIntake(t), application.NewTriggerDeliveryDispatchHandler(fixture.deps))
}

type triggerView struct {
	Outcome               string   `json:"outcome"`
	Refusal               string   `json:"refusal"`
	Missing               []string `json:"missing"`
	UndecidedReason       string   `json:"undecidedReason"`
	Task                  string   `json:"task"`
	Kind                  string   `json:"kind"`
	Objects               []string `json:"objects"`
	Place                 string   `json:"place"`
	Conditions            string   `json:"conditions"`
	ContinuationReference string   `json:"continuationReference"`
}

func decodeTrigger(t *testing.T, recorder *httptest.ResponseRecorder) triggerView {
	t.Helper()
	var view triggerView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.Bytes(), err)
	}
	return view
}

func triggerBody(segment, object string) map[string]any {
	return map[string]any{"segment": segment, "object": object, "occurredAt": "2026-09-05T15:00:00Z"}
}

// Covers: 一拍打通——对象凭`已交接`在派送段里、三条缝都答了 → 201 DISPATCH_TASK_FORMED，Place 是 PS 的 `DPR-1:` 串
// 逐字（本口不解析、不重拼）；同一拍重投 200 DISPATCH_TASK_ALREADY_FORMED 交回同一任务。
func TestTriggeringADeliveryDispatchFormsTheTaskWithTheOwnersPlaceReference(t *testing.T) {
	fixture := newTriggerFixture(t, "parcel-1")
	seedHandoverParticipation(t, fixture.segments, "parcel-1")
	endpoint := fixture.endpoint(t)

	formed := postTo(t, endpoint, "/transport-fulfillment-delivery-dispatch-triggers", triggerBody("segment-1", "parcel-1"))
	if formed.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", formed.Code, formed.Body.String())
	}
	view := decodeTrigger(t, formed)
	if view.Outcome != "DISPATCH_TASK_FORMED" || view.Kind != "DELIVERY" || len(view.Objects) != 1 || view.Objects[0] != "parcel-1" {
		t.Fatalf("view = %+v", view)
	}
	if view.Place != fixture.reference.String() || view.Place != "DPR-1:tenant-1/request-1/DELIVERY_PLACE/baseline" {
		t.Fatalf("place = %q, want the PS reference verbatim %q", view.Place, fixture.reference.String())
	}
	if view.Conditions != "delivery-conditions/contract-1/v3" {
		t.Fatalf("conditions = %q", view.Conditions)
	}

	replay := postTo(t, endpoint, "/transport-fulfillment-delivery-dispatch-triggers", triggerBody("segment-1", "parcel-1"))
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200; body = %s", replay.Code, replay.Body.String())
	}
	if again := decodeTrigger(t, replay); again.Outcome != "DISPATCH_TASK_ALREADY_FORMED" || again.Task != view.Task {
		t.Fatalf("replay view = %+v, want the same task already formed", again)
	}
}

// Covers: 结果代数其余格在传输层的透出——不是触发事实 200 带 refusal、所有者答没有 200 带 missing 名单（不留续办引用）、
// 未决 200 带 undecidedReason 与续办引用、未受理 200。都是形成了的答案，不是 4xx/5xx。
func TestTriggerOutcomesAreFormedAnswersWithTheirOwnCells(t *testing.T) {
	t.Run("segment not found is a refusal", func(t *testing.T) {
		fixture := newTriggerFixture(t)
		response := postTo(t, fixture.endpoint(t), "/transport-fulfillment-delivery-dispatch-triggers", triggerBody("segment-9", "parcel-1"))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		if view := decodeTrigger(t, response); view.Outcome != "NOT_A_DELIVERY_TRIGGER" || view.Refusal != "SEGMENT_NOT_FOUND" || view.ContinuationReference != "" {
			t.Fatalf("view = %+v", view)
		}
	})

	t.Run("the owner answering no delivery place is a missing requirement", func(t *testing.T) {
		fixture := newTriggerFixture(t) // PS 不认识任何成员：集运单元或不可见对象走的就是这一格
		seedHandoverParticipation(t, fixture.segments, "consolidation-unit-7")
		response := postTo(t, fixture.endpoint(t), "/transport-fulfillment-delivery-dispatch-triggers", triggerBody("segment-1", "consolidation-unit-7"))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		view := decodeTrigger(t, response)
		if view.Outcome != "REQUIREMENT_MISSING" || len(view.Missing) != 1 || view.Missing[0] != "DELIVERY_PLACE" || view.ContinuationReference != "" || view.Task != "" {
			t.Fatalf("view = %+v", view)
		}
	})

	t.Run("a registry failure is undecided with a continuation", func(t *testing.T) {
		fixture := newTriggerFixture(t)
		fixture.segments.findErr = errors.New("registry down")
		response := postTo(t, fixture.endpoint(t), "/transport-fulfillment-delivery-dispatch-triggers", triggerBody("segment-1", "parcel-1"))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		if view := decodeTrigger(t, response); view.Outcome != "DISPATCH_UNDECIDED" || view.UndecidedReason != "SEGMENT_REGISTRY_UNAVAILABLE" || view.ContinuationReference == "" {
			t.Fatalf("view = %+v", view)
		}
	})

	t.Run("a malformed trigger is not accepted", func(t *testing.T) {
		fixture := newTriggerFixture(t)
		response := postTo(t, fixture.endpoint(t), "/transport-fulfillment-delivery-dispatch-triggers", map[string]any{"segment": "", "object": "parcel-1", "occurredAt": "2026-09-05T15:00:00Z"})
		if response.Code != http.StatusOK || decodeTrigger(t, response).Outcome != "INPUT_NOT_ACCEPTED" {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
}

// Covers: ADR-0022 的错误支——执行器 error 是没形成答案（5xx 无 outcome）；Intake 构造不出是 400。
func TestTriggerEndpointKeepsTheAnswerFormedDistinction(t *testing.T) {
	t.Run("an orchestration error forms no answer", func(t *testing.T) {
		endpoint := tfhttp.NewTriggerDeliveryDispatchEndpoint(newTriggerIntake(t), failingTriggerer{})
		response := postTo(t, endpoint, "/transport-fulfillment-delivery-dispatch-triggers", triggerBody("segment-1", "parcel-1"))
		if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	})

	t.Run("a malformed body is the caller's problem", func(t *testing.T) {
		endpoint := tfhttp.NewTriggerDeliveryDispatchEndpoint(newTriggerIntake(t), unreachableTriggerer{t: t})
		response := postTo(t, endpoint, "/transport-fulfillment-delivery-dispatch-triggers", map[string]any{"occurredAt": "not-a-time"})
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("only POST", func(t *testing.T) {
		endpoint := tfhttp.NewTriggerDeliveryDispatchEndpoint(newTriggerIntake(t), unreachableTriggerer{t: t})
		request := httptest.NewRequest(http.MethodGet, "/transport-fulfillment-delivery-dispatch-triggers", nil)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("status = %d allow = %q", response.Code, response.Header().Get("Allow"))
		}
	})
}

// Covers: ADR-0055 / ADR-0085 决定二——写准入不另立形；装字面量 UnconfiguredIntake 时 403、不读体、不构造命令、
// 执行器不被调。这正是「拍频作配置不作默认、值留空如实答未配置」在传输层的样子。
func TestAnUnconfiguredTriggerIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	endpoint := tfhttp.NewTriggerDeliveryDispatchEndpoint(tfhttp.UnconfiguredIntake{}, unreachableTriggerer{t: t})
	probe := &readProbe{}
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-delivery-dispatch-triggers", probe)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
	if probe.read {
		t.Fatal("an unconfigured intake read the business content")
	}
}

// ---- 执行器替身 ----

type failingTriggerer struct{}

func (failingTriggerer) Trigger(context.Context, application.TriggerDeliveryDispatchCommand) (application.TriggerDeliveryDispatchResult, error) {
	return application.TriggerDeliveryDispatchResult{}, fmt.Errorf("executor exploded")
}

type unreachableTriggerer struct{ t *testing.T }

func (triggerer unreachableTriggerer) Trigger(context.Context, application.TriggerDeliveryDispatchCommand) (application.TriggerDeliveryDispatchResult, error) {
	triggerer.t.Fatal("the executor was reached although the intake refused")
	return application.TriggerDeliveryDispatchResult{}, nil
}
