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
	locationTypeListPath      = "/warehouse-fulfillment-location-types"
	locationTypeRevisionsPath = "/warehouse-fulfillment-location-type-revisions"
)

type locationTypeListAnswer struct {
	Outcome       string             `json:"outcome"`
	LocationTypes []locationTypeEcho `json:"locationTypes"`
	Page          struct {
		Size  int     `json:"size"`
		Next  *string `json:"next"`
		Total *int64  `json:"total"`
	} `json:"page"`
}

func decodeLocationTypeList(t *testing.T, response *httptest.ResponseRecorder) locationTypeListAnswer {
	t.Helper()
	var answer locationTypeListAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("列表答复体 %q：%v", response.Body.String(), err)
	}
	return answer
}

// Covers: UC-WF-001 查阅（库位类型的部分）——各库位类型当前修订的列表：答 200 LOCATION_TYPES_LISTED，每行带当前修订、库位类型编码、名称、登记
// 责任方与登记时刻；租户取自认证结果、页大小取渠道契约值，不取自查询串；page 一格照目录读口先例。
func TestTheLocationTypeListAnswersTheCurrentRevisionsOfTheTenantsTypes(t *testing.T) {
	catalogue := &fakeLocationTypeCatalogue{
		current: map[string][]ports.LocationTypeRow{
			tenantA: {syntheticLocationTypeRow("SYN-LT-01", 2, false), syntheticLocationTypeRow("SYN-LT-02", 3, true)},
			tenantB: {syntheticLocationTypeRow("SYN-LT-OTHER", 1, false)},
		},
		next: "cursor-after-location-type",
	}
	fixture := newLocationTypeReadFixture(t, catalogue)

	// 查询串里自报的租户不是目录读口认的键：按册的声明解不开，答 400，不采信。
	assertProblem(t, "自报租户", get(fixture.list, locationTypeListPath+"?tenantId="+tenantB), http.StatusBadRequest, "MALFORMED_REQUEST")

	response := get(fixture.list, locationTypeListPath)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	assertKeys(t, "list", decodeObject(t, response), "outcome", "locationTypes", "page")
	answer := decodeLocationTypeList(t, response)
	if answer.Outcome != "LOCATION_TYPES_LISTED" || len(answer.LocationTypes) != 2 {
		t.Fatalf("答复 = %+v", answer)
	}
	first, second := answer.LocationTypes[0], answer.LocationTypes[1]
	if first.LocationTypeCode != "SYN-LT-01" || first.Revision != 2 || first.Name != "合成货架位" ||
		first.Registrant != operatorOne || first.RegisteredAt != "2026-11-18T09:00:00Z" || first.Deactivated {
		t.Fatalf("第一行 = %+v", first)
	}
	if second.LocationTypeCode != "SYN-LT-02" || second.Revision != 3 || !second.Deactivated {
		t.Fatalf("第二行 = %+v", second)
	}
	if answer.Page.Size != 200 || answer.Page.Next == nil || *answer.Page.Next != "cursor-after-location-type" ||
		answer.Page.Total == nil || *answer.Page.Total != 2 {
		t.Fatalf("page = %+v", answer.Page)
	}
	if catalogue.gotTenant != tenantA || catalogue.gotLimit != 200 {
		t.Fatalf("读口收到租户 %q、页大小 %d，want 认证交回的 %q 与 200", catalogue.gotTenant, catalogue.gotLimit, tenantA)
	}
}

// Covers: UC-WF-001 查阅与 AT-WF-009（库位类型的部分）——空册答 200 与空数组，不答未配置，也没有默认库位类型：locationTypes 是 []，不是 null。
func TestAnEmptyLocationTypeRegisterListsAnEmptyArray(t *testing.T) {
	fixture := newLocationTypeReadFixture(t, &fakeLocationTypeCatalogue{})

	response := get(fixture.list, locationTypeListPath)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"locationTypes":[]`) {
		t.Fatalf("空册答复 = %d %s，want 200 与 \"locationTypes\":[]", response.Code, response.Body.String())
	}
	if answer := decodeLocationTypeList(t, response); answer.Outcome != "LOCATION_TYPES_LISTED" ||
		answer.Page.Total == nil || *answer.Page.Total != 0 || answer.Page.Next != nil {
		t.Fatalf("答复 = %+v", answer)
	}
}

// Covers: ADR-0144——列表按 ports.LocationTypePages 解码：筛选维 deactivated 与排序维 locationTypeCode 照声明进读口；册没声明的维度、
// 词表外的筛选值答 400 MALFORMED_REQUEST 并带理由，不进读口。
func TestTheLocationTypeListDecodesItsQueryByTheCataloguesDeclaration(t *testing.T) {
	catalogue := &fakeLocationTypeCatalogue{}
	fixture := newLocationTypeReadFixture(t, catalogue)

	response := get(fixture.list, locationTypeListPath+"?deactivated=true&sort=locationTypeCode&q=SYN")
	if response.Code != http.StatusOK {
		t.Fatalf("合法查询：%d %s", response.Code, response.Body.String())
	}
	if filter := catalogue.gotQuery.Filters["deactivated"]; len(filter) != 1 || filter[0] != "true" {
		t.Fatalf("读口收到的筛选 = %v，want deactivated=true", catalogue.gotQuery.Filters)
	}
	if catalogue.gotQuery.Sort.Field != "locationTypeCode" || catalogue.gotQuery.Keyword != "SYN" {
		t.Fatalf("读口收到的排序 = %+v、检索词 = %q", catalogue.gotQuery.Sort, catalogue.gotQuery.Keyword)
	}

	listedBefore := catalogue.listed
	for name, target := range map[string]string{
		"dimension the catalogue does not declare": locationTypeListPath + "?warehouseCode=SYN-WH-01",
		"filter value outside the vocabulary":      locationTypeListPath + "?deactivated=maybe",
		"sort dimension not declared":              locationTypeListPath + "?sort=name",
		"broken percent escape":                    locationTypeListPath + "?q=%zz",
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

// Covers: AT-WF-010（库位类型的部分）——查阅口的信封各格：发行方未配置、令牌无效、没有主数据与运营查阅读授予（包括只持登记册配置写的
// 操作者）、身份依赖读不动，各答其格，读面一次都没被问；两个读口都一样。
func TestTheLocationTypeReadLinesAnswerTheEnvelopeGradesWithoutAskingTheReader(t *testing.T) {
	for _, testCase := range envelopeCases {
		t.Run(testCase.name, func(t *testing.T) {
			catalogue := &fakeLocationTypeCatalogue{}
			fixture := newLocationTypeReadFixture(t, catalogue)
			fixture.authenticator.err = testCase.err

			for name, handlerAndTarget := range map[string]struct {
				handler http.Handler
				target  string
			}{
				"list":      {fixture.list, locationTypeListPath},
				"revisions": {fixture.revisionsLine, locationTypeRevisionsPath + "?locationTypeCode=SYN-LT-01"},
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

// Covers: AT-WF-010（库位类型的部分）——先认证、后读查询串：令牌不过的调用方，带着坏查询也只看到 401，看不到查询校验的结果。
func TestLocationTypeAuthenticationComesBeforeTheQueryIsJudged(t *testing.T) {
	fixture := newLocationTypeReadFixture(t, &fakeLocationTypeCatalogue{})
	fixture.authenticator.err = warehousehttp.ErrOperatorCredentialRejected

	assertProblem(t, "list", get(fixture.list, locationTypeListPath+"?deactivated=maybe"), http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")
	assertProblem(t, "revisions", get(fixture.revisionsLine, locationTypeRevisionsPath+"?nothing=here"), http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")
}

// Covers: UC-WF-001「权限、隔离与审计」——读不动不形成答案：答 500 NO_ANSWER_FORMED，不折成空册或没有历史。
func TestALocationTypeReaderThatCannotBeReachedFormsNoAnswerInsteadOfAnEmptyRegister(t *testing.T) {
	fixture := newLocationTypeReadFixture(t, &fakeLocationTypeCatalogue{fail: errors.New("read side down")})

	assertProblem(t, "list", get(fixture.list, locationTypeListPath), http.StatusInternalServerError, "NO_ANSWER_FORMED")
	assertProblem(t, "revisions", get(fixture.revisionsLine, locationTypeRevisionsPath+"?locationTypeCode=SYN-LT-01"),
		http.StatusInternalServerError, "NO_ANSWER_FORMED")
}

// Covers: UC-WF-001 查阅「一个库位类型的修订史」——答 200 LOCATION_TYPE_REVISIONS_LISTED，顶层回显库位类型编码，各修订按读面给的次序（修订号
// 升序）原样交出，每一笔带当时的登记责任方与登记时刻；编码逐字交给读口，不去空白。
func TestTheLocationTypeRevisionHistoryAnswersEveryRevisionOfOneType(t *testing.T) {
	const code = " SYN-LT 01\t"
	catalogue := &fakeLocationTypeCatalogue{history: map[string][]ports.LocationTypeRow{
		tenantA + "\x00" + code: {syntheticLocationTypeRow(code, 1, false), syntheticLocationTypeRow(code, 2, true)},
	}}
	fixture := newLocationTypeReadFixture(t, catalogue)

	response := get(fixture.revisionsLine, locationTypeRevisionsPath+"?locationTypeCode="+url.QueryEscape(code))

	if response.Code != http.StatusOK {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	assertKeys(t, "revisions", decodeObject(t, response), "outcome", "locationTypeCode", "revisions")
	var answer struct {
		Outcome          string             `json:"outcome"`
		LocationTypeCode string             `json:"locationTypeCode"`
		Revisions        []locationTypeEcho `json:"revisions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Outcome != "LOCATION_TYPE_REVISIONS_LISTED" || answer.LocationTypeCode != code || len(answer.Revisions) != 2 ||
		answer.Revisions[0].Revision != 1 || answer.Revisions[1].Revision != 2 || !answer.Revisions[1].Deactivated ||
		answer.Revisions[0].Registrant != operatorOne {
		t.Fatalf("答复 = %+v", answer)
	}
	if catalogue.gotRevTenant != tenantA || catalogue.gotRevCode != code {
		t.Fatalf("读口收到租户 %q、库位类型编码 %q，want 认证交回的租户与逐字的编码", catalogue.gotRevTenant, catalogue.gotRevCode)
	}
}

// Covers: UC-WF-001「权限、隔离与审计」——别的租户的同编码库位类型在本租户一律查不到，不泄露是否存在：这个库位类型在本租户没有，与别的租户有
// 同编码库位类型，两种情形下答复体逐字节相同（200 与空数组），不答 404。
func TestALocationTypeOfAnotherTenantAnswersExactlyLikeOneThatDoesNotExist(t *testing.T) {
	nobody := newLocationTypeReadFixture(t, &fakeLocationTypeCatalogue{})
	elsewhere := newLocationTypeReadFixture(t, &fakeLocationTypeCatalogue{history: map[string][]ports.LocationTypeRow{
		tenantB + "\x00SYN-LT-SHARED": {syntheticLocationTypeRow("SYN-LT-SHARED", 1, false)},
	}})

	target := locationTypeRevisionsPath + "?locationTypeCode=SYN-LT-SHARED"
	absent, otherTenants := get(nobody.revisionsLine, target), get(elsewhere.revisionsLine, target)

	if absent.Code != http.StatusOK || !strings.Contains(absent.Body.String(), `"revisions":[]`) {
		t.Fatalf("没登记过的库位类型答 %d %s，want 200 与空数组", absent.Code, absent.Body.String())
	}
	if otherTenants.Code != absent.Code || otherTenants.Body.String() != absent.Body.String() {
		t.Fatalf("别的租户的同编码库位类型答 %d %s，与没登记过的库位类型答 %d %s 不同：泄露了是否存在",
			otherTenants.Code, otherTenants.Body.String(), absent.Code, absent.Body.String())
	}
}

// Covers: UC-WF-001 查阅——修订史口的查询串只收恰一个非空的 locationTypeCode：缺、重复、别的键、空白、写坏的百分号转义，都答 400
// MALFORMED_REQUEST 并带理由，不进读口；理由不回显取值。
func TestTheLocationTypeRevisionHistoryRefusesAQueryThatDoesNotNameExactlyOneType(t *testing.T) {
	catalogue := &fakeLocationTypeCatalogue{}
	fixture := newLocationTypeReadFixture(t, catalogue)
	for name, target := range map[string]string{
		"missing":                  locationTypeRevisionsPath,
		"given twice":              locationTypeRevisionsPath + "?locationTypeCode=A&locationTypeCode=B",
		"another key":              locationTypeRevisionsPath + "?locationTypeCode=A&limit=5",
		"only another key":         locationTypeRevisionsPath + "?code=A",
		"a warehouse code instead": locationTypeRevisionsPath + "?warehouseCode=A",
		"blank":                    locationTypeRevisionsPath + "?locationTypeCode=%20%20",
		"empty":                    locationTypeRevisionsPath + "?locationTypeCode=",
		"broken percent":           locationTypeRevisionsPath + "?locationTypeCode=SYN-SECRET%zz",
		"NUL in the code":          locationTypeRevisionsPath + "?locationTypeCode=A%00",
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
func TestTheLocationTypeReadLinesRefuseOtherMethodsBeforeAuthentication(t *testing.T) {
	catalogue := &fakeLocationTypeCatalogue{}
	fixture := newLocationTypeReadFixture(t, catalogue)
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

// Covers: ADR-0055 决定三——渠道未配置时 UnconfiguredIntake 对库位类型的两个读口同答 403 ACCESS_CHANNEL_NOT_CONFIGURED：不读查询串、不问读面。
func TestTheUnconfiguredIntakeAnswersTheLocationTypeReadLinesWithoutAskingAnything(t *testing.T) {
	catalogue := &fakeLocationTypeCatalogue{}
	list := warehousehttp.NewQueryLocationTypesEndpoint(warehousehttp.UnconfiguredIntake{}, catalogue)
	revisions := warehousehttp.NewQueryLocationTypeRevisionsEndpoint(warehousehttp.UnconfiguredIntake{}, catalogue)

	assertProblem(t, "列表", get(list, locationTypeListPath+"?deactivated=bogus"), http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	assertProblem(t, "修订史", get(revisions, locationTypeRevisionsPath), http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	if catalogue.listed != 0 || catalogue.revisions != 0 {
		t.Fatal("未配置仍问了读面")
	}
}

// 编译期锁缝：读面假件实现的就是库位类型各读口端点消费的读口。
var (
	_ warehousehttp.LocationTypeCatalogueReader = (*fakeLocationTypeCatalogue)(nil)
	_ warehousehttp.LocationTypeRevisionsReader = (*fakeLocationTypeCatalogue)(nil)
)
