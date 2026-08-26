package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证接单规则包目录上列携两族阶段内容声明(0013,票
// admin-web-page-wiring-frontier/07):三族子表各聚各的不互相翻倍、声明壳缺席与
// 「声明了但为空」可分辨。

// Covers: 三族子表(规则引用、允许来源、终局声明)同挂一份规则包时行数各自如实。
//
// 这一条是笛卡尔积回归。三族若都以 LEFT JOIN 挂上来再 GROUP BY,连接结果是
// 3×2×2=12 行,`json_agg` 于是把每一族都数重(规则 3→12、来源 2→12、终局 2→12)。
// 数字取得互不相同正是为了让那种失败一眼可辨:三族若同时变成 12,不会有任何一族的
// 断言碰巧还对。原实现只有一族子表,这个形状看不出问题,第二族一挂就出。
func TestRulePackageCatalogueDoesNotFanOutAcrossStageContentFamilies(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	owner := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-fanout", "v1")
	mustSaveVersion(t, transactor, ctx, repository, owner)

	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveAcceptanceRulePackage(txCtx, rulePackageBodyOf(t, owner,
			pcAssembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-1"),
			pcAssembledRule(t, domain.ShipmentInvariantRules, "RULE/invariant-1"),
			pcAssembledRule(t, domain.CrossFieldConditionRules, "RULE/cross-1"),
		))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveIntakeQualification(txCtx,
			intakeQualificationOf(t, owner, domain.DeclaredNodeIntake, domain.DeclaredOffsitePickup))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveFinalRule(txCtx, finalRuleOf(t, owner, "FINAL/effective-delivery"))
	}, ports.DeclarationSaved)

	rows, err := catalogue.ListAcceptanceRulePackages(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列规则包:%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("上列 %d 行,want 1", len(rows))
	}
	row := rows[0]

	if len(row.Rules) != 3 {
		t.Fatalf("规则引用 %d 条,want 3(三族互相翻倍了)", len(row.Rules))
	}
	if !row.HasIntakeQualification {
		t.Fatal("收寄资格声明壳在库里却答未声明")
	}
	if len(row.AllowedIntakeSources) != 2 ||
		row.AllowedIntakeSources[0] != "NODE_INTAKE" || row.AllowedIntakeSources[1] != "OFFSITE_PICKUP" {
		t.Fatalf("允许来源变形:%v", row.AllowedIntakeSources)
	}
	if len(row.IntakeQualificationRefs) != 1 ||
		row.IntakeQualificationRefs[0] != "INTAKE-QUAL/customs-precheck" {
		t.Fatalf("资格引用变形:%v", row.IntakeQualificationRefs)
	}
	if !row.HasFinalRules {
		t.Fatal("终局规则声明壳在库里却答未声明")
	}
	if len(row.FinalRules) != 2 {
		t.Fatalf("终局声明 %d 条,want 2", len(row.FinalRules))
	}
	byOutcome := map[string]string{}
	for _, final := range row.FinalRules {
		byOutcome[final.Outcome] = final.FinalKind
	}
	if byOutcome["EFFECTIVE_DELIVERY"] != "FINAL/effective-delivery" ||
		byOutcome["RETURN_COMPLETED"] != "FINAL/return" {
		t.Fatalf("终局声明变形:%+v", row.FinalRules)
	}
}

// Covers: 声明壳缺席与「声明了资格但资格引用为空」在结果上各不相同。
//
// 两态在 IntakeQualificationRefs 上都是空数组——只有 HasIntakeQualification 分得开,
// 而两者的恢复动作相反:前者去登记声明,后者无事可做(那份规则包就是显式声明了「无
// 硬资格」)。领域为此专门允许资格引用为空清单而不允许来源为空,库上父子两表照做。
func TestRulePackageCatalogueSeparatesUndeclaredStageContentFromEmptyDeclaration(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	bare := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-bare", "v1")
	emptyQual := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-empty-qual", "v1")
	mustSaveVersion(t, transactor, ctx, repository, bare)
	mustSaveVersion(t, transactor, ctx, repository, emptyQual)

	for _, owner := range []domain.CommercialVersion{bare, emptyQual} {
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveAcceptanceRulePackage(txCtx, rulePackageBodyOf(t, owner,
				pcAssembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-1"),
			))
		}, ports.DeclarationSaved)
	}

	// 显式声明「允许节点收寄、无硬资格」:资格引用清单为空,但声明本身在场。
	explicit, err := domain.NewIntakeQualificationContent(emptyQual,
		[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake}, nil)
	if err != nil {
		t.Fatalf("组空资格声明:%v", err)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveIntakeQualification(txCtx, explicit)
	}, ports.DeclarationSaved)

	rows, err := catalogue.ListAcceptanceRulePackages(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列规则包:%v", err)
	}
	byID := map[string]ports.AcceptanceRulePackageRow{}
	for _, row := range rows {
		byID[row.ObjectID] = row
	}

	if row := byID["rules-bare"]; row.HasIntakeQualification || row.HasFinalRules ||
		len(row.AllowedIntakeSources) != 0 || len(row.IntakeQualificationRefs) != 0 || len(row.FinalRules) != 0 {
		t.Fatalf("没声明过阶段内容的规则包答出了声明:%+v", row)
	}
	row := byID["rules-empty-qual"]
	if !row.HasIntakeQualification {
		t.Fatal("显式声明的空资格被答成未声明——这两态的恢复动作相反")
	}
	if len(row.IntakeQualificationRefs) != 0 {
		t.Fatalf("空资格清单答出了 %v", row.IntakeQualificationRefs)
	}
	if len(row.AllowedIntakeSources) != 1 || row.AllowedIntakeSources[0] != "NODE_INTAKE" {
		t.Fatalf("允许来源变形:%v", row.AllowedIntakeSources)
	}
	if row.HasFinalRules {
		t.Fatal("没声明终局规则却答已声明——两族不得互相顶格")
	}
}
