package warehousehttp_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const (
	locationListPath      = "/warehouse-fulfillment-locations"
	locationRevisionsPath = "/warehouse-fulfillment-location-revisions"
)

type locationListAnswer struct {
	Outcome   string         `json:"outcome"`
	Locations []locationEcho `json:"locations"`
	Page      struct {
		Size  int     `json:"size"`
		Next  *string `json:"next"`
		Total *int64  `json:"total"`
	} `json:"page"`
}

func decodeLocationList(t *testing.T, response *httptest.ResponseRecorder) locationListAnswer {
	t.Helper()
	var answer locationListAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("列表答复体 %q：%v", response.Body.String(), err)
	}
	return answer
}

// Covers: UC-WF-001 查阅（库位的部分）——各库位当前修订的列表：答 200 LOCATIONS_LISTED，每行带当前修订、仓编码、库位编码、所属库区、
// 库位类型与优先级（没有就整键缺席）、登记责任方与登记时刻；租户取自认证结果、页大小取渠道契约值，不取自查询串；page 一格照目录读口先例。
func TestTheLocationListAnswersTheCurrentRevisionsOfTheTenantsLocations(t *testing.T) {
	typed, priority := "SYN-LT-01", int32(0)
	withBoth := syntheticLocationRow("SYN-WH-01", "SYN-LOC-01", 2, false)
	withBoth.LocationTypeCode, withBoth.Priority = &typed, &priority
	catalogue := &fakeLocationCatalogue{
		current: map[string][]ports.LocationRow{
			tenantA: {withBoth, syntheticLocationRow("SYN-WH-02", "SYN-LOC-01", 3, true)},
			tenantB: {syntheticLocationRow("SYN-WH-OTHER", "SYN-LOC-OTHER", 1, false)},
		},
		next: "cursor-after-location",
	}
	fixture := newLocationReadFixture(t, catalogue)

	// 查询串里自报的租户不是目录读口认的键：按册的声明解不开，答 400，不采信。
	assertProblem(t, "自报租户", get(fixture.list, locationListPath+"?tenantId="+tenantB), http.StatusBadRequest, "MALFORMED_REQUEST")

	response := get(fixture.list, locationListPath)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	assertKeys(t, "list", decodeObject(t, response), "outcome", "locations", "page")
	answer := decodeLocationList(t, response)
	if answer.Outcome != "LOCATIONS_LISTED" || len(answer.Locations) != 2 {
		t.Fatalf("答复 = %+v", answer)
	}
	first, second := answer.Locations[0], answer.Locations[1]
	if first.WarehouseCode != "SYN-WH-01" || first.LocationCode != "SYN-LOC-01" || first.Revision != 2 || first.ZoneCode != "SYN-ZONE-01" ||
		first.LocationTypeCode == nil || *first.LocationTypeCode != "SYN-LT-01" || first.Priority == nil || *first.Priority != 0 ||
		first.Registrant != operatorOne || first.RegisteredAt != "2026-11-18T09:00:00Z" || first.Deactivated {
		t.Fatalf("第一行 = %+v", first)
	}
	if second.WarehouseCode != "SYN-WH-02" || second.Revision != 3 || !second.Deactivated || second.LocationTypeCode != nil || second.Priority != nil {
		t.Fatalf("第二行 = %+v", second)
	}
	// 没有库位类型与优先级的行，那两个键整键缺席。
	var raw struct {
		Locations []map[string]json.RawMessage `json:"locations"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, "没有类型与优先级的行", raw.Locations[1], "warehouseCode", "locationCode", "revision", "zoneCode", "deactivated", "registrant", "registeredAt")
	assertKeys(t, "带类型与优先级 0 的行", raw.Locations[0], "warehouseCode", "locationCode", "revision", "zoneCode", "locationTypeCode", "priority", "deactivated", "registrant", "registeredAt")
	if answer.Page.Size != 200 || answer.Page.Next == nil || *answer.Page.Next != "cursor-after-location" ||
		answer.Page.Total == nil || *answer.Page.Total != 2 {
		t.Fatalf("page = %+v", answer.Page)
	}
	if catalogue.gotTenant != tenantA || catalogue.gotLimit != 200 {
		t.Fatalf("读口收到租户 %q、页大小 %d，want 认证交回的 %q 与 200", catalogue.gotTenant, catalogue.gotLimit, tenantA)
	}
}

// Covers: UC-WF-001 查阅与 AT-WF-009（库位的部分）——空册答 200 与空数组，不答未配置，也没有默认库位：locations 是 []，不是 null。
func TestAnEmptyLocationRegisterListsAnEmptyArray(t *testing.T) {
	fixture := newLocationReadFixture(t, &fakeLocationCatalogue{})

	response := get(fixture.list, locationListPath)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"locations":[]`) {
		t.Fatalf("空册答复 = %d %s，want 200 与 \"locations\":[]", response.Code, response.Body.String())
	}
	if answer := decodeLocationList(t, response); answer.Outcome != "LOCATIONS_LISTED" || answer.Page.Total == nil || *answer.Page.Total != 0 || answer.Page.Next != nil {
		t.Fatalf("答复 = %+v", answer)
	}
}

// Covers: ADR-0144——列表按 ports.LocationPages 解码：筛选维 warehouseCode、zoneCode、deactivated 与排序维 warehouseCode 照声明进读口；
// 册没声明的维度、词表外的筛选值答 400 MALFORMED_REQUEST 并带理由，不进读口。
func TestTheLocationListDecodesItsQueryByTheCataloguesDeclaration(t *testing.T) {
	catalogue := &fakeLocationCatalogue{}
	fixture := newLocationReadFixture(t, catalogue)

	response := get(fixture.list, locationListPath+"?warehouseCode=SYN-WH-01&zoneCode=SYN-ZONE-01&deactivated=true&sort=warehouseCode&q=SYN")
	if response.Code != http.StatusOK {
		t.Fatalf("合法查询：%d %s", response.Code, response.Body.String())
	}
	if filter := catalogue.gotQuery.Filters["deactivated"]; len(filter) != 1 || filter[0] != "true" {
		t.Fatalf("读口收到的筛选 = %v，want deactivated=true", catalogue.gotQuery.Filters)
	}
	if filter := catalogue.gotQuery.Filters["warehouseCode"]; len(filter) != 1 || filter[0] != "SYN-WH-01" {
		t.Fatalf("读口收到的筛选 = %v，want warehouseCode=SYN-WH-01", catalogue.gotQuery.Filters)
	}
	if filter := catalogue.gotQuery.Filters["zoneCode"]; len(filter) != 1 || filter[0] != "SYN-ZONE-01" {
		t.Fatalf("读口收到的筛选 = %v，want zoneCode=SYN-ZONE-01", catalogue.gotQuery.Filters)
	}
	if catalogue.gotQuery.Sort.Field != "warehouseCode" || catalogue.gotQuery.Keyword != "SYN" {
		t.Fatalf("读口收到的排序 = %+v、检索词 = %q", catalogue.gotQuery.Sort, catalogue.gotQuery.Keyword)
	}

	listedBefore := catalogue.listed
	for name, target := range map[string]string{
		"dimension the catalogue does not declare": locationListPath + "?priority=1",
		"filter value outside the vocabulary":      locationListPath + "?deactivated=maybe",
		"sort dimension not declared":              locationListPath + "?sort=locationCode",
		"empty warehouse filter":                   locationListPath + "?warehouseCode=",
		"empty zone filter":                        locationListPath + "?zoneCode=",
		"broken percent escape":                    locationListPath + "?q=%zz",
	} {
		response := get(fixture.list, target)
		object := assertProblem(t, name, response, http.StatusBadRequest, "MALFORMED_REQUEST")
		if stringMember(t, object, "detail") == "" {
			t.Fatalf("%s：答复没带理由：%s", name, response.Body.String())
		}
	}
	if catalogue.listed != listedBefore {
		t.Fatalf("解不开的查询进了读口 %d 次", catalogue.listed-listedBefore)
	}
}

// Covers: AT-WF-010（库位的部分）——查阅口的信封各格：发行方未配置、令牌无效、没有主数据与运营查阅读授予（包括只持登记册配置写的
// 操作者）、身份依赖读不动，各答其格，读面一次都没被问；两个读口都一样。
func TestTheLocationReadLinesAnswerTheEnvelopeGradesWithoutAskingTheReader(t *testing.T) {
	for _, testCase := range envelopeCases {
		t.Run(testCase.name, func(t *testing.T) {
			catalogue := &fakeLocationCatalogue{}
			fixture := newLocationReadFixture(t, catalogue)
			fixture.authenticator.err = testCase.err

			for name, handlerAndTarget := range map[string]struct {
				handler http.Handler
				target  string
			}{
				"list":      {fixture.list, locationListPath},
				"revisions": {fixture.revisionsLine, locationRevisionsPath + "?warehouseCode=SYN-WH-01&locationCode=SYN-LOC-01"},
			} {
				response := get(handlerAndTarget.handler, handlerAndTarget.target)
				object := assertProblem(t, name, response, testCase.status, testCase.code)
				if _, present := object["detail"]; present && (testCase.status == http.StatusForbidden || testCase.status >= 500) {
					t.Fatalf("%s：%d 的问题详情带了 detail", name, testCase.status)
				}
			}
			if catalogue.listed != 0 || catalogue.revisions != 0 {
				t.Fatalf("信封一层的拒绝问了读面：列表 %d 次、修订史 %d 次", catalogue.listed, catalogue.revisions)
			}
		})
	}
}

// Covers: AT-WF-010（库位的部分）——先认证、后读查询串：令牌不过的调用方，带着坏查询也只看到 401，看不到查询校验的结果。
func TestLocationAuthenticationComesBeforeTheQueryIsJudged(t *testing.T) {
	fixture := newLocationReadFixture(t, &fakeLocationCatalogue{})
	fixture.authenticator.err = warehousehttp.ErrOperatorCredentialRejected

	assertProblem(t, "list", get(fixture.list, locationListPath+"?deactivated=maybe"), http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")
	assertProblem(t, "revisions", get(fixture.revisionsLine, locationRevisionsPath+"?nothing=here"), http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")
}

// Covers: UC-WF-001「权限、隔离与审计」——读不动不形成答案：答 500 NO_ANSWER_FORMED，不折成空册或没有历史。
func TestALocationReaderThatCannotBeReachedFormsNoAnswerInsteadOfAnEmptyRegister(t *testing.T) {
	fixture := newLocationReadFixture(t, &fakeLocationCatalogue{fail: errors.New("read side down")})

	assertProblem(t, "list", get(fixture.list, locationListPath), http.StatusInternalServerError, "NO_ANSWER_FORMED")
	assertProblem(t, "revisions", get(fixture.revisionsLine, locationRevisionsPath+"?warehouseCode=SYN-WH-01&locationCode=SYN-LOC-01"),
		http.StatusInternalServerError, "NO_ANSWER_FORMED")
}

// Covers: UC-WF-001 查阅「一个库位的修订史」——答 200 LOCATION_REVISIONS_LISTED，顶层回显仓编码与库位编码，各修订按读面给的次序（修订号
// 升序）原样交出，每一笔带当时的登记责任方与登记时刻；两个编码逐字交给读口，不去空白。
func TestTheLocationRevisionHistoryAnswersEveryRevisionOfOneLocation(t *testing.T) {
	const warehouse, location = " SYN-WH 01 ", "\tSYN-LOC 01"
	catalogue := &fakeLocationCatalogue{history: map[string][]ports.LocationRow{
		tenantA + "\x00" + warehouse + "\x00" + location: {syntheticLocationRow(warehouse, location, 1, false), syntheticLocationRow(warehouse, location, 2, true)},
	}}
	fixture := newLocationReadFixture(t, catalogue)

	response := get(fixture.revisionsLine,
		locationRevisionsPath+"?warehouseCode="+url.QueryEscape(warehouse)+"&locationCode="+url.QueryEscape(location))

	if response.Code != http.StatusOK {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	assertKeys(t, "revisions", decodeObject(t, response), "outcome", "warehouseCode", "locationCode", "revisions")
	var answer struct {
		Outcome       string         `json:"outcome"`
		WarehouseCode string         `json:"warehouseCode"`
		LocationCode  string         `json:"locationCode"`
		Revisions     []locationEcho `json:"revisions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Outcome != "LOCATION_REVISIONS_LISTED" || answer.WarehouseCode != warehouse || answer.LocationCode != location || len(answer.Revisions) != 2 ||
		answer.Revisions[0].Revision != 1 || answer.Revisions[1].Revision != 2 || !answer.Revisions[1].Deactivated ||
		answer.Revisions[0].Registrant != operatorOne {
		t.Fatalf("答复 = %+v", answer)
	}
	if catalogue.gotRevTenant != tenantA || catalogue.gotRevWH != warehouse || catalogue.gotRevCode != location {
		t.Fatalf("读口收到租户 %q、仓编码 %q、库位编码 %q，want 认证交回的租户与逐字的两个编码",
			catalogue.gotRevTenant, catalogue.gotRevWH, catalogue.gotRevCode)
	}
}

// Covers: UC-WF-001「权限、隔离与审计」与 AT-WF-003——别的租户的同编码库位在本租户一律查不到，不泄露是否存在：这个库位在本租户
// 没有，与别的租户有同编码库位，两种情形下答复体逐字节相同（200 与空数组），不答 404。
func TestALocationOfAnotherTenantAnswersExactlyLikeOneThatDoesNotExist(t *testing.T) {
	nobody := newLocationReadFixture(t, &fakeLocationCatalogue{})
	elsewhere := newLocationReadFixture(t, &fakeLocationCatalogue{history: map[string][]ports.LocationRow{
		tenantB + "\x00SYN-WH-01\x00SYN-LOC-SHARED": {syntheticLocationRow("SYN-WH-01", "SYN-LOC-SHARED", 1, false)},
	}})

	target := locationRevisionsPath + "?warehouseCode=SYN-WH-01&locationCode=SYN-LOC-SHARED"
	absent, otherTenants := get(nobody.revisionsLine, target), get(elsewhere.revisionsLine, target)

	if absent.Code != http.StatusOK || !strings.Contains(absent.Body.String(), `"revisions":[]`) {
		t.Fatalf("没登记过的库位答 %d %s，want 200 与空数组", absent.Code, absent.Body.String())
	}
	if otherTenants.Code != absent.Code || otherTenants.Body.String() != absent.Body.String() {
		t.Fatalf("别的租户的同编码库位答 %d %s，与没登记过的库位答 %d %s 不同：泄露了是否存在",
			otherTenants.Code, otherTenants.Body.String(), absent.Code, absent.Body.String())
	}
}

// Covers: UC-WF-001 查阅——修订史口的查询串只收恰各一个非空的 warehouseCode 与 locationCode：缺、重复、别的键、空白、写坏的百分号转义，
// 都答 400 MALFORMED_REQUEST 并带理由，不进读口；理由不回显取值。
func TestTheLocationRevisionHistoryRefusesAQueryThatDoesNotNameExactlyOneLocation(t *testing.T) {
	catalogue := &fakeLocationCatalogue{}
	fixture := newLocationReadFixture(t, catalogue)
	for name, target := range map[string]string{
		"missing":                  locationRevisionsPath,
		"location code missing":    locationRevisionsPath + "?warehouseCode=A",
		"warehouse code missing":   locationRevisionsPath + "?locationCode=L",
		"warehouse given twice":    locationRevisionsPath + "?warehouseCode=A&warehouseCode=B&locationCode=L",
		"location given twice":     locationRevisionsPath + "?warehouseCode=A&locationCode=L&locationCode=M",
		"another key":              locationRevisionsPath + "?warehouseCode=A&locationCode=L&limit=5",
		"zone code instead":        locationRevisionsPath + "?warehouseCode=A&zoneCode=Z",
		"only another key":         locationRevisionsPath + "?code=A",
		"warehouse blank":          locationRevisionsPath + "?warehouseCode=%20%20&locationCode=L",
		"location blank":           locationRevisionsPath + "?warehouseCode=A&locationCode=%20",
		"location empty":           locationRevisionsPath + "?warehouseCode=A&locationCode=",
		"broken percent":           locationRevisionsPath + "?warehouseCode=SYN-SECRET%zz&locationCode=L",
		"NUL in the location code": locationRevisionsPath + "?warehouseCode=A&locationCode=L%00",
		"semicolon separator":      locationRevisionsPath + "?warehouseCode=A;locationCode=L",
	} {
		t.Run(name, func(t *testing.T) {
			response := get(fixture.revisionsLine, target)
			object := assertProblem(t, name, response, http.StatusBadRequest, "MALFORMED_REQUEST")
			if stringMember(t, object, "detail") == "" || strings.Contains(response.Body.String(), "SYN-SECRET") {
				t.Fatalf("答复要带理由、且不回显取值：%s", response.Body.String())
			}
		})
	}
	if catalogue.revisions != 0 {
		t.Fatalf("解不开的查询进了读口 %d 次", catalogue.revisions)
	}
}

// Covers: ADR-0055 「未配置不改变方法约束」——各读口只收 GET，方法不对答 405 并带 Allow，不认证、不问读面。
func TestTheLocationReadLinesRefuseOtherMethodsBeforeAuthentication(t *testing.T) {
	catalogue := &fakeLocationCatalogue{}
	fixture := newLocationReadFixture(t, catalogue)
	for name, handler := range map[string]http.Handler{"list": fixture.list, "revisions": fixture.revisionsLine} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(method, "/x", strings.NewReader(`{}`)))
			assertProblem(t, name+" "+method, response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			if allow := response.Header().Get("Allow"); allow != http.MethodGet {
				t.Fatalf("%s %s：Allow = %q，want GET", name, method, allow)
			}
		}
	}
	if fixture.authenticator.asked != 0 || catalogue.listed != 0 || catalogue.revisions != 0 {
		t.Fatalf("方法不对仍认证了 %d 次、问读面 %d 次", fixture.authenticator.asked, catalogue.listed+catalogue.revisions)
	}
}

// Covers: ADR-0055 决定三——渠道未配置时 UnconfiguredIntake 对库位的两个读口同答 403 ACCESS_CHANNEL_NOT_CONFIGURED：不读查询串、不问读面。
func TestTheUnconfiguredIntakeAnswersTheLocationReadLinesWithoutAskingAnything(t *testing.T) {
	catalogue := &fakeLocationCatalogue{}
	list := warehousehttp.NewQueryLocationsEndpoint(warehousehttp.UnconfiguredIntake{}, catalogue)
	revisions := warehousehttp.NewQueryLocationRevisionsEndpoint(warehousehttp.UnconfiguredIntake{}, catalogue)

	assertProblem(t, "列表", get(list, locationListPath+"?deactivated=bogus"), http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	assertProblem(t, "修订史", get(revisions, locationRevisionsPath), http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	if catalogue.listed != 0 || catalogue.revisions != 0 {
		t.Fatal("未配置仍问了读面")
	}
}

// 编译期锁缝：读面假件实现的就是库位各读口端点消费的读口。
var (
	_ warehousehttp.LocationCatalogueReader = (*fakeLocationCatalogue)(nil)
	_ warehousehttp.LocationRevisionsReader = (*fakeLocationCatalogue)(nil)
)
