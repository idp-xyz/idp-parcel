package commercialhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对票 catalogue-read-pagination-rollout/11 迁到 ADR-0144 的各列表端点证查询参数的传输面：答复带 page（size 是
// Intake 定的页大小，next 与 total 照读口）、读口收到的是按本册声明解出的查询；未知键、集外排序维、词表外筛选值、解不开
// 的游标各答 400 带理由散文，且不进读口；解码先于 Intake——渠道未配置时坏参数答 400、好参数答 403。各册同形，表驱动，
// 每册各跑一遍。

// tenantPage 是各册旧替身答页的共用一段：读失败照答错，他租答空页，本租答全部行、总数即行数。
func tenantPage[Row any](err error, mine bool, rows []Row) (cataloguepage.Result[Row], error) {
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	if !mine {
		return cataloguepage.Result[Row]{}, nil
	}
	return cataloguepage.Result[Row]{Rows: rows, Total: int64(len(rows))}, nil
}

var pageListedAt = time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

// pageRecorder 记下读口收到的查询。recordPage 按那份查询给本页末行编下一页游标——游标只能由解出它的那份查询编，端点对它的
// 校验（摘要与本次条件是否相符）才证得到；identity 是那一行标识各段的取值，段数与各段的值类须合本册声明。
type pageRecorder struct {
	seen *cataloguepage.Query
}

func recordPage[Row any](recorder *pageRecorder, query cataloguepage.Query, row Row, identity ...string) (cataloguepage.Result[Row], error) {
	recorder.seen = &query
	next, err := query.CursorAfter(cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(pageListedAt),
		Identity: identity,
	})
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	return cataloguepage.Result[Row]{Rows: []Row{row}, Next: next, Total: 3}, nil
}

type pagingPartyIdentityReader struct {
	pageRecorder
}

func (reader *pagingPartyIdentityReader) ListBusinessParties(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.BusinessPartyRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.BusinessPartyRow{
		TenantID: "tenant-1", PartyID: "SYN-PARTY-1", Status: "EFFECTIVE",
		EffectiveFrom: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-PARTY-1")
}

func (reader *pagingPartyIdentityReader) ListGroupLegalEntities(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.GroupLegalEntityRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.GroupLegalEntityRow{
		TenantID: "tenant-1", LegalEntityID: "SYN-LE-1", PartyID: "SYN-PARTY-1", Status: "EFFECTIVE",
		EffectiveFrom: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-LE-1")
}

func (reader *pagingPartyIdentityReader) ListPartyRelationships(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.PartyRelationshipRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.PartyRelationshipRow{
		TenantID: "tenant-1", RelationshipID: "SYN-REL-1", HolderID: "SYN-PARTY-1", CounterpartyID: "SYN-PARTY-2",
		Role: "CUSTOMER", Status: "EFFECTIVE", EffectiveStartsAt: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-REL-1")
}

func (reader *pagingPartyIdentityReader) ListLegalEntityRevisions(
	context.Context, domain.TenantID, domain.LegalEntityReference,
) ([]ports.LegalEntityRevisionRow, error) {
	return nil, nil
}

func (reader *pagingPartyIdentityReader) ListBusinessPartyRevisions(
	context.Context, domain.TenantID, domain.PartyID,
) ([]ports.BusinessPartyRevisionRow, error) {
	return nil, nil
}

// pagedEndpointCase 是一册在表里的一行。selector 是册子选择器（如 kind=PRICE_POLICY），每次请求都带上；badFilter 只在本册
// 有带词表的筛选维时给。goodQuery 是一组合本册声明的条件，翻页那一步换条件拿旧游标就是拿它与 changedQuery 对照。
type pagedEndpointCase struct {
	name         string
	endpoint     func(commercialhttp.CommercialCatalogueIntake) http.Handler
	recorder     *pageRecorder
	collection   string
	selector     string
	defaultSort  string
	badSort      string
	badFilter    string
	goodQuery    string
	changedQuery string
}

func pagedEndpointCases() []pagedEndpointCase {
	identities := &pagingPartyIdentityReader{}
	return []pagedEndpointCase{
		{
			name: "业务参与方",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryBusinessPartiesEndpoint(intake, identities)
			},
			recorder: &identities.pageRecorder, collection: "parties",
			defaultSort: "-registeredAt", badSort: "status", badFilter: "status=ARCHIVED",
			goodQuery: "status=EFFECTIVE", changedQuery: "status=REGISTERED",
		},
		{
			name: "集团法人",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryGroupLegalEntitiesEndpoint(intake, identities)
			},
			recorder: &identities.pageRecorder, collection: "entities",
			defaultSort: "-registeredAt", badSort: "partyName", badFilter: "status=ACTIVE",
			goodQuery: "registrationCountry=CN&status=EFFECTIVE", changedQuery: "registrationCountry=SG&status=EFFECTIVE",
		},
		{
			name: "参与方关系",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryPartyRelationshipsEndpoint(intake, identities)
			},
			recorder: &identities.pageRecorder, collection: "relationships",
			defaultSort: "-registeredAt", badSort: "holderName", badFilter: "role=OWNER",
			goodQuery: "role=CUSTOMER&status=CANDIDATE&status=EFFECTIVE", changedQuery: "role=SUPPLIER&status=EFFECTIVE",
		},
	}
}

func getPaged(t *testing.T, endpoint http.Handler, selector, rawQuery string) (*httptest.ResponseRecorder, problemBody) {
	t.Helper()
	parts := make([]string, 0, 2)
	for _, part := range []string{selector, rawQuery} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?"+strings.Join(parts, "&"), nil))
	var problem problemBody
	if recorder.Code != http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
			t.Fatalf("decode problem %s: %v", recorder.Body.Bytes(), err)
		}
	}
	return recorder, problem
}

// Covers: 完成判据「答复带 page，size 等于 Intake 的页大小」与「换条件拿旧游标答 400」——page 三格齐在（ADR-0144 决定五），
// 读口收到的第一页查询是本册缺省序、没有游标；游标带着上一次的条件摘要，换了条件还拿它答 400，条件没变则把位置交给读口。
func TestMigratedCatalogueEndpointsCarryThePage(t *testing.T) {
	limit := catalogueQuery(t).Limit
	for _, tc := range pagedEndpointCases() {
		endpoint := tc.endpoint(intakeDouble{query: catalogueQuery(t)})
		tc.recorder.seen = nil
		recorder, _ := getPaged(t, endpoint, tc.selector, tc.goodQuery)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s 第一页 status = %d, body = %s", tc.name, recorder.Code, recorder.Body)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s decode %s: %v", tc.name, recorder.Body.Bytes(), err)
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(body[tc.collection], &rows); err != nil || len(rows) != 1 {
			t.Fatalf("%s 行集 %s = %s（%v），want 读口答的那一行", tc.name, tc.collection, body[tc.collection], err)
		}
		var page struct {
			Size  int     `json:"size"`
			Next  *string `json:"next"`
			Total *int64  `json:"total"`
		}
		if err := json.Unmarshal(body["page"], &page); err != nil {
			t.Fatalf("%s page = %s: %v", tc.name, body["page"], err)
		}
		if page.Size != limit || page.Next == nil || *page.Next == "" || page.Total == nil || *page.Total != 3 {
			t.Fatalf("%s page = %s，want size %d（Intake 的页大小）、next 在场、total 3", tc.name, body["page"], limit)
		}
		if seen := tc.recorder.seen; seen == nil || seen.Sort.String() != tc.defaultSort || seen.After != nil {
			t.Fatalf("%s 读口收到的第一页查询 = %+v，want 缺省序 %s 且无游标", tc.name, seen, tc.defaultSort)
		}

		cursor := "after=" + url.QueryEscape(*page.Next)
		stale, problem := getPaged(t, endpoint, tc.selector, tc.changedQuery+"&"+cursor)
		if stale.Code != http.StatusBadRequest || problem.Error.Detail != "游标与本次的排序或筛选不符，请从第一页重取" {
			t.Fatalf("%s 换条件拿旧游标 status = %d, problem = %+v", tc.name, stale.Code, problem)
		}
		next, _ := getPaged(t, endpoint, tc.selector, tc.goodQuery+"&"+cursor)
		if next.Code != http.StatusOK || tc.recorder.seen.After == nil {
			t.Fatalf("%s 同条件翻页 status = %d, 读口收到的位置 = %+v", tc.name, next.Code, tc.recorder.seen.After)
		}
	}
}

// Covers: 完成判据「未知键、集外排序维、词表外筛选值、解不开的游标各答 400 带理由」与「未配置渠道时坏参数 400、好参数 403」——
// 查询参数一律封闭集（ADR-0144 决定三、四）：多给的键若被静默忽略，页面以为筛了、服务端其实没筛。坏参数不进读口。
func TestMigratedCatalogueEndpointsRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	for _, tc := range pagedEndpointCases() {
		endpoint := tc.endpoint(intakeDouble{query: catalogueQuery(t)})
		refusals := []struct {
			query  string
			reason string
		}{
			{"tenant=SYN-TENANT-02", "tenant"},
			{"sort=" + tc.badSort, tc.badSort},
			{"sort=-" + tc.badSort, tc.badSort},
			{"after=SYN-not-a-cursor", "请从第一页重取"},
		}
		if tc.badFilter != "" {
			_, value, _ := strings.Cut(tc.badFilter, "=")
			refusals = append(refusals, struct {
				query  string
				reason string
			}{tc.badFilter, value})
		}
		for _, refusal := range refusals {
			tc.recorder.seen = nil
			recorder, problem := getPaged(t, endpoint, tc.selector, refusal.query)
			if recorder.Code != http.StatusBadRequest || problem.Error.Code != "MALFORMED_REQUEST" ||
				!strings.Contains(problem.Error.Detail, refusal.reason) {
				t.Fatalf("%s %s: status = %d, problem = %+v；want 400 MALFORMED_REQUEST 且理由提到 %q",
					tc.name, refusal.query, recorder.Code, problem, refusal.reason)
			}
			if tc.recorder.seen != nil {
				t.Fatalf("%s %s: 坏参数进了读口", tc.name, refusal.query)
			}
		}

		unconfigured := tc.endpoint(intakeDouble{err: commercialhttp.ErrAccessChannelNotConfigured})
		if recorder, problem := getPaged(t, unconfigured, tc.selector, "sort="+tc.badSort); recorder.Code != http.StatusBadRequest ||
			!strings.Contains(problem.Error.Detail, tc.badSort) {
			t.Fatalf("%s 未配置渠道 + 坏参数 status = %d, problem = %+v，want 400 且理由点名参数（解码先于 Intake）",
				tc.name, recorder.Code, problem)
		}
		if recorder, _ := getPaged(t, unconfigured, tc.selector, tc.goodQuery); recorder.Code != http.StatusForbidden {
			t.Fatalf("%s 未配置渠道 + 好参数 status = %d，want 403", tc.name, recorder.Code)
		}
	}
}
