package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
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
	customer domain.CustomerAccountReference
	parcel   domain.TrackedParcelReference
}

type readerDouble struct {
	views map[readerKey]domain.CustomerTrackingView
	err   error
}

func (double readerDouble) FindCurrent(
	_ context.Context,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
) (domain.CustomerTrackingView, bool, error) {
	if double.err != nil {
		return domain.CustomerTrackingView{}, false, double.err
	}
	view, found := double.views[readerKey{customer: customer, parcel: parcel}]
	return view, found, nil
}

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

func serve(t *testing.T, intake visibilityhttp.QueryIntake, reader visibilityhttp.TrackingViewReader, method string) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewQueryCustomerTrackingViewEndpoint(intake, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, "/customer-tracking-view", nil))
	return recorder
}

// Covers: UC-VE-008 `AT-VE-150`「C1 授权用户按本账户包裹查询→只返回 C1 当前有效客户
// 视图和采用版本」与「四维三态如实透出不合成」——展示维带内容引用，待确认与不展示
// 维只有状态没有内容，版本与投影锚逐字段在场。
func TestAnAuthorizedQueryReturnsTheCurrentViewVerbatim(t *testing.T) {
	reader := readerDouble{views: map[readerKey]domain.CustomerTrackingView{
		{value(t, domain.NewCustomerAccountReference, "customer-1"), value(t, domain.NewTrackedParcelReference, "parcel-1")}: mixedView(t),
	}}

	recorder := serve(t, intakeDouble{query: queryOf(t, "customer-1", "parcel-1")}, reader, http.MethodGet)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Outcome string `json:"outcome"`
		View    struct {
			Version           string `json:"version"`
			BasedOnProjection string `json:"basedOnProjection"`
			Milestones        struct {
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
	if body.View.Version != "view-7" || body.View.BasedOnProjection != "projection-9" {
		t.Fatalf("view = %+v; 版本与投影锚必须逐字段在场", body.View)
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
		{value(t, domain.NewCustomerAccountReference, "customer-1"), value(t, domain.NewTrackedParcelReference, "parcel-1")}: mixedView(t),
	}}

	crossAccount := serve(t, intakeDouble{query: queryOf(t, "customer-2", "parcel-1")}, reader, http.MethodGet)
	missing := serve(t, intakeDouble{query: queryOf(t, "customer-2", "parcel-404")}, reader, http.MethodGet)

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

// Covers: UC-VE-008 `AT-VE-153` 的翻译半边「只有外部运单号但没有客户账户授权→不受理
// 查询，标识本身不作为访问凭证」——构造不出带账户的查询即 400，无 outcome。
func TestAQueryWithoutAccountAuthorityIsMalformed(t *testing.T) {
	recorder := serve(t, intakeDouble{err: visibilityhttp.ErrMalformedRequest}, readerDouble{}, http.MethodGet)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "MALFORMED_REQUEST" {
		t.Fatalf("code = %q, want MALFORMED_REQUEST", body.Error.Code)
	}
}

// Covers: UC-VE-008 `AT-VE-168` 的读侧半边「授权或视图结果保存失败→……不伪装为无
// 轨迹或无权查询」与 ADR-0022——库读不回是 5xx 无 outcome，不是 VIEW_NOT_FOUND。
func TestAnUnreadableStoreIsNoAnswerNotNotFound(t *testing.T) {
	reader := readerDouble{err: errors.New("store down")}

	recorder := serve(t, intakeDouble{query: queryOf(t, "customer-1", "parcel-1")}, reader, http.MethodGet)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "" || body.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("body = %+v; 故障不得伪装成业务答案", body)
	}
}

// Covers: 传输纪律——方法不对与翻译层故障各归其码，都不带业务 outcome。
func TestTransportGridsStayClosed(t *testing.T) {
	wrongMethod := serve(t, intakeDouble{query: queryOf(t, "customer-1", "parcel-1")}, readerDouble{}, http.MethodPost)
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q, want 405/GET", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	intakeDown := serve(t, intakeDouble{err: errors.New("identity provider down")}, readerDouble{}, http.MethodGet)
	if intakeDown.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", intakeDown.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(intakeDown.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "INTAKE_FAILED" {
		t.Fatalf("code = %q, want INTAKE_FAILED", body.Error.Code)
	}
}
