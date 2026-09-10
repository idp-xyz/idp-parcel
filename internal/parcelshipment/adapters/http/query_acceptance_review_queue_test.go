package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 复核队列端点的传输层测试（票 acceptance-review-read-face/01）。分派、方法门与错误
// 分流照 /shipment-request-views 的形，这里另证三样本端点特有的：队列行的复核留痕
// 转写、详情连判断三组一并作答、判断读不回不砍半作答。

func reviewQueueRecord(t *testing.T, requestID string) ports.AcceptanceReviewQueueRecord {
	t.Helper()
	return ports.AcceptanceReviewQueueRecord{
		ShipmentRequestSummaryRecord: viewSummaryRecord(t, requestID),
		HasAttempt:                   true,
		LastAttemptReason:            "COMMERCIAL_BASIS_UNAVAILABLE",
		LastAttemptContinuation:      "CONT-1",
		LastAttemptedAt:              viewSubmittedAt.Add(30 * time.Minute),
		ReviewCompleted:              true,
		ReviewAuthority:              "AUTH-RULE-9",
		ReviewReviewer:               "REVIEWER-OP-7",
		ReviewEvidence:               "EVIDENCE-42",
		ReviewCompletedAt:            viewSubmittedAt.Add(3 * time.Hour),
	}
}

func reviewCaseDetail(t *testing.T) ports.ShipmentRequestDetailRecord {
	t.Helper()
	detail := viewDetailRecord(t)
	// 复核语境：任务运行中、停在等待人工复核、复核已录完成待续办，决定尚未形成。
	detail.State = domain.ShipmentRequestSubmitted
	detail.HasDecision = false
	detail.Decision = ports.AcceptanceDecisionViewRecord{}
	detail.Task.State = domain.AcceptanceTaskRunning
	detail.Task.WaitingOn = domain.ResumeByManualReview
	detail.Task.ReviewCompleted = true
	detail.Task.ReviewAuthority = "AUTH-RULE-9"
	detail.Task.ReviewReviewer = "REVIEWER-OP-7"
	detail.Task.ReviewEvidence = "EVIDENCE-42"
	detail.Task.ReviewCompletedAt = viewSubmittedAt.Add(3 * time.Hour)
	return detail
}

func recordedJudgmentsFixture(t *testing.T) ports.RecordedJudgments {
	t.Helper()
	echoed, err := domain.NewEchoedAsOfPolicy(
		domain.ReachabilityJudgmentKind,
		mustValue(t, domain.NewAsOfSemanticsReference, "ACCEPTANCE"),
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-1"),
	)
	if err != nil {
		t.Fatalf("回显时点政策：%v", err)
	}
	reachAsOf, err := domain.NewJudgmentAsOf(viewSubmittedAt.Add(90*time.Minute), echoed)
	if err != nil {
		t.Fatalf("可达性时点：%v", err)
	}
	reachable, err := domain.NewReachabilityJudgment(domain.ReachabilityJudgmentSpec{
		JudgmentID: mustValue(t, domain.NewReachabilityJudgmentID, "REACH-1"),
		ParcelID:   mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Value:      domain.ReachabilityReachable,
		AsOf:       reachAsOf,
	})
	if err != nil {
		t.Fatalf("可达性判断：%v", err)
	}
	notApplicable, err := domain.NewReachabilityJudgment(domain.ReachabilityJudgmentSpec{
		ParcelID: mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
		Value:    domain.ReachabilityNotApplicable,
		Basis:    mustValue(t, domain.NewReachabilityBasisReference, "NO-NETWORK-DUTY-7"),
		AsOf:     reachAsOf,
	})
	if err != nil {
		t.Fatalf("不适用判断：%v", err)
	}

	controlEchoed, err := domain.NewEchoedAsOfPolicy(
		domain.FinancialControlJudgmentKind,
		mustValue(t, domain.NewAsOfSemanticsReference, "ACCEPTANCE"),
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-1"),
	)
	if err != nil {
		t.Fatalf("控制回显政策：%v", err)
	}
	controlAsOf, err := domain.NewJudgmentAsOf(viewSubmittedAt.Add(95*time.Minute), controlEchoed)
	if err != nil {
		t.Fatalf("控制时点：%v", err)
	}
	// 组合策略：冻结成立、信用受限——结论 RESTRICTED，逐项两项都要透出（ADR-0125）。
	freezeItem, err := domain.NewControlItemResult(
		domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied, domain.ControlBasisReference{})
	if err != nil {
		t.Fatalf("冻结项：%v", err)
	}
	creditItem, err := domain.NewControlItemResult(
		domain.CreditCheckControlItem, 2, domain.ControlItemRestricted,
		mustValue(t, domain.NewControlBasisReference, "AVAILABLE_CREDIT_INSUFFICIENT"))
	if err != nil {
		t.Fatalf("信用项：%v", err)
	}
	control, err := domain.NewExecutedFinancialControlResult(domain.ExecutedFinancialControlSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, "CTRL-1"),
		Items:     []domain.ControlItemResult{freezeItem, creditItem},
		JointPass: domain.AllControlsPass,
		AsOf:      controlAsOf,
	})
	if err != nil {
		t.Fatalf("财务控制结果：%v", err)
	}
	// 受限项采用了正文登记的失败处置与责任引用（ADR-0132 决定三）：读面要把两格照实透出，成立项不带。
	adopted, err := domain.NewAdoptedControlDisposition(
		domain.AuthorizedDispositionOnControlFailure,
		mustValue(t, domain.NewControlResponsibilityReference, "CONTRACT-CLAUSE-7"))
	if err != nil {
		t.Fatalf("采用引用：%v", err)
	}
	control, err = control.AdoptControlDispositions(map[domain.ControlItemKind]domain.AdoptedControlDisposition{
		domain.CreditCheckControlItem: adopted,
	})
	if err != nil {
		t.Fatalf("采用处置：%v", err)
	}

	return ports.RecordedJudgments{
		Reachability:                []domain.ReachabilityJudgment{reachable, notApplicable},
		FinancialControl:            control,
		AdoptedCommercialResolution: mustValue(t, domain.NewCommercialResolutionID, "RES-1"),
	}
}

type reviewQueueReaderDouble struct {
	rows    []ports.AcceptanceReviewQueueRecord
	listErr error
	detail  ports.ShipmentRequestDetailRecord
	found   bool
	findErr error
}

func (double *reviewQueueReaderDouble) ListAwaitingManualReview(
	_ context.Context,
	_ domain.AuthorizedQueryScope,
	_ int,
) ([]ports.AcceptanceReviewQueueRecord, error) {
	if double.listErr != nil {
		return nil, double.listErr
	}
	return double.rows, nil
}

func (double *reviewQueueReaderDouble) FindVisibleByID(
	_ context.Context,
	_ domain.AuthorizedQueryScope,
	_ domain.ShipmentRequestID,
) (ports.ShipmentRequestDetailRecord, bool, error) {
	if double.findErr != nil {
		return ports.ShipmentRequestDetailRecord{}, false, double.findErr
	}
	return double.detail, double.found, nil
}

type judgmentsReaderDouble struct {
	recorded ports.RecordedJudgments
	err      error
	// loadedVersion 收下详情分支读判断时给出的提交版本：复核看的必须是详情里那一版的判断。
	loadedVersion domain.SubmissionVersionID
}

func (double *judgmentsReaderDouble) LoadRecordedJudgments(
	_ context.Context,
	_ domain.TenantID,
	_ domain.ShipmentRequestID,
	version domain.SubmissionVersionID,
) (ports.RecordedJudgments, error) {
	double.loadedVersion = version
	if double.err != nil {
		return ports.RecordedJudgments{}, double.err
	}
	return double.recorded, nil
}

func getReviewQueue(
	t *testing.T,
	intake shipmenthttp.ShipmentRequestViewsIntake,
	reader shipmenthttp.AcceptanceReviewQueueReader,
	judgments shipmenthttp.RecordedJudgmentsReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewQueryAcceptanceReviewQueueEndpoint(intake, reader, judgments)
	request := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: 队列列表答案已形成即 200 + REVIEW_QUEUE_LISTED，行内容按读口记录逐字段透出
// （概要 + 最近处理记录 + 复核完成留痕）；空队列交回空数组——「暂无待复核」是答案。
func TestReviewQueueListTranscribesEntries(t *testing.T) {
	intake := &viewsIntakeDouble{scope: viewsScope(t)}
	bare := viewSummaryRecord(t, "REQ-2")
	response := getReviewQueue(t, intake,
		&reviewQueueReaderDouble{rows: []ports.AcceptanceReviewQueueRecord{
			reviewQueueRecord(t, "REQ-1"),
			{ShipmentRequestSummaryRecord: bare},
		}},
		&judgmentsReaderDouble{},
		http.MethodGet, "/acceptance-review-queue")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if !intake.listCalled || intake.detailCalled {
		t.Fatal("无标识参数的请求该走列表分支")
	}
	var body struct {
		Outcome string `json:"outcome"`
		Entries []struct {
			ShipmentRequestID       string `json:"shipmentRequestId"`
			CustomerAccountID       string `json:"customerAccountId"`
			State                   string `json:"state"`
			SubmittedAt             string `json:"submittedAt"`
			LastAttemptReason       string `json:"lastAttemptReason"`
			LastAttemptContinuation string `json:"lastAttemptContinuation"`
			LastAttemptedAt         string `json:"lastAttemptedAt"`
			ReviewCompleted         bool   `json:"reviewCompleted"`
			ReviewAuthority         string `json:"reviewAuthority"`
			ReviewReviewer          string `json:"reviewReviewer"`
			ReviewEvidence          string `json:"reviewEvidence"`
			ReviewCompletedAt       string `json:"reviewCompletedAt"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "REVIEW_QUEUE_LISTED" || len(body.Entries) != 2 {
		t.Fatalf("outcome = %q entries = %d", body.Outcome, len(body.Entries))
	}
	first := body.Entries[0]
	if first.ShipmentRequestID != "REQ-1" || first.CustomerAccountID != "CUST-1" ||
		first.State != "SUBMITTED" || first.SubmittedAt != "2026-09-06T11:00:00Z" {
		t.Fatalf("概要变形：%+v", first)
	}
	if first.LastAttemptReason != "COMMERCIAL_BASIS_UNAVAILABLE" ||
		first.LastAttemptContinuation != "CONT-1" ||
		first.LastAttemptedAt != "2026-09-06T11:30:00Z" {
		t.Fatalf("最近处理记录变形：%+v", first)
	}
	if !first.ReviewCompleted || first.ReviewAuthority != "AUTH-RULE-9" ||
		first.ReviewReviewer != "REVIEWER-OP-7" || first.ReviewEvidence != "EVIDENCE-42" ||
		first.ReviewCompletedAt != "2026-09-06T14:00:00Z" {
		t.Fatalf("复核留痕变形：%+v", first)
	}
	second := body.Entries[1]
	if second.ReviewCompleted || second.ReviewAuthority != "" || second.LastAttemptReason != "" {
		t.Fatalf("没录过的留痕长出来了：%+v", second)
	}

	empty := getReviewQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&reviewQueueReaderDouble{}, &judgmentsReaderDouble{},
		http.MethodGet, "/acceptance-review-queue")
	var emptyBody map[string]json.RawMessage
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if string(emptyBody["entries"]) != "[]" {
		t.Fatalf(`entries = %s, want []（空队列不是 null）`, emptyBody["entries"])
	}
}

// Covers: 详情分支连判断三组一并作答——委托详情整份复用委托查阅的形，review 块照任务
// 文档报「停在哪、复核录了没」，recordedJudgments 三组照登记转写（`不适用`带依据不带
// 标识、财务控制`已冻结`带标识不带依据、采用解析原样）。
func TestReviewCaseAnswersDetailWithJudgments(t *testing.T) {
	intake := &viewsIntakeDouble{scope: viewsScope(t)}
	judgments := &judgmentsReaderDouble{recorded: recordedJudgmentsFixture(t)}
	response := getReviewQueue(t, intake,
		&reviewQueueReaderDouble{detail: reviewCaseDetail(t), found: true},
		judgments,
		http.MethodGet, "/acceptance-review-queue?shipmentRequestId=REQ-1")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if !intake.detailCalled || intake.listCalled {
		t.Fatal("带标识参数的请求该走详情分支")
	}
	// 复核看的是详情里当前版本的判断：读判断按详情记录的提交版本，不按别的版本、也不跨版本。
	if want := reviewCaseDetail(t).SubmissionVersionID; judgments.loadedVersion != want {
		t.Fatalf("judgments loaded for version %q, want the detail's %q", judgments.loadedVersion, want)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Request struct {
			ShipmentRequestID string `json:"shipmentRequestId"`
			State             string `json:"state"`
			AcceptanceTask    struct {
				State string `json:"state"`
			} `json:"acceptanceTask"`
			Decision *json.RawMessage `json:"decision"`
		} `json:"request"`
		Review struct {
			WaitingOn   string `json:"waitingOn"`
			Completed   bool   `json:"completed"`
			Authority   string `json:"authority"`
			Reviewer    string `json:"reviewer"`
			Evidence    string `json:"evidence"`
			CompletedAt string `json:"completedAt"`
		} `json:"review"`
		Judgments struct {
			Reachability []struct {
				ParcelID   string `json:"parcelId"`
				Value      string `json:"value"`
				JudgmentID string `json:"judgmentId"`
				Basis      string `json:"basis"`
				AsOfAt     string `json:"asOfAt"`
			} `json:"reachability"`
			FinancialControl *struct {
				Outcome            string `json:"outcome"`
				ResultID           string `json:"resultId"`
				Basis              string `json:"basis"`
				AsOfAt             string `json:"asOfAt"`
				JointPassCondition string `json:"jointPassCondition"`
				OccupationFormed   bool   `json:"occupationFormed"`
				Items              []struct {
					Kind               string `json:"kind"`
					Order              uint32 `json:"order"`
					Conclusion         string `json:"conclusion"`
					Basis              string `json:"basis"`
					FailureDisposition string `json:"failureDisposition"`
					Responsibility     string `json:"responsibility"`
				} `json:"items"`
			} `json:"financialControl"`
			AdoptedResolutionID string `json:"adoptedResolutionId"`
		} `json:"recordedJudgments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "REVIEW_CASE" ||
		body.Request.ShipmentRequestID != "REQ-1" || body.Request.State != "SUBMITTED" ||
		body.Request.AcceptanceTask.State != "RUNNING" || body.Request.Decision != nil {
		t.Fatalf("委托详情变形：%+v", body.Request)
	}
	if body.Review.WaitingOn != "MANUAL_REVIEW" || !body.Review.Completed ||
		body.Review.Authority != "AUTH-RULE-9" || body.Review.Reviewer != "REVIEWER-OP-7" ||
		body.Review.Evidence != "EVIDENCE-42" || body.Review.CompletedAt != "2026-09-06T14:00:00Z" {
		t.Fatalf("review 块变形：%+v", body.Review)
	}
	if len(body.Judgments.Reachability) != 2 {
		t.Fatalf("reachability = %d, want 2", len(body.Judgments.Reachability))
	}
	reachable := body.Judgments.Reachability[0]
	if reachable.ParcelID != "parcel-1" || reachable.Value != "REACHABLE" ||
		reachable.JudgmentID != "REACH-1" || reachable.Basis != "" ||
		reachable.AsOfAt != "2026-09-06T12:30:00Z" {
		t.Fatalf("可达判断变形：%+v", reachable)
	}
	notApplicable := body.Judgments.Reachability[1]
	if notApplicable.ParcelID != "parcel-2" || notApplicable.Value != "NOT_APPLICABLE" ||
		notApplicable.JudgmentID != "" || notApplicable.Basis != "NO-NETWORK-DUTY-7" {
		t.Fatalf("不适用判断变形：%+v", notApplicable)
	}
	// 结论、条件与逐项一起透出（ADR-0125）：事后看接受判断的人要看得见「同一请求还记了什么」，而不是
	// 只见一个 RESTRICTED；OccupationFormed 让读的人知道结论受限时第一项仍占着钱。
	control := body.Judgments.FinancialControl
	if control == nil ||
		control.Outcome != "RESTRICTED" ||
		control.ResultID != "CTRL-1" ||
		control.Basis != "AVAILABLE_CREDIT_INSUFFICIENT" ||
		control.AsOfAt != "2026-09-06T12:35:00Z" ||
		control.JointPassCondition != "ALL_CONTROLS_PASS" ||
		!control.OccupationFormed {
		t.Fatalf("财务控制变形：%+v", control)
	}
	if len(control.Items) != 2 ||
		control.Items[0].Kind != "PREPAID_FREEZE" || control.Items[0].Order != 1 ||
		control.Items[0].Conclusion != "SATISFIED" || control.Items[0].Basis != "" ||
		control.Items[1].Kind != "CREDIT_CHECK" || control.Items[1].Order != 2 ||
		control.Items[1].Conclusion != "RESTRICTED" || control.Items[1].Basis != "AVAILABLE_CREDIT_INSUFFICIENT" {
		t.Fatalf("逐项变形：%+v", control.Items)
	}
	// 受限项上的采用引用两格照实透出、成立项两格缺席（ADR-0132 决定三、四）。
	if control.Items[0].FailureDisposition != "" || control.Items[0].Responsibility != "" ||
		control.Items[1].FailureDisposition != "AUTHORIZED_DISPOSITION" ||
		control.Items[1].Responsibility != "CONTRACT-CLAUSE-7" {
		t.Fatalf("采用引用两格变形：%+v", control.Items)
	}
	if body.Judgments.AdoptedResolutionID != "RES-1" {
		t.Fatalf("采用解析 = %q", body.Judgments.AdoptedResolutionID)
	}
}

// Covers: 判断三样零值即缺席——没有任何一轮判断的委托，reachability 空数组、
// financialControl 与 adoptedResolutionId 整格缺席，不造「空结果」冒充判断过。
func TestReviewCaseReportsAbsentJudgmentsHonestly(t *testing.T) {
	response := getReviewQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&reviewQueueReaderDouble{detail: reviewCaseDetail(t), found: true},
		&judgmentsReaderDouble{},
		http.MethodGet, "/acceptance-review-queue?shipmentRequestId=REQ-1")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body struct {
		Judgments map[string]json.RawMessage `json:"recordedJudgments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(body.Judgments["reachability"]) != "[]" {
		t.Fatalf(`reachability = %s, want []`, body.Judgments["reachability"])
	}
	if _, present := body.Judgments["financialControl"]; present {
		t.Fatal("尚未形成的财务控制长出了字段")
	}
	if _, present := body.Judgments["adoptedResolutionId"]; present {
		t.Fatal("尚未采用的解析长出了字段")
	}
}

// Covers: CONTEXT「统一不可见结果」——不存在、越权与其他作用域同一答复：404 + 单一
// code（与委托查阅同一枚），4xx 不带 outcome；未配置 Intake 对两条分支同答 403。
func TestReviewQueueInvisibleAndUnconfiguredAnswers(t *testing.T) {
	invisible := getReviewQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&reviewQueueReaderDouble{found: false}, &judgmentsReaderDouble{},
		http.MethodGet, "/acceptance-review-queue?shipmentRequestId=REQ-GHOST")
	if invisible.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", invisible.Code)
	}
	if code := problemCode(t, invisible); code != "SHIPMENT_REQUEST_NOT_VISIBLE" {
		t.Fatalf("code = %q", code)
	}
	assertNoOutcomeField(t, invisible)

	for _, target := range []string{
		"/acceptance-review-queue",
		"/acceptance-review-queue?shipmentRequestId=REQ-1",
	} {
		response := getReviewQueue(t, shipmenthttp.UnconfiguredIntake{},
			&reviewQueueReaderDouble{}, &judgmentsReaderDouble{},
			http.MethodGet, target)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", target, response.Code)
		}
		if code := problemCode(t, response); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q", target, code)
		}
	}
}

// Covers: ADR-0022 的 4xx/5xx 分流——读不回与判断读不回都是「没形成答案」的 5xx
// （砍半作答会让复核对着不完整的事实拍板），非 GET 是 405 带 Allow。
func TestReviewQueueFailuresSplitByRetryAction(t *testing.T) {
	listDown := getReviewQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&reviewQueueReaderDouble{listErr: errors.New("db down")}, &judgmentsReaderDouble{},
		http.MethodGet, "/acceptance-review-queue")
	if listDown.Code != http.StatusInternalServerError {
		t.Fatalf("list failure status = %d", listDown.Code)
	}
	if code := problemCode(t, listDown); code != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q", code)
	}

	findDown := getReviewQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&reviewQueueReaderDouble{findErr: errors.New("db down")}, &judgmentsReaderDouble{},
		http.MethodGet, "/acceptance-review-queue?shipmentRequestId=REQ-1")
	if findDown.Code != http.StatusInternalServerError {
		t.Fatalf("find failure status = %d", findDown.Code)
	}

	judgmentsDown := getReviewQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&reviewQueueReaderDouble{detail: reviewCaseDetail(t), found: true},
		&judgmentsReaderDouble{err: errors.New("db down")},
		http.MethodGet, "/acceptance-review-queue?shipmentRequestId=REQ-1")
	if judgmentsDown.Code != http.StatusInternalServerError {
		t.Fatalf("judgments failure status = %d", judgmentsDown.Code)
	}
	if code := problemCode(t, judgmentsDown); code != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q; 判断读不回砍半作答等于让复核对不完整事实拍板", code)
	}

	wrongMethod := getReviewQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&reviewQueueReaderDouble{}, &judgmentsReaderDouble{},
		http.MethodPost, "/acceptance-review-queue")
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("allow = %q", allow)
	}
}
