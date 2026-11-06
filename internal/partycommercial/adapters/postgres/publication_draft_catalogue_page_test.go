package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证开放中的发布载体按 ADR-0144 分页（票 catalogue-read-pagination-rollout/11）：翻页中途录入新载体
// 不重不漏、空册答空页；状态按领域原词筛，q 覆盖对象号、版本、范围、录入者与批准者，% 按字面；本构建读不回的载体照样受筛、
// 占页里的位置、计入总数，游标从它接得下去。夹具取值一律 SYN-，不借 draftShellIn 那份带非合成取值的壳与正文。

const draftPageTenant = "SYN-TENANT-01"

// makeDraftUnreadable 把合成租户名下一行的规范化版本改成本构建不认得的 PCC-9（摘要串的前缀随之，库上 CHECK 要求两者一致）。
func makeDraftUnreadable(t *testing.T, pool *pgxpool.Pool, objectID string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`UPDATE party_commercial.publication_draft
		    SET content_canonicalization = 'PCC-9', content_digest = 'PCC-9:SYN-FUTURE-BUILD'
		  WHERE tenant_id = $1 AND object_id = $2`, draftPageTenant, objectID); err != nil {
		t.Fatalf("改写 %s：%v", objectID, err)
	}
}

// synDraft 录入合成租户的一份 kind 类载体，录入时刻是 draftSubmittedAt 之后 minute 分钟：缺省序按录入时刻倒序，次序由
// 夹具给定，不靠库里的 now()。信用政策带正文与一条服务产品引用，同 draftShellIn 的形状；服务产品是无正文的册。
func synDraft(
	t *testing.T,
	transactor bentoapp.Transactor,
	drafts *adapter.PublicationDrafts,
	kind domain.CommercialObjectKind,
	objectID, submitter string,
	minute int,
) domain.PublicationDraft {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	shell := domain.PublicationDraftShell{
		TenantID:  pcTenant(t, draftPageTenant),
		Kind:      kind,
		ObjectID:  pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:   pcValue(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:     pcValue(t, domain.NewCommercialScopeReference, "SYN-SCOPE-1"),
		Effective: interval,
	}
	content := domain.PublicationContent{Kind: kind}
	if kind == domain.CreditPolicyObject {
		shell.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.ServiceProductObject: pcValue(t, domain.NewCommercialObjectID, "SYN-PRODUCT-1"),
		}
		open, err := domain.NewEffectiveInterval(effectiveAtRow, time.Time{})
		if err != nil {
			t.Fatalf("正文区间：%v", err)
		}
		content.CreditPolicy = &domain.CreditPolicyBody{
			LegalEntity: pcValue(t, domain.NewLegalEntityReference, "SYN-LE-1"),
			Level:       pcValue(t, domain.NewAuthorityLevel, "SYN-LEVEL-COMMERCIAL"),
			ChargeType:  pcValue(t, domain.NewChargeTypeReference, "SYN-CHARGE-FREIGHT"),
			Limit:       creditAmountLimit(t, 100),
			Effective:   open,
		}
	}
	draft, err := domain.SubmitPublicationDraft(shell, content,
		pcValue(t, domain.NewOperatorSubjectReference, submitter), draftSubmittedAt.Add(time.Duration(minute)*time.Minute))
	if err != nil {
		t.Fatalf("录入载体 %s：%v", objectID, err)
	}
	mustSubmitDraft(t, transactor, t.Context(), drafts, draft)
	return draft
}

// openDraftObjectID 与 openDraftLabel 把一行抹成一格：前者只取对象号，后者是「对象号/状态」，读不回的一行再缀 /unreadable，
// 读得回与读不回的同在一页里比次序。
func openDraftObjectID(open ports.OpenPublicationDraft) string {
	if draft, readable := open.Draft(); readable {
		return draft.ObjectID().String()
	}
	row, _ := open.Unreadable()
	return row.ObjectID().String()
}

func openDraftLabel(open ports.OpenPublicationDraft) string {
	if draft, readable := open.Draft(); readable {
		return draft.ObjectID().String() + "/" + draft.Status().String()
	}
	row, _ := open.Unreadable()
	return row.ObjectID().String() + "/" + row.Status().String() + "/unreadable"
}

func openDraftLister(
	ctx context.Context,
	drafts *adapter.PublicationDrafts,
	tenant domain.TenantID,
	label func(ports.OpenPublicationDraft) string,
) pageLister[string] {
	return shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.OpenPublicationDraft], error) {
		return drafts.ListOpenDrafts(ctx, tenant, limit, query)
	}, label)
}

// seedOpenDrafts 登筛选、q 与读不回用例共用的载体：A 待批准的信用政策、B 已批准的服务产品、C 待批准而本构建读不回的信用
// 政策、D 已发布的信用政策（不在册）。录入时刻 A 最早、D 最晚。
func seedOpenDrafts(t *testing.T) (*adapter.PublicationDrafts, domain.TenantID) {
	t.Helper()
	drafts, transactor, pool := unreadableDraftRegistry(t)
	ctx := t.Context()
	synDraft(t, transactor, drafts, domain.CreditPolicyObject, "SYN-DRAFT-A", "SYN-SUBMITTER-1", 1)
	approved := synDraft(t, transactor, drafts, domain.ServiceProductObject, "SYN-DRAFT-B", "SYN-SUBMITTER-2", 2)
	mustAdvanceDraft(t, transactor, ctx, drafts, approveDraft(t, approved, "SYN-APPROVER-1"))
	synDraft(t, transactor, drafts, domain.CreditPolicyObject, "SYN-DRAFT-C", "SYN-SUBMITTER-2", 3)
	makeDraftUnreadable(t, pool, "SYN-DRAFT-C")
	published := approveDraft(t, synDraft(t, transactor, drafts, domain.CreditPolicyObject, "SYN-DRAFT-D", "SYN-SUBMITTER-1", 4),
		"SYN-APPROVER-1")
	mustAdvanceDraft(t, transactor, ctx, drafts, published)
	done, err := published.MarkPublished(draftPublishedAt)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	mustAdvanceDraft(t, transactor, ctx, drafts, done)
	return drafts, pcTenant(t, draftPageTenant)
}

// Covers: 票 11 完成判据——开放中的发布载体翻页中途录入新载体不重不漏；空册答空页、共 0。
func TestOpenDraftCataloguePagesWithoutRepeatsOrGapsWhileDraftsArrive(t *testing.T) {
	drafts, _, transactor, _ := newDraftRegistry(t)
	ctx := t.Context()
	minute := 0
	assertPagesWithoutRepeatsOrGaps(t, ports.OpenPublicationDraftPages,
		openDraftLister(ctx, drafts, pcTenant(t, draftPageTenant), openDraftObjectID),
		func(id string) {
			minute++
			synDraft(t, transactor, drafts, domain.CreditPolicyObject, id, "SYN-SUBMITTER-1", minute)
		},
		func(id string) string { return id }, "SYN-DRAFT-PAGE-")
	assertEmptyCatalogue(t, "开放中的发布载体", ports.OpenPublicationDraftPages,
		openDraftLister(ctx, drafts, pcTenant(t, "SYN-TENANT-EMPTY"), openDraftObjectID))
}

// Covers: 票 11 完成判据——状态按领域原词筛（维内取并，与 q 取交），q 覆盖声明的那几格且不分大小写、% 按字面，总数与本页
// 同一组条件；读不回的一行按库上的列受筛，与读得回的一样。
func TestOpenDraftCatalogueFiltersAndSearchesUnreadableRowsAlike(t *testing.T) {
	drafts, tenant := seedOpenDrafts(t)
	const (
		a = "SYN-DRAFT-A/PENDING_APPROVAL"
		b = "SYN-DRAFT-B/APPROVED"
		c = "SYN-DRAFT-C/PENDING_APPROVAL/unreadable"
	)
	assertFilteredPages(t, "开放中的发布载体", ports.OpenPublicationDraftPages,
		openDraftLister(t.Context(), drafts, tenant, openDraftLabel), func(label string) string { return label },
		[]pageCase{
			{"", c + "," + b + "," + a},
			{"status=PENDING_APPROVAL", c + "," + a},
			{"status=APPROVED", b},
			{"status=PENDING_APPROVAL&status=APPROVED", c + "," + b + "," + a},
			{"q=syn-submitter-2", c + "," + b},
			{"q=syn-submitter-2&status=PENDING_APPROVAL", c},
			{"q=SYN-APPROVER", b},
			{"q=SYN-DRAFT-A", a},
			{"q=V1&status=PENDING_APPROVAL", c + "," + a},
			{"q=SCOPE-1&status=APPROVED", b},
			{"sort=submittedAt", a + "," + b + "," + c},
			{"sort=submittedAt&status=PENDING_APPROVAL", a + "," + c},
			{"q=%25", ""},
		})
}

// Covers: AT-PC-071 的分页一半——读不回的载体占页里的位置、计入总数，游标从它接得下去：位置取自库上的列，不取自折不回的
// 正文，读面不必先把行读回来才排得出次序。
func TestUnreadableOpenDraftsKeepTheirPlaceAcrossPages(t *testing.T) {
	drafts, tenant := seedOpenDrafts(t)
	ctx := t.Context()
	first, err := drafts.ListOpenDrafts(ctx, tenant, 1, firstPage(t, ports.OpenPublicationDraftPages))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	if got := joinedBy(first.Rows, openDraftLabel); got != "SYN-DRAFT-C/PENDING_APPROVAL/unreadable" || first.Total != 3 || first.Next == "" {
		t.Fatalf("第一页 = %s（共 %d，next %q），want 读不回的 C 排在最前、共 3、有下一页", got, first.Total, first.Next)
	}
	rest, err := drafts.ListOpenDrafts(ctx, tenant, 2, nextPage(t, ports.OpenPublicationDraftPages, "", first.Next))
	if err != nil {
		t.Fatalf("接读不回那一行往后：%v", err)
	}
	if got := joinedBy(rest.Rows, openDraftLabel); got != "SYN-DRAFT-B/APPROVED,SYN-DRAFT-A/PENDING_APPROVAL" || rest.Total != 3 || rest.Next != "" {
		t.Fatalf("其余 = %s（共 %d，next %q），want B、A 且取尽", got, rest.Total, rest.Next)
	}
}
