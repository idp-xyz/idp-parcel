package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证版本壳三册（服务产品、客户合同、供应商协议）按 ADR-0144 分页（票
// catalogue-read-pagination-rollout/11）：翻页中途发布新版本不重不漏、空册答空页；status 按折进退役之后的位置筛，q 覆盖
// 对象、版本、范围与各册另覆盖的正文格，% 按字面。夹具全部合成。

// shellCatalogueCase 是版本壳一册在本文件里的样子。行抹成「对象/版本」一格，三册同一张表断言；content 给 LIVE 那一版登本册的
// 正文，让 q 有一格只在正文里出现的串可命中。
type shellCatalogueCase struct {
	name       string
	kind       domain.CommercialObjectKind
	prefix     string
	pages      *cataloguepage.Catalogue
	list       pageLister[string]
	content    func(t *testing.T, fixture cutFixture, version domain.CommercialVersion)
	contentHit string
}

func shellCatalogueCases(t *testing.T, catalogue *adapter.OperationsCatalogue, tenant domain.TenantID) []shellCatalogueCase {
	t.Helper()
	ctx := t.Context()
	return []shellCatalogueCase{
		{
			name: "服务产品", kind: domain.ServiceProductObject, prefix: "SYN-PRODUCT-", pages: ports.ServiceProductPages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.ServiceProductCatalogueRow], error) {
				return catalogue.ListServiceProducts(ctx, tenant, limit, query)
			}, func(row ports.ServiceProductCatalogueRow) string { return row.ObjectID + "/" + row.VersionLabel }),
			content: func(t *testing.T, fixture cutFixture, version domain.CommercialVersion) {
				mustSaveServiceProduct(t, fixture.transactor, t.Context(), fixture.publications, serviceProductOf(t, version))
			},
			contentHit: "network_service",
		},
		{
			name: "客户合同", kind: domain.CustomerContractObject, prefix: "SYN-CONTRACT-", pages: ports.CustomerContractPages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.CustomerContractCatalogueRow], error) {
				return catalogue.ListCustomerContracts(ctx, tenant, limit, query)
			}, func(row ports.CustomerContractCatalogueRow) string { return row.ObjectID + "/" + row.VersionLabel }),
			content: func(t *testing.T, fixture cutFixture, version domain.CommercialVersion) {
				contract, err := domain.NewCustomerContract(version, pcValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-LIVE"),
					pcValue(t, domain.NewCommercialObjectID, "rules-1"), nil)
				if err != nil {
					t.Fatalf("合同正文：%v", err)
				}
				mustSaveDeclaration(t, fixture.transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
					return fixture.publications.SaveCustomerContractContent(txCtx, contract)
				}, ports.DeclarationSaved)
			},
			contentHit: "syn-customer-live",
		},
		{
			name: "供应商协议", kind: domain.SupplierAgreementObject, prefix: "SYN-AGREEMENT-", pages: ports.SupplierAgreementPages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.SupplierAgreementCatalogueRow], error) {
				return catalogue.ListSupplierAgreements(ctx, tenant, limit, query)
			}, func(row ports.SupplierAgreementCatalogueRow) string { return row.ObjectID + "/" + row.VersionLabel }),
			content: func(t *testing.T, fixture cutFixture, version domain.CommercialVersion) {
				agreement, err := domain.NewSupplierAgreement(version,
					pcValue(t, domain.NewPartyID, "SYN-SUPPLIER-LIVE"),
					pcValue(t, domain.NewLegalEntityReference, "SYN-LEGAL-1"),
					pcScope(t),
					pcValue(t, domain.NewPricingPlanReference, "SYN-PLAN-BUY-1"),
					version.Effective())
				if err != nil {
					t.Fatalf("协议正文：%v", err)
				}
				mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
					_, err := fixture.publications.SaveSupplierAgreement(txCtx, agreement)
					return err
				})
			},
			contentHit: "syn-supplier-live",
		},
	}
}

// shellLister 把一册的取页抹成「对象/版本」一格。
func shellLister[Row any](
	list func(cataloguepage.Query, int) (cataloguepage.Result[Row], error),
	label func(Row) string,
) pageLister[string] {
	return func(query cataloguepage.Query, limit int) (cataloguepage.Result[string], error) {
		page, err := list(query, limit)
		labels := make([]string, len(page.Rows))
		for index, row := range page.Rows {
			labels[index] = label(row)
		}
		return cataloguepage.Result[string]{Rows: labels, Next: page.Next, Total: page.Total}, err
	}
}

// shellVersionFixture 经领域重建门造一份 tenant-1 的 kind 类版本：在 publishedAt 发布，从 cutRowPredecessorStart 起至
// cutRowEnd。已发布未生效的版本不带生效时刻（重建门会拒）；客户合同照 cutVersion 指名 rules-1。
func shellVersionFixture(
	t *testing.T,
	kind domain.CommercialObjectKind,
	objectID, label string,
	publishedAt time.Time,
	status domain.CommercialVersionStatus,
) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(cutRowPredecessorStart, cutRowEnd)
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "SYN-APPROVAL-"+objectID+"-"+label),
		pcValue(t, domain.NewCommercialSourceReference, "SYN-SOURCE-1"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	spec := domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          kind,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "SYN-DIGEST-"+objectID+"-"+label),
		Effective:     interval,
		Status:        status,
		Approval:      approval,
		PublishedAt:   publishedAt,
	}
	if status == domain.CommercialVersionEffective {
		spec.EffectiveAt = cutRowPredecessorStart
	}
	if kind == domain.CustomerContractObject {
		spec.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.AcceptanceRulePackageObject: pcValue(t, domain.NewCommercialObjectID, "rules-1"),
		}
	}
	version, err := domain.RehydrateCommercialVersion(spec)
	if err != nil {
		t.Fatalf("重建 %s 版本：%v", kind, err)
	}
	return version
}

func shellCatalogueOf(t *testing.T, fixture cutFixture) *adapter.OperationsCatalogue {
	t.Helper()
	catalogue, err := adapter.NewOperationsCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	return catalogue
}

// Covers: 完成判据「翻页期间插入新行不重不漏」与「空册答空页」，三册各一遍。缺省序是发布时刻降序，夹具逐个晚一分钟发布；三册
// 共一个租户，各读各的 object_kind。
func TestVersionShellCataloguesPageWithoutRepeatsOrGapsWhileVersionsArrive(t *testing.T) {
	fixture := newCutFixture(t)
	catalogue := shellCatalogueOf(t, fixture)
	published := publishedAtRow
	objectOf := func(label string) string {
		objectID, _, _ := strings.Cut(label, "/")
		return objectID
	}
	for _, shell := range shellCatalogueCases(t, catalogue, pcTenant(t, "tenant-1")) {
		assertPagesWithoutRepeatsOrGaps(t, shell.pages, shell.list, func(id string) {
			published = published.Add(time.Minute)
			fixture.save(t, shellVersionFixture(t, shell.kind, id, "v1", published, domain.CommercialVersionEffective))
		}, objectOf, shell.prefix)
	}
	for _, shell := range shellCatalogueCases(t, catalogue, pcTenant(t, "SYN-TENANT-EMPTY")) {
		assertEmptyCatalogue(t, shell.name, shell.pages, shell.list)
	}
}

// Covers: 完成判据「筛选与 q 收窄、total 随之变；q 里的 % 按字面」。status 按折进退役之后的位置筛：同起点退役另册保存、版本行上
// 仍是已生效，筛 RETIRED 要筛得出它，筛 EFFECTIVE 不得混进它。每册四版：已发布未生效的 PENDING、已生效的 LIVE（登了正文）、
// REISSUED 的 v1 让位给同起点的 v2 退役；发布时刻依次晚一分钟。
func TestVersionShellCataloguesFilterTheFoldedStatusAndSearch(t *testing.T) {
	fixture := newCutFixture(t)
	catalogue := shellCatalogueOf(t, fixture)
	for _, shell := range shellCatalogueCases(t, catalogue, pcTenant(t, "tenant-1")) {
		at := func(minutes int) time.Time { return publishedAtRow.Add(time.Duration(minutes) * time.Minute) }
		pending := shellVersionFixture(t, shell.kind, shell.prefix+"PENDING", "v1", at(1), domain.CommercialVersionPublished)
		live := shellVersionFixture(t, shell.kind, shell.prefix+"LIVE", "v1", at(2), domain.CommercialVersionEffective)
		old := shellVersionFixture(t, shell.kind, shell.prefix+"REISSUED", "v1", at(3), domain.CommercialVersionEffective)
		reissue := shellVersionFixture(t, shell.kind, shell.prefix+"REISSUED", "v2", at(4), domain.CommercialVersionEffective)
		for _, version := range []domain.CommercialVersion{pending, live, old, reissue} {
			fixture.save(t, version)
		}
		shell.content(t, fixture, live)
		if result := fixture.retire(t, retireCommand(t, old, "v2", "SYN-RETIRE-"+shell.prefix+"PAGE")); result.Outcome() != application.VersionClosureRegistered {
			t.Fatalf("%s retire outcome = %q (cause %v)", shell.name, result.Outcome(), result.Cause())
		}

		label := func(name string) string { return shell.prefix + name }
		assertFilteredPages(t, shell.name, shell.pages, shell.list, func(row string) string { return row }, []pageCase{
			{"", label("REISSUED/v2") + "," + label("REISSUED/v1") + "," + label("LIVE/v1") + "," + label("PENDING/v1")},
			{"status=RETIRED", label("REISSUED/v1")},
			{"status=EFFECTIVE", label("REISSUED/v2") + "," + label("LIVE/v1")},
			{"status=PUBLISHED&status=RETIRED", label("REISSUED/v1") + "," + label("PENDING/v1")},
			{"sort=objectId", label("LIVE/v1") + "," + label("PENDING/v1") + "," + label("REISSUED/v1") + "," + label("REISSUED/v2")},
			{"sort=-objectId&q=reissued", label("REISSUED/v2") + "," + label("REISSUED/v1")},
			{"q=v2", label("REISSUED/v2")},
			{"q=SCOPE-1&status=EFFECTIVE", label("REISSUED/v2") + "," + label("LIVE/v1")},
			{"q=" + shell.contentHit, label("LIVE/v1")},
			{"q=%25", ""},
		})
	}
}
