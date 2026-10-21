package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证派送待形成册的运营一侧（ADR-0230 决定三至五、七）：补拍口（运营决定）答要求缺失落册，查阅页按租户列出
// 待形成清单，按册重问复用同一个执行器、幂等、不另立任务；到期重问在重问间隔没登记时答未配置，登记之后按最近登记的那一版筛。交接经
// 生产控制事实编排登记，册、任务、间隔都是真表，生产的端点构造函数；只三条要求缝是替身。测试输入是合成数据，只记 `S`。

const reaskSegment = "SYN-DELIVERY-SEGMENT-70"

// reaskRig 在 taskReferenceRig 之上补齐运营一侧：按册重问、查阅页、重问间隔登记三口，执行器与补拍口同一组替身所有者。
type reaskRig struct {
	*taskReferenceRig
	reask    http.Handler
	records  http.Handler
	cadences http.Handler
}

func newReaskRig(t *testing.T) *reaskRig {
	t.Helper()
	base := newTaskReferenceRig(t)
	trigger, err := deliveryDispatchTriggerOver(base.db, base.owners, base.owners, base.owners)
	if err != nil {
		t.Fatalf("装配派送发起执行器：%v", err)
	}
	reask, err := deliveryDispatchReaskOver(base.db, trigger)
	if err != nil {
		t.Fatalf("装配按册重问：%v", err)
	}
	registration, err := buildReaskCadenceRegistration(base.db)
	if err != nil {
		t.Fatalf("装配重问间隔登记：%v", err)
	}
	catalogue, err := tfpostgres.NewReviewCatalogue(base.db)
	if err != nil {
		t.Fatalf("查阅读口：%v", err)
	}
	readers, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	readIntakes, err := buildOperatorRegistryIntakes(readers)
	if err != nil {
		t.Fatal(err)
	}
	writers, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	writeIntakes, err := buildOperatorRegistryIntakes(writers)
	if err != nil {
		t.Fatal(err)
	}
	return &reaskRig{
		taskReferenceRig: base,
		reask:            tfhttp.NewReaskDeliveryDispatchesEndpoint(base.decisions, reask),
		records:          tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(readIntakes.transportOperationsRead, catalogue),
		cadences:         tfhttp.NewRegisterReaskCadenceEndpoint(writeIntakes.transport, registration),
	}
}

func serveReaskOperator(t *testing.T, handler http.Handler, method, target, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("%s %s 答复不是 JSON：%d %s", method, target, recorder.Code, recorder.Body.String())
	}
	return recorder.Code, decoded
}

func reaskOutcomeOf(t *testing.T, body map[string]json.RawMessage) string {
	t.Helper()
	var outcome string
	if err := json.Unmarshal(body["outcome"], &outcome); err != nil {
		t.Fatalf("答复没有 outcome：%v", body)
	}
	return outcome
}

type reaskItemView struct {
	Segment string   `json:"segment"`
	Object  string   `json:"object"`
	Outcome string   `json:"outcome"`
	Missing []string `json:"missing"`
	Task    string   `json:"task"`
}

func (rig *reaskRig) reaskWith(t *testing.T, body string) (string, []reaskItemView, map[string]json.RawMessage) {
	t.Helper()
	status, answer := serveReaskOperator(t, rig.reask, http.MethodPost, "/transport-fulfillment-delivery-dispatch-reasks", body)
	if status != http.StatusOK {
		t.Fatalf("按册重问 %s 答 %d %v", body, status, answer)
	}
	var items []reaskItemView
	if err := json.Unmarshal(answer["items"], &items); err != nil {
		t.Fatalf("答复没有 items：%v", answer)
	}
	return reaskOutcomeOf(t, answer), items, answer
}

func (rig *reaskRig) pendingListed(t *testing.T) []string {
	t.Helper()
	status, answer := serveReaskOperator(t, rig.records, http.MethodGet, "/transport-fulfillment-records?registry=delivery-dispatch-pending", "")
	if status != http.StatusOK || reaskOutcomeOf(t, answer) != "DELIVERY_DISPATCH_PENDINGS_LISTED" {
		t.Fatalf("查阅页答 %d %v", status, answer)
	}
	var rows []struct {
		Segment string   `json:"segment"`
		Object  string   `json:"object"`
		Missing []string `json:"missing"`
	}
	if err := json.Unmarshal(answer["pendings"], &rows); err != nil {
		t.Fatal(err)
	}
	var objects []string
	for _, row := range rows {
		objects = append(objects, row.Segment+"/"+row.Object+"/"+strings.Join(row.Missing, ","))
	}
	return objects
}

// Covers: AT-TF-288、AT-TF-284、AT-TF-285、AT-TF-282（ADR-0230 决定三、四、七）——补拍口挂运营决定：两件对象的地点所有者答「没有」，
// 各答 REQUIREMENT_MISSING 并落册；查阅页列出本租户这两件（段、对象、缺地点），别的租户那一条不出现。所有者补齐之后按册重问全部：
// 两件逐件形成任务、册上了结，查阅页清空；再问一次零件、任务仍两项；对其中一件再补拍答已在册。
func TestOperatorsSeeThePendingDeliveriesAndReaskThemWithoutDuplicates(t *testing.T) {
	rig := newReaskRig(t)
	rig.owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = true })
	rig.enter(t, "SYN-PARCEL-70-A", reaskSegment, "SYN-HANDOVER-SCOPE-70", "SYN-HANDOVER-RESULT/70-A/v1", tfdomain.SegmentServesFinalDelivery)
	rig.enter(t, "SYN-PARCEL-70-B", reaskSegment, "SYN-HANDOVER-SCOPE-70", "SYN-HANDOVER-RESULT/70-B/v1", tfdomain.SegmentServiceActionUndeclared)
	for _, object := range []string{"SYN-PARCEL-70-A", "SYN-PARCEL-70-B"} {
		if status, view, raw := rig.tick(t, reaskSegment, object, "2026-01-02T09:00:00Z"); status != http.StatusOK || view.Outcome != "REQUIREMENT_MISSING" {
			t.Fatalf("补拍 %s 答 %d %s, want 200 REQUIREMENT_MISSING", object, status, raw)
		}
	}
	other, err := tfpostgres.NewDeliveryDispatchPendings(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := rig.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return other.RecordMissing(txCtx, tfports.DeliveryDispatchPendingKey{
			TenantID: mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-02"),
			Segment:  mustValue(t, tfdomain.NewFulfillmentSegmentReference, reaskSegment),
			Object:   mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-70-A"),
		}, mustValue(t, tfdomain.NewParticipationBasisReference, "SYN-BASIS"), []string{"DELIVERY_WINDOW"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	}); err != nil {
		t.Fatal(err)
	}
	listed := rig.pendingListed(t)
	want := []string{reaskSegment + "/SYN-PARCEL-70-A/DELIVERY_PLACE", reaskSegment + "/SYN-PARCEL-70-B/DELIVERY_PLACE"}
	if strings.Join(listed, ";") != strings.Join(want, ";") {
		t.Fatalf("查阅页列出 %v, want %v——只列本租户待形成的", listed, want)
	}

	rig.owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = false })
	outcome, items, _ := rig.reaskWith(t, `{"scope":"ALL","limit":10}`)
	formedA := items[0].Task
	if outcome != "PENDING_DELIVERY_DISPATCHES_REASKED" || len(items) != 2 ||
		items[0].Outcome != "DISPATCH_TASK_FORMED" || items[1].Outcome != "DISPATCH_TASK_FORMED" {
		t.Fatalf("按册重问 = %s %+v, want 两件都形成", outcome, items)
	}
	if rows := rig.taskRows(t); rows != 2 {
		t.Fatalf("任务 = %d 项, want 2", rows)
	}
	if listed := rig.pendingListed(t); len(listed) != 0 {
		t.Fatalf("形成之后查阅页仍列 %v", listed)
	}

	if _, items, _ := rig.reaskWith(t, `{"scope":"ALL","limit":10}`); len(items) != 0 {
		t.Fatalf("再问一次问了 %+v, want 零件", items)
	}
	if status, view, raw := rig.tick(t, reaskSegment, "SYN-PARCEL-70-A", "2026-01-03T09:00:00Z"); status != http.StatusOK ||
		view.Outcome != "DISPATCH_TASK_ALREADY_FORMED" || view.Task != formedA {
		t.Fatalf("再补拍答 %d %s, want DISPATCH_TASK_ALREADY_FORMED 交回 %s", status, raw, formedA)
	}
	if rows := rig.taskRows(t); rows != 2 {
		t.Fatalf("任务 = %d 项, want 仍 2", rows)
	}
}

// Covers: AT-TF-286、AT-TF-287、AT-TF-291（ADR-0230 决定五）——到期重问而本租户没登记重问间隔：答 REASK_CADENCE_NOT_CONFIGURED、一件也
// 不问，所有者一次也不被问。登记一版（登记册配置写，201），同版本同正文重放 200 EXISTING_VERSION、异正文 CONTENT_CONFLICT；之后到期
// 重问按它筛、答复带所凭的版本与到期时刻；再登一版很长的间隔，最近登记的那一版生效，没有一件到期。查阅页那一格列出两版、新登的在前。
func TestReaskingWhatIsDueNeedsTheTenantsRegisteredCadence(t *testing.T) {
	rig := newReaskRig(t)
	rig.owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = true })
	rig.enter(t, "SYN-PARCEL-70-C", reaskSegment, "SYN-HANDOVER-SCOPE-70", "SYN-HANDOVER-RESULT/70-C/v1", tfdomain.SegmentServesFinalDelivery)
	if status, view, raw := rig.tick(t, reaskSegment, "SYN-PARCEL-70-C", "2026-01-02T09:00:00Z"); status != http.StatusOK || view.Outcome != "REQUIREMENT_MISSING" {
		t.Fatalf("补拍答 %d %s", status, raw)
	}
	asked := rig.owners.askedSoFar()

	outcome, items, _ := rig.reaskWith(t, `{"scope":"DUE","limit":10}`)
	if outcome != "REASK_CADENCE_NOT_CONFIGURED" || len(items) != 0 || rig.owners.askedSoFar() != asked {
		t.Fatalf("未登记时到期重问 = %s %+v、所有者多被问 %d 次, want REASK_CADENCE_NOT_CONFIGURED、零件、0 次",
			outcome, items, rig.owners.askedSoFar()-asked)
	}

	register := func(body string, wantStatus int, wantOutcome string) {
		t.Helper()
		status, answer := serveReaskOperator(t, rig.cadences, http.MethodPost, "/transport-fulfillment-delivery-dispatch-reask-cadence-registrations", body)
		if status != wantStatus || reaskOutcomeOf(t, answer) != wantOutcome {
			t.Fatalf("登记 %s 答 %d %v, want %d %s", body, status, answer, wantStatus, wantOutcome)
		}
	}
	register(`{"version":"SYN-REASK/v1","intervalMinutes":60}`, http.StatusCreated, "REASK_CADENCE_REGISTERED")
	register(`{"version":"SYN-REASK/v1","intervalMinutes":60}`, http.StatusOK, "EXISTING_VERSION")
	register(`{"version":"SYN-REASK/v1","intervalMinutes":30}`, http.StatusOK, "CONTENT_CONFLICT")
	register(`{"version":"SYN-REASK/v0","intervalMinutes":0}`, http.StatusOK, "INPUT_NOT_ACCEPTED")

	outcome, items, answer := rig.reaskWith(t, `{"scope":"DUE","limit":10}`)
	var version string
	_ = json.Unmarshal(answer["cadenceVersion"], &version)
	if outcome != "PENDING_DELIVERY_DISPATCHES_REASKED" || version != "SYN-REASK/v1" || len(items) != 1 ||
		items[0].Object != "SYN-PARCEL-70-C" || items[0].Outcome != "REQUIREMENT_MISSING" {
		t.Fatalf("登记之后到期重问 = %s 凭 %q %+v, want 凭 SYN-REASK/v1 问到 SYN-PARCEL-70-C、仍缺", outcome, version, items)
	}
	if _, has := answer["dueBy"]; !has {
		t.Fatalf("到期重问答复没带到期时刻：%v", answer)
	}

	register(`{"version":"SYN-REASK/v2","intervalMinutes":5256000}`, http.StatusCreated, "REASK_CADENCE_REGISTERED")
	if _, items, _ := rig.reaskWith(t, `{"scope":"DUE","limit":10}`); len(items) != 0 {
		t.Fatalf("换成十年的间隔后仍问了 %+v——最近登记的那一版该生效", items)
	}

	status, listed := serveReaskOperator(t, rig.records, http.MethodGet, "/transport-fulfillment-records?registry=delivery-dispatch-reask-cadence", "")
	var cadences []struct {
		Version         string `json:"version"`
		IntervalMinutes int64  `json:"intervalMinutes"`
	}
	if err := json.Unmarshal(listed["cadences"], &cadences); err != nil || status != http.StatusOK {
		t.Fatalf("间隔那一格答 %d %v", status, listed)
	}
	if len(cadences) != 2 || cadences[0].Version != "SYN-REASK/v2" || cadences[1].IntervalMinutes != 60 {
		t.Fatalf("间隔那一格列出 %+v, want v2 在前、v1 六十分钟", cadences)
	}
}

// Covers: ADR-0230 决定四——按册重问挂运营决定「派送重问」：持别种授予的操作者答 403 OPERATOR_NOT_GRANTED，不进编排。
func TestTheReaskFaceNeedsTheDeliveryDispatchReaskGrant(t *testing.T) {
	minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: []accessidentity.DecisionKind{accessidentity.DecisionDispatchTaskRegistration}}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := tfhttp.NewReaskDeliveryDispatchesEndpoint(decisions.transport, unwiredDeliveryDispatchReask{})
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-delivery-dispatch-reasks", strings.NewReader(`{"scope":"ALL","limit":1}`))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || problemCode(t, recorder) != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("只持建派送任务授予：答 %d %s, want 403 OPERATOR_NOT_GRANTED", recorder.Code, recorder.Body.String())
	}
}

// placeUnreadableFor 让收件地点那一缝对指名的几件读不动、其余照替身所有者答：扮演「队头那几件一直答未决」。
type placeUnreadableFor struct {
	owners  *taskReferenceOwners
	objects map[string]bool
}

func (place placeUnreadableFor) LoadDeliveryPlace(
	ctx context.Context, tenant tfdomain.TenantID, object tfdomain.CarriedObjectReference,
) (string, tfports.RequirementResolution, error) {
	if place.objects[object.String()] {
		return "", tfports.RequirementResolutionInvalid, errors.New("parcel shipment 读面不可用")
	}
	return place.owners.LoadDeliveryPlace(ctx, tenant, object)
}

// Covers: AT-TF-292（ADR-0230 决定四）——队头两件一直答未决、件数上限是 2：第一次按册重问只问到它们、答复说还剩 1 件；第二次先问到
// 后面那一件并形成任务，不被队头卡住。
func TestUndecidedEntriesDoNotStarveTheRestOfTheReaskQueue(t *testing.T) {
	rig := newReaskRig(t)
	rig.owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = true })
	for index, object := range []string{"SYN-PARCEL-70-D", "SYN-PARCEL-70-E", "SYN-PARCEL-70-F"} {
		action := tfdomain.SegmentServiceActionUndeclared
		if index == 0 {
			action = tfdomain.SegmentServesFinalDelivery
		}
		rig.enter(t, object, reaskSegment, "SYN-HANDOVER-SCOPE-70", "SYN-HANDOVER-RESULT/"+object+"/v1", action)
		if status, view, raw := rig.tick(t, reaskSegment, object, time.Date(2026, 1, 2, 9, index, 0, 0, time.UTC).Format(time.RFC3339)); status != http.StatusOK ||
			view.Outcome != "REQUIREMENT_MISSING" {
			t.Fatalf("补拍 %s 答 %d %s", object, status, raw)
		}
	}
	rig.owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = false })
	trigger, err := deliveryDispatchTriggerOver(rig.db,
		placeUnreadableFor{owners: rig.owners, objects: map[string]bool{"SYN-PARCEL-70-D": true, "SYN-PARCEL-70-E": true}},
		rig.owners, rig.owners)
	if err != nil {
		t.Fatal(err)
	}
	reask, err := deliveryDispatchReaskOver(rig.db, trigger)
	if err != nil {
		t.Fatal(err)
	}
	rig.reask = tfhttp.NewReaskDeliveryDispatchesEndpoint(rig.decisions, reask)

	_, items, answer := rig.reaskWith(t, `{"scope":"ALL","limit":2}`)
	if len(items) != 2 || items[0].Outcome != "DISPATCH_UNDECIDED" || items[1].Outcome != "DISPATCH_UNDECIDED" || string(answer["remaining"]) != "1" {
		t.Fatalf("第一次重问 = %+v remaining=%s, want 队头两件未决、还剩 1", items, answer["remaining"])
	}
	_, items, _ = rig.reaskWith(t, `{"scope":"ALL","limit":2}`)
	if len(items) == 0 || items[0].Object != "SYN-PARCEL-70-F" || items[0].Outcome != "DISPATCH_TASK_FORMED" {
		t.Fatalf("第二次重问 = %+v, want 先问到 SYN-PARCEL-70-F 并形成", items)
	}
	if rows := rig.taskRows(t); rows != 1 {
		t.Fatalf("任务 = %d 项, want 1", rows)
	}
}

// failingFor 包着真执行器：对指名的那一件不交答复、只交错误，其余照真执行器答。扮演「册上某一件这一拍给不出答复」。
type failingFor struct {
	inner  tfapp.DeliveryDispatchTrigger
	object string
}

func (trigger failingFor) Trigger(ctx context.Context, command tfapp.TriggerDeliveryDispatchCommand) (tfapp.TriggerDeliveryDispatchResult, error) {
	if command.Object == trigger.object {
		return tfapp.TriggerDeliveryDispatchResult{}, errors.New("SYN 执行器给不出答复")
	}
	return trigger.inner.Trigger(ctx, command)
}

// Covers: AT-TF-293（ADR-0230 决定四）——按册重问问到某一件给不出答复：整口答 500 没形成答案，停在那一件；排在它前面的那一件已各自
// 提交——任务落了、册上了结；它自己与后面的仍待形成。再按一次从册上仍待形成的接着问。
func TestAReaskThatHitsAnUnanswerableEntryKeepsWhatItAlreadyCommitted(t *testing.T) {
	rig := newReaskRig(t)
	rig.owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = true })
	for index, object := range []string{"SYN-PARCEL-70-H", "SYN-PARCEL-70-I", "SYN-PARCEL-70-J"} {
		action := tfdomain.SegmentServiceActionUndeclared
		if index == 0 {
			action = tfdomain.SegmentServesFinalDelivery
		}
		rig.enter(t, object, reaskSegment, "SYN-HANDOVER-SCOPE-70", "SYN-HANDOVER-RESULT/"+object+"/v1", action)
		if status, view, raw := rig.tick(t, reaskSegment, object, time.Date(2026, 1, 2, 9, index, 0, 0, time.UTC).Format(time.RFC3339)); status != http.StatusOK ||
			view.Outcome != "REQUIREMENT_MISSING" {
			t.Fatalf("补拍 %s 答 %d %s", object, status, raw)
		}
	}
	rig.owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = false })
	trigger, err := deliveryDispatchTriggerOver(rig.db, rig.owners, rig.owners, rig.owners)
	if err != nil {
		t.Fatal(err)
	}
	reask, err := deliveryDispatchReaskOver(rig.db, failingFor{inner: trigger, object: "SYN-PARCEL-70-I"})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := tfhttp.NewReaskDeliveryDispatchesEndpoint(rig.decisions, reask)
	status, answer := serveReaskOperator(t, endpoint, http.MethodPost, "/transport-fulfillment-delivery-dispatch-reasks", `{"scope":"ALL","limit":10}`)
	if status != http.StatusInternalServerError {
		t.Fatalf("问到给不出答复的那一件：答 %d %v, want 500", status, answer)
	}
	if _, has := answer["outcome"]; has {
		t.Fatalf("没形成答案却带了 outcome：%v", answer)
	}
	if rows := rig.taskRows(t); rows != 1 {
		t.Fatalf("任务 = %d 项, want 1——排在前面的那一件已提交", rows)
	}
	if listed := rig.pendingListed(t); strings.Join(listed, ";") !=
		reaskSegment+"/SYN-PARCEL-70-I/DELIVERY_PLACE;"+reaskSegment+"/SYN-PARCEL-70-J/DELIVERY_PLACE" {
		t.Fatalf("查阅页列出 %v, want 给不出答复的那一件与后面那一件仍待形成", listed)
	}

	_, items, _ := rig.reaskWith(t, `{"scope":"ALL","limit":10}`)
	if len(items) != 2 || items[0].Outcome != "DISPATCH_TASK_FORMED" || items[1].Outcome != "DISPATCH_TASK_FORMED" {
		t.Fatalf("换回真执行器再按一次 = %+v, want 两件都形成", items)
	}
}
