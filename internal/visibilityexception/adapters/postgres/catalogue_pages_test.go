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
// 翻页期间插入新行，已翻过的页不重出、未翻的页不漏行，总数随之变；total 随筛选与 q 变；q 里的 % 与 _ 按字面；
// 各可排维小页走完与一页取完同序；空册答空页、总数为零。同形的几册共用 exercisePaging，每册各过一遍。

// pagingCase 是一册交给 exercisePaging 的全部输入。
type pagingCase[Row any] struct {
	catalogue *cataloguepage.Catalogue
	// selector 是端点替这一册带上的选择器：读面不看它，但它进游标摘要，翻页时照端点那样带着。
	selector url.Values
	tenant   domain.TenantID
	// seed 种本租户的第 n 行；exercisePaging 先种 1 到 5，翻过第一页之后再种 front——它须排在缺省序最前面。
	seed  func(t *testing.T, n int)
	front int
	list  func(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (
		cataloguepage.Result[Row], error)
	id func(Row) string
	// want 是先种的五行在缺省序里的标识。
	want []string
	// narrowed 是在这五行上收窄的几问与各自应得的总数。
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

	// 第一页翻过之后在缺省序最前面插一行：游标接着本页末行往后取，这一行不进后面的页，已翻过的行也不重出。
	list.seed(t, list.front)
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

// Covers: 追踪投影册——每包裹一行、按 derivedAt 新近在前；q 覆盖包裹与版本标识，不分大小写。
func TestTrackingProjectionPagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	fixture := newProjectionFixture(t)
	tenant := projectionTenant(t, "SYN-TENANT-01")
	exercisePaging(t, pagingCase[domain.TrackingProjection]{
		catalogue: ports.TrackingProjectionPages,
		tenant:    tenant,
		seed: func(t *testing.T, n int) {
			t.Helper()
			projection := listProjection(t, fmt.Sprintf("SYN-PARCEL-%d", n), fmt.Sprintf("SYN-VERSION-%d", n),
				projectionBaseAt.Add(hours(n)))
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				return fixture.projections.Save(txCtx, tenant, projection)
			})
		},
		front: 6,
		list:  fixture.projections.ListCurrent,
		id:    func(projection domain.TrackingProjection) string { return projection.Parcel().String() },
		want:  numbered("SYN-PARCEL-", 5, 4, 3, 2, 1),
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
		front: 6,
		list:  fixture.review.ListExceptionCases,
		id:    func(row ports.ExceptionCaseCatalogueRow) string { return row.CaseID },
		want:  numbered("SYN-CASE-", 5, 4, 3, 2, 1),
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
			parcel := fmt.Sprintf("SYN-PARCEL-%d", n)
			outcome := domain.NoCaseNeeded
			if n%2 == 0 {
				outcome = domain.ManualReviewRequired
			}
			episode := openedEpisode(t, fmt.Sprintf("SYN-EPISODE-%d", n), parcel, "SYN-STALLED", factBaseAt.Add(hours(n)))
			record := raisedRecord(t, episode, tenant.String(), parcel, "SYN-STALLED", outcome)
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				return fixture.episodes.SaveRaised(txCtx, record)
			})
		},
		front: 6,
		list:  fixture.review.ListSignalEpisodes,
		id:    func(row ports.SignalEpisodeCatalogueRow) string { return row.EpisodeID },
		want:  numbered("SYN-EPISODE-", 5, 4, 3, 2, 1),
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
		front: 6,
		list:  fixture.review.ListDispositionRequests,
		id:    func(row ports.DispositionRequestCatalogueRow) string { return row.RequestID },
		want:  numbered("SYN-REQUEST-", 5, 4, 3, 2, 1),
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
			notification := reviewNotification(t, fmt.Sprintf("SYN-NOTICE-%d", n), fmt.Sprintf("SYN-CUSTOMER-%d", n),
				fmt.Sprintf("SYN-EPISODE-%d", n), etaGapBaseAt.Add(hours(n)))
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				_, err := fixture.notifications.Save(txCtx, tenant, notification)
				return err
			})
		},
		front: 6,
		list:  fixture.review.ListCustomerNotifications,
		id:    func(row ports.CustomerNotificationCatalogueRow) string { return row.NotificationID },
		want:  numbered("SYN-NOTICE-", 5, 4, 3, 2, 1),
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
			claim := receivedClaim(t, batch, fmt.Sprintf("SYN-ITEM-%d", n))
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
		front: 0,
		list:  fixture.review.ListClaimItems,
		id:    claimID,
		want:  numbered("SYN-BATCH-A/SYN-ITEM-", 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("screen", "ELIGIBLE"), 3},
			{params("screen", "INELIGIBLE"), 0},
			{params("conclusion", "NOT_ESTABLISHED"), 1},
			{params("screen", "ELIGIBLE", "conclusion", "NOT_ESTABLISHED"), 1},
			{params("q", "SYN-ITEM-2"), 1},
			{params("q", "clm-syn-batch-a-syn-item-4"), 1},
			{params("q", "SYN-ITEM", "screen", "ELIGIBLE"), 3},
		},
		sorts: map[string][]string{
			"submittedAt": append(numbered("SYN-BATCH-A/SYN-ITEM-", 1, 2, 3, 4, 5), "SYN-BATCH-B/SYN-ITEM-0"),
		},
	})
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
			matter := openedMatter(t, fmt.Sprintf("SYN-MATTER-%d", n), fmt.Sprintf("SYN-CASE-%d", n),
				fmt.Sprintf("SYN-PARTY-%d", n), fmt.Sprintf("SYN-PARCEL-%d/loss", n))
			fixture.inTx(t, t.Context(), func(txCtx context.Context) error {
				_, err := fixture.recoveries.Save(txCtx, tenant, matter)
				return err
			})
		},
		front: 6,
		list:  fixture.review.ListRecoveryMatters,
		id:    func(row ports.RecoveryMatterCatalogueRow) string { return row.MatterID },
		want:  numbered("SYN-MATTER-", 5, 4, 3, 2, 1),
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
// 都在册上；按生效起点新近在前，q 覆盖版本与批准人。
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
		front: 6,
		list:  list(t, fixture),
		id:    version,
		want:  numbered(prefix, 5, 4, 3, 2, 1),
		narrowed: []narrowing{
			{params("q", fmt.Sprintf("%s3", prefix)), 1},
			{params("q", "syn-approver"), 5},
		},
		sorts: map[string][]string{
			"effectiveFrom": numbered(prefix, 1, 2, 3, 4, 5, 6),
		},
	})
}

func TestMilestoneMappingCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	versionedCatalogue(t, "MILESTONE_MAPPING", ports.MilestoneMappingCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, version string, from time.Time) {
			t.Helper()
			registration := ports.MilestoneMappingRegistration{
				Header:  ports.CatalogVersionHeader{Version: version, ApprovedBy: "SYN-APPROVER", EffectiveFrom: from},
				Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "PICKED_UP")},
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
			registration := triageRegistration(t, version, from)
			registration.Header.ApprovedBy = "SYN-APPROVER"
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
			registration := disclosureRegistration(t, version, from)
			registration.Header.ApprovedBy = "SYN-APPROVER"
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
			registration := exceptionDisclosureRuleRegistration(t, version, from)
			registration.Header.ApprovedBy = "SYN-APPROVER"
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

// keyedCatalogue 是按键登记的目录共用的翻页用例：键升序（缺省序），第 0 号键排在最前，翻过第一页之后才登。
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
		front:    0,
		list:     list(t, fixture),
		id:       key,
		want:     numbered(keyPrefix, 1, 2, 3, 4, 5),
		narrowed: narrowed,
		sorts: map[string][]string{
			"-" + sortKey: numbered(keyPrefix, 5, 4, 3, 2, 1, 0),
		},
	})
}

func TestNotificationPolicyCataloguePagesHoldUnderInsertsAndNarrowing(t *testing.T) {
	keyedCatalogue(t, "NOTIFICATION_POLICY", "policy", "SYN-POLICY-", ports.NotificationPolicyCataloguePages,
		func(t *testing.T, fixture *registrarFixture, tenant domain.TenantID, key string) {
			t.Helper()
			registration := notificationRegistration(t, key)
			registration.ApprovedBy = "SYN-APPROVER"
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
			registration := eligibilityRegistration(t, key)
			registration.Header.Version = "SYN-ELIGIBILITY/v1"
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
			registration := authorizationRegistration(t, key, "SYN-APPLICANT-1")
			registration.Header.Version = "SYN-AUTH/v1"
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

// Covers: 冲突信号规则册一租户至多一行，翻页期间插入在这一册上无从演示；这里证那一行一页即末页、total 为 1，q 收窄
// 与按字面，空册答空页。
func TestConflictSignalRuleCataloguePagesAnswerTheSingleRow(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "SYN-TENANT-01")
	registration := conflictSignalRuleRegistration(t, "SYN-CONFLICT/v1")
	registration.ApprovedBy = "SYN-APPROVER"
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
