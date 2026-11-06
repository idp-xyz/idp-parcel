package nodeopshttp_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件证 /node-operations-records 各册迁到 ADR-0144 之后的传输面（票 catalogue-read-pagination-rollout/06）：registry 挑册，其余
// 查询参数照那一册的声明解成查询对象、原样交给读口，答复带 page；声明之外的键、排序维与筛选值，对不上本次条件的游标，都答 400 带
// 理由散文；渠道未配置时参数写对写错都答 403。夹具全为合成（S 级）。

// recordsRegistry 是一册在端点上的样子：册名、答复里装行的键、一组在声明之内的查询参数与它该解成的排序和筛选。按实物那一册的
// 读口是 ListIdentityReviewUnits，替身把它收到的查询记在 gotUnitsQuery。
type recordsRegistry struct {
	name        string
	key         string
	byUnit      bool
	parameters  string
	wantSort    cataloguepage.Sort
	wantFilters map[string][]string
}

var recordsRegistries = []recordsRegistry{
	{
		name: "reception", key: "receptions",
		parameters:  "&sort=recordedAt&kind=" + ports.RecordPendingIdentification.String() + "&q=SYN",
		wantSort:    cataloguepage.Sort{Field: "recordedAt"},
		wantFilters: map[string][]string{"kind": {ports.RecordPendingIdentification.String()}},
	},
	{
		name: "unidentified-item", key: "items",
		parameters:  "&sort=recordedAt&identityCheck=" + ports.IdentityUnchecked.String() + "&q=SYN",
		wantSort:    cataloguepage.Sort{Field: "recordedAt"},
		wantFilters: map[string][]string{"identityCheck": {ports.IdentityUnchecked.String()}},
	},
	{
		name: "identity-review-unit", key: "units", byUnit: true,
		parameters:  "&sort=latestRecordedAt&q=SYN",
		wantSort:    cataloguepage.Sort{Field: "latestRecordedAt"},
		wantFilters: map[string][]string{},
	},
	{
		name: "consolidation-unit", key: "units",
		parameters:  "&sort=-unitId&phase=" + domain.ConsolidationPhaseSealed + "&q=SYN",
		wantSort:    cataloguepage.Sort{Field: "unitId", Descending: true},
		wantFilters: map[string][]string{"phase": {domain.ConsolidationPhaseSealed}},
	},
	{
		name: "consolidation-member", key: "members",
		parameters:  "&sort=recordedAt&action=" + domain.RemoveMemberAction.String() + "&q=SYN",
		wantSort:    cataloguepage.Sort{Field: "recordedAt"},
		wantFilters: map[string][]string{"action": {domain.RemoveMemberAction.String()}},
	},
	{
		name: "physical-control", key: "transitions",
		parameters:  "&sort=recordedAt&outcome=" + domain.ControlNotAdvanced.String() + "&q=SYN",
		wantSort:    cataloguepage.Sort{Field: "recordedAt"},
		wantFilters: map[string][]string{"outcome": {domain.ControlNotAdvanced.String()}},
	},
}

// Covers: ADR-0144 决定五、六——各册的答复都带 page（size 取 Intake 给的页大小，next 与 total 原样带出）；registry 之外的查询参数照
// 那一册的声明解成查询对象，原样交给读口。
func TestNodeOperationsRecordsCarryThePageAndPassTheDecodedQuery(t *testing.T) {
	for _, registry := range recordsRegistries {
		t.Run(registry.name, func(t *testing.T) {
			register := &stubReviewCatalogue{next: "SYN-cursor", total: 7}
			response := serveGet(recordsEndpoint(register), "/node-operations-records?registry="+registry.name+registry.parameters)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			page := pageOf(t, response)
			if string(page["size"]) != "25" || string(page["next"]) != `"SYN-cursor"` || string(page["total"]) != "7" {
				t.Fatalf("page 走样：%s", response.Body.String())
			}
			got := register.gotQuery
			if registry.byUnit {
				got = register.gotUnitsQuery
			}
			if got.Sort != registry.wantSort || got.Keyword != "SYN" || got.After != nil || !reflect.DeepEqual(got.Filters, registry.wantFilters) {
				t.Fatalf("读口收到的查询走样：%+v", got)
			}
			if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
				t.Fatalf("读口收到 tenant=%q limit=%d", register.gotTenant, register.gotLimit)
			}
		})
	}
}

// Covers: ADR-0144 决定五——空册是内容不是错误：200、空数组、page.total 为 0、page.next 为 null，各册一致。
func TestEmptyRecordsAnswerAZeroTotal(t *testing.T) {
	for _, registry := range recordsRegistries {
		t.Run(registry.name, func(t *testing.T) {
			response := serveGet(recordsEndpoint(&stubReviewCatalogue{}), "/node-operations-records?registry="+registry.name)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if raw := arrayAt(t, response, registry.key); raw != "[]" {
				t.Fatalf("%s = %s, want []", registry.key, raw)
			}
			page := pageOf(t, response)
			if string(page["size"]) != "25" || string(page["next"]) != "null" || string(page["total"]) != "0" {
				t.Fatalf("空册的 page 走样：%s", response.Body.String())
			}
		})
	}
}

// Covers: registry=unidentified-item 答复里的 units 是按实物那一册不带条件的第一页：items 的排序、筛选、q 与游标不套到它头上，
// 往后翻在 registry=identity-review-unit 那一口。
func TestUnidentifiedItemsListTheUnitsFirstPageWithoutTheItemsConditions(t *testing.T) {
	conditions := url.Values{
		"registry": {"unidentified-item"}, "sort": {"recordedAt"}, "identityCheck": {ports.IdentityUnchecked.String()}, "q": {"SYN"},
	}
	cursor := mintCursor(t, ports.UnidentifiedItemPages, conditions, "SYN-SCAN-1")
	register := &stubReviewCatalogue{}
	response := serveGet(recordsEndpoint(register), "/node-operations-records?"+withCursor(conditions, cursor).Encode())
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if items := register.gotQuery; items.After == nil || items.Keyword != "SYN" {
		t.Fatalf("items 那一问收到的查询 = %+v", items)
	}
	units := register.gotUnitsQuery
	if units.Sort != (cataloguepage.Sort{Field: "latestRecordedAt", Descending: true}) || units.Keyword != "" || units.After != nil ||
		len(units.Filters) != 0 {
		t.Fatalf("units 那一问收到的查询 = %+v, want 按实物那一册的缺省第一页", units)
	}
}

// Covers: ADR-0144 决定一、三、四——集外的键（含自报租户与旧式的页大小）、另一册的筛选维、集外的排序维与筛选值、空筛选值、重复的
// 单值键、解不开的游标，都答 400 带理由散文，读口不被触到。
func TestNodeOperationsRecordsRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	endpoint := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(grantedIntake(), unreachableReviewCatalogue{t: t})
	for name, query := range map[string]string{
		"自报租户":      "registry=reception&tenant=SYN-TENANT-02",
		"页大小":       "registry=consolidation-unit&limit=500",
		"另一册的筛选维":   "registry=reception&phase=" + domain.ConsolidationPhaseOpen,
		"集外排序维":     "registry=reception&sort=unitId",
		"聚合册按登记时刻排": "registry=identity-review-unit&sort=-recordedAt",
		"本册不列的种类":   "registry=reception&kind=" + ports.RecordIntakeNotFormed.String(),
		"空筛选值":      "registry=physical-control&outcome=",
		"重复的排序":     "registry=consolidation-member&sort=recordedAt&sort=-recordedAt",
		"解不开的游标":    "registry=unidentified-item&after=SYN-not-a-cursor",
	} {
		t.Run(name, func(t *testing.T) {
			assertPageQueryRefused(t, serveGet(endpoint, "/node-operations-records?"+query))
		})
	}
}

// Covers: ADR-0144 决定一、四——游标只在铸它的那一册、那组条件下有效：换了筛选、换了 q、换了排序，或拿到另一册去用（registry 是
// 选择器，进游标摘要），都答 400 带「游标与本次的排序或筛选不符」那一句，读口不被触到；同一组条件下解成位置原样交给读口。
func TestNodeOperationsRecordsCursorsBelongToTheirCatalogueAndConditions(t *testing.T) {
	conditions := url.Values{"registry": {"reception"}, "kind": {ports.RecordPendingIdentification.String()}, "q": {"SYN"}}
	cursor := mintCursor(t, ports.ReceptionPages, conditions, "SYN-SCAN-1")

	register := &stubReviewCatalogue{}
	if response := serveGet(recordsEndpoint(register), "/node-operations-records?"+withCursor(conditions, cursor).Encode()); response.Code != http.StatusOK {
		t.Fatalf("同一组条件续读：%d %s", response.Code, response.Body.String())
	}
	if after := register.gotQuery.After; after == nil || after.Value != cataloguepage.FormatInstant(catalogueBaseAt) ||
		!slices.Equal(after.Identity, []string{"SYN-SCAN-1"}) {
		t.Fatalf("读口收到的位置 = %+v", after)
	}

	endpoint := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(grantedIntake(), unreachableReviewCatalogue{t: t})
	for name, changed := range map[string]url.Values{
		"换了筛选":  {"registry": {"reception"}, "kind": {ports.RecordIntakeFormed.String()}, "q": {"SYN"}},
		"换了 q":  {"registry": {"reception"}, "kind": {ports.RecordPendingIdentification.String()}, "q": {"SYN-OTHER"}},
		"换了排序":  {"registry": {"reception"}, "kind": {ports.RecordPendingIdentification.String()}, "q": {"SYN"}, "sort": {"recordedAt"}},
		"拿到另一册": {"registry": {"unidentified-item"}, "q": {"SYN"}},
	} {
		t.Run(name, func(t *testing.T) {
			detail := assertPageQueryRefused(t, serveGet(endpoint, "/node-operations-records?"+withCursor(changed, cursor).Encode()))
			if !strings.Contains(detail, "游标与本次的排序或筛选不符") {
				t.Fatalf("理由散文 = %q", detail)
			}
		})
	}
}

// Covers: spec 取舍「端点先 Intake、后解码查询参数」（ADR-0055 决定三）——渠道未配置时，册名认得出，翻页、排序、筛选参数写对写错都
// 答 403，答复不随这些参数变，读口不被触到。
func TestUnconfiguredRecordsAnswer403WhateverThePageParameters(t *testing.T) {
	endpoint := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(nodeopshttp.UnconfiguredIntake{}, unreachableReviewCatalogue{t: t})
	for _, registry := range recordsRegistries {
		for _, parameters := range []string{registry.parameters, "&sort=nowhere", "&tenant=SYN-TENANT-02", "&after=SYN-not-a-cursor"} {
			t.Run(registry.name+parameters, func(t *testing.T) {
				response := serveGet(endpoint, "/node-operations-records?registry="+registry.name+parameters)
				if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
					t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
				}
				assertNoOutcome(t, response)
			})
		}
	}
}

// Covers: registry=identity-review-unit 把按实物那一册单独列出、可往后翻：每行带作业实物、它待识别各版里最近一次的登记时刻
// （latestRecordedAt，这一册的排序维，按 RFC 3339 转写）与各链当前有效版本；答复带 page，读口收到 Intake 给的租户与页大小。
func TestIdentityReviewUnitRegistryListsUnitsWithTheirLatestRecording(t *testing.T) {
	latest := catalogueBaseAt.Add(time.Hour)
	catalogue := reviewUnitsCatalogue{
		stubReviewCatalogue: &stubReviewCatalogue{next: "SYN-cursor", total: 3},
		reviewUnits: []ports.IdentityReviewUnitRow{{
			Unit: "SYN-UNIT-1", LatestRecordedAt: latest,
			Heads: []ports.IdentityReviewHead{{
				SourceID: "SYN-SCAN-1", Kind: ports.RecordPendingIdentification.String(), Node: "SYN-NODE-1", JudgmentVersion: "SYN-V-1",
				Candidates: []string{}, ReceivedAt: catalogueBaseAt, RecordedAt: latest,
			}},
		}},
	}
	response := serveGet(nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(grantedIntake(), catalogue),
		"/node-operations-records?registry=identity-review-unit")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
		Units   []struct {
			Unit             string                       `json:"unit"`
			LatestRecordedAt string                       `json:"latestRecordedAt"`
			Heads            []map[string]json.RawMessage `json:"heads"`
		} `json:"units"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "IDENTITY_REVIEW_UNITS_LISTED" || len(body.Units) != 1 || body.Units[0].Unit != "SYN-UNIT-1" ||
		body.Units[0].LatestRecordedAt != latest.Format(time.RFC3339Nano) || len(body.Units[0].Heads) != 1 ||
		string(body.Units[0].Heads[0]["sourceId"]) != `"SYN-SCAN-1"` {
		t.Fatalf("答复走样：%s", response.Body.String())
	}
	if page := pageOf(t, response); string(page["next"]) != `"SYN-cursor"` || string(page["total"]) != "3" {
		t.Fatalf("page 走样：%s", response.Body.String())
	}
	if catalogue.gotTenant != "TENANT-1" || catalogue.gotLimit != 25 {
		t.Fatalf("读口收到 tenant=%q limit=%d", catalogue.gotTenant, catalogue.gotLimit)
	}
}

// mintCursor 在一组查询参数下铸出「末行在 catalogueBaseAt、标识为 identity」之后的游标，就是读面在那组条件下翻页时交出的那种。
func mintCursor(t *testing.T, catalogue *cataloguepage.Catalogue, conditions url.Values, identity ...string) string {
	t.Helper()
	query, err := catalogue.Decode(conditions)
	if err != nil {
		t.Fatalf("解铸游标的那组条件：%v", err)
	}
	cursor, err := query.CursorAfter(cataloguepage.Position{Value: cataloguepage.FormatInstant(catalogueBaseAt), Identity: identity})
	if err != nil {
		t.Fatalf("铸游标：%v", err)
	}
	return cursor
}

// withCursor 在一组查询参数上接一个游标，交回新的一组，不改原来那组。
func withCursor(values url.Values, cursor string) url.Values {
	copied := url.Values{}
	for key, given := range values {
		copied[key] = slices.Clone(given)
	}
	copied.Set("after", cursor)
	return copied
}
