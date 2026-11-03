package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证按运单号查询那一口（ADR-0356；UC-VE-008 `AT-VE-231` 至 `AT-VE-234`、`AT-VE-236` 的 HTTP 半边）：整单四格、逐件答复与按
// 包裹查逐字相同、游标翻页、坏请求不读委托、任一读不动整页未形成，以及认证与授予各格与按包裹查同形。parcel-shipment 那一侧是
// 替身，真库上的走法见 cmd/parcel-api 的装配用例。

// shipmentIntakeDouble 是按运单号查询的 Intake 替身：交回给定的查询键或错误。
type shipmentIntakeDouble struct {
	query visibilityhttp.ShipmentTrackingQuery
	err   error
}

func (double shipmentIntakeDouble) IntakeShipmentTrackingQuery(context.Context, *http.Request) (visibilityhttp.ShipmentTrackingQuery, error) {
	return double.query, double.err
}

// listedParcel 是替身委托上的一件：包裹号与客户包裹件号（空即没声明）。
type listedParcel struct {
	parcel, reference string
}

// shipmentLocatorDouble 是整单读口的替身。standing 不是逐件列出时原样答它；是时把 parcels 按位次 1..n 照页请求切给调用方。asked
// 记被问的页，err 非空即读不动。
type shipmentLocatorDouble struct {
	standing ports.ShipmentTrackingStanding
	parcels  []listedParcel
	err      error
	asked    []ports.ShipmentParcelPage
}

func (double *shipmentLocatorDouble) LocateCustomerShipment(
	_ context.Context, _ domain.TenantID, _ domain.CustomerAccountReference, _ domain.ShipmentDocumentNumberReference,
	page ports.ShipmentParcelPage,
) (ports.LocatedCustomerShipment, error) {
	double.asked = append(double.asked, page)
	if double.err != nil {
		return ports.LocatedCustomerShipment{}, double.err
	}
	if double.standing != ports.ShipmentParcelsListed {
		return ports.LocatedCustomerShipment{Standing: double.standing}, nil
	}
	located := ports.LocatedCustomerShipment{Standing: ports.ShipmentParcelsListed, Total: len(double.parcels)}
	for index, member := range double.parcels {
		position := index + 1
		if position <= page.AfterPosition {
			continue
		}
		if len(located.Parcels) == page.Limit {
			located.More = true
			break
		}
		parcel, err := domain.NewTrackedParcelReference(member.parcel)
		if err != nil {
			return ports.LocatedCustomerShipment{}, err
		}
		listed := ports.ShipmentParcel{Parcel: parcel, Position: position}
		if member.reference != "" {
			reference, err := domain.NewCustomerParcelReference(member.reference)
			if err != nil {
				return ports.LocatedCustomerShipment{}, err
			}
			listed.CustomerParcelReference, listed.HasCustomerParcelReference = reference, true
		}
		located.Parcels = append(located.Parcels, listed)
	}
	return located, nil
}

func shipmentQueryOf(t *testing.T, customer, number string, pageSize int) visibilityhttp.ShipmentTrackingQuery {
	t.Helper()
	return visibilityhttp.ShipmentTrackingQuery{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Customer: value(t, domain.NewCustomerAccountReference, customer),
		Number:   value(t, domain.NewShipmentDocumentNumberReference, number),
		PageSize: pageSize,
	}
}

func getShipmentTracking(endpoint http.Handler, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

// shipmentAnswer 是逐件列出那一格解开的形状；每件留原始字节，好与按包裹查的答复逐字比。
type shipmentAnswer struct {
	Outcome string            `json:"outcome"`
	Parcels []json.RawMessage `json:"parcels"`
	Page    struct {
		Size  int     `json:"size"`
		Next  *string `json:"next"`
		Total *int    `json:"total"`
	} `json:"page"`
}

func decodeShipmentAnswer(t *testing.T, response *httptest.ResponseRecorder) shipmentAnswer {
	t.Helper()
	var answer shipmentAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return answer
}

// Covers: `AT-VE-231`（HTTP 半边）与 ADR-0356 决定三的不变式——三件次序照 parcel-shipment 给的；每件带 parcelId，声明过件号的带
// customerParcelReference、没声明的整键缺席；去掉这两格，每件剩下的 outcome 与 view 与同一时刻按包裹查那一件的整个答复逐字相同：
// 有视图的答当前视图，视图未形成而唯一归属请求方的答当前无可见结果，被另一份已接受委托同时声明的答无权查询。
func TestEachListedParcelAnswersExactlyWhatTheByParcelQueryAnswers(t *testing.T) {
	reader := readerDouble{views: map[readerKey]domain.CustomerTrackingView{
		{value(t, domain.NewTenantID, "tenant-1"), value(t, domain.NewCustomerAccountReference, "customer-1"), value(t, domain.NewTrackedParcelReference, "parcel-1")}: mixedView(t),
	}}
	attributions := &attributionDouble{
		owners:    map[string]string{"parcel-1": "customer-1", "parcel-2": "customer-1"},
		ambiguous: map[string]bool{"parcel-3": true},
	}
	locator := &shipmentLocatorDouble{standing: ports.ShipmentParcelsListed, parcels: []listedParcel{
		{"parcel-1", "CUST-1"}, {"parcel-2", ""}, {"parcel-3", "CUST-3"},
	}}
	endpoint := visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
		shipmentIntakeDouble{query: shipmentQueryOf(t, "customer-1", "P20261102000001", 50)}, locator, reader, attributions)

	listed := getShipmentTracking(endpoint, "/customer-shipment-tracking?documentNumber=P20261102000001")
	if listed.Code != http.StatusOK {
		t.Fatalf("status = %d %s", listed.Code, listed.Body.String())
	}
	answer := decodeShipmentAnswer(t, listed)
	if answer.Outcome != "PARCELS_LISTED" || len(answer.Parcels) != 3 || answer.Page.Size != 50 || answer.Page.Next != nil ||
		answer.Page.Total == nil || *answer.Page.Total != 3 {
		t.Fatalf("整单 = %s", listed.Body.String())
	}

	wantOutcomes := []string{"CURRENT_VIEW", "NO_VISIBLE_RESULT", "VIEW_NOT_FOUND"}
	for index, raw := range answer.Parcels {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatal(err)
		}
		member := locator.parcels[index]
		if string(item["parcelId"]) != `"`+member.parcel+`"` {
			t.Fatalf("第 %d 件 parcelId = %s, want %s", index, item["parcelId"], member.parcel)
		}
		reference, declared := item["customerParcelReference"]
		if declared != (member.reference != "") || (declared && string(reference) != `"`+member.reference+`"`) {
			t.Fatalf("%s 的 customerParcelReference = %s（在场 %v）", member.parcel, reference, declared)
		}
		if _, present := item["replaces"]; present {
			t.Fatalf("%s 谱系未建模却带了 replaces", member.parcel)
		}
		delete(item, "parcelId")
		delete(item, "customerParcelReference")
		remainder, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}

		single := serveParcelQuery(t, reader, attributions, member.parcel)
		var singleBody map[string]json.RawMessage
		if err := json.Unmarshal(single.Body.Bytes(), &singleBody); err != nil {
			t.Fatal(err)
		}
		singleBytes, err := json.Marshal(singleBody)
		if err != nil {
			t.Fatal(err)
		}
		if string(remainder) != string(singleBytes) {
			t.Errorf("%s：列表里 %s，按包裹查 %s", member.parcel, remainder, singleBytes)
		}
		if outcome := outcomeOfBody(t, single); outcome != wantOutcomes[index] {
			t.Errorf("%s 答 %s, want %s", member.parcel, outcome, wantOutcomes[index])
		}
		// 不经解码重编也逐字相同：列表里那一件的原文，以按包裹查整个答复去掉开头的花括号收尾。
		if !strings.HasSuffix(string(raw), strings.TrimPrefix(strings.TrimSpace(single.Body.String()), "{")) {
			t.Errorf("%s：列表原文 %s 不以按包裹查的答复 %s 收尾", member.parcel, raw, single.Body.String())
		}
	}
}

// serveParcelQuery 用同一组视图与归属替身问按包裹查那一口。
func serveParcelQuery(t *testing.T, reader readerDouble, attributions *attributionDouble, parcel string) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewQueryCustomerTrackingViewEndpoint(intakeDouble{query: queryOf(t, "customer-1", parcel)}, reader, attributions)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/customer-tracking-view", nil))
	return recorder
}

// Covers: `AT-VE-233`、`AT-VE-232`（HTTP 半边）——待接受、没有服务、查无此单各只有 outcome，不读视图也不问归属；查无此单那一格不论
// 成因都是同一副字节（成因在 parcel-shipment 口上就不分，真库上几种成因同答见 cmd/parcel-api 的装配用例）。
func TestTheOtherStandingsAnswerOnlyTheirOutcome(t *testing.T) {
	for standing, want := range map[ports.ShipmentTrackingStanding]string{
		ports.ShipmentAwaitingAcceptance:   `{"outcome":"AWAITING_ACCEPTANCE"}`,
		ports.ShipmentClosedWithoutService: `{"outcome":"CLOSED_WITHOUT_SERVICE"}`,
		ports.ShipmentNotFound:             `{"outcome":"SHIPMENT_NOT_FOUND"}`,
	} {
		attributions := &attributionDouble{}
		endpoint := visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
			shipmentIntakeDouble{query: shipmentQueryOf(t, "customer-1", "P20261102000001", 50)},
			&shipmentLocatorDouble{standing: standing}, unreachableViewReader{t}, attributions)
		answer := getShipmentTracking(endpoint, "/customer-shipment-tracking?documentNumber=P20261102000001")
		if answer.Code != http.StatusOK || strings.TrimSpace(answer.Body.String()) != want {
			t.Errorf("%s = %d %s, want 200 %s", standing, answer.Code, answer.Body.String(), want)
		}
		if attributions.asked != 0 {
			t.Errorf("%s 问了归属", standing)
		}
	}
}

// Covers: `AT-VE-234` 的翻页半边——页大小取 Intake 按渠道契约定的那个，调用方给不了；顺着 page.next 逐页答完，每页 total 都是全集件数，
// 末页 next 为 null；下一页严格接在上一页末件之后，不跳件、不重出。
func TestTheParcelsArePagedByTheCursor(t *testing.T) {
	locator := &shipmentLocatorDouble{standing: ports.ShipmentParcelsListed, parcels: []listedParcel{
		{"parcel-1", ""}, {"parcel-2", ""}, {"parcel-3", ""}, {"parcel-4", ""}, {"parcel-5", ""},
	}}
	endpoint := visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
		shipmentIntakeDouble{query: shipmentQueryOf(t, "customer-1", "P20261102000001", 2)}, locator, readerDouble{}, &attributionDouble{})

	var read []string
	target := "/customer-shipment-tracking?documentNumber=P20261102000001"
	for page := 1; page <= 4; page++ {
		response := getShipmentTracking(endpoint, target)
		answer := decodeShipmentAnswer(t, response)
		if response.Code != http.StatusOK || answer.Page.Size != 2 || answer.Page.Total == nil || *answer.Page.Total != 5 {
			t.Fatalf("第 %d 页 = %d %s", page, response.Code, response.Body.String())
		}
		for _, raw := range answer.Parcels {
			var item struct {
				ParcelID string `json:"parcelId"`
			}
			if err := json.Unmarshal(raw, &item); err != nil {
				t.Fatal(err)
			}
			read = append(read, item.ParcelID)
		}
		if answer.Page.Next == nil {
			break
		}
		target = "/customer-shipment-tracking?documentNumber=P20261102000001&after=" + url.QueryEscape(*answer.Page.Next)
	}
	if strings.Join(read, ",") != "parcel-1,parcel-2,parcel-3,parcel-4,parcel-5" {
		t.Fatalf("逐页读到 %v", read)
	}
	if len(locator.asked) != 3 || locator.asked[1].AfterPosition != 2 || locator.asked[2].AfterPosition != 4 || locator.asked[0].Limit != 2 {
		t.Fatalf("交给 parcel-shipment 的页 = %+v", locator.asked)
	}
}

// Covers: `AT-VE-234` 的坏请求半边——解不开的游标、拿另一个号的游标来翻这个号、Intake 构造不出查询（自报账户、多给参数、缺号），都答
// 400 MALFORMED_REQUEST、不带 outcome，且一次都没问 parcel-shipment：结果与号是否存在、属于谁无关。
func TestBadRequestsNeverReadTheShipment(t *testing.T) {
	locator := &shipmentLocatorDouble{standing: ports.ShipmentParcelsListed, parcels: []listedParcel{{"parcel-1", ""}, {"parcel-2", ""}}}
	otherNumber := visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
		shipmentIntakeDouble{query: shipmentQueryOf(t, "customer-1", "P20261102000009", 1)}, locator, readerDouble{}, &attributionDouble{})
	first := decodeShipmentAnswer(t, getShipmentTracking(otherNumber, "/customer-shipment-tracking?documentNumber=P20261102000009"))
	if first.Page.Next == nil {
		t.Fatal("另一个号的第一页没给游标")
	}
	foreignCursor := *first.Page.Next
	locator.asked = nil

	endpoint := visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
		shipmentIntakeDouble{query: shipmentQueryOf(t, "customer-1", "P20261102000001", 1)}, locator, readerDouble{}, &attributionDouble{})
	for name, target := range map[string]string{
		"garbled cursor":          "/customer-shipment-tracking?documentNumber=P20261102000001&after=not-a-cursor",
		"empty cursor":            "/customer-shipment-tracking?documentNumber=P20261102000001&after=",
		"another number's cursor": "/customer-shipment-tracking?documentNumber=P20261102000001&after=" + url.QueryEscape(foreignCursor),
	} {
		response := getShipmentTracking(endpoint, target)
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Errorf("%s = %d %s, want 400 MALFORMED_REQUEST", name, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}
	malformed := visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
		shipmentIntakeDouble{err: visibilityhttp.ErrMalformedRequest}, locator, readerDouble{}, &attributionDouble{})
	if response := getShipmentTracking(malformed, "/customer-shipment-tracking?customerAccount=customer-2"); response.Code != http.StatusBadRequest {
		t.Errorf("Intake 立不起查询 = %d %s", response.Code, response.Body.String())
	}
	if len(locator.asked) != 0 {
		t.Fatalf("坏请求问了 parcel-shipment %d 次", len(locator.asked))
	}
}

// Covers: `AT-VE-236`（HTTP 半边）——委托读不动、某一件的视图读不动、某一件的归属读不动，整页都答 500 NO_ANSWER_FORMED、不带 outcome，
// 不降为查无此单，也不出半页。
func TestAnyUnreadablePieceAnswersNoAnswerFormedForTheWholePage(t *testing.T) {
	listed := func() *shipmentLocatorDouble {
		return &shipmentLocatorDouble{standing: ports.ShipmentParcelsListed, parcels: []listedParcel{{"parcel-1", ""}, {"parcel-2", ""}}}
	}
	viewed := readerDouble{views: map[readerKey]domain.CustomerTrackingView{
		{value(t, domain.NewTenantID, "tenant-1"), value(t, domain.NewCustomerAccountReference, "customer-1"), value(t, domain.NewTrackedParcelReference, "parcel-1")}: mixedView(t),
	}}
	for name, testCase := range map[string]struct {
		locator      *shipmentLocatorDouble
		reader       readerDouble
		attributions *attributionDouble
	}{
		"shipment unreadable":    {&shipmentLocatorDouble{err: errors.New("parcel shipment down")}, readerDouble{}, &attributionDouble{}},
		"view store unreadable":  {listed(), readerDouble{err: errors.New("view store down")}, &attributionDouble{}},
		"attribution unreadable": {listed(), viewed, &attributionDouble{err: errors.New("parcel shipment down")}},
	} {
		endpoint := visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
			shipmentIntakeDouble{query: shipmentQueryOf(t, "customer-1", "P20261102000001", 50)}, testCase.locator, testCase.reader, testCase.attributions)
		response := getShipmentTracking(endpoint, "/customer-shipment-tracking?documentNumber=P20261102000001")
		if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Errorf("%s = %d %s, want 500 NO_ANSWER_FORMED", name, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}
}

// Covers: 票 document-numbering/12 完成判据「认证、授予各格的形状与按包裹查一致」——同一种 Intake 失败在两口答同一个状态码、同一个
// 码、同样的响应头（requestId 逐请求生成，去掉再比），都不带 outcome；方法不对答 405 只许 GET。
func TestTheCustomerGradesMatchTheByParcelQuery(t *testing.T) {
	for name, failure := range map[string]error{
		"not configured":      visibilityhttp.ErrAccessChannelNotConfigured,
		"credential rejected": visibilityhttp.ErrCustomerClientCredentialRejected,
		"not granted":         visibilityhttp.ErrCustomerClientNotGranted,
		"dependency down":     visibilityhttp.ErrIdentityDependencyUnavailable,
		"malformed":           visibilityhttp.ErrMalformedRequest,
		"unexpected":          errors.New("tenant reference did not translate"),
	} {
		byNumber := getShipmentTracking(visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
			shipmentIntakeDouble{err: failure}, &shipmentLocatorDouble{}, readerDouble{}, &attributionDouble{}), "/customer-shipment-tracking")
		byParcel := serve(t, intakeDouble{err: failure}, readerDouble{}, noAttributions(), http.MethodGet)
		if byNumber.Code != byParcel.Code || problemCode(t, byNumber) != problemCode(t, byParcel) ||
			!reflect.DeepEqual(byNumber.Header(), byParcel.Header()) {
			t.Errorf("%s：按运单号 %d %s，按包裹 %d %s", name, byNumber.Code, byNumber.Body.String(), byParcel.Code, byParcel.Body.String())
		}
		assertNoOutcome(t, byNumber)
	}

	wrongMethod := httptest.NewRecorder()
	visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(shipmentIntakeDouble{}, &shipmentLocatorDouble{}, readerDouble{}, &attributionDouble{}).
		ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodPost, "/customer-shipment-tracking", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d Allow=%q", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}
}

// shipmentViewerDouble 是按运单号查询的认证方替身。
type shipmentViewerDouble struct {
	viewer visibilityhttp.CustomerViewer
	err    error
}

func (double shipmentViewerDouble) AuthenticateShipmentTrackingQuery(context.Context, visibilityhttp.PresentedCustomerCredential) (visibilityhttp.CustomerViewer, error) {
	return double.viewer, double.err
}

func shipmentIntake(t *testing.T, authenticator visibilityhttp.CustomerShipmentTrackingAuthenticator) *visibilityhttp.CustomerShipmentTrackingIntake {
	t.Helper()
	intake, err := visibilityhttp.NewCustomerShipmentTrackingIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

// Covers: ADR-0356 决定一——查询键的租户与账户取自认证结果，号逐字取自 documentNumber（两端空白也照收，不折算），页大小是渠道契约的
// 50，调用方给不了。
func TestShipmentTrackingIntakeKeysTheQueryByTheAuthenticatedAccount(t *testing.T) {
	intake := shipmentIntake(t, shipmentViewerDouble{viewer: viewerOf(t, "customer-1")})
	for raw, want := range map[string]string{
		"P20261102000001":    "P20261102000001",
		"%20P20261102000001": " P20261102000001",
	} {
		query, err := intake.IntakeShipmentTrackingQuery(context.Background(), customerQuery("/customer-shipment-tracking?documentNumber="+raw, "t"))
		if err != nil {
			t.Fatalf("%s：%v", raw, err)
		}
		if query != shipmentQueryOf(t, "customer-1", want, 50) {
			t.Fatalf("%s：query = %+v", raw, query)
		}
	}
	withCursor, err := intake.IntakeShipmentTrackingQuery(context.Background(), customerQuery("/customer-shipment-tracking?documentNumber=P1&after=abc", "t"))
	if err != nil || withCursor.Number.String() != "P1" {
		t.Fatalf("带游标：%+v %v", withCursor, err)
	}
}

// Covers: `AT-VE-234` 与 AT-VE-171 的 Intake 半边——查询串封闭：自报账户或租户、ADR-0144 的 sort 与 q、任何别的一格都答坏请求；号缺席、
// 给两次、全空白，游标给两次，解不开的段落，也答坏请求。先认证、后读查询串。
func TestShipmentTrackingQueryStringIsClosed(t *testing.T) {
	intake := shipmentIntake(t, shipmentViewerDouble{viewer: viewerOf(t, "customer-1")})
	for name, target := range map[string]string{
		"self-declared account": "/customer-shipment-tracking?documentNumber=P1&customerAccount=customer-2",
		"self-declared tenant":  "/customer-shipment-tracking?documentNumber=P1&tenant=tenant-2",
		"sort":                  "/customer-shipment-tracking?documentNumber=P1&sort=-position",
		"keyword":               "/customer-shipment-tracking?documentNumber=P1&q=x",
		"parcel":                "/customer-shipment-tracking?documentNumber=P1&parcel=parcel-1",
		"no number":             "/customer-shipment-tracking",
		"number twice":          "/customer-shipment-tracking?documentNumber=P1&documentNumber=P2",
		"blank number":          "/customer-shipment-tracking?documentNumber=%20",
		"cursor twice":          "/customer-shipment-tracking?documentNumber=P1&after=a&after=b",
		"bad percent escape":    "/customer-shipment-tracking?documentNumber=P1&%zz=x",
	} {
		if _, err := intake.IntakeShipmentTrackingQuery(context.Background(), customerQuery(target, "t")); !errors.Is(err, visibilityhttp.ErrMalformedRequest) {
			t.Errorf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}

	refused := shipmentIntake(t, shipmentViewerDouble{err: visibilityhttp.ErrCustomerClientNotGranted})
	_, err := refused.IntakeShipmentTrackingQuery(context.Background(), customerQuery("/customer-shipment-tracking?customerAccount=customer-2", "t"))
	if !errors.Is(err, visibilityhttp.ErrCustomerClientNotGranted) || errors.Is(err, visibilityhttp.ErrMalformedRequest) {
		t.Fatalf("认证没过：err = %v, want 只认 ErrCustomerClientNotGranted", err)
	}
	if _, err := visibilityhttp.NewCustomerShipmentTrackingIntake(nil); err == nil {
		t.Fatal("没有认证方的 Intake 构造出来了")
	}
}

// Covers: ADR-0356 决定一「值立不起来答 `400`。这一判只看输入，与号存不存在无关」，与 ADR-0029 按恢复动作分格——查询值含 NUL
// 或不是合法 UTF-8，库存不下它；两口都在 Intake 上答 400 MALFORMED_REQUEST、不带 outcome，一次都不读（委托、视图、归属）。若放进读，
// 库拒收参数会答成 500 NO_ANSWER_FORMED，引客户端退避重发一个永远不会好的请求。
func TestUnstorableQueryValuesAreBadRequestsOnBothLines(t *testing.T) {
	for name, raw := range map[string]string{"NUL": "P2026%00", "invalid UTF-8": "P2026%FF"} {
		byNumber := httptest.NewRecorder()
		visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(
			shipmentIntake(t, shipmentViewerDouble{viewer: viewerOf(t, "customer-1")}), untouchedLocator{t}, unreachableViewReader{t}, unreachableAttributions{t},
		).ServeHTTP(byNumber, customerQuery("/customer-shipment-tracking?documentNumber="+raw, "t"))
		if byNumber.Code != http.StatusBadRequest || problemCode(t, byNumber) != "MALFORMED_REQUEST" {
			t.Errorf("按运单号 %s = %d %s, want 400 MALFORMED_REQUEST", name, byNumber.Code, byNumber.Body.String())
		}
		assertNoOutcome(t, byNumber)

		byParcel := httptest.NewRecorder()
		visibilityhttp.NewQueryCustomerTrackingViewEndpoint(
			customerIntake(t, viewerDouble{viewer: viewerOf(t, "customer-1")}), unreachableViewReader{t}, unreachableAttributions{t},
		).ServeHTTP(byParcel, customerQuery("/customer-tracking-view?parcel="+raw, "t"))
		if byParcel.Code != http.StatusBadRequest || problemCode(t, byParcel) != "MALFORMED_REQUEST" {
			t.Errorf("按包裹 %s = %d %s, want 400 MALFORMED_REQUEST", name, byParcel.Code, byParcel.Body.String())
		}
		assertNoOutcome(t, byParcel)
	}
}

// untouchedLocator 是整单读口的替身：被问一次就判失败。
type untouchedLocator struct{ t *testing.T }

func (locator untouchedLocator) LocateCustomerShipment(
	context.Context, domain.TenantID, domain.CustomerAccountReference, domain.ShipmentDocumentNumberReference, ports.ShipmentParcelPage,
) (ports.LocatedCustomerShipment, error) {
	locator.t.Error("坏输入读了委托")
	return ports.LocatedCustomerShipment{}, nil
}
