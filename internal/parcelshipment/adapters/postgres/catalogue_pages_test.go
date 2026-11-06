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

// 本文件对真实 PostgreSQL 16 证本上下文各册迁到 ADR-0144 之后的取页（票 catalogue-read-pagination-rollout/02）：每册按小页走完
// 全部页，拼起来恰是全集且次序即缺省序；筛选维与 q 改变 total；带计数的队列积压数不随条件变；空册答空页且 total 为 0。
// 键集机制本身由面单交易的用例与共用件证过，这里证的是各册落列：落错一列，次序就乱、页与页之间就重出或漏行。

// walk 用同一组条件按页大小 size 走完全部页，交回每页的标识与最后一页的 total。
func walk[Row any](
	t *testing.T,
	catalogue *cataloguepage.Catalogue,
	values url.Values,
	size int,
	list func(cataloguepage.Query) (cataloguepage.Result[Row], error),
	id func(Row) string,
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
		query = nextPage(t, catalogue, values, page.Next)
	}
}

func summaryID(summary ports.ShipmentRequestSummaryRecord) string {
	return summary.ShipmentRequestID.String()
}

// Covers: 委托查阅按提交时刻倒序翻页，作用域外的账户与别的租户不出现；state 落在存代码的列上（SUBMITTED 命中、ACCEPTED 零行），
// customerAccountId 在作用域之内收窄，q 命中来源请求键。
func TestShipmentRequestViewsPageWithinTheScope(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()
	seed := func(tenant, account, key, id string, at time.Duration) {
		mustInsert(t, transactor, ctx, repository,
			scopedSubmittedRequest(t, tenant, account, key, id, submittedAtFixture.Add(at)))
	}
	seed("tenant-1", "customer-1", "view-a", "REQ-A", 0)
	seed("tenant-1", "customer-1", "view-b", "REQ-B", time.Hour)
	seed("tenant-1", "customer-2", "view-c", "REQ-C", 2*time.Hour)
	seed("tenant-1", "customer-3", "view-d", "REQ-D", 3*time.Hour)
	seed("tenant-2", "customer-1", "view-e", "REQ-E", 4*time.Hour)
	scope := viewScope(t, "tenant-1", "customer-1", "customer-2")
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.ShipmentRequestSummaryRecord], error) {
		return views.ListVisible(ctx, scope, 2, query)
	}

	pages, total := walk(t, ports.ShipmentRequestViewPages, url.Values{}, 2, list, summaryID)
	if want := [][]string{{"REQ-C", "REQ-B"}, {"REQ-A"}}; !reflect.DeepEqual(pages, want) || total != 3 {
		t.Fatalf("翻页：得 %v / total %d，应为 %v / 3", pages, total, want)
	}
	for name, test := range map[string]struct {
		values url.Values
		want   [][]string
	}{
		"按账户收窄":     {values: url.Values{"customerAccountId": {"customer-1"}}, want: [][]string{{"REQ-B", "REQ-A"}}},
		"作用域外的账户":   {values: url.Values{"customerAccountId": {"customer-3"}}, want: [][]string{{}}},
		"状态落在代码列上":  {values: url.Values{"state": {"SUBMITTED"}}, want: [][]string{{"REQ-C", "REQ-B"}, {"REQ-A"}}},
		"没有已接受的":    {values: url.Values{"state": {"ACCEPTED"}}, want: [][]string{{}}},
		"q 命中来源请求键": {values: url.Values{"q": {"VIEW-B"}}, want: [][]string{{"REQ-B"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if got, _ := walk(t, ports.ShipmentRequestViewPages, test.values, 2, list, summaryID); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("得 %v，应为 %v", got, test.want)
			}
		})
	}
	empty, err := views.ListVisible(ctx, viewScope(t, "tenant-9", "customer-1"), 2, firstPage(t, ports.ShipmentRequestViewPages))
	if err != nil || len(empty.Rows) != 0 || empty.Total != 0 || empty.Next != "" {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}

// Covers: 复核队列按提交时刻正序（先来先审）翻页，customerAccountId 收窄后 total 随之变。
func TestAcceptanceReviewQueuePagesOldestFirst(t *testing.T) {
	views, repository, transactor := newShipmentRequestViews(t)
	ctx := t.Context()
	for index, account := range []string{"customer-1", "customer-2", "customer-1"} {
		id := fmt.Sprintf("REQ-RVW-%d", index+1)
		seedAdvanced(t, repository, transactor, "tenant-1", account, fmt.Sprintf("arq-page-%d", index+1), id,
			submittedAtFixture.Add(time.Duration(index)*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
				return awaitReview(t, request, "review-"+id)
			})
	}
	scope := viewScope(t, "tenant-1", "customer-1", "customer-2")
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.AcceptanceReviewQueueRecord], error) {
		return views.ListAwaitingManualReview(ctx, scope, 2, query)
	}
	id := func(record ports.AcceptanceReviewQueueRecord) string { return record.ShipmentRequestID.String() }

	pages, total := walk(t, ports.AcceptanceReviewQueuePages, url.Values{}, 2, list, id)
	if want := [][]string{{"REQ-RVW-1", "REQ-RVW-2"}, {"REQ-RVW-3"}}; !reflect.DeepEqual(pages, want) || total != 3 {
		t.Fatalf("翻页：得 %v / total %d，应为 %v / 3", pages, total, want)
	}
	narrowed, total := walk(t, ports.AcceptanceReviewQueuePages, url.Values{"customerAccountId": {"customer-1"}}, 2, list, id)
	if want := [][]string{{"REQ-RVW-1", "REQ-RVW-3"}}; !reflect.DeepEqual(narrowed, want) || total != 2 {
		t.Fatalf("收窄：得 %v / total %d，应为 %v / 2", narrowed, total, want)
	}
}

// Covers: 授权处置队列与复核队列共用同一份落列，这里按页大小 1 走一遍：受限项只对本页的委托点读，翻到第二页照样带出。
func TestAuthorizedDispositionQueuePagesOldestFirst(t *testing.T) {
	fixture := newDispositionQueueFixture(t)
	ctx := t.Context()
	for index, id := range []string{"REQ-DISP-P1", "REQ-DISP-P2"} {
		seedAdvanced(t, fixture.repository, fixture.transactor, "tenant-1", "customer-1", fmt.Sprintf("adq-page-%d", index+1), id,
			submittedAtFixture.Add(time.Duration(index)*time.Hour), func(request domain.ShipmentRequest) domain.ShipmentRequest {
				return awaitDisposition(t, request, "dispose-"+id)
			})
		recordAdoptedRestrictedControl(t, fixture, "tenant-1", id, "version-1", id+"/version-1")
	}
	scope := viewScope(t, "tenant-1", "customer-1")
	var restricted []int
	pages, total := walk(t, ports.AuthorizedDispositionQueuePages, url.Values{}, 1,
		func(query cataloguepage.Query) (cataloguepage.Result[ports.AuthorizedDispositionQueueRecord], error) {
			page, err := fixture.views.ListAwaitingAuthorizedDisposition(ctx, scope, 1, query)
			for _, row := range page.Rows {
				restricted = append(restricted, len(row.RestrictedItems))
			}
			return page, err
		},
		func(record ports.AuthorizedDispositionQueueRecord) string { return record.ShipmentRequestID.String() })
	if want := [][]string{{"REQ-DISP-P1"}, {"REQ-DISP-P2"}}; !reflect.DeepEqual(pages, want) || total != 2 {
		t.Fatalf("翻页：得 %v / total %d，应为 %v / 2", pages, total, want)
	}
	if !reflect.DeepEqual(restricted, []int{1, 1}) {
		t.Fatalf("每页的受限项：得 %v，应为各一项", restricted)
	}
}

// Covers: 渠道择优并列冲突按决定时刻倒序翻页；q 命中产品—渠道映射；选择器进游标摘要——不收窄时拿到的游标换上按对象收窄的
// 选择器就解不开（换了收窄对象就是另一份列表）。
func TestTiedChannelSelectionDecisionsPageAndKeepTheSubjectInTheCursor(t *testing.T) {
	registry, tenant, subject := tiedDecisionsFixture(t)
	every := url.Values{"view": {"tied"}}
	list := func(query cataloguepage.Query) (cataloguepage.Result[domain.ChannelSelectionDecision], error) {
		return registry.ListTiedChannelSelectionDecisions(t.Context(), tenant, ports.EveryTiedChannelSelection(), 2, query)
	}
	id := func(decision domain.ChannelSelectionDecision) string { return decision.ID().String() }

	pages, total := walk(t, ports.TiedChannelSelectionPages, every, 2, list, id)
	if want := [][]string{{"tie-later", "tie-other-subject"}, {"tie-earlier"}}; !reflect.DeepEqual(pages, want) || total != 3 {
		t.Fatalf("翻页：得 %v / total %d，应为 %v / 3", pages, total, want)
	}
	if got, total := walk(t, ports.TiedChannelSelectionPages, url.Values{"view": {"tied"}, "q": {"mapping-2"}}, 2, list, id); !reflect.DeepEqual(got, [][]string{{"tie-other-subject"}}) || total != 1 {
		t.Fatalf("q：得 %v / total %d", got, total)
	}

	first, err := list(pageQuery(t, ports.TiedChannelSelectionPages, every))
	if err != nil {
		t.Fatal(err)
	}
	narrowed := url.Values{"view": {"tied"}, "scope": {subject.Scope().String()}, "mapping": {subject.Mapping().String()}, "after": {first.Next}}
	if _, err := ports.TiedChannelSelectionPages.Decode(narrowed); err == nil {
		t.Fatal("不收窄时的游标换上收窄的选择器应解不开")
	}
}

// Covers: 释放未受理册按登记时刻正序翻页，行标识三段都进键；按账户与 q 收窄时 total 跟着变，未处理计数仍是整册积压。
func TestControlReleaseRefusalsPageWhileTheBacklogStaysWhole(t *testing.T) {
	rig := newRefusalHandlingRig(t)
	for index := 1; index <= 3; index++ {
		rig.record(t, refusalFor(t, "tenant-1", fmt.Sprintf("REQ-REFUSE-%d", index), "VERSION-1",
			adoptionRecordAt.Add(time.Duration(index)*time.Minute), syntheticScope(t)))
	}
	tenant := psTenant(t, "tenant-1")
	page := func(values url.Values, size int) ports.UnhandledControlReleaseRefusals {
		t.Helper()
		listed, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), tenant, size, pageQuery(t, ports.ControlReleaseRefusalPages, values))
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

	first := page(url.Values{}, 2)
	if !reflect.DeepEqual(ids(first), []string{"REQ-REFUSE-1", "REQ-REFUSE-2"}) || first.Total != 3 || first.Count != 3 || first.Next == "" {
		t.Fatalf("第一页：得 %v / total %d / count %d", ids(first), first.Total, first.Count)
	}
	second, err := rig.catalogue.ListUnhandledControlReleaseRefusals(t.Context(), tenant, 2,
		nextPage(t, ports.ControlReleaseRefusalPages, url.Values{}, first.Next))
	if err != nil || !reflect.DeepEqual(ids(second), []string{"REQ-REFUSE-3"}) || second.Next != "" {
		t.Fatalf("第二页：得 %v（%v）", ids(second), err)
	}

	account := first.Refusals[0].CustomerAccountID.String()
	if mine := page(url.Values{"customerAccountId": {account}}, 10); mine.Total != 3 || mine.Count != 3 {
		t.Fatalf("按本账户：total %d / count %d", mine.Total, mine.Count)
	}
	if none := page(url.Values{"customerAccountId": {"SYN-ACCOUNT-NONE"}}, 10); none.Total != 0 || len(none.Refusals) != 0 || none.Count != 3 {
		t.Fatalf("按别的账户：total %d / count %d，积压应仍是 3", none.Total, none.Count)
	}
	if one := page(url.Values{"q": {"refuse-2"}}, 10); !reflect.DeepEqual(ids(one), []string{"REQ-REFUSE-2"}) || one.Total != 1 || one.Count != 3 {
		t.Fatalf("q：得 %v / total %d / count %d", ids(one), one.Total, one.Count)
	}
	if empty := page(url.Values{}, 10); empty.Total != 3 {
		t.Fatalf("不带条件：total %d", empty.Total)
	}
}

// Covers: 取消先后冲突册按发现时刻正序翻页，四段标识进键；boundaryKind 收窄时 total 跟着变、未解开计数仍是整册积压；空册答空页。
func TestCancellationOrderConflictsPageAndFilterByBoundaryKind(t *testing.T) {
	rig := newConflictDecisionRig(t)
	record := func(parcel, reference string, kind domain.CancellationBoundaryKind, at time.Duration) {
		rig.recordConflict(t, "tenant-1",
			conflictOf(t, parcel, "PCXL-1", reference, kind, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)),
			conflictDetectedAt.Add(at))
	}
	record("SYN-PARCEL-C1", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, 0)
	record("SYN-PARCEL-C2", "CFEP-1@CFEV-1", domain.CarrierFirstEffectivePickupBoundary, time.Minute)
	record("SYN-PARCEL-C3", "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, 2*time.Minute)
	tenant := psTenant(t, "tenant-1")
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
	second := page(url.Values{}, 2, first.Next)
	if !reflect.DeepEqual(parcels(first), []string{"SYN-PARCEL-C1", "SYN-PARCEL-C2"}) ||
		!reflect.DeepEqual(parcels(second), []string{"SYN-PARCEL-C3"}) || second.Next != "" || first.Total != 3 || first.Count != 3 {
		t.Fatalf("翻页：得 %v / %v，total %d，count %d", parcels(first), parcels(second), first.Total, first.Count)
	}
	pickups := page(url.Values{"boundaryKind": {"CARRIER_FIRST_EFFECTIVE_PICKUP"}}, 10, "")
	if !reflect.DeepEqual(parcels(pickups), []string{"SYN-PARCEL-C2"}) || pickups.Total != 1 || pickups.Count != 3 {
		t.Fatalf("按边界种类：得 %v / total %d / count %d", parcels(pickups), pickups.Total, pickups.Count)
	}
	empty, err := rig.catalogue.ListUnresolvedCancellationOrderConflicts(t.Context(), psTenant(t, "tenant-9"), 10,
		firstPage(t, ports.CancellationOrderConflictPages))
	if err != nil || len(empty.Conflicts) != 0 || empty.Total != 0 || empty.Count != 0 {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}

// Covers: 等回应的处置请求按记下时刻正序翻页，（请求键、包裹）两段标识进键；q 收窄时 total 跟着变、积压数不变。
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
	for index := 1; index <= 3; index++ {
		mustSaveCancellation(t, db.Transactor(), ctx, cancellations, ports.CancellationRecord{
			Key:           cancellationKey(t, "tenant-1", fmt.Sprintf("SYN-STOP-%d", index), fmt.Sprintf("SYN-PARCEL-P%d", index)),
			ContentDigest: fmt.Sprintf("digest-%d", index),
			Kind:          ports.RecordDispositionPending,
			IntakeVersion: mustBuild(t, domain.NewSourceResultVersion, "SRV-9"),
			DecidedAt:     adoptionRecordAt.Add(time.Duration(index) * time.Minute),
		})
	}
	tenant := psTenant(t, "tenant-1")
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
	second, err := requests.ListUnansweredPostIntakeDispositionRequests(ctx, tenant, 2,
		nextPage(t, ports.PostIntakeDispositionRequestPages, url.Values{}, first.Next))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys(first), []string{"SYN-STOP-1", "SYN-STOP-2"}) || !reflect.DeepEqual(keys(second), []string{"SYN-STOP-3"}) ||
		second.Next != "" || first.Total != 3 || first.Count != 3 {
		t.Fatalf("翻页：得 %v / %v，total %d，count %d", keys(first), keys(second), first.Total, first.Count)
	}
	one, err := requests.ListUnansweredPostIntakeDispositionRequests(ctx, tenant, 10,
		pageQuery(t, ports.PostIntakeDispositionRequestPages, url.Values{"q": {"syn-parcel-p2"}}))
	if err != nil || !reflect.DeepEqual(keys(one), []string{"SYN-STOP-2"}) || one.Total != 1 || one.Count != 3 {
		t.Fatalf("q：得 %v / total %d / count %d（%v）", keys(one), one.Total, one.Count, err)
	}
}
