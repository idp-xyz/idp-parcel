package postgres_test

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证本上下文各列表读口的 ADR-0144 契约（票 catalogue-read-pagination-rollout/03）：
// 翻过第一页之后插入一行，后面各页不重出、不漏行，总数随之变——倒序册的新行落在已翻过的那头，正序册的新行落在
// 末尾、在末页出现；total 随筛选与 q 变；q 里的 % 与 _ 按字面；各可排维小页走完得应有的次序；空册答空页、总数
// 为零。同形的几册共用 exercisePaging，每册各过一遍。种的行一律取 SYN- 合成值，所以这里自备构造器，不借别的测试
// 文件里钉着非合成值的那些。

// pagingCase 是一册交给 exercisePaging 的全部输入。
type pagingCase[Row any] struct {
	catalogue *cataloguepage.Catalogue
	// selector 是端点替这一册带上的选择器：读面不看它，但它进游标摘要，翻页时照端点那样带着。
	selector url.Values
	tenant   domain.TenantID
	// seed 种本租户的第 n 行。exercisePaging 先种 1 到 5，翻过第一页之后再种 inserted 那一行：倒序册取排在缺省序
	// 最前的一行，正序册取排在最后的一行。
	seed     func(t *testing.T, n int)
	inserted int
	list     func(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (
		cataloguepage.Result[Row], error)
	id func(Row) string
	// want 是插入之后从第一页一路翻到末页走过的标识：倒序册里没有新行，正序册以新行收尾，原有五行各出现恰好一次。
	want []string
	// narrowed 是在先种的五行上收窄的几问与各自应得的总数。
	narrowed []narrowing
	// sorts 是缺省序之外的排序与它在六行上应得的次序。
	sorts map[string][]string
}

type narrowing struct {
	values url.Values
	total  int64
}

func pagingValues(selector, values url.Values) url.Values {
	merged := url.Values{}
	for key, given := range selector {
		merged[key] = given
	}
	for key, given := range values {
		merged[key] = given
	}
	return merged
}

func idsOf[Row any](rows []Row, id func(Row) string) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, id(row))
	}
	return ids
}

// walkPages 按 limit 2 从第一页走到末页，交回走过的标识与末页的总数。
func walkPages[Row any](t *testing.T, list pagingCase[Row], values url.Values) ([]string, int64) {
	t.Helper()
	page, err := list.list(t.Context(), list.tenant, 2, pageQuery(t, list.catalogue, values))
	if err != nil {
		t.Fatalf("%v 第一页：%v", values, err)
	}
	walked := idsOf(page.Rows, list.id)
	for pages := 1; page.Next != ""; pages++ {
		if pages > 6 {
			t.Fatalf("%v 翻了 %d 页还没到末页：%v", values, pages, walked)
		}
		page, err = list.list(t.Context(), list.tenant, 2, nextPage(t, list.catalogue, values, page.Next))
		if err != nil {
			t.Fatalf("%v 第 %d 页：%v", values, pages+1, err)
		}
		walked = append(walked, idsOf(page.Rows, list.id)...)
	}
	return walked, page.Total
}

func exercisePaging[Row any](t *testing.T, list pagingCase[Row]) {
	t.Helper()
	ctx := t.Context()
	for n := 1; n <= 5; n++ {
		list.seed(t, n)
	}

	for _, narrowing := range list.narrowed {
		values := pagingValues(list.selector, narrowing.values)
		page, err := list.list(ctx, list.tenant, 50, pageQuery(t, list.catalogue, values))
		if err != nil {
			t.Fatalf("%v：%v", narrowing.values, err)
		}
		if page.Total != narrowing.total || int64(len(page.Rows)) != narrowing.total {
			t.Errorf("%v：total = %d 行 %v，want %d", narrowing.values, page.Total, idsOf(page.Rows, list.id), narrowing.total)
		}
	}
	for _, keyword := range []string{"%", "SYN_"} {
		values := pagingValues(list.selector, url.Values{"q": {keyword}})
		page, err := list.list(ctx, list.tenant, 50, pageQuery(t, list.catalogue, values))
		if err != nil || page.Total != 0 || len(page.Rows) != 0 {
			t.Errorf("q=%q 应按字面、一行不中：total = %d 行 %v err = %v", keyword, page.Total, idsOf(page.Rows, list.id), err)
		}
	}

	base := pagingValues(list.selector, url.Values{})
	first, err := list.list(ctx, list.tenant, 2, pageQuery(t, list.catalogue, base))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	if first.Total != 5 || first.Next == "" {
		t.Fatalf("第一页 total = %d next = %q", first.Total, first.Next)
	}
	seen := idsOf(first.Rows, list.id)

	// 第一页翻过之后插一行，游标接着本页末行往后取：倒序册的新行排在已翻过的那头，不进后面的页；正序册的新行排在
	// 末尾，在末页出现。两种都不许已翻过的行重出、未翻的行漏掉。
	list.seed(t, list.inserted)
	page := first
	for pages := 1; page.Next != ""; pages++ {
		if pages > len(list.want) {
			t.Fatalf("翻了 %d 页还没到末页：%v", pages, seen)
		}
		page, err = list.list(ctx, list.tenant, 2, nextPage(t, list.catalogue, base, page.Next))
		if err != nil {
			t.Fatalf("第 %d 页：%v", pages+1, err)
		}
		if page.Total != 6 {
			t.Fatalf("插入之后第 %d 页 total = %d，want 6", pages+1, page.Total)
		}
		seen = append(seen, idsOf(page.Rows, list.id)...)
	}
	if !reflect.DeepEqual(seen, list.want) {
		t.Fatalf("翻完的行 %v，want %v", seen, list.want)
	}

	for sort, want := range list.sorts {
		values := pagingValues(list.selector, url.Values{"sort": {sort}})
		walked, total := walkPages(t, list, values)
		if total != 6 || !reflect.DeepEqual(walked, want) {
			t.Errorf("sort=%s 小页走完得 %v（total %d），want %v", sort, walked, total, want)
		}
	}

	empty, err := list.list(ctx, build(t, domain.NewTenantID, "SYN-TENANT-02"), 2, pageQuery(t, list.catalogue, base))
	if err != nil || len(empty.Rows) != 0 || empty.Total != 0 || empty.Next != "" {
		t.Fatalf("空册：行 %d total = %d next = %q err = %v", len(empty.Rows), empty.Total, empty.Next, err)
	}
}

func numbered(prefix string, ns ...int) []string {
	ids := make([]string, 0, len(ns))
	for _, n := range ns {
		ids = append(ids, fmt.Sprintf("%s%d", prefix, n))
	}
	return ids
}

func params(pairs ...string) url.Values {
	built := url.Values{}
	for index := 0; index+1 < len(pairs); index += 2 {
		built.Add(pairs[index], pairs[index+1])
	}
	return built
}

func hours(n int) time.Duration { return time.Duration(n) * time.Hour }

// pagedProjection 造第 n 件包裹的投影：一条已归类的条目，派生于 projectionBaseAt 之后 n 小时。
func pagedProjection(t *testing.T, n int) domain.TrackingProjection {
	t.Helper()
	parcel := build(t, domain.NewTrackedParcelReference, fmt.Sprintf("SYN-PARCEL-%d", n))
	fact, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      domain.SourceNodeOperations,
		Parcel:      parcel,
		Fact:        build(t, domain.NewSourceFactReference, fmt.Sprintf("SYN-SCAN-%d", n)),
		Kind:        build(t, domain.NewSourceFactKind, "SYN-NODE-INTAKE"),
		Version:     build(t, domain.NewSourceFactVersion, "SYN-V1"),
		OccurredAt:  projectionBaseAt,
		EffectiveAt: projectionBaseAt.Add(time.Hour),
		ReceivedAt:  projectionBaseAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造事实 %d：%v", n, err)
	}
	classified, err := domain.ClassifyMilestone(fact,
		build(t, domain.NewMilestoneReference, "SYN-PICKED-UP"),
		build(t, domain.NewMappingVersionReference, "SYN-MILESTONE-MAP/v1"))
	if err != nil {
		t.Fatalf("归类 %d：%v", n, err)
	}
	projection, err := domain.DeriveTrackingProjection(
		build(t, domain.NewProjectionVersionID, fmt.Sprintf("SYN-VERSION-%d", n)),
		parcel, []domain.MilestoneClassification{classified}, projectionBaseAt.Add(hours(n)))
	if err != nil {
		t.Fatalf("派生投影 %d：%v", n, err)
	}
	return projection
}

// Covers: 追踪投影册——每包裹一行、按 derivedAt 新近在前；q 覆盖包裹与版本标识，不分大小写。
func TestTrackingProjectionPagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	fixture := newProjectionFixture(t)
	tenant := projectionTenant(t, "SYN-TENANT-01")
	exercisePaging(t, pagingCase[domain.TrackingProjection]{
		catalogue: ports.TrackingProjectionPages,
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			projection := pagedProjection(t, n)
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				return fixture.projections.Save(txCtx, tenant, projection)
			})
		},
		inserted: 6,
		list:     fixture.projections.ListCurrent,
		id:       func(projection domain.TrackingProjection) string { return projection.Parcel().String() },
		want:     numbered("SYN-PARCEL-", 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("q", "syn-parcel-3"), 1},
			{params("q", "SYN-VERSION-2"), 1},
			{params("q", "SYN-"), 5},
		},
		sorts: map[string][]string{
			"derivedAt": numbered("SYN-PARCEL-", 1, 2, 3, 4, 5, 6),
			"parcel":    numbered("SYN-PARCEL-", 1, 2, 3, 4, 5, 6),
			"-parcel":   numbered("SYN-PARCEL-", 6, 5, 4, 3, 2, 1),
		},
	})
}

// Covers: 异常案件册——按 establishedAt 新近在前；phase 维内为或、与 q 之间为与。
func TestExceptionCasePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	tenant := reviewValue(t, domain.NewTenantID, "SYN-TENANT-01")
	exercisePaging(t, pagingCase[ports.ExceptionCaseCatalogueRow]{
		catalogue: ports.ExceptionCasePages,
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			establishedAt := caseBaseAt.Add(hours(n))
			phase, firstResponse := "AWAITING_RESPONSE", (*time.Time)(nil)
			if n%2 == 0 {
				responded := establishedAt.Add(30 * time.Minute)
				phase, firstResponse = "IN_PROGRESS", &responded
			}
			if _, err := fixture.pool.Exec(t.Context(),
				`INSERT INTO visibility_exception.exception_case
					(tenant_id, case_id, root_parcel, impact_scope, responsible_team, phase, established_at, first_response)
				 VALUES ($1, $2, $3, 'SYN-SCOPE/v1', 'SYN-TEAM-A', $4, $5, $6)`,
				tenant.String(), fmt.Sprintf("SYN-CASE-%d", n), fmt.Sprintf("SYN-PARCEL-%d", n),
				phase, establishedAt, firstResponse,
			); err != nil {
				t.Fatalf("写入案件 %d：%v", n, err)
			}
		},
		inserted: 6,
		list:     fixture.review.ListExceptionCases,
		id:       func(row ports.ExceptionCaseCatalogueRow) string { return row.CaseID },
		want:     numbered("SYN-CASE-", 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("phase", "IN_PROGRESS"), 2},
			{params("phase", "AWAITING_RESPONSE"), 3},
			{params("phase", "AWAITING_RESPONSE", "phase", "IN_PROGRESS"), 5},
			{params("phase", "CLOSED"), 0},
			{params("q", "SYN-CASE-3"), 1},
			{params("q", "syn-parcel-2"), 1},
			{params("q", "SYN-TEAM", "phase", "IN_PROGRESS"), 2},
		},
		sorts: map[string][]string{
			"rootParcel":     numbered("SYN-CASE-", 1, 2, 3, 4, 5, 6),
			"-rootParcel":    numbered("SYN-CASE-", 6, 5, 4, 3, 2, 1),
			"establishedAt":  numbered("SYN-CASE-", 1, 2, 3, 4, 5, 6),
			"-establishedAt": numbered("SYN-CASE-", 6, 5, 4, 3, 2, 1),
		},
	})
}

// pagedEpisode 造第 n 期发作期连同它的分诊结论：首次命中于 factBaseAt 之后 n 小时。结论不取自动建案——那一格要
// 成对带案件（ports.RaisedSignalRecord），与本册的翻页无关。
func pagedEpisode(t *testing.T, tenant domain.TenantID, n int, outcome domain.TriageOutcome) ports.RaisedSignalRecord {
	t.Helper()
	parcel := build(t, domain.NewTrackedParcelReference, fmt.Sprintf("SYN-PARCEL-%d", n))
	kind := build(t, domain.NewExceptionSignalKindReference, "SYN-STALLED")
	fact := build(t, domain.NewSourceFactReference, fmt.Sprintf("SYN-FACT-%d", n))
	cause, err := domain.SourceFactCausalCondition(domain.SourceTransportFulfillment, fact)
	if err != nil {
		t.Fatalf("因果条件 %d：%v", n, err)
	}
	hitAt := factBaseAt.Add(hours(n))
	episode, err := domain.OpenEpisode(build(t, domain.NewEpisodeID, fmt.Sprintf("SYN-EPISODE-%d", n)), domain.SignalHit{
		Kind:       kind,
		Parcel:     parcel,
		Cause:      cause,
		Rule:       build(t, domain.NewSignalRuleVersionReference, "SYN-SIGNAL-RULE/v1"),
		Confidence: build(t, domain.NewConfidenceReference, "SYN-CONFIDENCE-HIGH"),
		Basis: []domain.SignalFactBasis{{
			Source:    domain.SourceTransportFulfillment,
			Fact:      fact,
			Version:   build(t, domain.NewSourceFactVersion, "SYN-V1"),
			Judgement: fmt.Sprintf("SYN-JUDGEMENT-%d", n),
		}},
		At: hitAt,
	})
	if err != nil {
		t.Fatalf("开启发作期 %d：%v", n, err)
	}
	conclusion, err := domain.ConcludeTriage(episode.ID(), outcome,
		build(t, domain.NewSignalRuleVersionReference, "SYN-TRIAGE-RULE/v1"), hitAt.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("分诊结论 %d：%v", n, err)
	}
	return ports.RaisedSignalRecord{Tenant: tenant, Parcel: parcel, Kind: kind, Episode: episode, Conclusion: conclusion}
}

// Covers: 信号发作期册——按 startedAt 新近在前、同刻按发作期标识倒序；outcome 按行上的分诊结论收窄。
func TestSignalEpisodePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	tenant := reviewValue(t, domain.NewTenantID, "SYN-TENANT-01")
	exercisePaging(t, pagingCase[ports.SignalEpisodeCatalogueRow]{
		catalogue: ports.SignalEpisodePages,
		selector:  params("registry", "signal-episode"),
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			outcome := domain.NoCaseNeeded
			if n%2 == 0 {
				outcome = domain.ManualReviewRequired
			}
			record := pagedEpisode(t, tenant, n, outcome)
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				return fixture.episodes.SaveRaised(txCtx, record)
			})
		},
		inserted: 6,
		list:     fixture.review.ListSignalEpisodes,
		id:       func(row ports.SignalEpisodeCatalogueRow) string { return row.EpisodeID },
		want:     numbered("SYN-EPISODE-", 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("outcome", "MANUAL_REVIEW"), 2},
			{params("outcome", "NO_CASE"), 3},
			{params("outcome", "MANUAL_REVIEW", "outcome", "NO_CASE"), 5},
			{params("outcome", "AUTO_ESTABLISH"), 0},
			{params("q", "SYN-EPISODE-4"), 1},
			{params("q", "syn-parcel-3"), 1},
			{params("q", "SYN-EPISODE-4", "outcome", "NO_CASE"), 0},
		},
		sorts: map[string][]string{
			"startedAt":  numbered("SYN-EPISODE-", 1, 2, 3, 4, 5, 6),
			"-lastHitAt": numbered("SYN-EPISODE-", 6, 5, 4, 3, 2, 1),
			"parcel":     numbered("SYN-EPISODE-", 1, 2, 3, 4, 5, 6),
		},
	})
}

// pagedRequest 造第 n 份处置请求：案件、范围与发送时刻各不相同（「当前至多一份」按案件+动作+范围），偶数份随后记上拒绝。
func pagedRequest(t *testing.T, n int) *domain.DispositionRequest {
	t.Helper()
	sentAt := dispositionBaseAt.Add(hours(n))
	request, err := domain.SendDispositionRequest(domain.DispositionRequestSpec{
		ID:               reviewValue(t, domain.NewDispositionRequestID, fmt.Sprintf("SYN-REQUEST-%d", n)),
		Case:             reviewValue(t, domain.NewCaseID, fmt.Sprintf("SYN-CASE-%d", n)),
		Target:           domain.SourceNetworkRouting,
		Action:           reviewValue(t, domain.NewRequestedActionReference, "SYN-REROUTE"),
		Scope:            reviewValue(t, domain.NewRequestScopeReference, fmt.Sprintf("SYN-PARCEL-%d/remaining", n)),
		Reason:           "SYN route deviation",
		Evidence:         reviewValue(t, domain.NewRequestEvidenceReference, fmt.Sprintf("SYN-EVIDENCE-%d", n)),
		IntentVersion:    1,
		SentAt:           sentAt,
		AcceptanceWindow: sentAt.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造处置请求 %d：%v", n, err)
	}
	return request
}

// Covers: 处置请求册——按 sentAt 新近在前；judgment 按已记下的源判断收窄，未答复的行不中任何词。
func TestDispositionRequestPagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	tenant := reviewValue(t, domain.NewTenantID, "SYN-TENANT-01")
	exercisePaging(t, pagingCase[ports.DispositionRequestCatalogueRow]{
		catalogue: ports.DispositionRequestPages,
		selector:  params("registry", "disposition-request"),
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			request := pagedRequest(t, n)
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				_, err := fixture.requests.Save(txCtx, tenant, request)
				return err
			})
			if n%2 != 0 {
				return
			}
			sent, exists, err := fixture.requests.FindByID(t.Context(), tenant, request.ID())
			if err != nil || !exists {
				t.Fatalf("读回请求 %d：exists=%v err=%v", n, exists, err)
			}
			if err := sent.RecordSourceJudgment(domain.RequestRefused, dispositionBaseAt.Add(hours(n)+time.Hour)); err != nil {
				t.Fatalf("记判断 %d：%v", n, err)
			}
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				_, err := fixture.requests.Save(txCtx, tenant, sent)
				return err
			})
		},
		inserted: 6,
		list:     fixture.review.ListDispositionRequests,
		id:       func(row ports.DispositionRequestCatalogueRow) string { return row.RequestID },
		want:     numbered("SYN-REQUEST-", 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("judgment", "REFUSED"), 2},
			{params("judgment", "ACCEPTED"), 0},
			{params("judgment", "ACCEPTED", "judgment", "REFUSED"), 2},
			{params("q", "SYN-REQUEST-3"), 1},
			{params("q", "syn-case-5"), 1},
			{params("q", "SYN-REROUTE"), 5},
		},
		sorts: map[string][]string{
			"sentAt": numbered("SYN-REQUEST-", 1, 2, 3, 4, 5, 6),
		},
	})
}

// pagedNotification 造第 n 份客户通知：披露决定于 etaGapBaseAt 之后 n 小时，客户与发作期各不相同。
func pagedNotification(t *testing.T, n int) *domain.CustomerNotification {
	t.Helper()
	decidedAt := etaGapBaseAt.Add(hours(n))
	decision, err := domain.DecideDisclosure(
		build(t, domain.NewEpisodeID, fmt.Sprintf("SYN-EPISODE-%d", n)),
		build(t, domain.NewCustomerAccountReference, fmt.Sprintf("SYN-CUSTOMER-%d", n)),
		build(t, domain.NewDisclosurePolicyReference, "SYN-DISCLOSURE-POLICY/v1"),
		domain.DiscloseToCustomer,
		build(t, domain.NewDisclosureContentReference, "SYN-CONTENT/v1"),
		decidedAt,
	)
	if err != nil {
		t.Fatalf("构造披露决定 %d：%v", n, err)
	}
	notification, err := domain.GenerateNotification(
		build(t, domain.NewNotificationID, fmt.Sprintf("SYN-NOTICE-%d", n)),
		decision,
		decidedAt.Add(24*time.Hour),
		build(t, domain.NewNotificationChannelReference, "SYN-PORTAL-MESSAGE/v1"),
		build(t, domain.NewDisclosurePolicyReference, "SYN-NOTIFY-POLICY/v1"),
		decidedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("构造通知 %d：%v", n, err)
	}
	return notification
}

// Covers: 客户通知册——按 decidedAt 新近在前；不开筛选维，q 覆盖通知、客户、发作期与渠道。
func TestCustomerNotificationPagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	tenant := reviewValue(t, domain.NewTenantID, "SYN-TENANT-01")
	exercisePaging(t, pagingCase[ports.CustomerNotificationCatalogueRow]{
		catalogue: ports.CustomerNotificationPages,
		selector:  params("registry", "customer-notification"),
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			notification := pagedNotification(t, n)
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				_, err := fixture.notifications.Save(txCtx, tenant, notification)
				return err
			})
		},
		inserted: 6,
		list:     fixture.review.ListCustomerNotifications,
		id:       func(row ports.CustomerNotificationCatalogueRow) string { return row.NotificationID },
		want:     numbered("SYN-NOTICE-", 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("q", "SYN-NOTICE-2"), 1},
			{params("q", "syn-customer-4"), 1},
			{params("q", "SYN-EPISODE"), 5},
		},
		sorts: map[string][]string{
			"decidedAt": numbered("SYN-NOTICE-", 1, 2, 3, 4, 5, 6),
		},
	})
}

// pagedClaim 造批次 batch 里的索赔项 item：各项同刻提交（claimBaseAt），次序全靠（批次，项）收尾。
func pagedClaim(t *testing.T, batch, item string) *domain.ClaimItem {
	t.Helper()
	claim, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:             build(t, domain.NewClaimItemID, item),
		Batch:          build(t, domain.NewClaimBatchReference, batch),
		DocumentNumber: build(t, domain.NewDocumentNumber, "SYN-CLM-"+batch+"-"+item),
		Customer:       build(t, domain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
		Applicant:      build(t, domain.NewApplicantReference, "SYN-APPLICANT-1"),
		Contract:       build(t, domain.NewContractScopeReference, "SYN-CONTRACT-SCOPE/v1"),
		Target:         build(t, domain.NewRequestScopeReference, "SYN-PARCEL-1/loss"),
		Kind:           build(t, domain.NewClaimKindReference, "SYN-LOSS"),
		SubmittedAt:    claimBaseAt,
	})
	if err != nil {
		t.Fatalf("受理索赔 %s/%s：%v", batch, item, err)
	}
	return claim
}

// Covers: 索赔项册——同刻提交的行全靠（批次，项）收尾，翻页照样不重不漏；screen 与 conclusion 两维之间为与。
func TestClaimItemPagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	tenant := reviewValue(t, domain.NewTenantID, "SYN-TENANT-01")
	claimID := func(row ports.ClaimItemCatalogueRow) string { return row.Batch + "/" + row.ItemID }
	exercisePaging(t, pagingCase[ports.ClaimItemCatalogueRow]{
		catalogue: ports.ClaimItemPages,
		selector:  params("registry", "claim-item"),
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			batch := "SYN-BATCH-A"
			if n == 0 {
				batch = "SYN-BATCH-B"
			}
			claim := pagedClaim(t, batch, fmt.Sprintf("SYN-ITEM-%d", n))
			if n%2 != 0 {
				if err := claim.ScreenEligibility(domain.ClaimEligible, "SYN basis", claimBaseAt.Add(time.Hour)); err != nil {
					t.Fatalf("资格审核 %d：%v", n, err)
				}
			}
			if n == 1 {
				if err := claim.ConcludeLiability(domain.LiabilityNotEstablished,
					claimBaseAt.Add(240*time.Hour), claimBaseAt.Add(3*time.Hour)); err != nil {
					t.Fatalf("形成结论 %d：%v", n, err)
				}
			}
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				_, err := fixture.claims.Save(txCtx, tenant, claim)
				return err
			})
		},
		inserted: 0,
		list:     fixture.review.ListClaimItems,
		id:       claimID,
		want:     numbered("SYN-BATCH-A/SYN-ITEM-", 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("screen", "ELIGIBLE"), 3},
			{params("screen", "INELIGIBLE"), 0},
			{params("conclusion", "NOT_ESTABLISHED"), 1},
			{params("screen", "ELIGIBLE", "conclusion", "NOT_ESTABLISHED"), 1},
			{params("q", "SYN-ITEM-2"), 1},
			{params("q", "syn-clm-syn-batch-a-syn-item-4"), 1},
			{params("q", "SYN-ITEM", "screen", "ELIGIBLE"), 3},
		},
		sorts: map[string][]string{
			"submittedAt": append(numbered("SYN-BATCH-A/SYN-ITEM-", 1, 2, 3, 4, 5), "SYN-BATCH-B/SYN-ITEM-0"),
		},
	})
}

// pagedMatter 造第 n 项追偿事项：各项同刻建立（claimBaseAt），次序全靠事项标识收尾。
func pagedMatter(t *testing.T, n int) domain.RecoveryMatter {
	t.Helper()
	matter, err := domain.OpenRecoveryMatter(domain.RecoveryMatterSpec{
		ID:           build(t, domain.NewRecoveryMatterID, fmt.Sprintf("SYN-MATTER-%d", n)),
		Case:         build(t, domain.NewCaseID, fmt.Sprintf("SYN-CASE-%d", n)),
		Counterparty: build(t, domain.NewCounterpartyReference, fmt.Sprintf("SYN-PARTY-%d", n)),
		Basis:        build(t, domain.NewLiabilityBasisReference, "SYN-SUPPLIER-AGREEMENT/v1"),
		LegalEntity:  build(t, domain.NewLegalEntityReference, "SYN-ENTITY-1"),
		Scope:        build(t, domain.NewRequestScopeReference, fmt.Sprintf("SYN-PARCEL-%d/loss", n)),
		Evidence:     build(t, domain.NewRequestEvidenceReference, fmt.Sprintf("SYN-EVIDENCE-%d", n)),
		Deadline:     claimBaseAt.Add(14 * 24 * time.Hour),
		OpenedAt:     claimBaseAt,
	})
	if err != nil {
		t.Fatalf("建立追偿事项 %d：%v", n, err)
	}
	return matter
}

// Covers: 追偿事项册——同刻建立的行按事项标识倒序收尾；q 覆盖事项、案件与对方。
func TestRecoveryMatterPagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	tenant := reviewValue(t, domain.NewTenantID, "SYN-TENANT-01")
	exercisePaging(t, pagingCase[ports.RecoveryMatterCatalogueRow]{
		catalogue: ports.RecoveryMatterPages,
		selector:  params("registry", "recovery-matter"),
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			matter := pagedMatter(t, n)
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				_, err := fixture.recoveries.Save(txCtx, tenant, matter)
				return err
			})
		},
		inserted: 6,
		list:     fixture.review.ListRecoveryMatters,
		id:       func(row ports.RecoveryMatterCatalogueRow) string { return row.MatterID },
		want:     numbered("SYN-MATTER-", 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("q", "SYN-MATTER-3"), 1},
			{params("q", "syn-party-2"), 1},
			{params("q", "SYN-CASE-4"), 1},
		},
		sorts: map[string][]string{
			"openedAt": numbered("SYN-MATTER-", 1, 2, 3, 4, 5, 6),
		},
	})
}

// versionedCatalogue 是区间型目录共用的翻页用例：第 n 版生效于 listBaseAt 之后 n 个 48 小时，后一版接续闭合前一版，
// 都在册上；按生效起点新近在前，翻过第一页之后登的第 6 版排在已翻过的那头；q 覆盖版本与批准人。
func versionedCatalogue[Row any](
	t *testing.T,
	kind string,
	catalogue *cataloguepage.Catalogue,
	register func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, version string, from time.Time),
	list func(*testing.T, *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
		cataloguepage.Result[Row], error),
	version func(Row) string,
) {
	t.Helper()
	fixture := newRegistrarFixture(t)
	tenant := listTenant(t, "SYN-TENANT-01")
	prefix := "SYN-" + kind + "/v"
	exercisePaging(t, pagingCase[Row]{
		catalogue: catalogue,
		selector:  params("kind", kind),
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			register(t, fixture, tenant, fmt.Sprintf("%s%d", prefix, n), listBaseAt.Add(hours(48*n)))
		},
		inserted: 6,
		list:     list(t, fixture),
		id:       version,
		want:     numbered(prefix, 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("q", fmt.Sprintf("%s3", prefix)), 1},
			{params("q", "syn-approver"), 5},
		},
		sorts: map[string][]string{
			"effectiveFrom": numbered(prefix, 1, 2, 3, 4, 5, 6),
		},
	})
}

func versionHeader(version string, from time.Time) ports.CatalogVersionHeader {
	return ports.CatalogVersionHeader{Version: version, ApprovedBy: "SYN-APPROVER", EffectiveFrom: from}
}

func TestMilestoneMappingCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	versionedCatalogue(t, "MILESTONE_MAPPING", ports.MilestoneMappingCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, version string, from time.Time) {
			t.Helper()
			registration := ports.MilestoneMappingRegistration{
				Header:  versionHeader(version, from),
				Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "SYN-NODE-INTAKE", "SYN-PICKED-UP")},
			}
			registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
				return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, registration)
			})
		},
		func(t *testing.T, fixture *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
			cataloguepage.Result[ports.MilestoneMappingCatalogueRow], error) {
			return listCatalogue(t, fixture).ListMilestoneMappings
		},
		func(row ports.MilestoneMappingCatalogueRow) string { return row.Version })
}

func TestTriageRuleCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	versionedCatalogue(t, "TRIAGE_RULE", ports.TriageRuleCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, version string, from time.Time) {
			t.Helper()
			registration := ports.TriageRuleRegistration{
				Header: versionHeader(version, from),
				Entries: []ports.TriageRuleEntry{{
					Kind:       build(t, domain.NewExceptionSignalKindReference, "SYN-DELIVERY-FAILED"),
					Confidence: build(t, domain.NewConfidenceReference, "SYN-CARRIER-CONFIRMED"),
					Outcome:    domain.AutoEstablishCase,
					Team:       build(t, domain.NewResponsibleTeamReference, "SYN-TEAM-DELIVERY"),
				}},
			}
			registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
				return fixture.registrar.RegisterTriageRules(txCtx, tenant, registration)
			})
		},
		func(t *testing.T, fixture *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
			cataloguepage.Result[ports.TriageRuleCatalogueRow], error) {
			return listCatalogue(t, fixture).ListTriageRules
		},
		func(row ports.TriageRuleCatalogueRow) string { return row.Version })
}

func TestDisclosurePolicyCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	versionedCatalogue(t, "DISCLOSURE_POLICY", ports.DisclosurePolicyCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, version string, from time.Time) {
			t.Helper()
			milestones, err := domain.ShowDimension(build(t, domain.NewViewContentReference, "SYN-PUBLIC-MILESTONES/v1"))
			if err != nil {
				t.Fatalf("构造展示维：%v", err)
			}
			note, err := domain.ShowDimension(build(t, domain.NewViewContentReference, "SYN-NOTE/plain"))
			if err != nil {
				t.Fatalf("构造说明维：%v", err)
			}
			registration := ports.DisclosurePolicyRegistration{
				Header: versionHeader(version, from),
				Entries: []ports.DisclosurePolicyEntry{{
					Customer:   build(t, domain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
					Milestones: milestones,
					ETA:        domain.PendDimension(),
					Final:      domain.WithholdDimension(),
					Note:       note,
				}},
			}
			registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
				return fixture.registrar.RegisterDisclosurePolicy(txCtx, tenant, registration)
			})
		},
		func(t *testing.T, fixture *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
			cataloguepage.Result[ports.DisclosurePolicyCatalogueRow], error) {
			return listCatalogue(t, fixture).ListDisclosurePolicies
		},
		func(row ports.DisclosurePolicyCatalogueRow) string { return row.Version })
}

func TestExceptionDisclosureRuleCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	versionedCatalogue(t, "EXCEPTION_DISCLOSURE_RULE", ports.ExceptionDisclosureRuleCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, version string, from time.Time) {
			t.Helper()
			registration := ports.ExceptionDisclosureRuleRegistration{
				Header: versionHeader(version, from),
				Entries: []ports.ExceptionDisclosureRuleEntry{{
					Customer:    build(t, domain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
					Kind:        build(t, domain.NewExceptionSignalKindReference, "SYN-DELIVERY-FAILED"),
					Confidence:  build(t, domain.NewConfidenceReference, "SYN-CARRIER-CONFIRMED"),
					Disclosable: true,
					AutoRelease: true,
					Content:     build(t, domain.NewDisclosureContentReference, "SYN-DELIVERY-FAILED/plain"),
				}},
			}
			registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
				return fixture.registrar.RegisterExceptionDisclosureRules(txCtx, tenant, registration)
			})
		},
		func(t *testing.T, fixture *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
			cataloguepage.Result[ports.ExceptionDisclosureRuleCatalogueRow], error) {
			return listCatalogue(t, fixture).ListExceptionDisclosureRules
		},
		func(row ports.ExceptionDisclosureRuleCatalogueRow) string { return row.Version })
}

// keyedCatalogue 是按键登记的目录共用的翻页用例：缺省序是键升序，翻过第一页之后登的第 6 号键排在末尾。
func keyedCatalogue[Row any](
	t *testing.T,
	kind, sortKey, keyPrefix string,
	catalogue *cataloguepage.Catalogue,
	register func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, key string),
	list func(*testing.T, *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
		cataloguepage.Result[Row], error),
	key func(Row) string,
	narrowed []narrowing,
) {
	t.Helper()
	fixture := newRegistrarFixture(t)
	tenant := listTenant(t, "SYN-TENANT-01")
	exercisePaging(t, pagingCase[Row]{
		catalogue: catalogue,
		selector:  params("kind", kind),
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			register(t, fixture, tenant, fmt.Sprintf("%s%d", keyPrefix, n))
		},
		inserted: 6,
		list:     list(t, fixture),
		id:       key,
		want:     numbered(keyPrefix, 1, 2, 3, 4, 5, 6),
		narrowed: narrowed,
		sorts: map[string][]string{
			"-" + sortKey: numbered(keyPrefix, 6, 5, 4, 3, 2, 1),
		},
	})
}

func TestNotificationPolicyCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	keyedCatalogue(t, "NOTIFICATION_POLICY", "policy", "SYN-POLICY-", ports.NotificationPolicyCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, key string) {
			t.Helper()
			registration := ports.NotificationPolicyRegistration{
				Policy:        build(t, domain.NewDisclosurePolicyReference, key),
				Channel:       build(t, domain.NewNotificationChannelReference, "SYN-EMAIL"),
				DeadlineAfter: 72 * time.Hour,
				Obligation:    build(t, domain.NewDisclosurePolicyReference, "SYN-ACK-REQUIRED"),
				ApprovedBy:    "SYN-APPROVER",
			}
			registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
				return fixture.registrar.RegisterNotificationPolicy(txCtx, tenant, registration)
			})
		},
		func(t *testing.T, fixture *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
			cataloguepage.Result[ports.NotificationPolicyCatalogueRow], error) {
			return listCatalogue(t, fixture).ListNotificationPolicies
		},
		func(row ports.NotificationPolicyCatalogueRow) string { return row.Policy },
		[]narrowing{{params("q", "SYN-POLICY-3"), 1}, {params("q", "email"), 5}})
}

func TestClaimEligibilityCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	keyedCatalogue(t, "CLAIM_ELIGIBILITY", "contract", "SYN-CONTRACT-", ports.ClaimEligibilityCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, key string) {
			t.Helper()
			registration := ports.ClaimEligibilityRegistration{
				Header:       ports.CatalogApprovalHeader{Version: "SYN-ELIGIBILITY/v1", ApprovedBy: "SYN-APPROVER"},
				Contract:     build(t, domain.NewContractScopeReference, key),
				Customer:     build(t, domain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
				CoveredKinds: []domain.ClaimKindReference{build(t, domain.NewClaimKindReference, "SYN-LOSS")},
			}
			registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
				return fixture.registrar.RegisterClaimEligibility(txCtx, tenant, registration)
			})
		},
		func(t *testing.T, fixture *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
			cataloguepage.Result[ports.ClaimEligibilityCatalogueRow], error) {
			return listCatalogue(t, fixture).ListClaimEligibilities
		},
		func(row ports.ClaimEligibilityCatalogueRow) string { return row.Contract },
		[]narrowing{{params("q", "SYN-CONTRACT-2"), 1}, {params("q", "syn-eligibility"), 5}})
}

func TestClaimAuthorizationCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	keyedCatalogue(t, "CLAIM_AUTHORIZATION", "customer", "SYN-CUSTOMER-", ports.ClaimAuthorizationCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, key string) {
			t.Helper()
			registration := ports.ClaimAuthorizationRegistration{
				Header:     ports.CatalogApprovalHeader{Version: "SYN-AUTH/v1", ApprovedBy: "SYN-APPROVER"},
				Customer:   build(t, domain.NewCustomerAccountReference, key),
				Applicants: []domain.ApplicantReference{build(t, domain.NewApplicantReference, "SYN-APPLICANT-1")},
			}
			registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
				return fixture.registrar.RegisterClaimAuthorization(txCtx, tenant, registration)
			})
		},
		func(t *testing.T, fixture *registrarFixture) func(context.Context, domain.TenantID, int, cataloguepage.Query) (
			cataloguepage.Result[ports.ClaimAuthorizationCatalogueRow], error) {
			return listCatalogue(t, fixture).ListClaimAuthorizations
		},
		func(row ports.ClaimAuthorizationCatalogueRow) string { return row.Customer },
		[]narrowing{{params("q", "SYN-CUSTOMER-4"), 1}, {params("q", "syn-auth"), 5}})
}

// Covers: 冲突信号规则册一租户至多一行（登记表主键只有租户），翻页期间插入在这一册上无从演示；这里证那一行一页即
// 末页、total 为 1，q 收窄与按字面，空册答空页。
func TestConflictSignalRuleCataloguePagesAnswerTheSingleRow(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "SYN-TENANT-01")
	registration := ports.ConflictSignalRuleRegistration{
		Kind:       build(t, domain.NewExceptionSignalKindReference, "SYN-FACT-CONFLICT"),
		Rule:       build(t, domain.NewSignalRuleVersionReference, "SYN-CONFLICT/v1"),
		Confidence: build(t, domain.NewConfidenceReference, "SYN-CHAIN-FORK"),
		ApprovedBy: "SYN-APPROVER",
	}
	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterConflictSignalRule(txCtx, tenant, registration)
	})
	selector := params("kind", "CONFLICT_SIGNAL_RULE")

	page, err := catalogue.ListConflictSignalRules(t.Context(), tenant, 1,
		pageQuery(t, ports.ConflictSignalRuleCataloguePages, selector))
	if err != nil || len(page.Rows) != 1 || page.Total != 1 || page.Next != "" {
		t.Fatalf("单行册：行 %d total = %d next = %q err = %v", len(page.Rows), page.Total, page.Next, err)
	}
	for keyword, total := range map[string]int64{"syn-conflict": 1, "SYN-OTHER": 0, "%": 0, "SYN_": 0} {
		narrowed, err := catalogue.ListConflictSignalRules(t.Context(), tenant, 1,
			pageQuery(t, ports.ConflictSignalRuleCataloguePages, pagingValues(selector, params("q", keyword))))
		if err != nil || narrowed.Total != total || int64(len(narrowed.Rows)) != total {
			t.Errorf("q=%q：total = %d 行 %d err = %v，want %d", keyword, narrowed.Total, len(narrowed.Rows), err, total)
		}
	}
	empty, err := catalogue.ListConflictSignalRules(t.Context(), listTenant(t, "SYN-TENANT-02"), 1,
		pageQuery(t, ports.ConflictSignalRuleCataloguePages, selector))
	if err != nil || len(empty.Rows) != 0 || empty.Total != 0 || empty.Next != "" {
		t.Fatalf("空册：行 %d total = %d err = %v", len(empty.Rows), empty.Total, err)
	}
}
