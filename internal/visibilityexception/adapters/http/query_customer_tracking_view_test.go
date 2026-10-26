package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	veps "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

type intakeDouble struct {
	query visibilityhttp.TrackingViewQuery
	err   error
}

func (double intakeDouble) IntakeQuery(
	_ context.Context,
	_ *http.Request,
) (visibilityhttp.TrackingViewQuery, error) {
	if double.err != nil {
		return visibilityhttp.TrackingViewQuery{}, double.err
	}
	return double.query, nil
}

type readerKey struct {
	tenant   domain.TenantID
	customer domain.CustomerAccountReference
	parcel   domain.TrackedParcelReference
}

type readerDouble struct {
	views map[readerKey]domain.CustomerTrackingView
	err   error
}

func (double readerDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
) (domain.CustomerTrackingView, bool, error) {
	if double.err != nil {
		return domain.CustomerTrackingView{}, false, double.err
	}
	view, found := double.views[readerKey{tenant: tenant, customer: customer, parcel: parcel}]
	return view, found, nil
}

// attributionDouble 是归属反查口（ports.ParcelCustomerAccountView）的替身：owners 按包裹答唯一归属账户，ambiguous 里的包裹答
// 多于一份的具名错误，err 非空即读不动。asked 记被问的次数：读到视图不问、读不到问一次（ADR-0246 决定二、三）。
type attributionDouble struct {
	owners    map[string]string
	ambiguous map[string]bool
	err       error
	asked     int
}

func (double *attributionDouble) FindCustomerAccount(
	_ context.Context,
	_ domain.TenantID,
	parcel domain.TrackedParcelReference,
) (domain.CustomerAccountReference, bool, error) {
	double.asked++
	none := domain.CustomerAccountReference{}
	if double.err != nil {
		return none, false, double.err
	}
	if double.ambiguous[parcel.String()] {
		return none, false, fmt.Errorf("%w: parcel %q", veps.ErrAmbiguousCustomerAccount, parcel.String())
	}
	owner, attributed := double.owners[parcel.String()]
	if !attributed {
		return none, false, nil
	}
	account, err := domain.NewCustomerAccountReference(owner)
	return account, err == nil, err
}

// noAttributions 是什么都不归属的反查口：读不到视图时答零份。
func noAttributions() *attributionDouble { return &attributionDouble{} }

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func queryOf(t *testing.T, customer, parcel string) visibilityhttp.TrackingViewQuery {
	t.Helper()
	return visibilityhttp.TrackingViewQuery{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Customer: value(t, domain.NewCustomerAccountReference, customer),
		Parcel:   value(t, domain.NewTrackedParcelReference, parcel),
	}
}

// mixedView 造一个三态齐备的视图：里程碑展示、ETA 待确认、终局不展示、说明展示。
func mixedView(t *testing.T) domain.CustomerTrackingView {
	t.Helper()
	milestones, err := domain.ShowDimension(value(t, domain.NewViewContentReference, "milestones/v3"))
	if err != nil {
		t.Fatalf("show milestones: %v", err)
	}
	note, err := domain.ShowDimension(value(t, domain.NewViewContentReference, "note/v1"))
	if err != nil {
		t.Fatalf("show note: %v", err)
	}
	view, err := domain.PublishCustomerView(
		value(t, domain.NewCustomerViewVersionID, "view-7"),
		value(t, domain.NewCustomerAccountReference, "customer-1"),
		value(t, domain.NewTrackedParcelReference, "parcel-1"),
		value(t, domain.NewProjectionVersionID, "projection-9"),
		domain.CustomerViewDimensions{
			Milestones: milestones,
			ETA:        domain.PendDimension(),
			Final:      domain.WithholdDimension(),
			Note:       note,
		},
		time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("publish view: %v", err)
	}
	return view
}

func serve(
	t *testing.T,
	intake visibilityhttp.QueryIntake,
	reader visibilityhttp.TrackingViewReader,
	attributions *attributionDouble,
	method string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewQueryCustomerTrackingViewEndpoint(intake, reader, attributions)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, "/customer-tracking-view", nil))
	return recorder
}

// Covers: UC-VE-008 `AT-VE-150`「C1 授权用户按本账户包裹查询→只返回 C1 当前有效客户
// 视图和采用版本」与「四维三态如实透出不合成」——展示维带内容引用，待确认与不展示
// 维只有状态没有内容；采用版本由 version 承担，内部投影版本不进响应体。
func TestAnAuthorizedQueryReturnsTheCurrentViewVerbatim(t *testing.T) {
	reader := readerDouble{views: map[readerKey]domain.CustomerTrackingView{
		{value(t, domain.NewTenantID, "tenant-1"), value(t, domain.NewCustomerAccountReference, "customer-1"), value(t, domain.NewTrackedParcelReference, "parcel-1")}: mixedView(t),
	}}

	attributions := &attributionDouble{owners: map[string]string{"parcel-1": "customer-1"}}

	recorder := serve(t, intakeDouble{query: queryOf(t, "customer-1", "parcel-1")}, reader, attributions, http.MethodGet)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	// 读到视图即答，热路径不多一次读（ADR-0246 决定二）。
	if attributions.asked != 0 {
		t.Fatalf("读到视图之后又问了 %d 次归属", attributions.asked)
	}
	var body struct {
		Outcome string `json:"outcome"`
		View    struct {
			Version    string `json:"version"`
			Milestones struct {
				State      string `json:"state"`
				ContentRef string `json:"contentRef"`
			} `json:"milestones"`
			ETA struct {
				State      string `json:"state"`
				ContentRef string `json:"contentRef"`
			} `json:"eta"`
			Final struct {
				State string `json:"state"`
			} `json:"final"`
		} `json:"view"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "CURRENT_VIEW" {
		t.Fatalf("outcome = %q, want CURRENT_VIEW", body.Outcome)
	}
	if body.View.Version != "view-7" {
		t.Fatalf("view = %+v; 采用版本必须在场", body.View)
	}
	// 投影是内部对象，客户的采用版本由 version 承担；带出投影版本，同一 version 会随视图
	// 跟上新投影而对客户答出不同内容。
	if strings.Contains(recorder.Body.String(), "basedOnProjection") ||
		strings.Contains(recorder.Body.String(), "projection-9") {
		t.Fatalf("响应体带出了内部投影版本：%s", recorder.Body.String())
	}
	if body.View.Milestones.State != "SHOWN" || body.View.Milestones.ContentRef != "milestones/v3" {
		t.Fatalf("milestones = %+v, want SHOWN with its content reference", body.View.Milestones)
	}
	if body.View.ETA.State != "PENDING_CONFIRMATION" || body.View.ETA.ContentRef != "" {
		t.Fatalf("eta = %+v; 待确认维不得带内容", body.View.ETA)
	}
	if body.View.Final.State != "NOT_DISCLOSED" {
		t.Fatalf("final = %+v, want NOT_DISCLOSED", body.View.Final)
	}
}

// Covers: UC-VE-008 `AT-VE-151`「C1 使用属于 C2 的有效外部标识查询→返回不泄露存在性
// 的无权结果」与 ADR-0029 探针纪律——跨账户探针与真不存在的探针必须逐字节同答，
// 任何差异都是存在性泄露的信道。
func TestACrossAccountProbeIsIndistinguishableFromAMissingParcel(t *testing.T) {
	reader := readerDouble{views: map[readerKey]domain.CustomerTrackingView{
		{value(t, domain.NewTenantID, "tenant-1"), value(t, domain.NewCustomerAccountReference, "customer-1"), value(t, domain.NewTrackedParcelReference, "parcel-1")}: mixedView(t),
	}}

	attributions := &attributionDouble{owners: map[string]string{"parcel-1": "customer-1"}}

	crossAccount := serve(t, intakeDouble{query: queryOf(t, "customer-2", "parcel-1")}, reader, attributions, http.MethodGet)
	missing := serve(t, intakeDouble{query: queryOf(t, "customer-2", "parcel-404")}, reader, attributions, http.MethodGet)

	if crossAccount.Code != http.StatusOK || missing.Code != http.StatusOK {
		t.Fatalf("status = %d/%d, want 200/200（答案已形成：你名下无此视图）", crossAccount.Code, missing.Code)
	}
	if crossAccount.Body.String() != missing.Body.String() {
		t.Fatalf("cross-account = %q missing = %q; 两种探针必须同答", crossAccount.Body.String(), missing.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(crossAccount.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "VIEW_NOT_FOUND" {
		t.Fatalf("outcome = %q, want VIEW_NOT_FOUND", body.Outcome)
	}
}

// Covers: UC-VE-008 `AT-VE-188` 的 C1 半边「C1 的包裹所在委托已`已接受`、没有客户视图；C1 查询它→得当前无可见结果」——读不到
// 视图，而归属反查答唯一归属请求方账户，答 200 NO_VISIBLE_RESULT，不带视图（ADR-0246 决定一、二）。
func TestTheAttributedAccountGetsNoVisibleResultBeforeItsViewForms(t *testing.T) {
	attributions := &attributionDouble{owners: map[string]string{"parcel-1": "customer-1"}}

	recorder := serve(t, intakeDouble{query: queryOf(t, "customer-1", "parcel-1")}, readerDouble{}, attributions, http.MethodGet)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != `{"outcome":"NO_VISIBLE_RESULT"}` {
		t.Fatalf("body = %s, want {\"outcome\":\"NO_VISIBLE_RESULT\"}（没有视图可带）", got)
	}
}

// Covers: UC-VE-008 `AT-VE-188` 的 C2 半边「C2 用同一包裹号查询→得与包裹不存在逐字相同的答复」与 `AT-VE-151`——视图未形成时，
// 归属别的账户、归属不可确定、多份当前已接受委托同时声明（连声明方之一来查也在内），都与不存在的包裹答同一份字节
// VIEW_NOT_FOUND（ADR-0246 决定一、二、五）。
func TestEveryOtherUnformedViewIsByteForByteAMissingParcel(t *testing.T) {
	attributions := &attributionDouble{
		owners:    map[string]string{"parcel-1": "customer-1"},
		ambiguous: map[string]bool{"parcel-conflict": true},
	}
	ask := func(customer, parcel string) *httptest.ResponseRecorder {
		return serve(t, intakeDouble{query: queryOf(t, customer, parcel)}, readerDouble{}, attributions, http.MethodGet)
	}

	missing := ask("customer-2", "parcel-404")
	if missing.Code != http.StatusOK || strings.TrimSpace(missing.Body.String()) != `{"outcome":"VIEW_NOT_FOUND"}` {
		t.Fatalf("不存在的包裹 = %d %s, want 200 VIEW_NOT_FOUND", missing.Code, missing.Body.String())
	}
	for name, probe := range map[string]*httptest.ResponseRecorder{
		"归属别的账户":           ask("customer-2", "parcel-1"),
		"多份委托同时声明，别人来查":    ask("customer-2", "parcel-conflict"),
		"多份委托同时声明，声明方之一来查": ask("customer-1", "parcel-conflict"),
	} {
		if probe.Code != missing.Code || probe.Body.String() != missing.Body.String() {
			t.Errorf("%s = %d %q，与不存在的包裹 %d %q 可分辨", name, probe.Code, probe.Body.String(), missing.Code, missing.Body.String())
		}
	}
}

// Covers: UC-VE-008 `AT-VE-188` 末句与 `AT-VE-168` 的客户归属读取半边「归属读不动时返回未形成」——反查读不动答 500
// NO_ANSWER_FORMED，与视图读不回同一格，不降成 VIEW_NOT_FOUND，也不降成 NO_VISIBLE_RESULT（ADR-0246 决定二；ADR-0029）。
func TestAnUnreadableAttributionIsNoAnswerNotABusinessGrade(t *testing.T) {
	attributions := &attributionDouble{err: errors.New("parcel shipment down")}

	recorder := serve(t, intakeDouble{query: queryOf(t, "customer-1", "parcel-1")}, readerDouble{}, attributions, http.MethodGet)

	if recorder.Code != http.StatusInternalServerError || problemCode(t, recorder) != "NO_ANSWER_FORMED" {
		t.Fatalf("answer = %d %s, want 500 NO_ANSWER_FORMED", recorder.Code, recorder.Body.String())
	}
	assertNoOutcome(t, recorder)
}

// Covers: ADR-0246 决定二、三——只在读不到视图时问归属，每一次读不到视图的查询都问且只问一次，不论答出什么：读的次数对归属方、
// 别的账户与不存在的包裹相同。
func TestEveryViewlessQueryAsksTheAttributionExactlyOnce(t *testing.T) {
	for name, query := range map[string]visibilityhttp.TrackingViewQuery{
		"归属方":   queryOf(t, "customer-1", "parcel-1"),
		"别的账户":  queryOf(t, "customer-2", "parcel-1"),
		"不存在的号": queryOf(t, "customer-2", "parcel-404"),
		"多份声明":  queryOf(t, "customer-1", "parcel-conflict"),
	} {
		attributions := &attributionDouble{
			owners:    map[string]string{"parcel-1": "customer-1"},
			ambiguous: map[string]bool{"parcel-conflict": true},
		}
		serve(t, intakeDouble{query: query}, readerDouble{}, attributions, http.MethodGet)
		if attributions.asked != 1 {
			t.Errorf("%s：问了 %d 次归属，want 1", name, attributions.asked)
		}
	}
}

// Covers: 坏请求那一格的映射——Intake 构造不出查询即 400 MALFORMED_REQUEST，不带 outcome。没有账户授权的请求不落这一格，
// 答认证那几格，见 cmd/parcel-api 的 TestCustomerTrackingViewLineRefusesOtherFamiliesAndClientsWithoutTheGrant。
func TestAMalformedQueryIsBadRequestWithoutOutcome(t *testing.T) {
	recorder := serve(t, intakeDouble{err: visibilityhttp.ErrMalformedRequest}, readerDouble{}, noAttributions(), http.MethodGet)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json（首方客户族，ADR-0140 决定三）", got)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "MALFORMED_REQUEST" {
		t.Fatalf("code = %q, want MALFORMED_REQUEST", body.Code)
	}
	assertNoOutcome(t, recorder)
}

// Covers: UC-VE-008 `AT-VE-168` 的读侧半边「授权或视图结果保存失败→……不伪装为无
// 轨迹或无权查询」与 ADR-0022——库读不回是 5xx 无 outcome，不是 VIEW_NOT_FOUND。
func TestAnUnreadableStoreIsNoAnswerNotNotFound(t *testing.T) {
	reader := readerDouble{err: errors.New("store down")}
	attributions := &attributionDouble{owners: map[string]string{"parcel-1": "customer-1"}}

	recorder := serve(t, intakeDouble{query: queryOf(t, "customer-1", "parcel-1")}, reader, attributions, http.MethodGet)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "" || body.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("body = %+v; 故障不得伪装成业务答案", body)
	}
	// 视图读不回不是「读不到视图」：没有可依据的结论，归属问了也无从作答。
	if attributions.asked != 0 {
		t.Fatalf("视图读不回之后又问了 %d 次归属", attributions.asked)
	}
}

// Covers: 传输纪律——方法不对与翻译层故障各归其码，都不带业务 outcome。
func TestTransportGridsStayClosed(t *testing.T) {
	wrongMethod := serve(t, intakeDouble{query: queryOf(t, "customer-1", "parcel-1")}, readerDouble{}, noAttributions(), http.MethodPost)
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q, want 405/GET", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	intakeDown := serve(t, intakeDouble{err: errors.New("identity provider down")}, readerDouble{}, noAttributions(), http.MethodGet)
	if intakeDown.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", intakeDown.Code)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(intakeDown.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "INTAKE_FAILED" {
		t.Fatalf("code = %q, want INTAKE_FAILED", body.Code)
	}
}
