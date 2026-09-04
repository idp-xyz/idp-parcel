package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证「渠道择优决定」登记册的伴生运营查阅读口（票 label-channel/23）：
// 并列冲突列表只列 TIED、按决定时刻倒序、可按对象收窄、租户隔离在 SQL 条件上、从未择优过答空；
// 按标识取一条带回逐候选四格与出局因由，别的租户拿同一个标识取不到；limit 非正拒。

// tiedDecisionsFixture 往库里落四条记录：同一租户同一对象两条并列（先后一小时）、同一租户另一对象
// 一条并列、同一租户一条选出唯一者（不该出现在冲突列表里）、另一租户一条并列（不该跨租户可见）。
func tiedDecisionsFixture(t *testing.T) (registry decisionReadRegistry, tenant domain.TenantID, subject domain.ChannelSelectionSubject) {
	t.Helper()
	adapterRegistry, transactor, _ := newChannelSelectionDecisions(t)
	ctx := t.Context()
	subject = selectionSubjectFixture(t, "scope-a", "mapping-1")
	otherSubject := selectionSubjectFixture(t, "scope-a", "mapping-2")

	earlierTie := decisionFixture(t, "tie-earlier", "tenant-1", subject, selectionDecidedAt,
		pricedCandidateFixture(t, "cand-a", "10.00", "eval-a1"),
		pricedCandidateFixture(t, "cand-b", "10.00", "eval-b1"),
		unpriceableCandidateFixture(t, "cand-c", domain.ChannelCostPendingEvidence),
	)
	laterTie := decisionFixture(t, "tie-later", "tenant-1", subject, selectionDecidedAt.Add(time.Hour),
		pricedCandidateFixture(t, "cand-a", "9.00", ""),
		pricedCandidateFixture(t, "cand-b", "9.00", ""),
	)
	otherSubjectTie := decisionFixture(t, "tie-other-subject", "tenant-1", otherSubject, selectionDecidedAt.Add(30*time.Minute),
		pricedCandidateFixture(t, "cand-x", "5.00", ""),
		pricedCandidateFixture(t, "cand-y", "5.00", ""),
	)
	resolved := decisionFixture(t, "resolved", "tenant-1", subject, selectionDecidedAt.Add(2*time.Hour),
		pricedCandidateFixture(t, "cand-a", "8.00", ""),
		pricedCandidateFixture(t, "cand-b", "9.00", ""),
	)
	otherTenantTie := decisionFixture(t, "tie-other-tenant", "tenant-2", subject, selectionDecidedAt.Add(3*time.Hour),
		pricedCandidateFixture(t, "cand-a", "7.00", ""),
		pricedCandidateFixture(t, "cand-b", "7.00", ""),
	)
	// 刻意打乱写入次序：列表顺序要来自决定时刻，不来自写入先后。
	for _, decision := range []domain.ChannelSelectionDecision{resolved, laterTie, otherTenantTie, earlierTie, otherSubjectTie} {
		mustAppendDecision(t, transactor, ctx, adapterRegistry, decision)
	}
	return adapterRegistry, earlierTie.Tenant(), subject
}

// decisionReadRegistry 是本文件消费的读口形状：与端口一致，同一只适配器同时是写侧登记册与读侧读口。
type decisionReadRegistry interface {
	ports.ChannelSelectionDecisionRegistry
	ports.ChannelSelectionDecisionRead
}

// Covers: 并列冲突列表只列结论为 TIED 的记录，按决定时刻倒序；选出唯一者的记录与别的租户的记录不混进来；
// 每条带回逐候选结果（并列各一条、出局带因由、评价引用有则带无则缺席）。
func TestTiedDecisionsAreListedNewestFirstWithinTheTenant(t *testing.T) {
	registry, tenant, _ := tiedDecisionsFixture(t)

	listed, err := registry.ListTiedChannelSelectionDecisions(t.Context(), tenant, ports.EveryTiedChannelSelection(), 10)
	if err != nil {
		t.Fatalf("列并列冲突：%v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("并列冲突 %d 条，want 3（两条同对象 + 一条另一对象；选出者与另一租户不列）", len(listed))
	}
	if listed[0].ID().String() != "tie-later" || listed[1].ID().String() != "tie-other-subject" || listed[2].ID().String() != "tie-earlier" {
		t.Fatalf("顺序应是最近的冲突在最上面，实得 %s / %s / %s", listed[0].ID(), listed[1].ID(), listed[2].ID())
	}
	for _, decision := range listed {
		if decision.Conclusion() != domain.ChannelSelectionConcludedTied {
			t.Fatalf("%s 的结论 = %s，冲突列表里不该有非 TIED 的记录", decision.ID(), decision.Conclusion())
		}
		if decision.Tenant() != tenant {
			t.Fatalf("%s 属租户 %s，跨租户可见", decision.ID(), decision.Tenant())
		}
	}

	earlier := listed[2]
	results := earlier.Results()
	if len(results) != 3 {
		t.Fatalf("tie-earlier 逐候选 %d 条，want 3", len(results))
	}
	if results[0].Outcome() != domain.ChannelCandidateTied || results[1].Outcome() != domain.ChannelCandidateTied {
		t.Fatalf("并列的两家应各记 TIED，实得 %s / %s", results[0].Outcome(), results[1].Outcome())
	}
	if evaluation, present := results[0].Evaluation(); !present || evaluation.String() != "eval-a1" {
		t.Fatalf("cand-a 的评价引用应带回：%v/%v", evaluation, present)
	}
	grade, excluded := results[2].Exclusion()
	if results[2].Outcome() != domain.ChannelCandidateExcluded || !excluded || grade != domain.ChannelCostPendingEvidence {
		t.Fatalf("cand-c 应是带 PENDING_EVIDENCE 因由的出局，实得 %s/%v/%v", results[2].Outcome(), grade, excluded)
	}
}

// Covers: 按对象收窄只剩该（范围 + 映射）下的冲突，仍倒序；limit 截页取最近的那几条。
func TestTiedDecisionsCanBeNarrowedToASubjectAndPaged(t *testing.T) {
	registry, tenant, subject := tiedDecisionsFixture(t)

	narrowed, err := registry.ListTiedChannelSelectionDecisions(t.Context(), tenant, ports.TiedChannelSelectionsOf(subject), 10)
	if err != nil {
		t.Fatalf("按对象收窄：%v", err)
	}
	if len(narrowed) != 2 || narrowed[0].ID().String() != "tie-later" || narrowed[1].ID().String() != "tie-earlier" {
		t.Fatalf("收窄后应只剩该对象的两条并按倒序，实得 %+v", ids(narrowed))
	}

	paged, err := registry.ListTiedChannelSelectionDecisions(t.Context(), tenant, ports.EveryTiedChannelSelection(), 1)
	if err != nil {
		t.Fatalf("截页：%v", err)
	}
	if len(paged) != 1 || paged[0].ID().String() != "tie-later" {
		t.Fatalf("limit=1 应只回最近的那条，实得 %+v", ids(paged))
	}
}

// Covers: 从未择优过的租户答空切片而不是错误（ADR-0077 Decision 四：空册本身就是内容）；limit 非正是
// 调用方编程错误，判据同本包其余列表读口。
func TestTiedDecisionsAnswerEmptyForATenantThatNeverSelected(t *testing.T) {
	registry, _, _ := tiedDecisionsFixture(t)

	empty, err := registry.ListTiedChannelSelectionDecisions(t.Context(), mustBuild(t, domain.NewTenantID, "tenant-never"), ports.EveryTiedChannelSelection(), 10)
	if err != nil {
		t.Fatalf("从未择优过的租户：%v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("应得空切片（非 nil），实得 %#v", empty)
	}

	if _, err := registry.ListTiedChannelSelectionDecisions(t.Context(), mustBuild(t, domain.NewTenantID, "tenant-1"), ports.EveryTiedChannelSelection(), 0); err == nil {
		t.Fatal("limit=0 应被拒")
	}
}

// Covers: 按（租户 + 决定标识）取一条带回整条记录连逐候选四格与出局因由；同一个标识在别的租户下取不到，
// 答「没有」而不是错误。
func TestADecisionIsFoundByIdentityOnlyWithinItsTenant(t *testing.T) {
	registry, tenant, subject := tiedDecisionsFixture(t)

	found, present, err := registry.FindChannelSelectionDecision(t.Context(), tenant, mustBuild(t, domain.NewChannelSelectionDecisionID, "tie-earlier"))
	if err != nil || !present {
		t.Fatalf("按标识取：present=%v err=%v", present, err)
	}
	if found.Subject() != subject || found.Conclusion() != domain.ChannelSelectionConcludedTied ||
		!found.DecidedAt().Equal(selectionDecidedAt) || found.Rule() != domain.ChannelSelectionByCostOnly {
		t.Fatalf("头部不对：%+v", found)
	}
	results := found.Results()
	if len(results) != 3 || results[2].Candidate().String() != "cand-c" {
		t.Fatalf("逐候选结果应按形成时次序带回三条：%+v", results)
	}

	resolved, present, err := registry.FindChannelSelectionDecision(t.Context(), tenant, mustBuild(t, domain.NewChannelSelectionDecisionID, "resolved"))
	if err != nil || !present || resolved.Conclusion() != domain.ChannelSelectionConcludedSelected {
		t.Fatalf("选出唯一者的记录按标识也取得到：present=%v err=%v conclusion=%s", present, err, resolved.Conclusion())
	}
	if selected, chosen := resolved.Selected(); !chosen || selected.String() != "cand-a" {
		t.Fatalf("选中者应是 cand-a：%v/%v", selected, chosen)
	}

	_, present, err = registry.FindChannelSelectionDecision(t.Context(), mustBuild(t, domain.NewTenantID, "tenant-2"), mustBuild(t, domain.NewChannelSelectionDecisionID, "tie-earlier"))
	if err != nil || present {
		t.Fatalf("别的租户拿同一个标识应取不到：present=%v err=%v", present, err)
	}
	_, present, err = registry.FindChannelSelectionDecision(t.Context(), tenant, mustBuild(t, domain.NewChannelSelectionDecisionID, "never-decided"))
	if err != nil || present {
		t.Fatalf("不存在的标识应答没有：present=%v err=%v", present, err)
	}
}

func ids(decisions []domain.ChannelSelectionDecision) []string {
	out := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		out = append(out, decision.ID().String())
	}
	return out
}
