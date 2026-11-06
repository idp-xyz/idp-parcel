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
			State:                   domain.AcceptanceTaskComplete,
			HasAttempt:              true,
			LastAttemptReason:       "COMMERCIAL_BASIS_UNAVAILABLE",
			LastAttemptContinuation: "CONT-1",
			LastAttemptedAt:         viewSubmittedAt.Add(30 * time.Minute),
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
				State                   string `json:"state"`
				LastAttemptReason       string `json:"lastAttemptReason"`
				LastAttemptContinuation string `json:"lastAttemptContinuation"`
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
	if view.AcceptanceTask.State != "COMPLETE" ||
		view.AcceptanceTask.LastAttemptReason != "COMMERCIAL_BASIS_UNAVAILABLE" ||
		view.AcceptanceTask.LastAttemptContinuation != "CONT-1" {
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
