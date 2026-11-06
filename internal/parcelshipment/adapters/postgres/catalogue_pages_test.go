package postgres_test

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证本上下文各册迁到 ADR-0144 之后的取页（票 catalogue-read-pagination-rollout/02）。每册都过同样几格：
// 按小页走完全部页，第一页之后插入一行新行——倒序册的新行落在已翻过的那头、后面各页不再出现；正序队列的新行排在末尾——原有各行
// 都恰好出现一次，不重出、不漏行；筛选维或 q 改变 total；空册答空页且 total 为 0；带计数的队列积压数不随条件变。

// walk 用同一组条件按页大小 size 走完全部页，交回每页的标识与末页的 total；between 不为 nil 时在第一页之后调一次（插入新行）。
func walk[Row any](
	t *testing.T,
	catalogue *cataloguepage.Catalogue,
	values url.Values,
	list func(cataloguepage.Query) (cataloguepage.Result[Row], error),
	id func(Row) string,
	between func(),
) ([][]string, int64) {
	t.Helper()
	var pages [][]string
	query := pageQuery(t, catalogue, values)
	for {
		page, err := list(query)
		if err != nil {
			t.Fatalf("取页：%v", err)
		}
		ids := make([]string, len(page.Rows))
		for index, row := range page.Rows {
			ids[index] = id(row)
		}
		pages = append(pages, ids)
		if page.Next == "" {
			return pages, page.Total
		}
		if len(pages) > 20 {
			t.Fatal("翻了二十页还没到末页：游标没有前进")
		}
		if len(pages) == 1 && between != nil {
			between()
		}
		query = nextPage(t, catalogue, values, page.Next)
	}
}

func expectPages(t *testing.T, what string, got [][]string, total int64, want [][]string, wantTotal int64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) || total != wantTotal {
		t.Fatalf("%s：得 %v / total %d，应为 %v / %d", what, got, total, want, wantTotal)
	}
}

func summaryID(summary ports.ShipmentRequestSummaryRecord) string {
	return summary.ShipmentRequestID.String()
}

// Covers: 委托查阅按提交时刻倒序翻页，翻页途中新提交的委托落在已翻过的那头；作用域外的账户与别的租户不出现；state 落在存代码的列上
// （SUBMITTED 命中、ACCEPTED 零行），customerAccountId 在作用域之内收窄，q 命中来源请求键；空册 total 为 0。
func TestShipmentRequestViewsPageWithinTheScope(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()
	seed := func(tenant, account, key, id string, at time.Duration) {
		mustInsert(t, transactor, ctx, repository,
			scopedSubmittedRequest(t, tenant, account, key, id, submittedAtFixture.Add(at)))
	}
	seed("SYN-TENANT-01", "SYN-CUSTOMER-01", "SYN-VIEW-A", "SYN-REQ-A", 0)
	seed("SYN-TENANT-01", "SYN-CUSTOMER-01", "SYN-VIEW-B", "SYN-REQ-B", time.Hour)
	seed("SYN-TENANT-01", "SYN-CUSTOMER-02", "SYN-VIEW-C", "SYN-REQ-C", 2*time.Hour)
	seed("SYN-TENANT-01", "SYN-CUSTOMER-03", "SYN-VIEW-D", "SYN-REQ-D", 3*time.Hour)
	seed("SYN-TENANT-02", "SYN-CUSTOMER-01", "SYN-VIEW-E", "SYN-REQ-E", 4*time.Hour)
	scope := viewScope(t, "SYN-TENANT-01", "SYN-CUSTOMER-01", "SYN-CUSTOMER-02")
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.ShipmentRequestSummaryRecord], error) {
		return views.ListVisible(ctx, scope, 2, query)
	}

	pages, total := walk(t, ports.ShipmentRequestViewPages, url.Values{}, list, summaryID, func() {
		seed("SYN-TENANT-01", "SYN-CUSTOMER-02", "SYN-VIEW-F", "SYN-REQ-F", 5*time.Hour)
	})
	expectPages(t, "翻页途中新提交一份", pages, total, [][]string{{"SYN-REQ-C", "SYN-REQ-B"}, {"SYN-REQ-A"}}, 4)
	for name, test := range map[string]struct {
		values url.Values
		want   [][]string
	}{
		"按账户收窄":     {values: url.Values{"customerAccountId": {"SYN-CUSTOMER-01"}}, want: [][]string{{"SYN-REQ-B", "SYN-REQ-A"}}},
		"作用域外的账户":   {values: url.Values{"customerAccountId": {"SYN-CUSTOMER-03"}}, want: [][]string{{}}},
		"状态落在代码列上":  {values: url.Values{"state": {"SUBMITTED"}}, want: [][]string{{"SYN-REQ-F", "SYN-REQ-C"}, {"SYN-REQ-B", "SYN-REQ-A"}}},
		"没有已接受的":    {values: url.Values{"state": {"ACCEPTED"}}, want: [][]string{{}}},
		"q 命中来源请求键": {values: url.Values{"q": {"syn-view-b"}}, want: [][]string{{"SYN-REQ-B"}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, total := walk(t, ports.ShipmentRequestViewPages, test.values, list, summaryID, nil)
			wantTotal := 0
			for _, page := range test.want {
				wantTotal += len(page)
			}
			expectPages(t, name, got, total, test.want, int64(wantTotal))
		})
	}
	empty, err := views.ListVisible(ctx, viewScope(t, "SYN-TENANT-09", "SYN-CUSTOMER-01"), 2, firstPage(t, ports.ShipmentRequestViewPages))
	if err != nil || len(empty.Rows) != 0 || empty.Total != 0 || empty.Next != "" {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}

// Covers: 复核队列按提交时刻正序（先来先审）翻页，翻页途中新停等复核的委托排在末尾；customerAccountId 收窄后 total 随之变；空册 total 为 0。
func TestAcceptanceReviewQueuePagesOldestFirst(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()
	await := func(index int, account string) {
		id := fmt.Sprintf("SYN-REQ-RVW-%d", index)
		seedAdvanced(t, repository, transactor, "SYN-TENANT-01", account, fmt.Sprintf("SYN-ARQ-%d", index), id,
			submittedAtFixture.Add(time.Duration(index)*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
				return awaitReview(t, request, "SYN-REVIEW-"+id)
			})
	}
	await(1, "SYN-CUSTOMER-01")
	await(2, "SYN-CUSTOMER-02")
	await(3, "SYN-CUSTOMER-01")
	scope := viewScope(t, "SYN-TENANT-01", "SYN-CUSTOMER-01", "SYN-CUSTOMER-02")
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.AcceptanceReviewQueueRecord], error) {
		return views.ListAwaitingManualReview(ctx, scope, 2, query)
	}
	id := func(record ports.AcceptanceReviewQueueRecord) string { return record.ShipmentRequestID.String() }

	pages, total := walk(t, ports.AcceptanceReviewQueuePages, url.Values{}, list, id, func() { await(4, "SYN-CUSTOMER-02") })
	expectPages(t, "翻页途中新停等一份", pages, total,
		[][]string{{"SYN-REQ-RVW-1", "SYN-REQ-RVW-2"}, {"SYN-REQ-RVW-3", "SYN-REQ-RVW-4"}}, 4)
	narrowed, total := walk(t, ports.AcceptanceReviewQueuePages, url.Values{"customerAccountId": {"SYN-CUSTOMER-01"}}, list, id, nil)
	expectPages(t, "按账户收窄", narrowed, total, [][]string{{"SYN-REQ-RVW-1", "SYN-REQ-RVW-3"}}, 2)
	empty, err := views.ListAwaitingManualReview(ctx, viewScope(t, "SYN-TENANT-09", "SYN-CUSTOMER-01"), 2, firstPage(t, ports.AcceptanceReviewQueuePages))
	if err != nil || len(empty.Rows) != 0 || empty.Total != 0 {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}

// Covers: 授权处置队列按提交时刻正序翻页（页大小 1），翻页途中新停等的委托排在末尾；受限项只对本页的委托点读、每页都带出；
// customerAccountId 收窄后 total 随之变；空册 total 为 0。
func TestAuthorizedDispositionQueuePagesOldestFirst(t *testing.T) {
	fixture := newDispositionQueueFixture(t)
	ctx := t.Context()
	await := func(index int, account string) {
		id := fmt.Sprintf("SYN-REQ-DISP-%d", index)
		seedAdvanced(t, fixture.repository, fixture.transactor, "SYN-TENANT-01", account, fmt.Sprintf("SYN-ADQ-%d", index), id,
			submittedAtFixture.Add(time.Duration(index)*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
				return awaitDisposition(t, request, "SYN-DISPOSE-"+id)
			})
		recordAdoptedRestrictedControl(t, fixture, "SYN-TENANT-01", id, "version-1", id+"/version-1")
	}
	await(1, "SYN-CUSTOMER-01")
	await(2, "SYN-CUSTOMER-02")
	scope := viewScope(t, "SYN-TENANT-01", "SYN-CUSTOMER-01", "SYN-CUSTOMER-02")
	var restricted []int
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.AuthorizedDispositionQueueRecord], error) {
		page, err := fixture.views.ListAwaitingAuthorizedDisposition(ctx, scope, 1, query)
		for _, row := range page.Rows {
			restricted = append(restricted, len(row.RestrictedItems))
		}
		return page, err
	}
	id := func(record ports.AuthorizedDispositionQueueRecord) string { return record.ShipmentRequestID.String() }

	pages, total := walk(t, ports.AuthorizedDispositionQueuePages, url.Values{}, list, id, func() { await(3, "SYN-CUSTOMER-01") })
	expectPages(t, "翻页途中新停等一份", pages, total,
		[][]string{{"SYN-REQ-DISP-1"}, {"SYN-REQ-DISP-2"}, {"SYN-REQ-DISP-3"}}, 3)
	if !reflect.DeepEqual(restricted, []int{1, 1, 1}) {
		t.Fatalf("每页的受限项：得 %v，应为各一项", restricted)
	}
	narrowed, total := walk(t, ports.AuthorizedDispositionQueuePages, url.Values{"customerAccountId": {"SYN-CUSTOMER-02"}}, list, id, nil)
	expectPages(t, "按账户收窄", narrowed, total, [][]string{{"SYN-REQ-DISP-2"}}, 1)
	empty, err := fixture.views.ListAwaitingAuthorizedDisposition(ctx, viewScope(t, "SYN-TENANT-09", "SYN-CUSTOMER-01"), 1,
		firstPage(t, ports.AuthorizedDispositionQueuePages))
	if err != nil || len(empty.Rows) != 0 || empty.Total != 0 {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}

// Covers: 渠道择优并列冲突按决定时刻倒序翻页，翻页途中新形成的冲突落在已翻过的那头；选出唯一者与别的租户不列；q 命中产品—渠道映射；
// 选择器进游标摘要（不收窄时的游标换上收窄的选择器解不开）；空册 total 为 0。
func TestTiedChannelSelectionDecisionsPageAndKeepTheSubjectInTheCursor(t *testing.T) {
	registry, transactor, _ := newChannelSelectionDecisions(t)
	ctx := t.Context()
	subject := selectionSubjectFixture(t, "SYN-SCOPE-A", "SYN-MAPPING-1")
	other := selectionSubjectFixture(t, "SYN-SCOPE-A", "SYN-MAPPING-2")
	tie := func(id, tenant string, on domain.ChannelSelectionSubject, at time.Duration) {
		mustAppendDecision(t, transactor, ctx, registry, decisionFixture(t, id, tenant, on, selectionDecidedAt.Add(at),
			pricedCandidateFixture(t, "SYN-CAND-A", "10.00", ""), pricedCandidateFixture(t, "SYN-CAND-B", "10.00", "")))
	}
	tie("SYN-TIE-1", "SYN-TENANT-01", subject, 0)
	tie("SYN-TIE-2", "SYN-TENANT-01", other, time.Hour)
	tie("SYN-TIE-3", "SYN-TENANT-01", subject, 2*time.Hour)
	tie("SYN-TIE-X", "SYN-TENANT-02", subject, 3*time.Hour)
	mustAppendDecision(t, transactor, ctx, registry, decisionFixture(t, "SYN-RESOLVED", "SYN-TENANT-01", subject, selectionDecidedAt.Add(4*time.Hour),
		pricedCandidateFixture(t, "SYN-CAND-A", "8.00", ""), pricedCandidateFixture(t, "SYN-CAND-B", "9.00", "")))
	tenant := mustBuild(t, domain.NewTenantID, "SYN-TENANT-01")
	every := url.Values{"view": {"tied"}}
	list := func(query cataloguepage.Query) (cataloguepage.Result[domain.ChannelSelectionDecision], error) {
		return registry.ListTiedChannelSelectionDecisions(ctx, tenant, ports.EveryTiedChannelSelection(), 2, query)
	}
	id := func(decision domain.ChannelSelectionDecision) string { return decision.ID().String() }

	pages, total := walk(t, ports.TiedChannelSelectionPages, every, list, id, func() { tie("SYN-TIE-4", "SYN-TENANT-01", other, 5*time.Hour) })
	expectPages(t, "翻页途中新形成一条", pages, total, [][]string{{"SYN-TIE-3", "SYN-TIE-2"}, {"SYN-TIE-1"}}, 4)
	byMapping, total := walk(t, ports.TiedChannelSelectionPages, url.Values{"view": {"tied"}, "q": {"syn-mapping-2"}}, list, id, nil)
	expectPages(t, "q 命中映射", byMapping, total, [][]string{{"SYN-TIE-4", "SYN-TIE-2"}}, 2)

	first, err := list(pageQuery(t, ports.TiedChannelSelectionPages, every))
	if err != nil || first.Next == "" {
		t.Fatalf("第一页：得 %+v（%v）", first, err)
	}
	narrowed := url.Values{"view": {"tied"}, "scope": {subject.Scope().String()}, "mapping": {subject.Mapping().String()}, "after": {first.Next}}
	if _, err := ports.TiedChannelSelectionPages.Decode(narrowed); err == nil {
		t.Fatal("不收窄时的游标换上收窄的选择器应解不开")
	}
	empty, err := registry.ListTiedChannelSelectionDecisions(ctx, mustBuild(t, domain.NewTenantID, "SYN-TENANT-09"),
		ports.EveryTiedChannelSelection(), 2, pageQuery(t, ports.TiedChannelSelectionPages, every))
	if err != nil || len(empty.Rows) != 0 || empty.Total != 0 {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}

// Covers: 释放未受理册按登记时刻正序翻页，行标识三段都进键，翻页途中新登记的一行排在末尾；按账户与 q 收窄时 total 跟着变、未处理计数
// 仍是整册积压；空册 total 与计数都为 0。
func TestControlReleaseRefusalsPageWhileTheBacklogStaysWhole(t *testing.T) {
	rig := newRefusalHandlingRig(t)
	record := func(index int) {
		rig.record(t, refusalFor(t, "SYN-TENANT-01", fmt.Sprintf("SYN-REQ-REFUSE-%d", index), "SYN-VERSION-1",
			adoptionRecordAt.Add(time.Duration(index)*time.Minute), syntheticScope(t)))
	}
	for index := 1; index <= 3; index++ {
		record(index)
	}
	tenant := psTenant(t, "SYN-TENANT-01")
	page := func(values url.Values, size int, after string) ports.UnhandledControlReleaseRefusals {
		t.Helper()
		query := pageQuery(t, ports.ControlReleaseRefusalPages, values)
		if after != "" {
			query = nextPage(t, ports.ControlReleaseRefusalPages, values, after)
		}
		listed, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), tenant, size, query)
		if err != nil {
			t.Fatal(err)
		}
		return listed
	}
	ids := func(listed ports.UnhandledControlReleaseRefusals) []string {
		out := make([]string, len(listed.Refusals))
		for index, refusal := range listed.Refusals {
			out[index] = refusal.ShipmentRequestID.String()
		}
		return out
	}

	first := page(url.Values{}, 2, "")
	if !reflect.DeepEqual(ids(first), []string{"SYN-REQ-REFUSE-1", "SYN-REQ-REFUSE-2"}) || first.Total != 3 || first.Count != 3 {
		t.Fatalf("第一页：得 %v / total %d / count %d", ids(first), first.Total, first.Count)
	}
	record(4)
	second := page(url.Values{}, 2, first.Next)
	if !reflect.DeepEqual(ids(second), []string{"SYN-REQ-REFUSE-3", "SYN-REQ-REFUSE-4"}) || second.Next != "" || second.Total != 4 || second.Count != 4 {
		t.Fatalf("翻页途中新登记一行之后的第二页：得 %v / next %q / total %d / count %d", ids(second), second.Next, second.Total, second.Count)
	}

	account := first.Refusals[0].CustomerAccountID.String()
	if mine := page(url.Values{"customerAccountId": {account}}, 10, ""); mine.Total != 4 || mine.Count != 4 {
		t.Fatalf("按本账户：total %d / count %d", mine.Total, mine.Count)
	}
	if none := page(url.Values{"customerAccountId": {"SYN-ACCOUNT-NONE"}}, 10, ""); none.Total != 0 || len(none.Refusals) != 0 || none.Count != 4 {
		t.Fatalf("按别的账户：total %d / count %d，积压应仍是 4", none.Total, none.Count)
	}
	if one := page(url.Values{"q": {"refuse-2"}}, 10, ""); !reflect.DeepEqual(ids(one), []string{"SYN-REQ-REFUSE-2"}) || one.Total != 1 || one.Count != 4 {
		t.Fatalf("q：得 %v / total %d / count %d", ids(one), one.Total, one.Count)
	}
	empty, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), psTenant(t, "SYN-TENANT-09"), 10,
		firstPage(t, ports.ControlReleaseRefusalPages))
	if err != nil || len(empty.Refusals) != 0 || empty.Total != 0 || empty.Count != 0 {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}

// Covers: 取消先后冲突册按发现时刻正序翻页，四段标识进键，翻页途中新发现的冲突排在末尾；boundaryKind 收窄时 total 跟着变、未解开计数仍是
// 整册积压；空册 total 与计数都为 0。
func TestCancellationOrderConflictsPageAndFilterByBoundaryKind(t *testing.T) {
	rig := newConflictDecisionRig(t)
	record := func(parcel, reference string, kind domain.CancellationBoundaryKind, at time.Duration) {
		rig.recordConflict(t, "SYN-TENANT-01",
			conflictOf(t, parcel, "SYN-PCXL-1", reference, kind, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)),
			conflictDetectedAt.Add(at))
	}
	record("SYN-PARCEL-C1", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, 0)
	record("SYN-PARCEL-C2", "CFEP-1@CFEV-1", domain.CarrierFirstEffectivePickupBoundary, time.Minute)
	record("SYN-PARCEL-C3", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, 2*time.Minute)
	tenant := psTenant(t, "SYN-TENANT-01")
	page := func(values url.Values, size int, after string) ports.UnresolvedCancellationOrderConflicts {
		t.Helper()
		query := pageQuery(t, ports.CancellationOrderConflictPages, values)
		if after != "" {
			query = nextPage(t, ports.CancellationOrderConflictPages, values, after)
		}
		listed, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), tenant, size, query)
		if err != nil {
			t.Fatal(err)
		}
		return listed
	}
	parcels := func(listed ports.UnresolvedCancellationOrderConflicts) []string {
		out := make([]string, len(listed.Conflicts))
		for index, conflict := range listed.Conflicts {
			out[index] = conflict.Parcel.String()
		}
		return out
	}

	first := page(url.Values{}, 2, "")
	record("SYN-PARCEL-C4", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, 3*time.Minute)
	second := page(url.Values{}, 2, first.Next)
	if !reflect.DeepEqual(parcels(first), []string{"SYN-PARCEL-C1", "SYN-PARCEL-C2"}) ||
		!reflect.DeepEqual(parcels(second), []string{"SYN-PARCEL-C3", "SYN-PARCEL-C4"}) || second.Next != "" ||
		first.Total != 3 || second.Total != 4 || second.Count != 4 {
		t.Fatalf("翻页：得 %v / %v，total %d → %d，count %d", parcels(first), parcels(second), first.Total, second.Total, second.Count)
	}
	pickups := page(url.Values{"boundaryKind": {"CARRIER_FIRST_EFFECTIVE_PICKUP"}}, 10, "")
	if !reflect.DeepEqual(parcels(pickups), []string{"SYN-PARCEL-C2"}) || pickups.Total != 1 || pickups.Count != 4 {
		t.Fatalf("按边界种类：得 %v / total %d / count %d", parcels(pickups), pickups.Total, pickups.Count)
	}
	empty, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "SYN-TENANT-09"), 10,
		firstPage(t, ports.CancellationOrderConflictPages))
	if err != nil || len(empty.Conflicts) != 0 || empty.Total != 0 || empty.Count != 0 {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}

// Covers: 等回应的处置请求按记下时刻正序翻页，（请求键、包裹）两段标识进键，翻页途中新记下的请求排在末尾；q 收窄时 total 跟着变、积压数
// 不变；空册 total 与计数都为 0。
func TestUnansweredPostIntakeDispositionRequestsPageOldestFirst(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	cancellations, err := adapter.NewParcelCancellations(db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := adapter.NewPostIntakeDispositionRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	pending := func(index int) {
		mustSaveCancellation(t, db.Transactor(), ctx, cancellations, ports.CancellationRecord{
			Key:           cancellationKey(t, "SYN-TENANT-01", fmt.Sprintf("SYN-STOP-%d", index), fmt.Sprintf("SYN-PARCEL-P%d", index)),
			ContentDigest: fmt.Sprintf("SYN-DIGEST-%d", index),
			Kind:          ports.RecordDispositionPending,
			IntakeVersion: mustBuild(t, domain.NewSourceResultVersion, "SYN-SRV-9"),
			DecidedAt:     adoptionRecordAt.Add(time.Duration(index) * time.Minute),
		})
	}
	for index := 1; index <= 3; index++ {
		pending(index)
	}
	tenant := psTenant(t, "SYN-TENANT-01")
	keys := func(listed ports.UnansweredPostIntakeDispositionRequests) []string {
		out := make([]string, len(listed.Requests))
		for index, request := range listed.Requests {
			out[index] = request.Key.RequestKey.String()
		}
		return out
	}

	first, err := requests.ListUnansweredPostIntakeDispositionRequests(ctx, tenant, 2, firstPage(t, ports.PostIntakeDispositionRequestPages))
	if err != nil {
		t.Fatal(err)
	}
	pending(4)
	second, err := requests.ListUnansweredPostIntakeDispositionRequests(ctx, tenant, 2,
		nextPage(t, ports.PostIntakeDispositionRequestPages, url.Values{}, first.Next))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys(first), []string{"SYN-STOP-1", "SYN-STOP-2"}) || !reflect.DeepEqual(keys(second), []string{"SYN-STOP-3", "SYN-STOP-4"}) ||
		second.Next != "" || first.Total != 3 || second.Total != 4 || second.Count != 4 {
		t.Fatalf("翻页：得 %v / %v，total %d → %d，count %d", keys(first), keys(second), first.Total, second.Total, second.Count)
	}
	one, err := requests.ListUnansweredPostIntakeDispositionRequests(ctx, tenant, 10,
		pageQuery(t, ports.PostIntakeDispositionRequestPages, url.Values{"q": {"syn-parcel-p2"}}))
	if err != nil || !reflect.DeepEqual(keys(one), []string{"SYN-STOP-2"}) || one.Total != 1 || one.Count != 4 {
		t.Fatalf("q：得 %v / total %d / count %d（%v）", keys(one), one.Total, one.Count, err)
	}
	empty, err := requests.ListUnansweredPostIntakeDispositionRequests(ctx, psTenant(t, "SYN-TENANT-09"), 10,
		firstPage(t, ports.PostIntakeDispositionRequestPages))
	if err != nil || len(empty.Requests) != 0 || empty.Total != 0 || empty.Count != 0 {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}
