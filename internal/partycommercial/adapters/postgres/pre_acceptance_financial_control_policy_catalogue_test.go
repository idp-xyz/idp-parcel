package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// Covers: 目录上列接受前财务控制策略册（票 admin-write-faces/06；ports.CommercialPolicyCatalogueRead
// 注释「正文表落库时按封闭集扩方法」在 0024 落库后的那一格）——上列的是第 5 类版本壳、正文左连接：
// 登了正文的行 HasContent 为真、共同通过条件与控制项俱在且控制项按判断顺序；只有壳的行 HasContent 为假
// 而不是从目录上消失（票面的缺口正是「发布得出来、管理台看不见」，壳可见是这张票的第一句）；别类的版本
// 壳与他租户的都不进本册；limit 非正拒；空册答空列表不答 error。
func TestPreAcceptanceFinancialControlPolicyCatalogueListsShellsWithAndWithoutContent(t *testing.T) {
	fixture := newControlPolicyFixture(t)
	repository, transactor := fixture.repository, fixture.transactor
	catalogue, err := adapter.NewOperationsCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	withContent := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-full", "v1", "digest-full")
	bare := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-bare", "v1", "digest-bare")
	theirs := policyVersionInTenant(t, "tenant-2", domain.PreAcceptanceFinancialControlPolicyObject, "fcp-theirs", "v1", "digest-theirs")
	// 另一类版本壳不得混进本册：本册按第 5 类上列，不按「有没有正文」上列。
	contract := effectiveVersionOfKind(t, domain.CustomerContractObject, "contract-1", "v1", "digest-contract")
	for _, version := range []domain.CommercialVersion{withContent, bare, theirs, contract} {
		mustSaveVersion(t, transactor, ctx, repository, version)
	}
	// 声明顺序故意与判断顺序相反：读回要按判断顺序，不按写入顺序。
	mustSaveControlPolicy(t, transactor, ctx, repository, controlPolicyOn(t, withContent,
		controlItemRow(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure, "operator-legal-1"),
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
	))
	mustSaveControlPolicy(t, transactor, ctx, repository, controlPolicyOn(t, theirs,
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
	))

	rows, err := catalogue.ListPreAcceptanceFinancialControlPolicies(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列接受前财务控制策略：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("上列 %d 行, want 2（他租户的与别类的版本壳都不得进本册）", len(rows))
	}
	byObject := map[string]ports.PreAcceptanceFinancialControlPolicyRow{}
	for _, row := range rows {
		byObject[row.ObjectID] = row
	}

	full := byObject["fcp-full"]
	if !full.HasContent || full.JointPassCondition != "ALL_CONTROLS_PASS" || full.Status != "EFFECTIVE" ||
		full.VersionLabel != "v1" || full.Scope != "scope-1" || full.RegisteredAt.IsZero() {
		t.Fatalf("登了正文的行 = %#v", full)
	}
	if len(full.Controls) != 2 {
		t.Fatalf("控制项 %d 行, want 2", len(full.Controls))
	}
	if full.Controls[0].Kind != "PREPAID_FREEZE" || full.Controls[0].EvaluationOrder != 1 ||
		full.Controls[0].ChargeScope != "charge-scope-a" || full.Controls[0].FailureDisposition != "REJECT" ||
		full.Controls[0].Responsibility != "customer-1" {
		t.Fatalf("第一项（按判断顺序）= %#v", full.Controls[0])
	}
	if full.Controls[1].Kind != "CREDIT_CHECK" || full.Controls[1].EvaluationOrder != 2 ||
		full.Controls[1].FailureDisposition != "AUTHORIZED_DISPOSITION" || full.Controls[1].Responsibility != "operator-legal-1" {
		t.Fatalf("第二项 = %#v", full.Controls[1])
	}

	empty := byObject["fcp-bare"]
	if empty.HasContent || empty.JointPassCondition != "" || len(empty.Controls) != 0 || !empty.RegisteredAt.IsZero() {
		t.Fatalf("只有壳的行 = %#v，正文各格该全部缺席", empty)
	}
	if empty.Status != "EFFECTIVE" || empty.Scope != "scope-1" || empty.PublishedAt.IsZero() || empty.EffectiveStartsAt.IsZero() {
		t.Fatalf("壳自身的列变形：%#v", empty)
	}

	if _, err := catalogue.ListPreAcceptanceFinancialControlPolicies(ctx, tenant, 0); err == nil {
		t.Fatal("limit 非正应被拒")
	}
	if rows, err := catalogue.ListPreAcceptanceFinancialControlPolicies(ctx, pcTenant(t, "tenant-3"), 10); err != nil || len(rows) != 0 {
		t.Fatalf("空册 = (%d 行, %v)，want 空列表且无 error", len(rows), err)
	}
}
