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

// ---- 应用编排的端口替身（真实 handler，非结果构造）----

type pickupAttemptStoreDouble struct {
	records map[string]ports.PickupAttemptRecord
	findErr error
}

func newPickupAttemptStore() *pickupAttemptStoreDouble {
	return &pickupAttemptStoreDouble{records: map[string]ports.PickupAttemptRecord{}}
}

func (double *pickupAttemptStoreDouble) FindByKey(
	_ context.Context,
	key ports.PickupAttemptKey,
) (ports.PickupAttemptRecord, bool, error) {
	if double.findErr != nil {
		return ports.PickupAttemptRecord{}, false, double.findErr
	}
	record, found := double.records[key.TenantID.String()+"|"+key.SourceID]
	return record, found, nil
}

func (double *pickupAttemptStoreDouble) Save(
	_ context.Context,
	record ports.PickupAttemptRecord,
) (ports.PickupSaveOutcome, error) {
	storeKey := record.Key.TenantID.String() + "|" + record.Key.SourceID
	if _, exists := double.records[storeKey]; exists {
		return ports.PickupAlreadyRecorded, nil
	}
	double.records[storeKey] = record
	return ports.PickupSaved, nil
}

type pickupHandoffDouble struct{}

func (pickupHandoffDouble) HandOffOffsitePickup(context.Context, ports.OffsitePickupHandoffIntent) error {
	return nil
}

// ---- 请求体与 intake 替身 ----

type pickupObjectBody struct {
	Object         string `json:"object"`
	Outcome        string `json:"outcome"`
	Basis          string `json:"basis"`
	Control        string `json:"control"`
	OccurredAt     string `json:"occurredAt"`
	PlannedSegment string `json:"plannedSegment"`
}

type pickupAttemptBody struct {
	SourceID        string             `json:"sourceId"`
	Task            string             `json:"task"`
	Attempt         string             `json:"attempt"`
	ExecutedBy      string             `json:"executedBy"`
	Place           string             `json:"place"`
	PlannedFrom     string             `json:"plannedFrom"`
	PlannedTo       string             `json:"plannedTo"`
	ArrivedAt       string             `json:"arrivedAt"`
	Evidence        string             `json:"evidence"`
	RescheduledFrom string             `json:"rescheduledFrom"`
	Segment         string             `json:"segment"`
	Objects         []pickupObjectBody `json:"objects"`
}

func attemptObjectOutcomeOf(raw string) domain.AttemptObjectOutcome {
	switch raw {
	case "PICKED_UP":
		return domain.ObjectPickedUp
	case "CUSTOMER_ABSENT":
		return domain.CustomerAbsent
	case "GOODS_NOT_READY":
		return domain.GoodsNotReady
	case "PACKAGING_UNACCEPTABLE":
		return domain.PackagingUnacceptable
	default:
		return domain.AttemptObjectOutcomeInvalid
	}
}

type pickupAttemptIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func parseRFC3339(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func (intake *pickupAttemptIntakeDouble) IntakePickupAttempt(
	_ context.Context,
	request *http.Request,
) (application.PerformOffsitePickupCommand, error) {
	if intake.err != nil {
		return application.PerformOffsitePickupCommand{}, intake.err
	}
	var body pickupAttemptBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.PerformOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	command := application.PerformOffsitePickupCommand{
		TenantID:        intake.tenant,
		SourceID:        body.SourceID,
		Task:            body.Task,
		Attempt:         body.Attempt,
		ExecutedBy:      body.ExecutedBy,
		Place:           body.Place,
		Evidence:        body.Evidence,
		RescheduledFrom: body.RescheduledFrom,
		Segment:         body.Segment,
	}
	var err error
	if command.PlannedFrom, err = parseRFC3339(body.PlannedFrom); err != nil {
		return application.PerformOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	if command.PlannedTo, err = parseRFC3339(body.PlannedTo); err != nil {
		return application.PerformOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	if command.ArrivedAt, err = parseRFC3339(body.ArrivedAt); err != nil {
		return application.PerformOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	for _, object := range body.Objects {
		submission := application.ObjectPickupSubmission{
			Outcome:        attemptObjectOutcomeOf(object.Outcome),
			PlannedSegment: object.PlannedSegment,
		}
		if submission.Object, err = domain.NewCarriedObjectReference(object.Object); err != nil {
			return application.PerformOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
		}
		if object.Basis != "" {
			if submission.Basis, err = domain.NewAttemptResultBasisReference(object.Basis); err != nil {
				return application.PerformOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
			}
		}
		if object.Control != "" {
			if submission.Control, err = domain.NewTransportControlReference(object.Control); err != nil {
				return application.PerformOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
			}
		}
		if submission.OccurredAt, err = parseRFC3339(object.OccurredAt); err != nil {
			return application.PerformOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
		}
		command.Objects = append(command.Objects, submission)
	}
	return command, nil
}

type pickupAttemptFixture struct {
	intake   *pickupAttemptIntakeDouble
	store    *pickupAttemptStoreDouble
	segments *segmentRegistryDouble
	perform  http.Handler
}

func newPickupAttemptFixture(t *testing.T) *pickupAttemptFixture {
	t.Helper()
	fixture := &pickupAttemptFixture{
		intake:   &pickupAttemptIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		store:    newPickupAttemptStore(),
		segments: newSegmentRegistry(),
	}
	handler := application.NewPerformOffsitePickupHandler(application.PerformOffsitePickupDeps{
		Attempts:   fixture.store,
		Segments:   fixture.segments,
		Versions:   &pickupVersionFactory{},
		Downstream: pickupHandoffDouble{},
		Clock:      tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.perform = tfhttp.NewPerformOffsitePickupEndpoint(fixture.intake, handler)
	return fixture
}

func pickedUp(object, control, plannedSegment string) pickupObjectBody {
	return pickupObjectBody{Object: object, Outcome: "PICKED_UP", Control: control, OccurredAt: "2026-09-05T10:05:00Z", PlannedSegment: plannedSegment}
}

func customerAbsent(object string) pickupObjectBody {
	return pickupObjectBody{Object: object, Outcome: "CUSTOMER_ABSENT", Basis: "reason-absent-1", OccurredAt: "2026-09-05T10:06:00Z"}
}

func defaultPickupAttemptBody(objects ...pickupObjectBody) pickupAttemptBody {
	return pickupAttemptBody{
		SourceID:    "source-1",
		Task:        "pickup-task-1",
		Attempt:     "attempt-1",
		ExecutedBy:  "courier-1",
		Place:       "customer-warehouse-1",
		PlannedFrom: "2026-09-05T09:00:00Z",
		PlannedTo:   "2026-09-05T12:00:00Z",
		ArrivedAt:   "2026-09-05T10:00:00Z",
		Evidence:    "attempt-evidence-1",
		Objects:     objects,
	}
}

type pickupAttemptObjectView struct {
	Object        string `json:"object"`
	Outcome       string `json:"outcome"`
	PickupVersion string `json:"pickupVersion"`
}

type segmentEntryView struct {
	Object                string `json:"object"`
	ContinuationReference string `json:"continuationReference"`
}

type pickupAttemptView struct {
	Outcome               string                    `json:"outcome"`
	UndecidedReason       string                    `json:"undecidedReason"`
	Attempt               string                    `json:"attempt"`
	Task                  string                    `json:"task"`
	Objects               []pickupAttemptObjectView `json:"objects"`
	ContinuationReference string                    `json:"continuationReference"`
	HandoffReference      string                    `json:"handoffReference"`
	SegmentEntries        []segmentEntryView        `json:"segmentEntries"`
	SegmentEntryRefusal   string                    `json:"segmentEntryRefusal"`
}

func decodePickupAttempt(t *testing.T, recorder *httptest.ResponseRecorder) pickupAttemptView {
	t.Helper()
	var view pickupAttemptView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: 票 04 专钉第 1 条在多对象口——**两层段引用都要到编排**：整次到访的 `segment` 与逐对象的
// `plannedSegment`。两成一败：成功对象各带自己的计划段进同一个段，失败对象不进段（CONTEXT「失败结果
// 不制造实际履约段」）；响应逐对象透出成败与版本（UC-TF-002：任务汇总只能由对象结果派生）。
func TestPerformOffsitePickupEntersEachPickedUpObjectWithItsOwnPlannedSegment(t *testing.T) {
	fixture := newPickupAttemptFixture(t)
	body := defaultPickupAttemptBody(
		pickedUp("parcel-1", "TRANSPORT-CONTROL/TF-1", "planned-1"),
		pickedUp("parcel-2", "TRANSPORT-CONTROL/TF-2", "planned-2"),
		customerAbsent("parcel-3"),
	)
	body.Segment = "segment-1"

	response := postTo(t, fixture.perform, "/transport-fulfillment/offsite-pickup-attempts", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodePickupAttempt(t, response)
	if view.Outcome != "ATTEMPT_RECORDED" || view.Attempt != "attempt-1" || view.Task != "pickup-task-1" {
		t.Fatalf("view = %+v", view)
	}
	if len(view.SegmentEntries) != 0 {
		t.Fatalf("segmentEntries = %+v，进段成功不该有欠账", view.SegmentEntries)
	}
	outcomes := map[string]pickupAttemptObjectView{}
	for _, object := range view.Objects {
		outcomes[object.Object] = object
	}
	if outcomes["parcel-1"].Outcome != "PICKED_UP" || outcomes["parcel-1"].PickupVersion == "" {
		t.Fatalf("parcel-1 = %+v", outcomes["parcel-1"])
	}
	if outcomes["parcel-3"].Outcome != "CUSTOMER_ABSENT" || outcomes["parcel-3"].PickupVersion != "" {
		t.Fatalf("parcel-3 = %+v（失败对象不该有揽收版本）", outcomes["parcel-3"])
	}

	for object, planned := range map[string]string{"parcel-1": "planned-1", "parcel-2": "planned-2"} {
		participation := fixture.segments.participationOf(t, "tenant-1", "segment-1", object)
		got, present := participation.PlannedSegment()
		if !present || got.String() != planned {
			t.Fatalf("%s plannedSegment = (%q, %v), want %q", object, got.String(), present, planned)
		}
	}
	record, found, err := fixture.segments.FindByKey(context.Background(), ports.FulfillmentSegmentKey{
		TenantID: httpValue(t, domain.NewTenantID, "tenant-1"),
		Segment:  httpValue(t, domain.NewFulfillmentSegmentReference, "segment-1"),
	})
	if err != nil || !found {
		t.Fatalf("segment: found=%v err=%v", found, err)
	}
	for _, participation := range record.Segment.Participations() {
		if participation.Object().String() == "parcel-3" {
			t.Fatal("失败对象 parcel-3 进了段")
		}
	}
}

// Covers: 票 04 专钉第 2 条在多对象口——段登记册故障时到访照登（201），`segmentEntries` **逐对象**列出
// 欠账，且只列成功对象：失败对象本来就不进段，不算欠账。
func TestPerformOffsitePickupExposesPerObjectSegmentDebt(t *testing.T) {
	fixture := newPickupAttemptFixture(t)
	fixture.segments.findErr = errors.New("segment registry down")
	body := defaultPickupAttemptBody(
		pickedUp("parcel-1", "TRANSPORT-CONTROL/TF-1", ""),
		pickedUp("parcel-2", "TRANSPORT-CONTROL/TF-2", ""),
		customerAbsent("parcel-3"),
	)
	body.Segment = "segment-1"

	response := postTo(t, fixture.perform, "/transport-fulfillment/offsite-pickup-attempts", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodePickupAttempt(t, response)
	owed := map[string]bool{}
	for _, entry := range view.SegmentEntries {
		if entry.ContinuationReference == "" {
			t.Fatalf("entry %+v 没有 continuationReference", entry)
		}
		owed[entry.Object] = true
	}
	if !owed["parcel-1"] || !owed["parcel-2"] || owed["parcel-3"] || len(owed) != 2 {
		t.Fatalf("segmentEntries = %+v，应恰为两个成功对象", view.SegmentEntries)
	}
}

// Covers: ADR-0022——重放、冲突、未受理、未决都是 200 的形成答案。
func TestPerformOffsitePickupReportsOKForEveryAnswerThatRecordedNothing(t *testing.T) {
	cases := map[string]struct {
		arrange    func(*pickupAttemptFixture) pickupAttemptBody
		posts      int
		want       string
		wantReason string
	}{
		"replay returns the existing result": {
			arrange: func(*pickupAttemptFixture) pickupAttemptBody {
				return defaultPickupAttemptBody(pickedUp("parcel-1", "TRANSPORT-CONTROL/TF-1", ""))
			},
			posts: 2,
			want:  "EXISTING_RESULT",
		},
		"different content under the same source is a conflict": {
			arrange: func(fixture *pickupAttemptFixture) pickupAttemptBody {
				seed := defaultPickupAttemptBody(pickedUp("parcel-1", "TRANSPORT-CONTROL/TF-1", ""))
				if response := postTo(t, fixture.perform, "/transport-fulfillment/offsite-pickup-attempts", seed); response.Code != http.StatusCreated {
					t.Fatalf("seed status = %d", response.Code)
				}
				return defaultPickupAttemptBody(pickedUp("parcel-1", "TRANSPORT-CONTROL/TF-9", ""))
			},
			posts: 1,
			want:  "SOURCE_CONFLICT",
		},
		"a visit without objects is not accepted": {
			arrange: func(*pickupAttemptFixture) pickupAttemptBody { return defaultPickupAttemptBody() },
			posts:   1,
			want:    "SOURCE_NOT_ACCEPTED",
		},
		"a store failure is undecided": {
			arrange: func(fixture *pickupAttemptFixture) pickupAttemptBody {
				fixture.store.findErr = errors.New("store down")
				return defaultPickupAttemptBody(pickedUp("parcel-1", "TRANSPORT-CONTROL/TF-1", ""))
			},
			posts:      1,
			want:       "PICKUP_UNDECIDED",
			wantReason: "PICKUP_STORE_UNAVAILABLE",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newPickupAttemptFixture(t)
			body := testCase.arrange(fixture)
			var response *httptest.ResponseRecorder
			for post := 0; post < testCase.posts; post++ {
				response = postTo(t, fixture.perform, "/transport-fulfillment/offsite-pickup-attempts", body)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
			}
			view := decodePickupAttempt(t, response)
			if view.Outcome != testCase.want || view.UndecidedReason != testCase.wantReason {
				t.Fatalf("view = %+v, want outcome %q reason %q", view, testCase.want, testCase.wantReason)
			}
			if testCase.wantReason != "" && view.ContinuationReference == "" {
				t.Fatal("未决没有带 continuationReference")
			}
		})
	}
}

func TestPerformOffsitePickupOrchestrationErrorFormsNoAnswer(t *testing.T) {
	fixture := newPickupAttemptFixture(t)
	endpoint := tfhttp.NewPerformOffsitePickupEndpoint(fixture.intake, failingPickupAttemptHandler{})

	response := postTo(t, endpoint, "/transport-fulfillment/offsite-pickup-attempts", defaultPickupAttemptBody(pickedUp("parcel-1", "TRANSPORT-CONTROL/TF-1", "")))

	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

type failingPickupAttemptHandler struct{}

func (failingPickupAttemptHandler) Handle(
	context.Context,
	application.PerformOffsitePickupCommand,
) (application.PerformOffsitePickupResult, error) {
	return application.PerformOffsitePickupResult{}, errors.New("orchestration exploded")
}
