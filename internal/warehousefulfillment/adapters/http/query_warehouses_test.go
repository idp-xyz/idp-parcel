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
	listPath      = "/warehouse-fulfillment-warehouses"
	revisionsPath = "/warehouse-fulfillment-warehouse-revisions"
)

type listAnswer struct {
	Outcome    string          `json:"outcome"`
	Warehouses []warehouseEcho `json:"warehouses"`
	Page       struct {
		Size  int     `json:"size"`
		Next  *string `json:"next"`
		Total *int64  `json:"total"`
	} `json:"page"`
}

func decodeList(t *testing.T, response *httptest.ResponseRecorder) listAnswer {
	t.Helper()
	var answer listAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("列表答复体 %q：%v", response.Body.String(), err)
	}
	return answer
}

// Covers: UC-WF-001 查阅——各仓当前修订的列表：答 200 WAREHOUSES_LISTED，每行带当前修订、登记责任方与登记时刻，业务时区与
// 同址物流节点没登记、没指名时整键缺席；租户取自认证结果、页大小取渠道契约值，不取自查询串；page 一格照目录读口先例。
func TestTheListAnswersTheCurrentRevisionsOfTheTenantsWarehouses(t *testing.T) {
	bare := syntheticRow("SYN-WH-02", 3, true)
	bare.HasBusinessTimeZone, bare.BusinessTimeZone = false, ""
	bare.HasColocatedNode, bare.ColocatedNode = false, ""
	catalogue := &fakeCatalogue{
		current: map[string][]ports.WarehouseRow{
			tenantA: {syntheticRow("SYN-WH-01", 2, false), bare},
			tenantB: {syntheticRow("SYN-WH-OTHER", 1, false)},
		},
		next: "cursor-after-SYN-WH-02",
	}
	fixture := newReadFixture(t, catalogue)

	response := get(fixture.list, listPath+"?tenantId="+tenantB)
	// 查询串里自报的租户不是目录读口认的键：按册的声明解不开，答 400，不采信。
	assertProblem(t, "自报租户", response, http.StatusBadRequest, "MALFORMED_REQUEST")

	response = get(fixture.list, listPath)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	assertKeys(t, "list", decodeObject(t, response), "outcome", "warehouses", "page")
	answer := decodeList(t, response)
	if answer.Outcome != "WAREHOUSES_LISTED" || len(answer.Warehouses) != 2 {
		t.Fatalf("答复 = %+v", answer)
	}
	first, second := answer.Warehouses[0], answer.Warehouses[1]
	if first.WarehouseCode != "SYN-WH-01" || first.Revision != 2 || first.Registrant != operatorOne ||
		first.RegisteredAt != "2026-11-18T09:00:00Z" || first.BusinessTimeZone != "Asia/Shanghai" || first.ColocatedNode != "SYN-NODE-01" {
		t.Fatalf("第一行 = %+v", first)
	}
	if second.WarehouseCode != "SYN-WH-02" || !second.Deactivated || second.BusinessTimeZone != "" || second.ColocatedNode != "" {
		t.Fatalf("第二行 = %+v", second)
	}
	var rows []map[string]json.RawMessage
	var raw struct {
		Warehouses []map[string]json.RawMessage `json:"warehouses"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	rows = raw.Warehouses
	if _, present := rows[1]["businessTimeZone"]; present {
		t.Fatalf("没登记的业务时区应整键缺席，不拿空串兼作没有：%v", keysOf(rows[1]))
	}
	if _, present := rows[1]["colocatedNode"]; present {
		t.Fatalf("没指名的同址物流节点应整键缺席：%v", keysOf(rows[1]))
	}
	if answer.Page.Size != 200 || answer.Page.Next == nil || *answer.Page.Next != "cursor-after-SYN-WH-02" ||
		answer.Page.Total == nil || *answer.Page.Total != 2 {
		t.Fatalf("page = %+v", answer.Page)
	}
	if catalogue.gotTenant != tenantA || catalogue.gotLimit != 200 {
		t.Fatalf("读口收到租户 %q、页大小 %d，want 认证交回的 %q 与 200", catalogue.gotTenant, catalogue.gotLimit, tenantA)
	}
}

// Covers: UC-WF-001 查阅与 AT-WF-009（仓的部分）——空册答 200 与空数组，不答未配置，也没有默认仓：warehouses 是 []，不是 null。
func TestAnEmptyRegisterListsAnEmptyArray(t *testing.T) {
	fixture := newReadFixture(t, &fakeCatalogue{})

	response := get(fixture.list, listPath)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"warehouses":[]`) {
		t.Fatalf("空册答复 = %d %s，want 200 与 \"warehouses\":[]", response.Code, response.Body.String())
	}
	if answer := decodeList(t, response); answer.Outcome != "WAREHOUSES_LISTED" || answer.Page.Total == nil || *answer.Page.Total != 0 || answer.Page.Next != nil {
		t.Fatalf("答复 = %+v", answer)
	}
}

// Covers: ADR-0144——列表按 ports.WarehousePages 解码：筛选维 deactivated 与排序维 warehouseCode 照声明进读口；册没声明的维度、
// 词表外的筛选值答 400 MALFORMED_REQUEST 并带理由，不进读口。
func TestTheListDecodesItsQueryByTheCataloguesDeclaration(t *testing.T) {
	catalogue := &fakeCatalogue{}
	fixture := newReadFixture(t, catalogue)

	response := get(fixture.list, listPath+"?deactivated=true&sort=warehouseCode&q=SYN")
	if response.Code != http.StatusOK {
		t.Fatalf("合法查询：%d %s", response.Code, response.Body.String())
	}
	if filter := catalogue.gotQuery.Filters["deactivated"]; len(filter) != 1 || filter[0] != "true" {
		t.Fatalf("读口收到的筛选 = %v，want deactivated=true", catalogue.gotQuery.Filters)
	}
	if catalogue.gotQuery.Sort.Field != "warehouseCode" || catalogue.gotQuery.Keyword != "SYN" {
		t.Fatalf("读口收到的排序 = %+v、检索词 = %q", catalogue.gotQuery.Sort, catalogue.gotQuery.Keyword)
	}

	listedBefore := catalogue.listed
	for name, target := range map[string]string{
		"dimension the catalogue does not declare": listPath + "?kind=AGENT",
		"filter value outside the vocabulary":      listPath + "?deactivated=maybe",
		"sort dimension not declared":              listPath + "?sort=name",
		"broken percent escape":                    listPath + "?q=%zz",
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

// Covers: AT-WF-010（仓的部分）——查阅口的信封各格：发行方未配置、令牌无效、没有主数据与运营查阅读授予（包括只持登记册配置写
// 的操作者）、身份依赖读不动，各答其格，读面一次都没被问；各读口都一样。
func TestTheReadLinesAnswerTheEnvelopeGradesWithoutAskingTheReader(t *testing.T) {
	for _, testCase := range envelopeCases {
		t.Run(testCase.name, func(t *testing.T) {
			catalogue := &fakeCatalogue{}
			fixture := newReadFixture(t, catalogue)
			fixture.authenticator.err = testCase.err

			for name, handlerAndTarget := range map[string]struct {
				handler http.Handler
				target  string
			}{
				"list":      {fixture.list, listPath},
				"revisions": {fixture.revisionsLine, revisionsPath + "?warehouseCode=SYN-WH-01"},
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

// Covers: AT-WF-010（仓的部分）——先认证、后读查询串：令牌不过的调用方，带着坏查询也只看到 401，看不到查询校验的结果。
func TestAuthenticationComesBeforeTheQueryIsJudged(t *testing.T) {
	fixture := newReadFixture(t, &fakeCatalogue{})
	fixture.authenticator.err = warehousehttp.ErrOperatorCredentialRejected

	assertProblem(t, "list", get(fixture.list, listPath+"?deactivated=maybe"), http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")
	assertProblem(t, "revisions", get(fixture.revisionsLine, revisionsPath+"?nothing=here"), http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")
}

// Covers: UC-WF-001「权限、隔离与审计」——读不动不形成答案：答 500 NO_ANSWER_FORMED，不折成空册或没有历史。
func TestAReaderThatCannotBeReachedFormsNoAnswerInsteadOfAnEmptyRegister(t *testing.T) {
	fixture := newReadFixture(t, &fakeCatalogue{fail: errors.New("read side down")})

	assertProblem(t, "list", get(fixture.list, listPath), http.StatusInternalServerError, "NO_ANSWER_FORMED")
	assertProblem(t, "revisions", get(fixture.revisionsLine, revisionsPath+"?warehouseCode=SYN-WH-01"), http.StatusInternalServerError, "NO_ANSWER_FORMED")
}

// Covers: UC-WF-001 查阅「一个仓的修订史」——答 200 WAREHOUSE_REVISIONS_LISTED，顶层回显仓编码，各修订按读面给的次序（修订号升序）
// 原样交出，每一笔带当时的登记责任方与登记时刻；仓编码逐字交给读口，不去空白。
func TestTheRevisionHistoryAnswersEveryRevisionOfOneWarehouse(t *testing.T) {
	catalogue := &fakeCatalogue{history: map[string][]ports.WarehouseRow{
		tenantA + "\x00" + " SYN-WH 01 ": {syntheticRow(" SYN-WH 01 ", 1, false), syntheticRow(" SYN-WH 01 ", 2, true)},
	}}
	fixture := newReadFixture(t, catalogue)

	response := get(fixture.revisionsLine, revisionsPath+"?warehouseCode="+url.QueryEscape(" SYN-WH 01 "))

	if response.Code != http.StatusOK {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	assertKeys(t, "revisions", decodeObject(t, response), "outcome", "warehouseCode", "revisions")
	var answer struct {
		Outcome       string          `json:"outcome"`
		WarehouseCode string          `json:"warehouseCode"`
		Revisions     []warehouseEcho `json:"revisions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Outcome != "WAREHOUSE_REVISIONS_LISTED" || answer.WarehouseCode != " SYN-WH 01 " || len(answer.Revisions) != 2 ||
		answer.Revisions[0].Revision != 1 || answer.Revisions[1].Revision != 2 || !answer.Revisions[1].Deactivated ||
		answer.Revisions[0].Registrant != operatorOne {
		t.Fatalf("答复 = %+v", answer)
	}
	if catalogue.gotRevTenant != tenantA || catalogue.gotRevCode != " SYN-WH 01 " {
		t.Fatalf("读口收到租户 %q、仓编码 %q，want 认证交回的租户与逐字的仓编码", catalogue.gotRevTenant, catalogue.gotRevCode)
	}
}

// Covers: UC-WF-001「权限、隔离与审计」——别的租户的同编码仓在本租户一律查不到，不泄露是否存在：这个仓在本租户没有，与别的租户
// 有同编码仓，两种情形下答复体逐字节相同（200 与空数组），不答 404。
func TestAWarehouseOfAnotherTenantAnswersExactlyLikeOneThatDoesNotExist(t *testing.T) {
	const code = "SYN-WH-SHARED"
	nobody := newReadFixture(t, &fakeCatalogue{})
	elsewhere := newReadFixture(t, &fakeCatalogue{history: map[string][]ports.WarehouseRow{
		tenantB + "\x00" + code: {syntheticRow(code, 1, false)},
	}})

	target := revisionsPath + "?warehouseCode=" + code
	absent, otherTenants := get(nobody.revisionsLine, target), get(elsewhere.revisionsLine, target)

	if absent.Code != http.StatusOK || !strings.Contains(absent.Body.String(), `"revisions":[]`) {
		t.Fatalf("没登记过的仓答 %d %s，want 200 与空数组", absent.Code, absent.Body.String())
	}
	if otherTenants.Code != absent.Code || otherTenants.Body.String() != absent.Body.String() {
		t.Fatalf("别的租户的同编码仓答 %d %s，与没登记过的仓答 %d %s 不同：泄露了是否存在",
			otherTenants.Code, otherTenants.Body.String(), absent.Code, absent.Body.String())
	}
}

// Covers: UC-WF-001 查阅——修订史口的查询串只收恰一个非空的 warehouseCode：缺、重复、别的键、空白、写坏的百分号转义，都答 400
// MALFORMED_REQUEST 并带理由，不进读口；理由不回显取值。
func TestTheRevisionHistoryRefusesAQueryThatDoesNotNameExactlyOneWarehouse(t *testing.T) {
	catalogue := &fakeCatalogue{}
	fixture := newReadFixture(t, catalogue)
	for name, target := range map[string]string{
		"missing":             revisionsPath,
		"given twice":         revisionsPath + "?warehouseCode=A&warehouseCode=B",
		"another key":         revisionsPath + "?warehouseCode=A&limit=5",
		"only another key":    revisionsPath + "?code=A",
		"blank":               revisionsPath + "?warehouseCode=%20%20",
		"empty":               revisionsPath + "?warehouseCode=",
		"broken percent":      revisionsPath + "?warehouseCode=SYN-SECRET%zz",
		"NUL in the code":     revisionsPath + "?warehouseCode=A%00",
		"semicolon separator": revisionsPath + "?warehouseCode=A;limit=5",
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
func TestTheReadLinesRefuseOtherMethodsBeforeAuthentication(t *testing.T) {
	catalogue := &fakeCatalogue{}
	fixture := newReadFixture(t, catalogue)
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

// 编译期锁缝：读面假件实现的就是各读口端点消费的读口。
var (
	_ warehousehttp.WarehouseCatalogueReader = (*fakeCatalogue)(nil)
	_ warehousehttp.WarehouseRevisionsReader = (*fakeCatalogue)(nil)
)
