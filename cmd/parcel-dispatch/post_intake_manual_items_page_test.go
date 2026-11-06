package main

import (
	"net/url"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: 待人工处理项迁到 ADR-0144 之后的取页（票 catalogue-read-pagination-rollout/02）：按所属决定的时刻正序翻页，四段标识（包裹、
// 决定版本、所有者、动作）进键；翻页途中新形成的一份拦截决定落下的两项排在末尾，原有各项恰好出现一次；按所有者收窄时 total 跟着变、
// 积压数不变；空册 total 与积压数都为 0。项只能经真的处置决定落下，所以这一册的真库用例在装配了派发链的 parcel-dispatch 里。
func TestPendingManualItemsPageOldestFirstWhileNewDecisionsArrive(t *testing.T) {
	fixture := newPostIntakeManualFixture(t)
	const first, second, third = "SYN-PARCEL-PAGE-1", "SYN-PARCEL-PAGE-2", "SYN-PARCEL-PAGE-3"
	for _, parcel := range []string{first, second, third} {
		fixture.arrangeIntake(t, parcel)
	}
	fixture.decideOperatorInitiated(t, first, psdomain.PostIntakeIntercept)
	fixture.dispatchUntilQuiet(t)
	fixture.decideOperatorInitiated(t, second, psdomain.PostIntakeIntercept)
	fixture.dispatchUntilQuiet(t)
	tenant := fixture.identity.TenantID()

	head, err := fixture.items.ListPendingPostIntakeManualItems(t.Context(), tenant, 3, firstPage(t, psports.PostIntakeManualItemPages))
	if err != nil || len(head.Items) != 3 || head.Next == "" || head.Total != 4 || head.Count != 4 {
		t.Fatalf("第一页 %d 项、total %d、count %d、next %q（%v）, want 3/4/4", len(head.Items), head.Total, head.Count, head.Next, err)
	}
	fixture.decideOperatorInitiated(t, third, psdomain.PostIntakeIntercept)
	fixture.dispatchUntilQuiet(t)
	tail, err := fixture.items.ListPendingPostIntakeManualItems(t.Context(), tenant, 3,
		nextPage(t, psports.PostIntakeManualItemPages, url.Values{}, head.Next))
	if err != nil || len(tail.Items) != 3 || tail.Next != "" || tail.Total != 6 || tail.Count != 6 {
		t.Fatalf("翻页途中新决定之后的第二页 %d 项、total %d、count %d、next %q（%v）, want 3/6/6", len(tail.Items), tail.Total, tail.Count, tail.Next, err)
	}
	seen := map[psdomain.ManualItemKey]bool{}
	for _, item := range append(head.Items, tail.Items...) {
		if seen[item.Key] {
			t.Fatalf("项 %+v 在两页里重出", item.Key)
		}
		seen[item.Key] = true
	}
	for _, item := range tail.Items[1:] {
		if item.Key.Parcel.String() != third {
			t.Fatalf("新决定的项应排在末尾，末尾是 %+v", item.Key)
		}
	}

	nodeOnly, err := fixture.items.ListPendingPostIntakeManualItems(t.Context(), tenant, 50,
		pageQuery(t, psports.PostIntakeManualItemPages, url.Values{"owner": {"NODE_OPERATIONS"}}))
	if err != nil || len(nodeOnly.Items) != 3 || nodeOnly.Total != 3 || nodeOnly.Count != 6 {
		t.Fatalf("按所有者收窄 %d 项、total %d、count %d（%v）, want 3/3/6", len(nodeOnly.Items), nodeOnly.Total, nodeOnly.Count, err)
	}
	empty, err := fixture.items.ListPendingPostIntakeManualItems(t.Context(), mustPS(t, psdomain.NewTenantID, "SYN-TENANT-EMPTY"), 50,
		firstPage(t, psports.PostIntakeManualItemPages))
	if err != nil || len(empty.Items) != 0 || empty.Total != 0 || empty.Count != 0 {
		t.Fatalf("空册：得 %+v（%v）", empty, err)
	}
}
