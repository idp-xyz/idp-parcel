package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证待确认的逐对象尝试结果更正两口的传输面（票 docs-consistency/code/90，ADR-0297）：两个接入面（作业事实、运营决定）交进来同一条
// 命令，运营决定面各按自己那一种决定认证、把提交操作者作为操作证据带进去，作业事实面不带；载荷封闭；只有真落了更正才答 201，回执不回显
// 操作者。夹具全是合成串，证据只记 S。

const pickupAttemptResultCorrectionBody = `{"sourceId":"SYN-DEVICE-01/attempt-90","object":"SYN-PARCEL-90-01","outcome":"PICKED_UP",` +
	`"source":"SYN-SOURCE/partner-late-receipt","control":"SYN-CONTROL/receipt-90","occurredAt":"2026-09-24T08:42:00+08:00",` +
	`"correctedAt":"2026-09-24T14:00:00+08:00","segment":"SYN-SEGMENT-90","plannedSegment":"SYN-PLANNED-90","segmentServiceAction":"PICKUP"}`

const deliveryAttemptResultCorrectionBody = `{"attempt":"SYN-DELIVERY-ATTEMPT-90","object":"SYN-PARCEL-90-02","outcome":"DELIVERED",` +
	`"source":"SYN-SOURCE/address-review-confirmed","occurredAt":"2026-09-25T10:07:00+08:00","correctedAt":"2026-09-25T15:00:00+08:00"}`

// Covers: `AT-TF-361`（传输面）——同一条命令两个接入面——运营决定面认证用各自那一种决定、操作者随命令带进去；作业事实面租户取认证结果、操作证据留空。
func TestTheAttemptResultCorrectionFacesTranslateTheSameCommandUnderTheirOwnFace(t *testing.T) {
	var asked []tfhttp.OperatorDecision
	operators := operatorIntake(t, authenticatorFake{asked: &asked})

	pickup, err := operators.IntakePickupAttemptResultCorrection(context.Background(), bearerRequest("t", pickupAttemptResultCorrectionBody))
	if err != nil {
		t.Fatalf("pickup: %v", err)
	}
	if pickup.TenantID.String() != operatorTenant || pickup.DecidedBy != operatorSubject || pickup.SourceID != "SYN-DEVICE-01/attempt-90" ||
		pickup.Object != "SYN-PARCEL-90-01" || pickup.Outcome != domain.ObjectPickedUp || pickup.Source.String() != "SYN-SOURCE/partner-late-receipt" ||
		pickup.Control.String() != "SYN-CONTROL/receipt-90" || pickup.Segment != "SYN-SEGMENT-90" || pickup.PlannedSegment != "SYN-PLANNED-90" ||
		pickup.SegmentServiceAction != "PICKUP" ||
		!pickup.OccurredAt.Equal(time.Date(2026, 9, 24, 0, 42, 0, 0, time.UTC)) || !pickup.CorrectedAt.Equal(time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("pickup command = %+v", pickup)
	}
	delivery, err := operators.IntakeDeliveryAttemptResultCorrection(context.Background(), bearerRequest("t", deliveryAttemptResultCorrectionBody))
	if err != nil {
		t.Fatalf("delivery: %v", err)
	}
	if delivery.TenantID.String() != operatorTenant || delivery.DecidedBy != operatorSubject || delivery.Attempt != "SYN-DELIVERY-ATTEMPT-90" ||
		delivery.Object != "SYN-PARCEL-90-02" || delivery.Outcome != domain.ObjectDelivered {
		t.Fatalf("delivery command = %+v", delivery)
	}
	if len(asked) != 2 || asked[0] != tfhttp.OperatorDecisionPickupAttemptResultConfirmation ||
		asked[1] != tfhttp.OperatorDecisionDeliveryAttemptResultConfirmation {
		t.Fatalf("问认证方的决定种类 = %v；两口各按自己那一种，能认定揽收的不因此能认定派送", asked)
	}

	workFact := workFactIntakeForTest(t)
	fromDevice, err := workFact.IntakePickupAttemptResultCorrection(context.Background(), commandRequest(pickupAttemptResultCorrectionBody))
	if err != nil {
		t.Fatalf("work fact pickup: %v", err)
	}
	if fromDevice.TenantID.String() != workFactTenant || fromDevice.DecidedBy != "" {
		t.Fatalf("作业事实面 = tenant %q decidedBy %q；设备身份不进载荷，操作证据一格留空", fromDevice.TenantID, fromDevice.DecidedBy)
	}
	fromDeviceDelivery, err := workFact.IntakeDeliveryAttemptResultCorrection(context.Background(), commandRequest(deliveryAttemptResultCorrectionBody))
	if err != nil || fromDeviceDelivery.DecidedBy != "" {
		t.Fatalf("work fact delivery: %+v err=%v", fromDeviceDelivery, err)
	}
}

// Covers: `AT-TF-361`（传输面）——先认证、后解载荷；载荷封闭（夹带租户或操作者一律 400）；词表外的结果词、解不出的时刻是坏报文，更正成待确认的词也是（待确认
// 只在更正之前存在，词表本身收它，由编排答未受理——这里钉的是词表外）。
func TestTheAttemptResultCorrectionFacesAuthenticateFirstAndDecodeClosed(t *testing.T) {
	denied := operatorIntake(t, authenticatorFake{err: tfhttp.ErrOperatorNotGranted})
	if _, err := denied.IntakePickupAttemptResultCorrection(context.Background(), bearerRequest("t", `not json`)); !errors.Is(err, tfhttp.ErrOperatorNotGranted) {
		t.Fatalf("未授予的调用方看到了载荷校验的结果：err = %v", err)
	}
	if _, err := denied.IntakeDeliveryAttemptResultCorrection(context.Background(), bearerRequest("", deliveryAttemptResultCorrectionBody)); !errors.Is(err, tfhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("没带令牌：err = %v", err)
	}

	intake := operatorIntake(t, authenticatorFake{})
	for name, body := range map[string]string{
		"夹带租户":       strings.Replace(pickupAttemptResultCorrectionBody, `"object"`, `"tenantId":"SYN-TENANT-02","object"`, 1),
		"夹带操作者":      strings.Replace(pickupAttemptResultCorrectionBody, `"object"`, `"decidedBy":"someone","object"`, 1),
		"词表外的结果词":    strings.Replace(pickupAttemptResultCorrectionBody, `"PICKED_UP"`, `"MAYBE"`, 1),
		"解不出的时刻":     strings.Replace(pickupAttemptResultCorrectionBody, `"2026-09-24T14:00:00+08:00"`, `"yesterday"`, 1),
		"派送口收揽收的拒收词": strings.Replace(deliveryAttemptResultCorrectionBody, `"DELIVERED"`, `"EXECUTOR_REFUSED"`, 1),
	} {
		var err error
		if strings.Contains(body, "attempt\"") && !strings.Contains(body, "sourceId") {
			_, err = intake.IntakeDeliveryAttemptResultCorrection(context.Background(), bearerRequest("t", body))
		} else {
			_, err = intake.IntakePickupAttemptResultCorrection(context.Background(), bearerRequest("t", body))
		}
		if !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// --- 端点 ---

type correctionAttemptFixture struct {
	attempt domain.FulfillmentAttempt
}

func newCorrectionAttemptFixture(t *testing.T, attemptID string, objects ...string) correctionAttemptFixture {
	t.Helper()
	spec := domain.FulfillmentAttemptSpec{
		Attempt:     mustTF(t, domain.NewAttemptReference, attemptID),
		Task:        mustTF(t, domain.NewDispatchTaskReference, "SYN-TASK-90"),
		ExecutedBy:  mustTF(t, domain.NewExecutingPartyReference, "SYN-COURIER-90"),
		Place:       mustTF(t, domain.NewAttemptPlaceReference, "SYN-PLACE-90"),
		PlannedFrom: time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC),
		PlannedTo:   time.Date(2026, 9, 24, 4, 0, 0, 0, time.UTC),
		ArrivedAt:   time.Date(2026, 9, 24, 0, 30, 0, 0, time.UTC),
		Evidence:    mustTF(t, domain.NewAttemptEvidenceReference, "SYN-EVIDENCE-90"),
	}
	spec.TenantID = mustTF(t, domain.NewTenantID, operatorTenant)
	for _, object := range objects {
		spec.Objects = append(spec.Objects, mustTF(t, domain.NewCarriedObjectReference, object))
	}
	attempt, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		t.Fatalf("form attempt: %v", err)
	}
	return correctionAttemptFixture{attempt: attempt}
}

type httpPickupCorrections struct {
	target ports.PickupResultCorrectionTarget
	found  bool
	stored *ports.PickupResultCorrectionRecord
}

func (double *httpPickupCorrections) LoadForCorrection(context.Context, ports.PickupAttemptKey, domain.CarriedObjectReference) (ports.PickupResultCorrectionTarget, bool, error) {
	target := double.target
	if double.stored != nil {
		target.Correction, target.HasCorrection = *double.stored, true
	}
	return target, double.found, nil
}

func (double *httpPickupCorrections) Save(_ context.Context, record ports.PickupResultCorrectionRecord) (ports.PickupResultCorrectionSaveOutcome, error) {
	double.stored = &record
	return ports.PickupResultCorrectionSaved, nil
}

type httpDeliveryCorrections struct {
	target ports.DeliveryResultCorrectionTarget
	stored *ports.DeliveryResultCorrectionRecord
}

func (double *httpDeliveryCorrections) LoadForCorrection(context.Context, ports.DeliveryAttemptKey, domain.CarriedObjectReference) (ports.DeliveryResultCorrectionTarget, bool, error) {
	target := double.target
	if double.stored != nil {
		target.Correction, target.HasCorrection = *double.stored, true
	}
	return target, true, nil
}

func (double *httpDeliveryCorrections) Save(_ context.Context, record ports.DeliveryResultCorrectionRecord) (ports.DeliveryResultCorrectionSaveOutcome, error) {
	double.stored = &record
	return ports.DeliveryResultCorrectionSaved, nil
}

type correctionClock struct{}

func (correctionClock) Now() time.Time { return time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC) }

func answerOf(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("answer %q: %v", recorder.Body.String(), err)
	}
	return answer
}

// Covers: `AT-TF-357`、`AT-TF-358`、`AT-TF-360`、`AT-TF-361`（传输面）——ADR-0022——只有这次新落了更正才答 201，并回显更正出来的确定结果、所凭来源、更正时刻与被回指的待确认那一版的来源与缺口；
// 重放、冲突、并非待确认都是形成了的答案（200）；回执不回显操作者（ADR-0303 的待遇）。揽收一侧。
func TestThePickupCorrectionFaceAnswers201OnlyForANewCorrectionAndNeverEchoesTheOperator(t *testing.T) {
	fixture := newCorrectionAttemptFixture(t, "SYN-ATTEMPT-90", "SYN-PARCEL-90-01", "SYN-PARCEL-90-02")
	pending, err := domain.FormPendingAttemptObjectResult(fixture.attempt, mustTF(t, domain.NewCarriedObjectReference, "SYN-PARCEL-90-01"),
		mustTF(t, domain.NewAttemptResultSourceReference, "SYN-SOURCE/vehicle-arrival"),
		mustTF(t, domain.NewAttemptResultBasisReference, "SYN-GAP/no-receipt"), time.Date(2026, 9, 24, 0, 45, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	determinate, err := domain.FormAttemptObjectResult(fixture.attempt, mustTF(t, domain.NewCarriedObjectReference, "SYN-PARCEL-90-02"),
		domain.CustomerAbsent, mustTF(t, domain.NewAttemptResultBasisReference, "SYN-REASON/absent"), time.Date(2026, 9, 24, 0, 45, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	corrections := &httpPickupCorrections{found: true, target: ports.PickupResultCorrectionTarget{Attempt: fixture.attempt, Result: pending}}
	handler := application.NewCorrectPickupAttemptResultHandler(application.CorrectPickupAttemptResultDeps{
		Corrections: corrections, Clock: correctionClock{},
	})
	endpoint := tfhttp.NewCorrectPickupAttemptResultEndpoint(operatorIntake(t, authenticatorFake{}), handler)
	body := `{"sourceId":"SYN-DEVICE-01/attempt-90","object":"SYN-PARCEL-90-01","outcome":"CUSTOMER_ABSENT",` +
		`"basis":"SYN-REASON/partner-confirms-absent","source":"SYN-SOURCE/partner-late-status",` +
		`"occurredAt":"2026-09-24T08:42:00+08:00","correctedAt":"2026-09-24T14:00:00+08:00"}`

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", body))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("新更正 = %d %s", recorder.Code, recorder.Body.String())
	}
	answer := answerOf(t, recorder)
	correction, _ := answer["correction"].(map[string]any)
	pendingBody, _ := correction["pending"].(map[string]any)
	if answer["outcome"] != "RESULT_CORRECTED" || answer["attempt"] != "SYN-ATTEMPT-90" || answer["object"] != "SYN-PARCEL-90-01" ||
		correction["outcome"] != "CUSTOMER_ABSENT" || correction["basis"] != "SYN-REASON/partner-confirms-absent" ||
		correction["source"] != "SYN-SOURCE/partner-late-status" || correction["occurredAt"] != "2026-09-24T00:42:00Z" ||
		correction["correctedAt"] != "2026-09-24T06:00:00Z" ||
		pendingBody["source"] != "SYN-SOURCE/vehicle-arrival" || pendingBody["gap"] != "SYN-GAP/no-receipt" {
		t.Fatalf("回显 = %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), operatorSubject) || corrections.stored == nil || corrections.stored.DecidedBy.String() != operatorSubject {
		t.Fatalf("操作者应只记在更正上、不回显：stored=%+v body=%s", corrections.stored, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", body))
	if recorder.Code != http.StatusOK || answerOf(t, recorder)["outcome"] != "EXISTING_CORRECTION" {
		t.Fatalf("重放 = %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", strings.Replace(body, "CUSTOMER_ABSENT", "GOODS_NOT_READY", 1)))
	if recorder.Code != http.StatusOK || answerOf(t, recorder)["outcome"] != "SOURCE_CONFLICT" {
		t.Fatalf("换说法 = %d %s", recorder.Code, recorder.Body.String())
	}

	notPending := &httpPickupCorrections{found: true, target: ports.PickupResultCorrectionTarget{Attempt: fixture.attempt, Result: determinate}}
	other := tfhttp.NewCorrectPickupAttemptResultEndpoint(operatorIntake(t, authenticatorFake{}),
		application.NewCorrectPickupAttemptResultHandler(application.CorrectPickupAttemptResultDeps{Corrections: notPending, Clock: correctionClock{}}))
	recorder = httptest.NewRecorder()
	other.ServeHTTP(recorder, bearerRequest("t", strings.Replace(body, "SYN-PARCEL-90-01", "SYN-PARCEL-90-02", 1)))
	if recorder.Code != http.StatusOK || answerOf(t, recorder)["outcome"] != "RESULT_NOT_PENDING" {
		t.Fatalf("确定结果 = %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", recorder.Code)
	}
}

// Covers: `AT-TF-359`、`AT-TF-360`（传输面）——派送一侧同上——更正成妥投答 201，重放 200，换说法 SOURCE_CONFLICT；回执不回显操作者。
func TestTheDeliveryCorrectionFaceAnswers201OnlyForANewCorrection(t *testing.T) {
	fixture := newCorrectionAttemptFixture(t, "SYN-DELIVERY-ATTEMPT-90", "SYN-PARCEL-90-02")
	pending, err := domain.FormPendingDeliveryAttemptResult(fixture.attempt, mustTF(t, domain.NewCarriedObjectReference, "SYN-PARCEL-90-02"),
		mustTF(t, domain.NewAttemptResultSourceReference, "SYN-SOURCE/signature-scan"),
		mustTF(t, domain.NewAttemptResultBasisReference, "SYN-CONFLICT/signature-vs-address"), time.Date(2026, 9, 24, 0, 45, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	corrections := &httpDeliveryCorrections{target: ports.DeliveryResultCorrectionTarget{Attempt: fixture.attempt, Result: pending}}
	endpoint := tfhttp.NewCorrectDeliveryAttemptResultEndpoint(operatorIntake(t, authenticatorFake{}),
		application.NewCorrectDeliveryAttemptResultHandler(application.CorrectDeliveryAttemptResultDeps{Corrections: corrections, Clock: correctionClock{}}))
	body := `{"attempt":"SYN-DELIVERY-ATTEMPT-90","object":"SYN-PARCEL-90-02","outcome":"DELIVERED",` +
		`"source":"SYN-SOURCE/address-review-confirmed","occurredAt":"2026-09-24T08:46:00+08:00","correctedAt":"2026-09-24T14:00:00+08:00"}`

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", body))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("新更正 = %d %s", recorder.Code, recorder.Body.String())
	}
	answer := answerOf(t, recorder)
	correction, _ := answer["correction"].(map[string]any)
	pendingBody, _ := correction["pending"].(map[string]any)
	if answer["outcome"] != "RESULT_CORRECTED" || answer["attempt"] != "SYN-DELIVERY-ATTEMPT-90" || correction["outcome"] != "DELIVERED" ||
		correction["source"] != "SYN-SOURCE/address-review-confirmed" || pendingBody["gap"] != "SYN-CONFLICT/signature-vs-address" ||
		strings.Contains(recorder.Body.String(), operatorSubject) {
		t.Fatalf("回显 = %s", recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", body))
	if recorder.Code != http.StatusOK || answerOf(t, recorder)["outcome"] != "EXISTING_CORRECTION" {
		t.Fatalf("重放 = %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, bearerRequest("t", strings.Replace(body, `"DELIVERED"`, `"WRONG_ADDRESS","basis":"SYN-REASON/wrong"`, 1)))
	if recorder.Code != http.StatusOK || answerOf(t, recorder)["outcome"] != "SOURCE_CONFLICT" {
		t.Fatalf("换说法 = %d %s", recorder.Code, recorder.Body.String())
	}
}

// Covers: `AT-TF-361`（传输面）——ADR-0055——接入渠道未配置时两口的两个接入面都如实答 403 ACCESS_CHANNEL_NOT_CONFIGURED，不读载荷、不构造命令。
func TestTheAttemptResultCorrectionFacesAnswerUnconfiguredWhenNoChannelIsWired(t *testing.T) {
	pickupEndpoint := tfhttp.NewCorrectPickupAttemptResultEndpoint(tfhttp.UnconfiguredIntake{}, unreachablePickupCorrector{})
	deliveryEndpoint := tfhttp.NewCorrectDeliveryAttemptResultEndpoint(tfhttp.UnconfiguredIntake{}, unreachableDeliveryCorrector{})
	for _, endpoint := range []http.Handler{pickupEndpoint, deliveryEndpoint} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, bearerRequest("t", `{}`))
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") {
			t.Fatalf("未配置 = %d %s", recorder.Code, recorder.Body.String())
		}
	}
}

type unreachablePickupCorrector struct{}

func (unreachablePickupCorrector) Correct(context.Context, application.CorrectPickupAttemptResultCommand) (application.CorrectPickupAttemptResultResult, error) {
	return application.CorrectPickupAttemptResultResult{}, errors.New("unreachable")
}

type unreachableDeliveryCorrector struct{}

func (unreachableDeliveryCorrector) Correct(context.Context, application.CorrectDeliveryAttemptResultCommand) (application.CorrectDeliveryAttemptResultResult, error) {
	return application.CorrectDeliveryAttemptResultResult{}, errors.New("unreachable")
}
