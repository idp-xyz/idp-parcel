package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

var viewSubmittedAt = time.Date(2026, 9, 6, 11, 0, 0, 0, time.UTC)

func viewsScope(t *testing.T) domain.AuthorizedQueryScope {
	t.Helper()
	scope, err := domain.NewAuthorizedQueryScope(
		mustValue(t, domain.NewQueryScopeReference, "SCOPE-GRANT-1"),
		mustValue(t, domain.NewTenantID, "TENANT-1"),
		[]domain.CustomerAccountID{mustValue(t, domain.NewCustomerAccountID, "CUST-1")},
	)
	if err != nil {
		t.Fatalf("查询作用域：%v", err)
	}
	return scope
}

func viewSummaryRecord(t *testing.T, requestID string) ports.ShipmentRequestSummaryRecord {
	t.Helper()
	return ports.ShipmentRequestSummaryRecord{
		CustomerAccountID:   mustValue(t, domain.NewCustomerAccountID, "CUST-1"),
		Source:              mustValue(t, domain.NewSource, "portal"),
		SourceRequestKey:    mustValue(t, domain.NewSourceRequestKey, "key-"+requestID),
		ShipmentRequestID:   mustValue(t, domain.NewShipmentRequestID, requestID),
		State:               domain.ShipmentRequestSubmitted,
		SubmissionVersionID: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		DeclaredParcelCount: 2,
		SubmittedAt:         viewSubmittedAt,
	}
}

func viewDetailRecord(t *testing.T) ports.ShipmentRequestDetailRecord {
	t.Helper()
	summary := viewSummaryRecord(t, "REQ-1")
	summary.State = domain.ShipmentRequestAccepted
	return ports.ShipmentRequestDetailRecord{
		ShipmentRequestSummaryRecord: summary,
		BatchID:                      mustValue(t, domain.NewSubmissionBatchID, "batch-1"),
		OccurredAt:                   viewSubmittedAt.Add(-time.Hour),
		ReceivedAt:                   viewSubmittedAt.Add(-time.Hour + time.Second),
		DeclaredParcels: []ports.DeclaredParcelViewRecord{
			{
				Parcel:         mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
				WeightValue:    "2.5",
				WeightUnit:     "kg",
				HasDimensions:  true,
				Length:         "30",
				Width:          "20",
				Height:         "10",
				DimensionsUnit: "cm",
			},
			{Parcel: mustValue(t, domain.NewDeclaredParcelID, "parcel-2")},
		},
		PriorVersionCount: 1,
		Task: ports.AcceptanceTaskViewRecord{
			State:                       domain.AcceptanceTaskComplete,
			HasAttempt:                  true,
			LastAttemptReason:           "FINANCIAL_CONTROL_NOT_CONFIGURED",
			LastAttemptResumePath:       domain.ResumeByOperatorRegistration,
			LastAttemptContinuation:     "CONT-1",
			LastAttemptedAt:             viewSubmittedAt.Add(30 * time.Minute),
			LastAttemptSettlementReason: "ESTIMATE_PRICE_CARD_NOT_CONFIGURED",
			LastAttemptServiceProduct:   "SYN-PROD-01",
		},
		HasDecision: true,
		Decision: ports.AcceptanceDecisionViewRecord{
			DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "accept-1"),
			Accepted:   true,
			DecidedAt:  viewSubmittedAt.Add(time.Hour),
		},
	}
}

type viewsIntakeDouble struct {
	scope        domain.AuthorizedQueryScope
	listErr      error
	detailErr    error
	listCalled   bool
	detailCalled bool
}

func (double *viewsIntakeDouble) IntakeListQuery(
	_ context.Context,
	_ *http.Request,
) (shipmenthttp.ShipmentRequestViewsQuery, error) {
	double.listCalled = true
	if double.listErr != nil {
		return shipmenthttp.ShipmentRequestViewsQuery{}, double.listErr
	}
	return shipmenthttp.ShipmentRequestViewsQuery{Scope: double.scope, Limit: 50}, nil
}

func (double *viewsIntakeDouble) IntakeDetailQuery(
	_ context.Context,
	request *http.Request,
) (shipmenthttp.ShipmentRequestViewQuery, error) {
	double.detailCalled = true
	if double.detailErr != nil {
		return shipmenthttp.ShipmentRequestViewQuery{}, double.detailErr
	}
	requestID, err := domain.NewShipmentRequestID(request.URL.Query().Get("shipmentRequestId"))
	if err != nil {
		return shipmenthttp.ShipmentRequestViewQuery{}, fmt.Errorf("%w: %v", shipmenthttp.ErrMalformedRequest, err)
	}
	return shipmenthttp.ShipmentRequestViewQuery{Scope: double.scope, RequestID: requestID}, nil
}

type viewsReaderDouble struct {
	rows    []ports.ShipmentRequestSummaryRecord
	listErr error
	detail  ports.ShipmentRequestDetailRecord
	found   bool
	findErr error
	// byNumber 是按单据号查到的行；asked 记下被问的那个号（没问过为空）。
	byNumber []ports.ShipmentRequestSummaryRecord
	asked    string
	// byIDs 是按标识集合查到的行；askedIDs 记下被问的那一组（没问过为 nil），askedScope 记下随它递来的作用域。
	byIDs      []ports.ShipmentRequestSummaryRecord
	askedIDs   []string
	askedScope domain.AuthorizedQueryScope
}

func (double *viewsReaderDouble) ListVisibleByIDs(
	_ context.Context,
	scope domain.AuthorizedQueryScope,
	requestIDs []domain.ShipmentRequestID,
) ([]ports.ShipmentRequestSummaryRecord, error) {
	double.askedIDs = []string{}
	for _, requestID := range requestIDs {
		double.askedIDs = append(double.askedIDs, requestID.String())
	}
	double.askedScope = scope
	if double.listErr != nil {
		return nil, double.listErr
	}
	return double.byIDs, nil
}

func (double *viewsReaderDouble) ListVisibleByDocumentNumber(
	_ context.Context,
	_ domain.AuthorizedQueryScope,
	number domain.ShipmentDocumentNumber,
) ([]ports.ShipmentRequestSummaryRecord, error) {
	double.asked = number.String()
	if double.listErr != nil {
		return nil, double.listErr
	}
	return double.byNumber, nil
}

func (double *viewsReaderDouble) ListVisible(
	_ context.Context,
	_ domain.AuthorizedQueryScope,
	_ int,
	_ cataloguepage.Query,
) (cataloguepage.Result[ports.ShipmentRequestSummaryRecord], error) {
	if double.listErr != nil {
		return cataloguepage.Result[ports.ShipmentRequestSummaryRecord]{}, double.listErr
	}
	return cataloguepage.Result[ports.ShipmentRequestSummaryRecord]{Rows: double.rows}, nil
}

func (double *viewsReaderDouble) FindVisibleByID(
	_ context.Context,
	_ domain.AuthorizedQueryScope,
	_ domain.ShipmentRequestID,
) (ports.ShipmentRequestDetailRecord, bool, error) {
	if double.findErr != nil {
		return ports.ShipmentRequestDetailRecord{}, false, double.findErr
	}
	return double.detail, double.found, nil
}

func getViews(
	t *testing.T,
	intake shipmenthttp.ShipmentRequestViewsIntake,
	reader shipmenthttp.ShipmentRequestViewsReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewQueryShipmentRequestViewsEndpoint(intake, reader)
	request := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: 列表答案已形成即 200 + outcome LISTED（ADR-0022），行内容按读口记录逐字段
// 透出；空列表交回空数组——「暂无可见委托」是答案，不是错误，也不泄露任何存在性。
func TestViewsListReportsRowsAndAnEmptyListAlike(t *testing.T) {
	intake := &viewsIntakeDouble{scope: viewsScope(t)}
	response := getViews(t, intake,
		&viewsReaderDouble{rows: []ports.ShipmentRequestSummaryRecord{
			viewSummaryRecord(t, "REQ-2"),
			viewSummaryRecord(t, "REQ-1"),
		}},
		http.MethodGet, "/shipment-request-views")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body struct {
		Outcome  string `json:"outcome"`
		Requests []struct {
			ShipmentRequestID   string `json:"shipmentRequestId"`
			CustomerAccountID   string `json:"customerAccountId"`
			Source              string `json:"source"`
			SourceRequestKey    string `json:"sourceRequestKey"`
			State               string `json:"state"`
			SubmissionVersionID string `json:"submissionVersionId"`
			DeclaredParcelCount int    `json:"declaredParcelCount"`
			SubmittedAt         string `json:"submittedAt"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "LISTED" || len(body.Requests) != 2 {
		t.Fatalf("outcome = %q rows = %d", body.Outcome, len(body.Requests))
	}
	first := body.Requests[0]
	if first.ShipmentRequestID != "REQ-2" || first.CustomerAccountID != "CUST-1" ||
		first.Source != "portal" || first.SourceRequestKey != "key-REQ-2" ||
		first.State != "SUBMITTED" || first.SubmissionVersionID != "version-1" ||
		first.DeclaredParcelCount != 2 || first.SubmittedAt != "2026-09-06T11:00:00Z" {
		t.Fatalf("行内容变形：%+v", first)
	}
	if !intake.listCalled || intake.detailCalled {
		t.Fatal("无标识参数的请求该走列表分支")
	}

	empty := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&viewsReaderDouble{}, http.MethodGet, "/shipment-request-views")
	if empty.Code != http.StatusOK {
		t.Fatalf("empty status = %d", empty.Code)
	}
	var emptyBody map[string]json.RawMessage
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if string(emptyBody["requests"]) != "[]" {
		t.Fatalf(`requests = %s, want []（空列表不是 null）`, emptyBody["requests"])
	}
}

// Covers: ADR-0355 决定十一——列表行带单据号；没有号的行不带 `documentNumber` 键（不答空串、不拿委托标识顶），只带
// `documentNumberAbsence` 说是上线前开立还是单据编号时区未配置。
//
// Covers: `AT-PS-178`。
func TestViewsListCarriesTheDocumentNumberOrWhyThereIsNone(t *testing.T) {
	numbered := viewSummaryRecord(t, "REQ-3")
	numbering, err := domain.NumberedShipment(mustValue(t, domain.NewShipmentDocumentNumber, "P20260906000003"))
	if err != nil {
		t.Fatal(err)
	}
	numbered.DocumentNumbering = numbering
	unconfigured := viewSummaryRecord(t, "REQ-2")
	unconfigured.DocumentNumbering, err = domain.UnnumberedShipment(domain.DocumentNumberingTimeZoneNotConfigured)
	if err != nil {
		t.Fatal(err)
	}
	legacy := viewSummaryRecord(t, "REQ-1")
	legacy.DocumentNumbering, err = domain.UnnumberedShipment(domain.IssuedBeforeDocumentNumbering)
	if err != nil {
		t.Fatal(err)
	}

	response := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&viewsReaderDouble{rows: []ports.ShipmentRequestSummaryRecord{numbered, unconfigured, legacy}},
		http.MethodGet, "/shipment-request-views")
	var body struct {
		Requests []map[string]json.RawMessage `json:"requests"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Requests) != 3 {
		t.Fatalf("decode: %v %s", err, response.Body.Bytes())
	}
	if string(body.Requests[0]["documentNumber"]) != `"P20260906000003"` || body.Requests[0]["documentNumberAbsence"] != nil {
		t.Fatalf("numbered row = %s", response.Body.Bytes())
	}
	for index, want := range map[int]string{1: `"NUMBERING_TIME_ZONE_NOT_CONFIGURED"`, 2: `"ISSUED_BEFORE_NUMBERING"`} {
		if _, present := body.Requests[index]["documentNumber"]; present {
			t.Fatalf("row %d carried a documentNumber key: %s", index, response.Body.Bytes())
		}
		if string(body.Requests[index]["documentNumberAbsence"]) != want {
			t.Fatalf("row %d absence = %s, want %s", index, body.Requests[index]["documentNumberAbsence"], want)
		}
	}
}

// Covers: `AT-PS-178`；ADR-0355 决定十一「可按单据号检索」——列表带 `documentNumber` 只问按号查的那一口、逐字递给读口，答 LISTED；
// 没查到是空数组不是 404；空号是坏请求；读不动是 5xx。
func TestViewsListFiltersByTheDocumentNumber(t *testing.T) {
	numbered := viewSummaryRecord(t, "REQ-7")
	numbering, err := domain.NumberedShipment(mustValue(t, domain.NewShipmentDocumentNumber, "P20261102000007"))
	if err != nil {
		t.Fatal(err)
	}
	numbered.DocumentNumbering = numbering
	reader := &viewsReaderDouble{byNumber: []ports.ShipmentRequestSummaryRecord{numbered}, rows: []ports.ShipmentRequestSummaryRecord{viewSummaryRecord(t, "REQ-1")}}
	response := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, reader, http.MethodGet, "/shipment-request-views?documentNumber=P20261102000007")
	var body struct {
		Outcome  string `json:"outcome"`
		Requests []struct {
			ShipmentRequestID string `json:"shipmentRequestId"`
			DocumentNumber    string `json:"documentNumber"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Code != http.StatusOK || body.Outcome != "LISTED" || len(body.Requests) != 1 ||
		body.Requests[0].ShipmentRequestID != "REQ-7" || body.Requests[0].DocumentNumber != "P20261102000007" {
		t.Fatalf("按号查 = %d %s", response.Code, response.Body.Bytes())
	}
	if reader.asked != "P20261102000007" {
		t.Fatalf("读口收到的号 = %q", reader.asked)
	}

	none := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{}, http.MethodGet, "/shipment-request-views?documentNumber=P404")
	var emptyBody map[string]json.RawMessage
	if err := json.Unmarshal(none.Body.Bytes(), &emptyBody); err != nil || none.Code != http.StatusOK || string(emptyBody["requests"]) != "[]" {
		t.Fatalf("没查到 = %d %s", none.Code, none.Body.Bytes())
	}

	if blank := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{}, http.MethodGet, "/shipment-request-views?documentNumber=%20"); blank.Code != http.StatusBadRequest {
		t.Fatalf("空号 = %d", blank.Code)
	}
	if failed := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{listErr: errors.New("down")}, http.MethodGet, "/shipment-request-views?documentNumber=P1"); failed.Code != http.StatusInternalServerError {
		t.Fatalf("读不动 = %d", failed.Code)
	}
}

// Covers: `AT-PS-180`；ADR-0355 决定十一——别的上下文的页面按页上那几份委托标识一次取回运单号：`shipmentRequestIds` 逗号分隔、
// 去重后逐个递给读口，作用域是列表查询那一份，答 LISTED 带各行的单据号；问到的委托一份也不可见时是空数组不是 404；空段、
// 多给一格、超出上限、与 `documentNumber` 并用都是坏请求且不读；读不动是 5xx。
func TestViewsListLooksUpASetOfShipmentRequestIDs(t *testing.T) {
	numbered := viewSummaryRecord(t, "REQ-7")
	numbering, err := domain.NumberedShipment(mustValue(t, domain.NewShipmentDocumentNumber, "P20261102000007"))
	if err != nil {
		t.Fatal(err)
	}
	numbered.DocumentNumbering = numbering
	scope := viewsScope(t)
	reader := &viewsReaderDouble{byIDs: []ports.ShipmentRequestSummaryRecord{numbered}, rows: []ports.ShipmentRequestSummaryRecord{viewSummaryRecord(t, "REQ-1")}}
	response := getViews(t, &viewsIntakeDouble{scope: scope}, reader, http.MethodGet, "/shipment-request-views?shipmentRequestIds=REQ-7,REQ-404,REQ-7")
	var body struct {
		Outcome  string `json:"outcome"`
		Requests []struct {
			ShipmentRequestID string `json:"shipmentRequestId"`
			DocumentNumber    string `json:"documentNumber"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Code != http.StatusOK || body.Outcome != "LISTED" || len(body.Requests) != 1 ||
		body.Requests[0].ShipmentRequestID != "REQ-7" || body.Requests[0].DocumentNumber != "P20261102000007" {
		t.Fatalf("按标识集合查 = %d %s", response.Code, response.Body.Bytes())
	}
	if strings.Join(reader.askedIDs, ",") != "REQ-7,REQ-404" {
		t.Fatalf("读口收到的标识 = %v", reader.askedIDs)
	}
	if !reflect.DeepEqual(reader.askedScope, scope) {
		t.Fatalf("读口收到的作用域不是列表查询那一份：%+v", reader.askedScope)
	}

	none := getViews(t, &viewsIntakeDouble{scope: scope}, &viewsReaderDouble{}, http.MethodGet, "/shipment-request-views?shipmentRequestIds=REQ-404")
	var emptyBody map[string]json.RawMessage
	if err := json.Unmarshal(none.Body.Bytes(), &emptyBody); err != nil || none.Code != http.StatusOK || string(emptyBody["requests"]) != "[]" {
		t.Fatalf("一份也不可见 = %d %s", none.Code, none.Body.Bytes())
	}

	atLimit := make([]string, 100)
	for index := range atLimit {
		atLimit[index] = fmt.Sprintf("REQ-%d", index)
	}
	limitReader := &viewsReaderDouble{}
	if limit := getViews(t, &viewsIntakeDouble{scope: scope}, limitReader, http.MethodGet, "/shipment-request-views?shipmentRequestIds="+strings.Join(atLimit, ",")); limit.Code != http.StatusOK || len(limitReader.askedIDs) != 100 {
		t.Fatalf("恰到上限 = %d，问了 %d 个", limit.Code, len(limitReader.askedIDs))
	}

	for name, target := range map[string]string{
		"空段":     "/shipment-request-views?shipmentRequestIds=REQ-1,,REQ-2",
		"空白段":    "/shipment-request-views?shipmentRequestIds=REQ-1,%20",
		"空值":     "/shipment-request-views?shipmentRequestIds=",
		"多给一格":   "/shipment-request-views?shipmentRequestIds=REQ-1&shipmentRequestIds=REQ-2",
		"超出上限":   "/shipment-request-views?shipmentRequestIds=" + strings.Join(append(atLimit, "REQ-100"), ","),
		"与运单号并用": "/shipment-request-views?shipmentRequestIds=REQ-1&documentNumber=P1",
	} {
		malformedReader := &viewsReaderDouble{}
		if malformed := getViews(t, &viewsIntakeDouble{scope: scope}, malformedReader, http.MethodGet, target); malformed.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d，要 400", name, malformed.Code)
		}
		if malformedReader.askedIDs != nil || malformedReader.asked != "" {
			t.Fatalf("%s：坏请求读了读口", name)
		}
	}

	if failed := getViews(t, &viewsIntakeDouble{scope: scope}, &viewsReaderDouble{listErr: errors.New("down")}, http.MethodGet, "/shipment-request-views?shipmentRequestIds=REQ-1"); failed.Code != http.StatusInternalServerError {
		t.Fatalf("读不动 = %d", failed.Code)
	}
}

// Covers: 单份查阅按 `shipmentRequestId` 参数分派到详情分支；已形成的详情整份透出，
// 决定词汇与撤回端点同一套（ACCEPTED），无画像成员测量字段如实缺席。
func TestViewsDetailReportsTheWholeView(t *testing.T) {
	intake := &viewsIntakeDouble{scope: viewsScope(t)}
	response := getViews(t, intake,
		&viewsReaderDouble{detail: viewDetailRecord(t), found: true},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if !intake.detailCalled || intake.listCalled {
		t.Fatal("带标识参数的请求该走详情分支")
	}
	var body struct {
		Outcome string `json:"outcome"`
		Request struct {
			ShipmentRequestID string `json:"shipmentRequestId"`
			State             string `json:"state"`
			BatchID           string `json:"batchId"`
			OccurredAt        string `json:"occurredAt"`
			ReceivedAt        string `json:"receivedAt"`
			DeclaredParcels   []struct {
				ParcelID            string `json:"parcelId"`
				DeclaredWeightValue string `json:"declaredWeightValue"`
				Dimensions          *struct {
					Length string `json:"length"`
					Unit   string `json:"unit"`
				} `json:"dimensions"`
			} `json:"declaredParcels"`
			PriorVersionCount int `json:"priorVersionCount"`
			AcceptanceTask    struct {
				State                       string `json:"state"`
				LastAttemptReason           string `json:"lastAttemptReason"`
				LastAttemptResumePath       string `json:"lastAttemptResumePath"`
				LastAttemptContinuation     string `json:"lastAttemptContinuation"`
				LastAttemptSettlementReason string `json:"lastAttemptSettlementReason"`
				LastAttemptServiceProduct   string `json:"lastAttemptServiceProduct"`
			} `json:"acceptanceTask"`
			Decision *struct {
				DecisionID string `json:"decisionId"`
				Kind       string `json:"kind"`
				DecidedAt  string `json:"decidedAt"`
			} `json:"decision"`
		} `json:"request"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	view := body.Request
	if body.Outcome != "REQUEST_VIEW" || view.ShipmentRequestID != "REQ-1" || view.State != "ACCEPTED" {
		t.Fatalf("outcome = %q view = %+v", body.Outcome, view)
	}
	if view.BatchID != "batch-1" ||
		view.OccurredAt != "2026-09-06T10:00:00Z" || view.ReceivedAt != "2026-09-06T10:00:01Z" {
		t.Fatalf("批次或信封时间变形：%+v", view)
	}
	if len(view.DeclaredParcels) != 2 {
		t.Fatalf("parcels = %d", len(view.DeclaredParcels))
	}
	if view.DeclaredParcels[0].DeclaredWeightValue != "2.5" ||
		view.DeclaredParcels[0].Dimensions == nil ||
		view.DeclaredParcels[0].Dimensions.Length != "30" ||
		view.DeclaredParcels[0].Dimensions.Unit != "cm" {
		t.Fatalf("画像变形：%+v", view.DeclaredParcels[0])
	}
	if view.DeclaredParcels[1].DeclaredWeightValue != "" || view.DeclaredParcels[1].Dimensions != nil {
		t.Fatalf("无画像成员长出了测量：%+v", view.DeclaredParcels[1])
	}
	if view.PriorVersionCount != 1 {
		t.Fatalf("prior versions = %d", view.PriorVersionCount)
	}
	// 这几格字段名与管理台票 declared-goods-lines/24 钉死（ADR-0380 决定三、四）：续办方交领域原词，结算原因不冠 SA-。
	if view.AcceptanceTask.State != "COMPLETE" ||
		view.AcceptanceTask.LastAttemptReason != "FINANCIAL_CONTROL_NOT_CONFIGURED" ||
		view.AcceptanceTask.LastAttemptResumePath != "OPERATOR_REGISTRATION" ||
		view.AcceptanceTask.LastAttemptContinuation != "CONT-1" ||
		view.AcceptanceTask.LastAttemptSettlementReason != "ESTIMATE_PRICE_CARD_NOT_CONFIGURED" ||
		view.AcceptanceTask.LastAttemptServiceProduct != "SYN-PROD-01" {
		t.Fatalf("任务面变形：%+v", view.AcceptanceTask)
	}
	if view.Decision == nil || view.Decision.Kind != "ACCEPTED" ||
		view.Decision.DecisionID != "accept-1" || view.Decision.DecidedAt != "2026-09-06T12:00:00Z" {
		t.Fatalf("决定摘要变形：%+v", view.Decision)
	}
}

// Covers: CONTEXT「统一不可见结果」与 ADR-0022「!found 一律 404」——不存在、越权与
// 其他作用域同一答复：404 + 单一 code，4xx 不带 outcome，也不带任何自由文本。
func TestViewsDetailAnswersInvisibleUniformly(t *testing.T) {
	response := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&viewsReaderDouble{found: false},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-GHOST")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if code := problemCode(t, response); code != "SHIPMENT_REQUEST_NOT_VISIBLE" {
		t.Fatalf("code = %q", code)
	}
	assertNoOutcomeField(t, response)
}

// Covers: ADR-0022 的 4xx/5xx 分流在查阅端点上——构造不出查询是 400（重发不会变），
// 接入解析故障与读不回都是 5xx（该重试），非 GET 是 405 带 Allow。
func TestViewsTransportFailuresSplitByRetryAction(t *testing.T) {
	malformed := getViews(t,
		&viewsIntakeDouble{listErr: fmt.Errorf("%w: bad limit", shipmenthttp.ErrMalformedRequest)},
		&viewsReaderDouble{}, http.MethodGet, "/shipment-request-views")
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d", malformed.Code)
	}
	if code := problemCode(t, malformed); code != "MALFORMED_REQUEST" {
		t.Fatalf("code = %q", code)
	}

	intakeDown := getViews(t,
		&viewsIntakeDouble{detailErr: errors.New("authenticator unreachable")},
		&viewsReaderDouble{}, http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1")
	if intakeDown.Code != http.StatusInternalServerError {
		t.Fatalf("intake failure status = %d", intakeDown.Code)
	}
	if code := problemCode(t, intakeDown); code != "INTAKE_FAILED" {
		t.Fatalf("code = %q", code)
	}

	listDown := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&viewsReaderDouble{listErr: errors.New("db down")},
		http.MethodGet, "/shipment-request-views")
	if listDown.Code != http.StatusInternalServerError {
		t.Fatalf("list failure status = %d", listDown.Code)
	}
	if code := problemCode(t, listDown); code != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q", code)
	}

	findDown := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&viewsReaderDouble{findErr: errors.New("db down")},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1")
	if findDown.Code != http.StatusInternalServerError {
		t.Fatalf("find failure status = %d", findDown.Code)
	}
	if code := problemCode(t, findDown); code != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q; 读不回伪装成不可见会让该重试的故障变成终局", code)
	}

	wrongMethod := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&viewsReaderDouble{}, http.MethodPost, "/shipment-request-views")
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("allow = %q", allow)
	}
}

// Covers: ADR-0373、ADR-0374 决定六（票 declared-goods-lines/04）——详情交出委托级货物明细（每格带值与来源标记）与平台货件，逐件交
// 客户件号、平台箱标号与这一箱的装箱分配，形状与提交口对应；没报的整键缺席。
func TestViewsDetailReportsGoodsAndPlatformDeclarations(t *testing.T) {
	record := viewDetailRecord(t)
	parcel1 := mustValue(t, domain.NewDeclaredParcelID, "parcel-1")
	parcel2 := mustValue(t, domain.NewDeclaredParcelID, "parcel-2")
	this := domain.SourceMarkThisDeclaration
	goods, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{
		Category: domain.ContentCategorySaleOfGoods, CategoryMark: this,
		Lines: []domain.GoodsLineSpec{
			{Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: domain.GoodsCellSpec{Value: "T-shirt", Mark: domain.SourceMarkBoxList}}},
				Quantity:       domain.GoodsCellSpec{Value: "48", Mark: this},
				CommodityCodes: []domain.CommodityCodeSpec{{Side: domain.CommodityCodeExportSide, Code: domain.GoodsCellSpec{Value: "6109100000", Mark: this}}},
				SalesOrder:     domain.SalesOrderReferenceSpec{OrderNumber: domain.GoodsCellSpec{Value: "1001", Mark: this}}},
		},
		Packing: []domain.PackingAllocationSpec{{Parcel: parcel1, Line: 1, Quantity: "24"}, {Parcel: parcel2, Line: 1, Quantity: "24"}},
	}, []domain.DeclaredParcelID{parcel1, parcel2})
	if err != nil {
		t.Fatalf("货物明细：%v", err)
	}
	record.Goods = goods
	shipment, err := domain.DeclarePlatformShipment("AMAZON", mustValue(t, domain.NewExternalIdentifierNumber, "FBA15SYNVIEW"), "")
	if err != nil {
		t.Fatalf("平台货件：%v", err)
	}
	record.PlatformShipment = shipment
	record.DeclaredParcels[0].CustomerParcelNumber = "CUST-0001"
	record.DeclaredParcels[1].PlatformBoxLabel = "FBA15SYNVIEWU000002"

	response := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: record, found: true},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1")
	var body struct {
		Request struct {
			Goods *struct {
				ContentCategory string `json:"contentCategory"`
				Lines           []struct {
					Line         int `json:"line"`
					Descriptions []struct {
						Lang string `json:"lang"`
						Text struct{ Value, Mark string }
					} `json:"descriptions"`
					Quantity       *struct{ Value, Mark string } `json:"quantity"`
					LineValue      *struct{ Value, Mark string } `json:"lineValue"`
					CommodityCodes []struct {
						Side string                       `json:"side"`
						Code struct{ Value, Mark string } `json:"code"`
					} `json:"commodityCodes"`
					SalesOrder *struct {
						OrderNumber *struct{ Value, Mark string } `json:"orderNumber"`
					} `json:"salesOrder"`
				} `json:"lines"`
			} `json:"goods"`
			PlatformShipment *struct {
				Platform    string `json:"platform"`
				ShipmentID  string `json:"shipmentId"`
				ReferenceID string `json:"referenceId"`
			} `json:"platformShipment"`
			DeclaredParcels []struct {
				ParcelID             string `json:"parcelId"`
				CustomerParcelNumber string `json:"customerParcelNumber"`
				PlatformBoxLabel     string `json:"platformBoxLabel"`
				Packing              []struct {
					Line     int    `json:"line"`
					Quantity string `json:"quantity"`
				} `json:"packing"`
			} `json:"declaredParcels"`
		} `json:"request"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	view := body.Request
	if view.Goods == nil || view.Goods.ContentCategory != "SALE_OF_GOODS" || len(view.Goods.Lines) != 1 {
		t.Fatalf("goods = %+v", view.Goods)
	}
	line := view.Goods.Lines[0]
	if line.Line != 1 || line.Descriptions[0].Text.Value != "T-shirt" || line.Descriptions[0].Text.Mark != "BOX_LIST" ||
		line.Quantity == nil || line.Quantity.Value != "48" || line.LineValue != nil ||
		line.CommodityCodes[0].Side != "EXPORT" || line.SalesOrder == nil || line.SalesOrder.OrderNumber.Value != "1001" {
		t.Fatalf("line = %+v；没报的格整键缺席", line)
	}
	if view.PlatformShipment == nil || view.PlatformShipment.ShipmentID != "FBA15SYNVIEW" || view.PlatformShipment.ReferenceID != "" {
		t.Fatalf("platform shipment = %+v", view.PlatformShipment)
	}
	parcels := view.DeclaredParcels
	if parcels[0].CustomerParcelNumber != "CUST-0001" || parcels[1].PlatformBoxLabel != "FBA15SYNVIEWU000002" ||
		len(parcels[0].Packing) != 1 || parcels[0].Packing[0].Quantity != "24" || parcels[1].Packing[0].Line != 1 {
		t.Fatalf("parcels = %+v", parcels)
	}

	plain := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: viewDetailRecord(t), found: true},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1")
	if strings.Contains(plain.Body.String(), `"goods"`) || strings.Contains(plain.Body.String(), `"platformShipment"`) {
		t.Fatalf("没报货物与平台货件的委托交出了这两键：%s", plain.Body.String())
	}
}

// Covers: 票 declared-goods-lines/04 完成判据「单件委托、多件不报分配」——只报整票清单时货物明细照交、各件不带 packing，单件委托
// 同样不带；跟踪号逐件交。
func TestViewsDetailReportsAWholeShipmentListWithoutPacking(t *testing.T) {
	for name, parcels := range map[string]int{"单件委托": 1, "多件整票清单": 2} {
		t.Run(name, func(t *testing.T) {
			record := viewDetailRecord(t)
			record.DeclaredParcels = record.DeclaredParcels[:parcels]
			members := make([]domain.DeclaredParcelID, 0, parcels)
			for _, parcel := range record.DeclaredParcels {
				members = append(members, parcel.Parcel)
			}
			goods, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{
				{Quantity: domain.GoodsCellSpec{Value: "3", Mark: domain.SourceMarkThisDeclaration}},
			}}, members)
			if err != nil {
				t.Fatalf("货物明细：%v", err)
			}
			record.Goods = goods
			record.DeclaredParcels[0].TrackingNumbers = []string{"SYN-TRACK-1"}
			response := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: record, found: true},
				http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1")
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
			}
			body := response.Body.String()
			if !strings.Contains(body, `"goods":{`) || strings.Contains(body, `"packing"`) {
				t.Fatalf("整票清单要交货物明细、不带 packing：%s", body)
			}
			if !strings.Contains(body, `"trackingNumbers":["SYN-TRACK-1"]`) || strings.Count(body, `"trackingNumbers"`) != 1 {
				t.Fatalf("跟踪号要只挂在有号的那一件上：%s", body)
			}
		})
	}
}

// Covers: 票 declared-goods-lines/06——货物明细范围上两条修订分叉时，详情不交 goods 而说 UNDETERMINED：查阅的人要分得清
// 「没报」与「说不出该按哪一版」；修订后的那一版被采用时说 ON_ADOPTED_VERSION。
func TestViewsDetailSaysWhereTheGoodsDetailsComeFrom(t *testing.T) {
	record := viewDetailRecord(t)
	record.GoodsStanding = domain.GoodsDetailsUndetermined
	body := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: record, found: true},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1").Body.String()
	if !strings.Contains(body, `"goodsStanding":"UNDETERMINED"`) || strings.Contains(body, `"goods":`) {
		t.Fatalf("分叉时要说 UNDETERMINED 且不交 goods：%s", body)
	}

	plain := viewDetailRecord(t)
	plain.GoodsStanding = domain.GoodsDetailsOnSubmission
	body = getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: plain, found: true},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1").Body.String()
	if strings.Contains(body, `"goodsStanding"`) {
		t.Fatalf("没报也没修订过的委托不带 goodsStanding：%s", body)
	}
}

// Covers: ADR-0376 决定一——受理任务停在等待受控补充时，详情交出缺的各项（判组、包裹、原因），查阅的人看得到缺的是哪一条、
// 哪几箱；不在等补充时整键缺席。
func TestViewsDetailNamesWhatTheSupplementIsWaitingFor(t *testing.T) {
	record := viewDetailRecord(t)
	record.Task.SupplementGaps = []ports.SupplementGapViewRecord{
		{Group: domain.RequiredDocumentCheck, Parcel: "parcel-2", Reason: "EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL"},
		{Group: domain.RequiredDocumentCheck, Reason: "PACKING_DECLARED"},
	}
	body := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: record, found: true},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1").Body.String()
	for _, want := range []string{
		`"supplementGaps":[`,
		`{"group":"` + domain.RequiredDocumentCheck.String() + `","parcelId":"parcel-2","reason":"EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL"}`,
		`{"group":"` + domain.RequiredDocumentCheck.String() + `","reason":"PACKING_DECLARED"}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("缺 %s：%s", want, body)
		}
	}
	plain := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: viewDetailRecord(t), found: true},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1").Body.String()
	if strings.Contains(plain, `"supplementGaps"`) {
		t.Fatalf("不在等补充也带了缺口：%s", plain)
	}
}
