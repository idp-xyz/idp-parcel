package tfhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证两个列表端点照 ADR-0144 收查询参数（票 catalogue-read-pagination-rollout/05，spec「端点先 Intake、后解码查询参数」）：
// 每一册的答复都多出 page 一格，size 取 Intake 给的页大小；配置了的渠道上，声明之外的键、排序维、词表外的筛选值、拿别的册或
// 别的条件编出的游标都是坏请求——答 400 带 detail、读口不被触到；声明之内的条件原样交给读口。渠道未配置时对一切请求同答
// 403，选择器与查询参数写对写错都一样（ADR-0055 决定三）。各册怎么取页由 postgres 读面的用例证。

// pagedIntakeSize 是 pagedIntake 裁决的页大小，取一个不像缺省值的数：答复里的 page.size 等于它，才认得出是从 Intake 来的。
const pagedIntakeSize = 7

func pagedIntake() grantedCatalogueIntake {
	return grantedCatalogueIntake{tenant: "SYN-TENANT-01", limit: pagedIntakeSize}
}

// unreachableFactReview 断言外部承运轨迹事实的读口未被触到。
type unreachableFactReview struct{ t *testing.T }

func (reader unreachableFactReview) ListCurrentExternalTrackingFacts(
	context.Context, domain.TenantID, domain.TrackingSourceReference, ports.EffectiveTimeReviewFilter, int, cataloguepage.Query,
) (cataloguepage.Result[ports.ExternalTrackingFactReviewRow], error) {
	reader.t.Helper()
	reader.t.Fatal("被拒的请求触到了外部承运轨迹事实的读口")
	return cataloguepage.Result[ports.ExternalTrackingFactReviewRow]{}, nil
}

// pageCellOf 取出答复里 page 一格的原文。
func pageCellOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	raw, present := fields["page"]
	if !present {
		t.Fatalf("答复缺 page 一格：%s", response.Body.String())
	}
	return string(raw)
}

// assertPageQueryRefused 断言一次查询参数的拒绝：400 MALFORMED_REQUEST，带说明错在哪的 detail，除 error 之外不带任何格。
func assertPageQueryRefused(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var body struct {
		Error struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if response.Code != http.StatusBadRequest || body.Error.Code != "MALFORMED_REQUEST" || body.Error.Detail == "" {
		t.Fatalf("应答 400 MALFORMED_REQUEST 并带 detail，得 %d %s", response.Code, response.Body.String())
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil || len(fields) != 1 {
		t.Fatalf("坏请求的答复除 error 之外还带了别的格：%s", response.Body.String())
	}
}

// assertChannelNotConfigured 断言未配置渠道的答复：403 ACCESS_CHANNEL_NOT_CONFIGURED，不带业务结果格。
func assertChannelNotConfigured(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("应答 403 ACCESS_CHANNEL_NOT_CONFIGURED，得 %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

// pagedCatalogue 是迁过的列表端点上的一册：good 是一问合规的列表请求（选择器照端点要求带上），bad 在它上面多一个本册声明
// 之外的参数；endpoints 交回接在配置了与未配置的渠道上的同一个端点，后者接的读口一碰即红。
type pagedCatalogue struct {
	name      string
	good, bad string
	endpoints func(t *testing.T) (configured, unconfigured http.Handler)
}

func pagedCatalogues() []pagedCatalogue {
	records := func(t *testing.T) (http.Handler, http.Handler) {
		return tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(pagedIntake(), &stubReviewCatalogue{}),
			tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(tfhttp.UnconfiguredIntake{}, unreachableReviewCatalogue{t: t})
	}
	var catalogues []pagedCatalogue
	for _, registry := range fulfillmentRegistries {
		good := "/transport-fulfillment-records?registry=" + registry
		catalogues = append(catalogues, pagedCatalogue{name: registry, good: good, bad: good + "&sort=nope", endpoints: records})
	}
	facts := "/transport-fulfillment-external-tracking-facts?source=SYN-SOURCE-A&view=pending"
	return append(catalogues, pagedCatalogue{
		name: "external-tracking-facts", good: facts, bad: facts + "&effectiveBasis=NOPE",
		endpoints: func(t *testing.T) (http.Handler, http.Handler) {
			return tfhttp.NewQueryExternalTrackingFactsEndpoint(pagedIntake(), &stubFactReview{}),
				tfhttp.NewQueryExternalTrackingFactsEndpoint(tfhttp.UnconfiguredIntake{}, unreachableFactReview{t: t})
		},
	})
}

// Covers: ADR-0144 决定四、五与 spec「端点先 Intake、后解码查询参数」——两个端点上的每一册各自：合规的一问带 page，size 取
// Intake 的页大小；集外参数答 400 带理由散文；渠道未配置时合规与不合规的一问都答 403、答复一字不差、读口不被触到
// （ADR-0055 决定三：答复不随请求内容变）。
func TestEveryMigratedCatalogueCarriesThePageAndDecodesAfterTheIntake(t *testing.T) {
	for _, catalogue := range pagedCatalogues() {
		t.Run(catalogue.name, func(t *testing.T) {
			configured, unconfigured := catalogue.endpoints(t)
			good := serveGet(configured, catalogue.good)
			if good.Code != http.StatusOK {
				t.Fatalf("合规的一问 status = %d；body = %s", good.Code, good.Body)
			}
			var page struct {
				Size int `json:"size"`
			}
			if err := json.Unmarshal([]byte(pageCellOf(t, good)), &page); err != nil || page.Size != pagedIntakeSize {
				t.Fatalf("page.size 不是 Intake 的页大小 %d：%s（%v）", pagedIntakeSize, good.Body, err)
			}
			assertPageQueryRefused(t, serveGet(configured, catalogue.bad))

			goodRefused, badRefused := serveGet(unconfigured, catalogue.good), serveGet(unconfigured, catalogue.bad)
			assertChannelNotConfigured(t, goodRefused)
			assertChannelNotConfigured(t, badRefused)
			if goodRefused.Body.String() != badRefused.Body.String() {
				t.Fatalf("未配置渠道的答复随请求内容变了：%s 对 %s", goodRefused.Body, badRefused.Body)
			}
		})
	}
}

// Covers: ADR-0144 决定五——各本册子的答复都多出 page 一格：size 是本次的页大小（接入面裁决的），next 照读口给的游标，
// 末页为 null；total 照读口给的总数。
func TestEveryRecordsRegistryAnswersAPageCell(t *testing.T) {
	for _, registry := range fulfillmentRegistries {
		t.Run(registry, func(t *testing.T) {
			more := serveGet(recordsEndpoint(&stubReviewCatalogue{next: "SYN-CURSOR"}), "/transport-fulfillment-records?registry="+registry)
			if more.Code != http.StatusOK || pageCellOf(t, more) != `{"size":25,"next":"SYN-CURSOR","total":0}` {
				t.Fatalf("还有下一页时 page 走样：%d %s", more.Code, more.Body.String())
			}
			last := serveGet(recordsEndpoint(&stubReviewCatalogue{}), "/transport-fulfillment-records?registry="+registry)
			if last.Code != http.StatusOK || pageCellOf(t, last) != `{"size":25,"next":null,"total":0}` {
				t.Fatalf("末页的 page 走样：%d %s", last.Code, last.Body.String())
			}
		})
	}
}

// Covers: ADR-0144 决定四——声明之外的查询参数是坏请求：集外键、集外排序维、词表外的筛选值、别的册才有的筛选维、解不开的
// 游标，答 400 带 detail，读口不被触到。判它在渠道准入之后（spec「端点先 Intake、后解码查询参数」）：渠道未配置时同一问答
// 403，没有凭据的调用方探不出各册开放了哪些查询维度。
func TestRecordsCatalogueRefusesQueriesOutsideTheDeclaration(t *testing.T) {
	refused := map[string]string{
		"集外键":         "registry=transport-schedule&unknown=1",
		"集外排序维":       "registry=capacity-pool&sort=-recordedAt",
		"排序维写两次":      "registry=transport-handover&sort=judgedAt&sort=object",
		"班次来历词表外":     "registry=transport-schedule&origin=SCHEDULED",
		"容量维度词表外":     "registry=capacity-pool&dimension=weight",
		"交接裁决词表外":     "registry=transport-handover&verdict=NOPE",
		"交付门禁读数词表外":   "registry=effective-delivery&gateReading=OPEN",
		"总单状态词表外":     "registry=carrier-master-document&standing=VOID",
		"准备结论词表外":     "registry=transport-preparation&outcome=NOPE",
		"未形成缘由词表外":    "registry=actual-fulfillment-unformed&reason=NOPE",
		"挡回依据词表外":     "registry=booking-answer-bounce&basis=NOPE",
		"复核清单一侧词表外":   "registry=pending-attempt-result&side=pickup",
		"订舱册的状态不是筛选维": "registry=booking-request&standing=CONFIRMED",
		"待形成册没有筛选维":   "registry=delivery-dispatch-pending&reason=NOPE",
		"解不开的游标":      "registry=effective-delivery&after=not-a-cursor",
	}
	for name, query := range refused {
		t.Run(name, func(t *testing.T) {
			target := "/transport-fulfillment-records?" + query
			configured := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(grantedIntake(), unreachableReviewCatalogue{t: t})
			assertPageQueryRefused(t, serveGet(configured, target))
			unconfigured := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(tfhttp.UnconfiguredIntake{}, unreachableReviewCatalogue{t: t})
			assertChannelNotConfigured(t, serveGet(unconfigured, target))
		})
	}
}

// Covers: ADR-0144 决定四与票 operator-channel/35——租户与页大小只取认证结果；查询串上自报的 tenant、limit 不在任何一册的声明
// 里，是集外键：配置了的渠道上答 400、一行不带，不静默忽略；不自报的同一问照接入面裁决的租户与页大小去读。
func TestRecordsCatalogueRefusesSelfReportedTenantAndLimit(t *testing.T) {
	const target = "/transport-fulfillment-records?registry=transport-schedule"
	for _, query := range []string{"&tenant=SYN-TENANT-09", "&limit=999"} {
		t.Run(query, func(t *testing.T) {
			endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(pagedIntake(), unreachableReviewCatalogue{t: t})
			assertPageQueryRefused(t, serveGet(endpoint, target+query))
		})
	}

	register := &stubReviewCatalogue{}
	response := serveGet(tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(pagedIntake(), register), target)
	if response.Code != http.StatusOK || register.gotTenant != "SYN-TENANT-01" || register.gotLimit != pagedIntakeSize {
		t.Fatalf("不自报的一问没照接入面去读：%d tenant=%q limit=%d %s",
			response.Code, register.gotTenant, register.gotLimit, response.Body.String())
	}
}

// Covers: 票 catalogue-read-pagination-rollout/05「按选择器分派的路由，每个取值一册、各有名字」——registry 的每个取值各认
// 自己那一册声明编出的游标，读口收到的位置就是游标里的那一个；同一枚游标拿到别的册去翻答 400：册名与选择器取值都进游标摘要。
func TestEveryRecordsRegistryIsItsOwnCatalogue(t *testing.T) {
	at := cataloguepage.FormatInstant(catalogueBaseAt)
	cases := []struct {
		registry  string
		catalogue *cataloguepage.Catalogue
		position  cataloguepage.Position
	}{
		{"transport-schedule", ports.TransportSchedulePages, cataloguepage.Position{Value: at, Identity: []string{"SYN-SCHED-1", "2"}}},
		{"transport-schedule-template", ports.ScheduleTemplatePages, cataloguepage.Position{Value: "SYN-TPL-1", Identity: []string{"SYN-TPL-1", "3"}}},
		{"capacity-pool", ports.CapacityPoolPages, cataloguepage.Position{Value: "SYN-POOL-1", Identity: []string{"SYN-POOL-1"}}},
		{"transport-handover", ports.TransportHandoverPages, cataloguepage.Position{Value: at, Identity: []string{"SYN-PARCEL-1", "SYN-SCOPE-1", "SYN-HV-1"}}},
		{"effective-delivery", ports.EffectiveDeliveryPages, cataloguepage.Position{Value: at, Identity: []string{"SYN-PARCEL-1", "SYN-ATTEMPT-1", "SYN-DV-1"}}},
		{"carrier-master-document", ports.CarrierMasterDocumentPages, cataloguepage.Position{Value: at, Identity: []string{"SYN-MAWB-1", "SYN-MDV-1"}}},
		{"transport-preparation", ports.TransportPreparationPages, cataloguepage.Position{Value: at, Identity: []string{"SYN-RPV-1#2", "SYN-PARCEL-1", "SYN-PREPARE/1"}}},
		{"delivery-dispatch-pending", ports.DeliveryDispatchPendingPages, cataloguepage.Position{Value: at, Identity: []string{"SYN-SEGMENT-1", "SYN-PARCEL-1"}}},
		{"delivery-dispatch-reask-cadence", ports.DeliveryDispatchReaskCadencePages, cataloguepage.Position{Value: at, Identity: []string{"SYN-CADENCE-1"}}},
		{"actual-fulfillment-unformed", ports.ActualFulfillmentUnformedPages, cataloguepage.Position{Value: at, Identity: []string{"SYN-SEGMENT-1", "SYN-PARCEL-1"}}},
		{"booking-request", ports.BookingRequestPages, cataloguepage.Position{Value: at, Identity: []string{"SYN-BOOKING-1"}}},
		{"booking-answer-bounce", ports.BookingAnswerBouncePages, cataloguepage.Position{Value: at, Identity: []string{"SYN-PARTY-1", "SYN-BOOKING-1", "SYN-ANSWER-1", "1"}}},
		{"pending-attempt-result", ports.PendingAttemptResultPages, cataloguepage.Position{Value: at, Identity: []string{"PICKUP", "SYN-ATTEMPT-1", "SYN-PARCEL-1", "SYN-DEVICE-1/1"}}},
	}
	if len(cases) != len(fulfillmentRegistries) {
		t.Fatalf("对照表与 registry 封闭集不一样长：%d 对 %d", len(cases), len(fulfillmentRegistries))
	}
	for index, testCase := range cases {
		t.Run(testCase.registry, func(t *testing.T) {
			query, err := testCase.catalogue.Decode(url.Values{ports.RecordsRegistrySelector: {testCase.registry}})
			if err != nil {
				t.Fatal(err)
			}
			cursor, err := query.CursorAfter(testCase.position)
			if err != nil {
				t.Fatal(err)
			}

			register := &stubReviewCatalogue{}
			response := serveGet(recordsEndpoint(register),
				"/transport-fulfillment-records?registry="+testCase.registry+"&after="+cursor)
			if response.Code != http.StatusOK {
				t.Fatalf("本册编出的游标被拒：%d %s", response.Code, response.Body.String())
			}
			if register.gotQuery.After == nil || !reflect.DeepEqual(*register.gotQuery.After, testCase.position) {
				t.Fatalf("读口收到的位置 = %+v，want %+v", register.gotQuery.After, testCase.position)
			}

			other := cases[(index+1)%len(cases)].registry
			endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(grantedIntake(), unreachableReviewCatalogue{t: t})
			assertPageQueryRefused(t, serveGet(endpoint, "/transport-fulfillment-records?registry="+other+"&after="+cursor))
		})
	}
}

// Covers: ADR-0144 决定四——声明之内的条件原样交给读口：排序维与方向、维内为或的筛选（去重升序）、去掉首尾空白的 q；
// 改了条件再拿旧游标去翻答 400（游标摘要与本次条件不符）。
func TestRecordsCataloguePassesTheDeclaredConditionsToTheReader(t *testing.T) {
	register := &stubReviewCatalogue{}
	response := serveGet(recordsEndpoint(register),
		"/transport-fulfillment-records?registry=transport-handover&sort=-object&verdict=REFUSED&verdict=HANDED_OVER&q=%20SYN-PARCEL%20")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	got := register.gotQuery
	if got.Sort != (cataloguepage.Sort{Field: "object", Descending: true}) ||
		!reflect.DeepEqual(got.Filters, map[string][]string{"verdict": {"HANDED_OVER", "REFUSED"}}) ||
		got.Keyword != "SYN-PARCEL" || got.After != nil {
		t.Fatalf("读口收到的条件走样：%+v", got)
	}

	cursor, err := got.CursorAfter(cataloguepage.Position{Value: "SYN-PARCEL-1", Identity: []string{"SYN-PARCEL-1", "SYN-SCOPE-1", "SYN-HV-1"}})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(grantedIntake(), unreachableReviewCatalogue{t: t})
	assertPageQueryRefused(t, serveGet(endpoint,
		"/transport-fulfillment-records?registry=transport-handover&sort=-object&verdict=REFUSED&q=SYN-PARCEL&after="+cursor))
}

// Covers: ADR-0144 决定五——外部承运轨迹事实当前版的答复多出 page 一格；source 与 view 仍是选择器，effectiveBasis、sort 与 q
// 原样交给读口。
func TestExternalTrackingFactsAnswerAPageCellAndPassTheDeclaredConditions(t *testing.T) {
	reader := &stubFactReview{rows: []ports.ExternalTrackingFactReviewRow{judgedReviewRow()}, next: "SYN-CURSOR"}
	endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), reader)

	response := getFacts(t, endpoint, "/x?source=SYN-SOURCE-A&view=current&effectiveBasis=JUDGED_BY_RULE&sort=-receivedAt&q=SYN-EVT")
	if response.Code != http.StatusOK || pageCellOf(t, response) != `{"size":25,"next":"SYN-CURSOR","total":1}` {
		t.Fatalf("答复或 page 走样：%d %s", response.Code, response.Body.String())
	}
	got := reader.gotQuery
	if reader.gotFilter != ports.EveryCurrentVersion || reader.gotSource != "SYN-SOURCE-A" ||
		got.Sort != (cataloguepage.Sort{Field: "receivedAt", Descending: true}) ||
		!reflect.DeepEqual(got.Filters, map[string][]string{"effectiveBasis": {"JUDGED_BY_RULE"}}) || got.Keyword != "SYN-EVT" {
		t.Fatalf("读口收到的条件走样：filter=%v source=%q query=%+v", reader.gotFilter, reader.gotSource, got)
	}

	// 同一枚游标换一个源或视图去翻答 400：源与视图的取值都进游标摘要。
	query, err := ports.ExternalTrackingFactPages.Decode(url.Values{"source": {"SYN-SOURCE-A"}, "view": {"current"}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := query.CursorAfter(cataloguepage.Position{
		Value: cataloguepage.FormatInstant(factReviewOccurredAt), Identity: []string{"SYN-EXTF-1", "SYN-EXTV-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted := &stubFactReview{}
	if response := getFacts(t, tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), accepted),
		"/x?source=SYN-SOURCE-A&view=current&after="+cursor); response.Code != http.StatusOK || accepted.gotQuery.After == nil {
		t.Fatalf("本源本视图编出的游标被拒：%d %s", response.Code, response.Body.String())
	}
	for _, target := range []string{"/x?source=SYN-SOURCE-B&view=current&after=" + cursor, "/x?source=SYN-SOURCE-A&view=pending&after=" + cursor} {
		endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), unreachableFactReview{t: t})
		assertPageQueryRefused(t, getFacts(t, endpoint, target))
	}
}

// Covers: ADR-0144 决定四——外部承运轨迹事实端点上声明之外的查询参数是坏请求：配置了的渠道上答 400 带 detail，读口不被触到；
// 自报的 tenant、limit 同属集外键。渠道未配置时同一问答 403（spec「端点先 Intake、后解码查询参数」）。
func TestExternalTrackingFactsRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	for name, query := range map[string]string{
		"集外键":       "&unknown=1",
		"集外排序维":     "&sort=recordedAt",
		"有效时间依据词表外": "&effectiveBasis=JUDGED",
		"解不开的游标":    "&after=not-a-cursor",
		"自报租户":      "&tenant=SYN-TENANT-09",
		"自报页大小":     "&limit=999",
	} {
		t.Run(name, func(t *testing.T) {
			target := "/x?source=SYN-SOURCE-A&view=pending" + query
			configured := tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), unreachableFactReview{t: t})
			assertPageQueryRefused(t, getFacts(t, configured, target))
			unconfigured := tfhttp.NewQueryExternalTrackingFactsEndpoint(tfhttp.UnconfiguredIntake{}, unreachableFactReview{t: t})
			assertChannelNotConfigured(t, getFacts(t, unconfigured, target))
		})
	}
}
