package nodeopshttp_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件证送站预报与送站交接单两条列表迁到 ADR-0144 之后的传输面（票 catalogue-read-pagination-rollout/06）：既有的节点、单据号与
// 区间参数登为选择器，原义不变、照旧读成筛选；翻页、排序与 q 照各自的声明解成查询对象原样交给读口，答复带 page；游标只在铸它的
// 那一条列表、那组条件下有效。夹具全为合成（S 级）。

// scopeReadNamed 按名字从 scopeReads 里挑一条读口：名单的次序归定义它的那个文件管，这里不按下标取。
func scopeReadNamed(t *testing.T, name string) scopeRead {
	t.Helper()
	for _, read := range scopeReads {
		if read.name == name {
			return read
		}
	}
	t.Fatalf("scopeReads 里没有 %q", name)
	return scopeRead{}
}

// Covers: ADR-0144 决定四、五、六——两条列表的答复都带 page（size 取 Intake 给的页大小，next 与 total 原样带出，空册 total 为 0、
// next 为 null）；排序与 q 解成查询对象交给读口，节点参数照旧读成筛选。
func TestDeliveryScopeListsCarryThePageAndPassTheDecodedQuery(t *testing.T) {
	cases := []struct {
		read     scopeRead
		key      string
		query    string
		wantSort cataloguepage.Sort
	}{
		{scopeReadNamed(t, "pre-advice list"), "preAdvices", "?node=SYN-NODE-1&sort=expectedArrivalAt&q=SYN", cataloguepage.Sort{Field: "expectedArrivalAt"}},
		{scopeReadNamed(t, "handover note list"), "handoverNotes", "?node=SYN-NODE-1&sort=openedAt&q=SYN", cataloguepage.Sort{Field: "openedAt"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.read.name, func(t *testing.T) {
			reader := &stubDeliveryScopeCatalogue{next: "SYN-cursor", total: 7}
			response := testCase.read.serve(grantedIntake(), reader, http.MethodGet, testCase.read.target+testCase.query)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if page := pageOf(t, response); string(page["size"]) != "25" || string(page["next"]) != `"SYN-cursor"` || string(page["total"]) != "7" {
				t.Fatalf("page 走样：%s", response.Body.String())
			}
			if got := reader.gotQuery; got.Sort != testCase.wantSort || got.Keyword != "SYN" || got.After != nil || len(got.Filters) != 0 {
				t.Fatalf("读口收到的查询走样：%+v", got)
			}
			if reader.gotFilter.Node.String() != "SYN-NODE-1" {
				t.Fatalf("节点参数没有照旧读成筛选：%+v", reader.gotFilter)
			}

			empty := testCase.read.serve(grantedIntake(), &stubDeliveryScopeCatalogue{}, http.MethodGet, testCase.read.target)
			if got := arrayAt(t, empty, testCase.key); got != "[]" {
				t.Fatalf("空册 %s = %s, want []", testCase.key, got)
			}
			if page := pageOf(t, empty); string(page["next"]) != "null" || string(page["total"]) != "0" {
				t.Fatalf("空册的 page 走样：%s", empty.Body.String())
			}
		})
	}
}

// Covers: ADR-0144 决定四「有状态的册至少放出状态一维」——预报的 withdrawn、交接单的 closed 照各自声明解成查询对象的筛选，维内为或、
// 去重升序，原样交给读口；选择器照旧读成筛选，不进查询对象。
func TestDeliveryScopeListsHandTheStatusDimensionToTheReader(t *testing.T) {
	for _, testCase := range []struct {
		read      scopeRead
		query     string
		dimension string
		want      []string
	}{
		{scopeReadNamed(t, "pre-advice list"), "?node=SYN-NODE-1&withdrawn=true&withdrawn=false&withdrawn=true", "withdrawn", []string{"false", "true"}},
		{scopeReadNamed(t, "handover note list"), "?node=SYN-NODE-1&closed=false", "closed", []string{"false"}},
	} {
		t.Run(testCase.read.name, func(t *testing.T) {
			reader := &stubDeliveryScopeCatalogue{}
			response := testCase.read.serve(grantedIntake(), reader, http.MethodGet, testCase.read.target+testCase.query)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if got := reader.gotQuery.Filters; len(got) != 1 || strings.Join(got[testCase.dimension], ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("读口收到的筛选 = %v，应只有 %s=%v", got, testCase.dimension, testCase.want)
			}
			if reader.gotFilter.Node.String() != "SYN-NODE-1" {
				t.Fatalf("节点参数没有照旧读成筛选：%+v", reader.gotFilter)
			}
		})
	}
}

// Covers: ADR-0144 决定一、三、四——集外的排序维（另一条列表的业务时刻、没有放出的登记时刻）、状态维词表外的取值、另一条列表的
// 状态维与解不开的游标答 400 带理由散文，读口不被触到；游标只在铸它的那组条件下有效：换了节点、加了区间（选择器进游标摘要），
// 或拿到另一条列表去用，都答 400 带「游标与本次的排序或筛选不符」那一句；同一组条件下解成位置原样交给读口。
func TestDeliveryScopeListsRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	preAdvices, handoverNotes := scopeReadNamed(t, "pre-advice list"), scopeReadNamed(t, "handover note list")
	for name, testCase := range map[string]struct {
		read  scopeRead
		query string
	}{
		"预报按开立排":     {preAdvices, "?sort=openedAt"},
		"交接单按预计送达排":  {handoverNotes, "?sort=-expectedArrivalAt"},
		"按登记时刻排":     {preAdvices, "?sort=-registeredAt"},
		"解不开的游标":     {handoverNotes, "?after=SYN-not-a-cursor"},
		"预报带交接单的区间键": {preAdvices, "?openedUntil=2026-10-21T00:00:00Z"},
		"预报的坏转义":     {preAdvices, "?q=%zz"},
		"交接单的坏转义":    {handoverNotes, "?node=SYN-NODE-1&q=%zz"},
		"预报状态词表外":    {preAdvices, "?withdrawn=SYN-NOT-A-WORD"},
		"交接单状态词表外":   {handoverNotes, "?closed=yes"},
		"预报带交接单的状态维": {preAdvices, "?closed=true"},
		"交接单带预报的状态维": {handoverNotes, "?withdrawn=false"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := &stubDeliveryScopeCatalogue{}
			assertPageQueryRefused(t, testCase.read.serve(grantedIntake(), reader, http.MethodGet, testCase.read.target+testCase.query))
			if reader.calls != 0 {
				t.Fatal("坏请求时读口被触到")
			}
		})
	}

	conditions := url.Values{"node": {"SYN-NODE-1"}}
	minted, err := ports.DeliveryPreAdvicePages.Decode(conditions)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := minted.CursorAfter(cataloguepage.Position{Value: cataloguepage.FormatInstant(scopeReadAt), Identity: []string{"SYN-PREADV-1"}})
	if err != nil {
		t.Fatal(err)
	}
	same := &stubDeliveryScopeCatalogue{}
	if response := preAdvices.serve(grantedIntake(), same, http.MethodGet,
		preAdvices.target+"?"+withCursor(conditions, cursor).Encode()); response.Code != http.StatusOK {
		t.Fatalf("同一组条件续读：%d %s", response.Code, response.Body.String())
	}
	if after := same.gotQuery.After; after == nil || after.Value != cataloguepage.FormatInstant(scopeReadAt) || after.Identity[0] != "SYN-PREADV-1" {
		t.Fatalf("读口收到的位置 = %+v", after)
	}
	for name, testCase := range map[string]struct {
		read       scopeRead
		conditions url.Values
	}{
		"换了节点":    {preAdvices, url.Values{"node": {"SYN-NODE-2"}}},
		"加了区间":    {preAdvices, url.Values{"node": {"SYN-NODE-1"}, "expectedArrivalFrom": {"2026-10-20T00:00:00Z"}}},
		"拿到交接单列表": {handoverNotes, conditions},
	} {
		t.Run(name, func(t *testing.T) {
			reader := &stubDeliveryScopeCatalogue{}
			response := testCase.read.serve(grantedIntake(), reader, http.MethodGet,
				testCase.read.target+"?"+withCursor(testCase.conditions, cursor).Encode())
			if detail := assertPageQueryRefused(t, response); !strings.Contains(detail, "游标与本次的排序或筛选不符") {
				t.Fatalf("理由散文 = %q", detail)
			}
			if reader.calls != 0 {
				t.Fatal("游标对不上时读口被触到")
			}
		})
	}
}

// Covers: 该票 spec「端点先 Intake、后解码查询参数」（ADR-0055 决定三）——渠道未配置时，两条列表对合规的一问（不带参数、带选择器
// 排序与 q）与坏排序、解不开的游标、集外的键同答一个 403：答复与不带参数的那一问逐字相同，读口不被触到。
func TestUnconfiguredDeliveryScopeListsAnswerOne403WhateverTheQuery(t *testing.T) {
	for _, testCase := range []struct {
		read      scopeRead
		compliant string
	}{
		{scopeReadNamed(t, "pre-advice list"), "?node=SYN-NODE-1&sort=expectedArrivalAt&q=SYN"},
		{scopeReadNamed(t, "handover note list"), "?node=SYN-NODE-1&sort=openedAt&q=SYN"},
	} {
		t.Run(testCase.read.name, func(t *testing.T) {
			reader := &stubDeliveryScopeCatalogue{}
			want := testCase.read.serve(nodeopshttp.UnconfiguredIntake{}, reader, http.MethodGet, testCase.read.target)
			if want.Code != http.StatusForbidden || problemCode(t, want) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("不带参数的一问：status = %d body = %s", want.Code, want.Body.String())
			}
			assertNoOutcome(t, want)
			for _, query := range []string{
				testCase.compliant, "?sort=nowhere", "?after=SYN-not-a-cursor", "?tenant=SYN-TENANT-02", "?q=%zz",
				"?withdrawn=maybe", "?closed=maybe",
			} {
				response := testCase.read.serve(nodeopshttp.UnconfiguredIntake{}, reader, http.MethodGet, testCase.read.target+query)
				if response.Code != want.Code || response.Body.String() != want.Body.String() {
					t.Fatalf("%s：status = %d body = %s，应与不带参数的一问逐字相同", query, response.Code, response.Body.String())
				}
			}
			if reader.calls != 0 {
				t.Fatal("渠道未配置时读口被触到")
			}
		})
	}
}
