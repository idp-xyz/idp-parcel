package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证客户服务规则册（票 party-commercial-context-gaps/05，ADR-0104，
// 0023 迁移）：两项正文往返、缺正文是合法缺席、同内容重放、异内容冲突且原行不动、租户是身份
// 不是过滤器、壳与正文适用声明分歧是坏数据、有父无子是坏数据、库上 CHECK 守住两格封闭与正时长。
//
// 夹具里的天数、日历、材料取值只是取值，不作断言依据——本册的值属实例半边，仓库不持有任何一份。

// customerServiceRuleFixture 把写口、读口、事务器与裸池从**同一个**库交出：pgtest.Pool 每次调用
// 都建一个全新的库，各建各的就各看各的（判据同 newDeclarationFixture）。裸池只给「坏数据」与
// CHECK 用例直接写表，正常路径一律经 SaveCustomerServiceRule。
type customerServiceRuleFixture struct {
	repository *adapter.CommercialPublications
	contents   *adapter.CustomerServiceRuleContents
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newCustomerServiceRuleFixture(t *testing.T) customerServiceRuleFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	contents, err := adapter.NewCustomerServiceRuleContents(db)
	if err != nil {
		t.Fatalf("构造客户服务规则读口：%v", err)
	}
	return customerServiceRuleFixture{
		repository: repository,
		contents:   contents,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

// customerServiceRuleVersionNaming 造一份`已生效`客户服务规则版本壳，壳上指名一个服务产品。
// 夹具通用的 effectiveVersionOfKind 只指名规则包，核适用声明那一道要壳上有产品引用才走得到。
func customerServiceRuleVersionNaming(t *testing.T, objectID, product string) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-1"),
		pcValue(t, domain.NewCommercialSourceReference, "source-1"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          domain.CustomerServiceRuleObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "digest-"+objectID),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
		References: map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.ServiceProductObject: pcValue(t, domain.NewCommercialObjectID, product),
		},
	})
	if err != nil {
		t.Fatalf("重建客户服务规则版本：%v", err)
	}
	return version
}

func deadlineRule(t *testing.T, kind domain.ClaimDeadlineKind, event string, days int, calendar string) domain.ClaimDeadlineRule {
	t.Helper()
	rule, err := domain.NewClaimDeadlineRule(
		kind,
		pcValue(t, domain.NewDeadlineStartEventReference, event),
		days,
		pcValue(t, domain.NewBusinessCalendarReference, calendar),
	)
	if err != nil {
		t.Fatalf("索赔期限规则：%v", err)
	}
	return rule
}

func materialsRule(t *testing.T, claimKind string, materials ...string) domain.MinimumMaterialsRule {
	t.Helper()
	references := make([]domain.MaterialRequirementReference, 0, len(materials))
	for _, material := range materials {
		references = append(references, pcValue(t, domain.NewMaterialRequirementReference, material))
	}
	rule, err := domain.NewMinimumMaterialsRule(pcValue(t, domain.NewClaimKindReference, claimKind), references)
	if err != nil {
		t.Fatalf("最低材料规则：%v", err)
	}
	return rule
}

func customerServiceRuleOn(
	t *testing.T,
	version domain.CommercialVersion,
	applicability domain.CustomerServiceRuleApplicability,
	deadlines []domain.ClaimDeadlineRule,
	materials []domain.MinimumMaterialsRule,
) domain.CustomerServiceRuleVersion {
	t.Helper()
	rule, err := domain.NewCustomerServiceRuleVersion(
		version,
		applicability,
		pcValue(t, domain.NewPartyID, "operator-1"),
		pcScope(t),
		deadlines,
		materials,
	)
	if err != nil {
		t.Fatalf("new customer service rule version: %v", err)
	}
	return rule
}

func appliesToProduct(t *testing.T, product string) domain.CustomerServiceRuleApplicability {
	t.Helper()
	return domain.CustomerServiceRuleAppliesToServiceProduct(pcValue(t, domain.NewCommercialObjectID, product))
}

func saveCustomerServiceRule(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	rule domain.CustomerServiceRuleVersion,
) ports.CustomerServiceRuleSaveOutcome {
	t.Helper()
	var outcome ports.CustomerServiceRuleSaveOutcome
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.SaveCustomerServiceRule(txCtx, rule)
		return err
	})
	return outcome
}

func mustSaveCustomerServiceRule(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	rule domain.CustomerServiceRuleVersion,
) {
	t.Helper()
	if outcome := saveCustomerServiceRule(t, transactor, ctx, repository, rule); outcome != ports.CustomerServiceRuleSaved {
		t.Fatalf("save outcome = %q, want SAVED", outcome)
	}
}

// Covers: ADR-0104 Decision 二——两项正文各自成行往返：期限按种类、材料按索赔类型带条目清单，
// 读回的每一行就是写下的那一行，且经领域构造门重建（壳与正文同一版本）。
func TestCustomerServiceRuleRoundTripsBothItems(t *testing.T) {
	fixture := newCustomerServiceRuleFixture(t)
	repository, contents, transactor := fixture.repository, fixture.contents, fixture.transactor
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	version := effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-1", "v1", "digest-csr1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, customerServiceRuleOn(t, version,
		appliesToProduct(t, "product-1"),
		[]domain.ClaimDeadlineRule{
			deadlineRule(t, domain.ConclusionReviewDeadline, "event-conclusion-notified", 15, "calendar-cn"),
			deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn"),
		},
		[]domain.MinimumMaterialsRule{materialsRule(t, "claim-loss", "material-photo", "material-invoice")},
	))

	rule, found, err := contents.LoadCustomerServiceRule(ctx, tenant, version)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if !rule.Version().SameVersionAs(version) {
		t.Fatal("正文挂回了另一个版本")
	}
	if product, applies := rule.Applicability().ServiceProduct(); !applies || product.String() != "product-1" {
		t.Fatalf("适用声明变形：%#v", rule.Applicability())
	}
	if _, applies := rule.Applicability().CustomerContract(); applies {
		t.Fatal("按产品适用的规则读回来多了一格合同")
	}
	if rule.ResponsibleParty().String() != "operator-1" || rule.Scope().String() != "scope-1" {
		t.Fatalf("责任方或范围变形：%s / %s", rule.ResponsibleParty(), rule.Scope())
	}

	deadlines := rule.ClaimDeadlines()
	if len(deadlines) != 2 {
		t.Fatalf("期限 %d 行, want 2", len(deadlines))
	}
	first, found := rule.ClaimDeadline(domain.FirstClaimDeadline)
	if !found || first.DurationDays() != 30 || first.StartEvent().String() != "event-delivered" ||
		first.Calendar().String() != "calendar-cn" {
		t.Fatalf("首次索赔期限变形：found=%v %#v", found, first)
	}
	review, found := rule.ClaimDeadline(domain.ConclusionReviewDeadline)
	if !found || review.DurationDays() != 15 || review.StartEvent().String() != "event-conclusion-notified" {
		t.Fatalf("结论复核期限变形：found=%v %#v", found, review)
	}
	if _, found := rule.ClaimDeadline(domain.MaterialSupplementDeadline); found {
		t.Fatal("没登记的资料补充期限读出了一条")
	}

	materials, found := rule.MinimumMaterialsFor(pcValue(t, domain.NewClaimKindReference, "claim-loss"))
	if !found {
		t.Fatal("材料清单没读回")
	}
	list := materials.Materials()
	if len(list) != 2 || list[0].String() != "material-invoice" || list[1].String() != "material-photo" {
		t.Fatalf("材料清单变形：%#v", list)
	}
}

// Covers: 缺正文是合法缺席（found=false），不是 error，也不是任何默认期限——VE 据以停在指名到维
// 的未决；凑一份就是发明实例参数。
func TestACustomerServiceRuleVersionWithoutContentIsNotFound(t *testing.T) {
	fixture := newCustomerServiceRuleFixture(t)
	repository, contents, transactor := fixture.repository, fixture.contents, fixture.transactor
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-bare", "v1", "digest-bare")
	mustSaveVersion(t, transactor, ctx, repository, version)

	rule, found, err := contents.LoadCustomerServiceRule(ctx, pcTenant(t, "tenant-1"), version)
	if err != nil {
		t.Fatalf("没正文被当成了错误：%v", err)
	}
	if found {
		t.Fatalf("没登记正文却读回了一份：%#v", rule)
	}
	if len(rule.ClaimDeadlines()) != 0 || len(rule.MinimumMaterials()) != 0 {
		t.Fatal("未登记的正文交回了期限或材料")
	}
}

// Covers: ADR-0031——同内容重放答`已登记`；改一条期限的时长、或改一份材料清单，都是`内容冲突`；
// 两者都不是 error，且原正文一行不动。冲突路径一行不写，否则「绝不覆盖」只对父行成立。
func TestSavingACustomerServiceRuleTwiceIsAReplayAndAChangedItemConflicts(t *testing.T) {
	fixture := newCustomerServiceRuleFixture(t)
	repository, contents, transactor := fixture.repository, fixture.contents, fixture.transactor
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	version := effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-1", "v1", "digest-csr1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	applicability := appliesToProduct(t, "product-1")
	original := customerServiceRuleOn(t, version, applicability,
		[]domain.ClaimDeadlineRule{deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")},
		[]domain.MinimumMaterialsRule{materialsRule(t, "claim-loss", "material-photo")},
	)
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, original)

	// 重放：同一份正文，声明顺序不同也算同一份——构造门已把两项各按键归档。
	replay := customerServiceRuleOn(t, version, applicability,
		[]domain.ClaimDeadlineRule{deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")},
		[]domain.MinimumMaterialsRule{materialsRule(t, "claim-loss", "material-photo")},
	)
	if outcome := saveCustomerServiceRule(t, transactor, ctx, repository, replay); outcome != ports.CustomerServiceRuleAlreadyRegistered {
		t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
	}

	changedDays := customerServiceRuleOn(t, version, applicability,
		[]domain.ClaimDeadlineRule{deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", 45, "calendar-cn")},
		[]domain.MinimumMaterialsRule{materialsRule(t, "claim-loss", "material-photo")},
	)
	if outcome := saveCustomerServiceRule(t, transactor, ctx, repository, changedDays); outcome != ports.CustomerServiceRuleContentConflict {
		t.Fatalf("changed deadline outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	changedMaterials := customerServiceRuleOn(t, version, applicability,
		[]domain.ClaimDeadlineRule{deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")},
		[]domain.MinimumMaterialsRule{materialsRule(t, "claim-loss", "material-photo", "material-invoice")},
	)
	if outcome := saveCustomerServiceRule(t, transactor, ctx, repository, changedMaterials); outcome != ports.CustomerServiceRuleContentConflict {
		t.Fatalf("changed materials outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	extraDeadline := customerServiceRuleOn(t, version, applicability,
		[]domain.ClaimDeadlineRule{
			deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn"),
			deadlineRule(t, domain.MaterialSupplementDeadline, "event-materials-requested", 10, "calendar-cn"),
		},
		[]domain.MinimumMaterialsRule{materialsRule(t, "claim-loss", "material-photo")},
	)
	if outcome := saveCustomerServiceRule(t, transactor, ctx, repository, extraDeadline); outcome != ports.CustomerServiceRuleContentConflict {
		t.Fatalf("extra deadline outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	rule, found, err := contents.LoadCustomerServiceRule(ctx, tenant, version)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if first, _ := rule.ClaimDeadline(domain.FirstClaimDeadline); first.DurationDays() != 30 {
		t.Fatalf("冲突写入改动了原期限：%#v", first)
	}
	if len(rule.ClaimDeadlines()) != 1 {
		t.Fatalf("冲突写入多写了期限行：%d", len(rule.ClaimDeadlines()))
	}
	materials, _ := rule.MinimumMaterialsFor(pcValue(t, domain.NewClaimKindReference, "claim-loss"))
	if len(materials.Materials()) != 1 {
		t.Fatalf("冲突写入改动了原材料清单：%#v", materials.Materials())
	}
}

// Covers: ADR-0003——租户是身份不是过滤器。拿另一个租户去读本租户的版本是 error 且不交内容；
// 他租户登记的同名版本不进本租户的读口。
func TestCustomerServiceRuleContentIsBoundToItsTenant(t *testing.T) {
	fixture := newCustomerServiceRuleFixture(t)
	repository, contents, transactor := fixture.repository, fixture.contents, fixture.transactor
	ctx := t.Context()

	mine := policyVersionInTenant(t, "tenant-1", domain.CustomerServiceRuleObject, "csr-1", "v1", "digest-mine")
	theirs := policyVersionInTenant(t, "tenant-2", domain.CustomerServiceRuleObject, "csr-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, customerServiceRuleOn(t, theirs,
		appliesToProduct(t, "product-1"),
		[]domain.ClaimDeadlineRule{deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")},
		nil,
	))

	if _, found, err := contents.LoadCustomerServiceRule(ctx, pcTenant(t, "tenant-2"), mine); err == nil || found {
		t.Fatalf("拿他租户身份读本租户版本：found=%v err=%v，应是 error 且不交内容", found, err)
	}
	if _, found, err := contents.LoadCustomerServiceRule(ctx, pcTenant(t, "tenant-1"), mine); err != nil || found {
		t.Fatalf("他租户的正文进了本租户的读口：found=%v err=%v", found, err)
	}
}

// Covers: ADR-0104 Decision 四——点读之后核「这一版挂的是不是我手上这份产品」。壳上指名 product-1
// 而正文说挂在 product-2，是坏数据：error 且不折成未登记；壳上没指名产品的版本照常读回。
func TestCustomerServiceRuleApplicabilityMustAgreeWithTheVersionShell(t *testing.T) {
	fixture := newCustomerServiceRuleFixture(t)
	repository, contents, transactor, pool := fixture.repository, fixture.contents, fixture.transactor, fixture.pool
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	t.Run("两处分歧", func(t *testing.T) {
		named := customerServiceRuleVersionNaming(t, "csr-named", "product-1")
		mustSaveVersion(t, transactor, ctx, repository, named)
		insertCustomerServiceRuleRows(t, pool, "csr-named", "product-2", true)

		_, found, err := contents.LoadCustomerServiceRule(ctx, tenant, named)
		if found || !errors.Is(err, domain.ErrCustomerServiceRuleApplicabilityMismatch) {
			t.Fatalf("found=%v err=%v；分歧应 error 且不得折成未登记", found, err)
		}
	})

	t.Run("壳上没指名", func(t *testing.T) {
		unnamed := effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-unnamed", "v1", "digest-unnamed")
		mustSaveVersion(t, transactor, ctx, repository, unnamed)
		insertCustomerServiceRuleRows(t, pool, "csr-unnamed", "product-2", true)

		rule, found, err := contents.LoadCustomerServiceRule(ctx, tenant, unnamed)
		if err != nil || !found {
			t.Fatalf("壳上没指名却拒了：found=%v err=%v", found, err)
		}
		if product, _ := rule.Applicability().ServiceProduct(); product.String() != "product-2" {
			t.Fatalf("适用声明 = %q", product)
		}
	})
}

// Covers: 有父行而两张子表都空是坏数据（领域要求至少一项）：error，不是 found=false——把损坏的
// 正文伪装成从未登记会让消费方去催一份其实已经写坏的配置。
func TestACustomerServiceRuleParentWithoutItemsIsBadData(t *testing.T) {
	fixture := newCustomerServiceRuleFixture(t)
	repository, contents, transactor, pool := fixture.repository, fixture.contents, fixture.transactor, fixture.pool
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-empty", "v1", "digest-empty")
	mustSaveVersion(t, transactor, ctx, repository, version)
	insertCustomerServiceRuleRows(t, pool, "csr-empty", "product-1", false)

	_, found, err := contents.LoadCustomerServiceRule(ctx, pcTenant(t, "tenant-1"), version)
	if err == nil || found {
		t.Fatalf("有父无子被读成了：found=%v err=%v", found, err)
	}
	if !errors.Is(err, domain.ErrInvalidCustomerServiceRuleVersion) {
		t.Fatalf("err = %v, want ErrInvalidCustomerServiceRuleVersion", err)
	}
}

// Covers: 库上 CHECK 守住绕开构造门的那条路——适用声明两列恰一非空、期限种类封闭三值、时长为正、
// 材料条目非空白。领域构造门拦得住经它进来的，CHECK 拦的是直接写表的。
func TestCustomerServiceRuleColumnsRejectShapesTheDomainRefuses(t *testing.T) {
	fixture := newCustomerServiceRuleFixture(t)
	repository, transactor, pool := fixture.repository, fixture.transactor, fixture.pool
	ctx := t.Context()
	version := effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-1", "v1", "digest-csr1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	insertParent := func(product, contract string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.customer_service_rule
				(tenant_id, object_kind, object_id, version_label,
				 service_product_id, customer_contract_id, responsible_party_id, scope_ref)
			 VALUES ('tenant-1', 10, 'csr-1', 'v1', `+product+`, `+contract+`, 'operator-1', 'scope-1')`)
		return err
	}
	if err := insertParent("NULL", "NULL"); err == nil {
		t.Fatal("两格都空的适用声明进了客户服务规则册")
	}
	if err := insertParent("'product-1'", "'contract-1'"); err == nil {
		t.Fatal("两格都有的适用声明进了客户服务规则册")
	}
	if err := insertParent("'  '", "NULL"); err == nil {
		t.Fatal("空白的产品引用进了客户服务规则册")
	}
	if err := insertParent("'product-1'", "NULL"); err != nil {
		t.Fatalf("合法父行进不去：%v", err)
	}

	insertDeadline := func(kind string, days int) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.customer_service_rule_claim_deadline
				(tenant_id, object_kind, object_id, version_label,
				 deadline_kind, start_event_ref, duration_days, calendar_ref)
			 VALUES ('tenant-1', 10, 'csr-1', 'v1', $1, 'event-delivered', $2, 'calendar-cn')`,
			kind, days)
		return err
	}
	if err := insertDeadline("SOMETHING_ELSE", 30); err == nil {
		t.Fatal("集外的期限种类进了索赔期限表")
	}
	if err := insertDeadline("FIRST_CLAIM", 0); err == nil {
		t.Fatal("零时长进了索赔期限表")
	}
	if err := insertDeadline("FIRST_CLAIM", -1); err == nil {
		t.Fatal("负时长进了索赔期限表")
	}
	if err := insertDeadline("FIRST_CLAIM", 30); err != nil {
		t.Fatalf("合法期限行进不去：%v", err)
	}
	if err := insertDeadline("FIRST_CLAIM", 45); err == nil {
		t.Fatal("同一种期限第二行进了索赔期限表")
	}

	insertMaterial := func(claimKind, material string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.customer_service_rule_minimum_material
				(tenant_id, object_kind, object_id, version_label, claim_kind_ref, material_ref)
			 VALUES ('tenant-1', 10, 'csr-1', 'v1', $1, $2)`,
			claimKind, material)
		return err
	}
	if err := insertMaterial("claim-loss", " "); err == nil {
		t.Fatal("空白材料条目进了最低材料表")
	}
	if err := insertMaterial("claim-loss", "material-photo"); err != nil {
		t.Fatalf("合法材料行进不去：%v", err)
	}
	if err := insertMaterial("claim-loss", "material-photo"); err == nil {
		t.Fatal("同一清单里同一条目第二次进了最低材料表")
	}
}

// insertCustomerServiceRuleRows 直接写父行（可选带一条期限行），只给「坏数据」用例用。
func insertCustomerServiceRuleRows(t *testing.T, pool *pgxpool.Pool, objectID, product string, withDeadline bool) {
	t.Helper()
	ctx := t.Context()
	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.customer_service_rule
			(tenant_id, object_kind, object_id, version_label,
			 service_product_id, customer_contract_id, responsible_party_id, scope_ref)
		 VALUES ('tenant-1', 10, $1, 'v1', $2, NULL, 'operator-1', 'scope-1')`,
		objectID, product); err != nil {
		t.Fatalf("直接写父行：%v", err)
	}
	if !withDeadline {
		return
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.customer_service_rule_claim_deadline
			(tenant_id, object_kind, object_id, version_label,
			 deadline_kind, start_event_ref, duration_days, calendar_ref)
		 VALUES ('tenant-1', 10, $1, 'v1', 'FIRST_CLAIM', 'event-delivered', 30, 'calendar-cn')`,
		objectID); err != nil {
		t.Fatalf("直接写期限行：%v", err)
	}
}
