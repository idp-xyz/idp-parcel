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
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证客户服务规则的层次读口（ADR-0176 决定二，spec「Testing Decisions」缝三）：两层在场
// 标志各自如实、底座多候选答`适用冲突`、声明的服务产品照闭包回落层收窄底座、租户是身份不是过滤器、权威读不到与
// 锚点缺席同答`解析未决`、底座正文坏数据走 error。读口经 postgres 的权威视图与正文点读口装配，与生产同一条路。
//
// 夹具里的天数与材料只是取值，用来认出交回的是哪一版正文，不作业务断言——本册的值属实例半边。

// layerFixture 把写口、层次读口、事务器与裸池从**同一个**库交出（判据同 customerServiceRuleFixture）。
type layerFixture struct {
	repository *adapter.CommercialPublications
	contents   *adapter.CustomerServiceRuleContents
	reader     *application.CustomerServiceRuleLayerReader
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newLayerFixture(t *testing.T) layerFixture {
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
	authority, err := adapter.NewCommercialAuthority(repository)
	if err != nil {
		t.Fatalf("构造权威视图：%v", err)
	}
	reader, err := application.NewCustomerServiceRuleLayerReader(authority, contents)
	if err != nil {
		t.Fatalf("构造层次读口：%v", err)
	}
	return layerFixture{
		repository: repository,
		contents:   contents,
		reader:     reader,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

// serviceRuleShellIn 造一份`已生效`客户服务规则版本壳。names 是壳上的 references：选法只看壳，指名客户合同的是
// 合同层，指名服务产品或什么都不指名的是产品层。
func serviceRuleShellIn(
	t *testing.T,
	tenant, objectID string,
	names map[domain.CommercialObjectKind]string,
) domain.CommercialVersion {
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
	references := make(map[domain.CommercialObjectKind]domain.CommercialObjectID, len(names))
	for kind, named := range names {
		references[kind] = pcValue(t, domain.NewCommercialObjectID, named)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, tenant),
		Kind:          domain.CustomerServiceRuleObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "digest-"+tenant+"-"+objectID),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
		References:    references,
	})
	if err != nil {
		t.Fatalf("重建客户服务规则版本 %s：%v", objectID, err)
	}
	return version
}

func shellNamesContract(contract string) map[domain.CommercialObjectKind]string {
	return map[domain.CommercialObjectKind]string{domain.CustomerContractObject: contract}
}

func shellNamesProduct(product string) map[domain.CommercialObjectKind]string {
	return map[domain.CommercialObjectKind]string{domain.ServiceProductObject: product}
}

// contractTierBody 是挂合同的一版正文：只写首次索赔期限一行，其余各行留给底座。
func contractTierBody(t *testing.T, version domain.CommercialVersion, contract string, days int) domain.CustomerServiceRuleVersion {
	t.Helper()
	return customerServiceRuleOn(t, version,
		domain.CustomerServiceRuleAppliesToCustomerContract(pcValue(t, domain.NewCommercialObjectID, contract)),
		[]domain.ClaimDeadlineRule{deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", days, "calendar-cn")},
		nil,
	)
}

// productBaseBody 是挂产品的一版正文：首次索赔期限与一份材料清单。
func productBaseBody(t *testing.T, version domain.CommercialVersion, product string, days int) domain.CustomerServiceRuleVersion {
	t.Helper()
	return customerServiceRuleOn(t, version, appliesToProduct(t, product),
		[]domain.ClaimDeadlineRule{deadlineRule(t, domain.FirstClaimDeadline, "event-delivered", days, "calendar-cn")},
		[]domain.MinimumMaterialsRule{materialsRule(t, "claim-loss", "material-photo", "material-invoice")},
	)
}

// layerAnchor 落在夹具版本的有效区间里：选法按锚点收窄，区间外的锚点会让每一层都读成零候选。
func layerAnchor(t *testing.T) domain.SelectionAnchor {
	t.Helper()
	anchor, err := domain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	return anchor
}

// firstClaimDays 认出交回的是哪一版正文：夹具里各版的首次索赔天数互不相同。没有这一行答 0。
func firstClaimDays(rule domain.CustomerServiceRuleVersion) int {
	row, found := rule.ClaimDeadline(domain.FirstClaimDeadline)
	if !found {
		return 0
	}
	return row.DurationDays()
}

var noDeclaredProduct = domain.CommercialObjectID{}

// Covers: 票 03 完成判据「合同版与底座版在场 / 不在场四种组合各有真库用例」。底座「不在场」有两种成因，各配一次：
// 范围里没有产品层的版本（选法答`无适用依据`），与选出了那一版但它没登正文（选法答`唯一解析`、正文未登记）。
// 合同版不在场是壳在、正文没登。两层的标志各自如实，一层缺席不连带另一层。
//
// 范围里那一版合同版本身不得被选作底座：没有产品层版本的那几格若答`唯一解析`，就是选法把挂合同的壳当成了产品版。
func TestTheLayeredReadAnswersEachTierPresenceOnItsOwn(t *testing.T) {
	type baseShape int
	const (
		baseWithContent baseShape = iota
		noBaseVersion
		baseWithoutContent
	)
	for _, test := range []struct {
		name            string
		contractContent bool
		base            baseShape
		wantOutcome     domain.ResolutionOutcome
		wantBase        bool
	}{
		{"合同版与底座版都在场", true, baseWithContent, domain.UniquelyResolved, true},
		{"合同版在场，范围里没有产品版", true, noBaseVersion, domain.NoApplicableBasis, false},
		{"合同版在场，底座那一版没登正文", true, baseWithoutContent, domain.UniquelyResolved, false},
		{"合同版没登正文，底座在场", false, baseWithContent, domain.UniquelyResolved, true},
		{"合同版没登正文，范围里没有产品版", false, noBaseVersion, domain.NoApplicableBasis, false},
		{"合同版与底座那一版都没登正文", false, baseWithoutContent, domain.UniquelyResolved, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLayerFixture(t)
			repository, transactor := fixture.repository, fixture.transactor
			ctx := t.Context()
			tenant := pcTenant(t, "tenant-1")

			contract := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
			mustSaveVersion(t, transactor, ctx, repository, contract)
			if test.contractContent {
				mustSaveCustomerServiceRule(t, transactor, ctx, repository, contractTierBody(t, contract, "contract-02", 10))
			}
			base := serviceRuleShellIn(t, "tenant-1", "csr-product-a", shellNamesProduct("product-a"))
			if test.base != noBaseVersion {
				mustSaveVersion(t, transactor, ctx, repository, base)
			}
			if test.base == baseWithContent {
				mustSaveCustomerServiceRule(t, transactor, ctx, repository, productBaseBody(t, base, "product-a", 30))
			}

			layers, err := fixture.reader.LoadCustomerServiceRuleLayers(
				ctx, tenant, pcScope(t), contract, layerAnchor(t), noDeclaredProduct)
			if err != nil {
				t.Fatalf("层次读口：%v", err)
			}

			if layers.HasContractTier != test.contractContent {
				t.Fatalf("合同版在场 = %v, want %v", layers.HasContractTier, test.contractContent)
			}
			if test.contractContent {
				if !layers.ContractTier.Version().SameVersionAs(contract) || firstClaimDays(layers.ContractTier) != 10 {
					t.Fatalf("合同版交回的不是合同那一版：%s，首次索赔 %d 天",
						layers.ContractTier.Version().ObjectID(), firstClaimDays(layers.ContractTier))
				}
				if named, hangs := layers.ContractTier.Applicability().CustomerContract(); !hangs || named.String() != "contract-02" {
					t.Fatalf("合同版的适用声明变形：%#v", layers.ContractTier.Applicability())
				}
			} else if len(layers.ContractTier.ClaimDeadlines()) != 0 {
				t.Fatal("合同版没登正文却交回了期限")
			}

			if layers.ProductBaseOutcome != test.wantOutcome {
				t.Fatalf("底座选法 = %q, want %q", layers.ProductBaseOutcome, test.wantOutcome)
			}
			if layers.HasProductBase != test.wantBase {
				t.Fatalf("底座在场 = %v, want %v", layers.HasProductBase, test.wantBase)
			}
			if test.wantBase {
				if !layers.ProductBase.Version().SameVersionAs(base) || firstClaimDays(layers.ProductBase) != 30 {
					t.Fatalf("底座交回的不是产品那一版：%s，首次索赔 %d 天",
						layers.ProductBase.Version().ObjectID(), firstClaimDays(layers.ProductBase))
				}
				materials, found := layers.ProductBase.MinimumMaterialsFor(pcValue(t, domain.NewClaimKindReference, "claim-loss"))
				if !found || len(materials.Materials()) != 2 {
					t.Fatalf("底座的材料清单没随正文交回：found=%v", found)
				}
			} else if len(layers.ProductBase.ClaimDeadlines()) != 0 || len(layers.ProductBase.MinimumMaterials()) != 0 {
				t.Fatal("底座不在场却交回了正文")
			}
		})
	}
}

// Covers: ADR-0176 决定二「底座版在层次读口内选，多候选照`适用冲突`纪律答」：同范围产品层两版——一版指名服务产品、
// 一版什么都没指名——答`适用冲突`，不挑其中一版、不交底座正文。合同版照常交回：底座冲突不连带合同版那一层。
func TestTwoProductBaseCandidatesAreAnApplicabilityConflictNotAPick(t *testing.T) {
	fixture := newLayerFixture(t)
	repository, transactor := fixture.repository, fixture.transactor
	ctx := t.Context()

	contract := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
	named := serviceRuleShellIn(t, "tenant-1", "csr-product-a", shellNamesProduct("product-a"))
	bare := serviceRuleShellIn(t, "tenant-1", "csr-bare", nil)
	for _, version := range []domain.CommercialVersion{contract, named, bare} {
		mustSaveVersion(t, transactor, ctx, repository, version)
	}
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, contractTierBody(t, contract, "contract-02", 10))
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, productBaseBody(t, named, "product-a", 30))
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, productBaseBody(t, bare, "product-a", 45))

	layers, err := fixture.reader.LoadCustomerServiceRuleLayers(
		ctx, pcTenant(t, "tenant-1"), pcScope(t), contract, layerAnchor(t), noDeclaredProduct)
	if err != nil {
		t.Fatalf("底座冲突被当成了错误：%v", err)
	}
	if layers.ProductBaseOutcome != domain.ApplicabilityConflict {
		t.Fatalf("底座选法 = %q, want APPLICABILITY_CONFLICT", layers.ProductBaseOutcome)
	}
	if layers.HasProductBase || len(layers.ProductBase.ClaimDeadlines()) != 0 {
		t.Fatalf("冲突时交回了一份底座（首次索赔 %d 天）", firstClaimDays(layers.ProductBase))
	}
	if !layers.HasContractTier || firstClaimDays(layers.ContractTier) != 10 {
		t.Fatalf("底座冲突连带丢了合同版：在场=%v", layers.HasContractTier)
	}
}

// Covers: 判断项「读口多带 declaredProduct」——底座与闭包回落层同一口径：同范围两个产品各有一版时，声明了哪个产品
// 底座就是哪一版；不声明则答`适用冲突`，与闭包键不带服务产品时的回落同一答法。
func TestTheDeclaredServiceProductNarrowsTheProductBaseAsTheClosureDoes(t *testing.T) {
	fixture := newLayerFixture(t)
	repository, transactor := fixture.repository, fixture.transactor
	ctx := t.Context()

	contract := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
	productA := serviceRuleShellIn(t, "tenant-1", "csr-product-a", shellNamesProduct("product-a"))
	productB := serviceRuleShellIn(t, "tenant-1", "csr-product-b", shellNamesProduct("product-b"))
	for _, version := range []domain.CommercialVersion{contract, productA, productB} {
		mustSaveVersion(t, transactor, ctx, repository, version)
	}
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, contractTierBody(t, contract, "contract-02", 10))
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, productBaseBody(t, productA, "product-a", 30))
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, productBaseBody(t, productB, "product-b", 20))

	ask := func(declared domain.CommercialObjectID) ports.CustomerServiceRuleLayers {
		t.Helper()
		layers, err := fixture.reader.LoadCustomerServiceRuleLayers(
			ctx, pcTenant(t, "tenant-1"), pcScope(t), contract, layerAnchor(t), declared)
		if err != nil {
			t.Fatalf("层次读口：%v", err)
		}
		return layers
	}

	for product, want := range map[string]struct {
		version domain.CommercialVersion
		days    int
	}{
		"product-a": {productA, 30},
		"product-b": {productB, 20},
	} {
		layers := ask(pcValue(t, domain.NewCommercialObjectID, product))
		if layers.ProductBaseOutcome != domain.UniquelyResolved || !layers.HasProductBase {
			t.Fatalf("声明 %s：底座选法 = %q，在场 = %v", product, layers.ProductBaseOutcome, layers.HasProductBase)
		}
		if !layers.ProductBase.Version().SameVersionAs(want.version) || firstClaimDays(layers.ProductBase) != want.days {
			t.Fatalf("声明 %s：底座 = %s（首次索赔 %d 天）", product,
				layers.ProductBase.Version().ObjectID(), firstClaimDays(layers.ProductBase))
		}
	}

	if layers := ask(noDeclaredProduct); layers.ProductBaseOutcome != domain.ApplicabilityConflict || layers.HasProductBase {
		t.Fatalf("不声明服务产品：底座选法 = %q，在场 = %v, want APPLICABILITY_CONFLICT 且不在场",
			layers.ProductBaseOutcome, layers.HasProductBase)
	}
}

// Covers: ADR-0003——租户是身份不是过滤器。拿另一个租户去读本租户的合同版是 error 且不交任何一层；他租户在同名范围
// 里登记的产品版不是本租户的底座候选。
func TestTheLayeredReadIsBoundToItsTenant(t *testing.T) {
	fixture := newLayerFixture(t)
	repository, transactor := fixture.repository, fixture.transactor
	ctx := t.Context()

	mine := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
	theirs := serviceRuleShellIn(t, "tenant-2", "csr-product-a", shellNamesProduct("product-a"))
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, contractTierBody(t, mine, "contract-02", 10))
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, productBaseBody(t, theirs, "product-a", 30))

	layers, err := fixture.reader.LoadCustomerServiceRuleLayers(
		ctx, pcTenant(t, "tenant-2"), pcScope(t), mine, layerAnchor(t), noDeclaredProduct)
	if err == nil || layers.HasContractTier || layers.HasProductBase {
		t.Fatalf("拿他租户身份读本租户合同版：err=%v 合同版在场=%v 底座在场=%v，应是 error 且不交内容",
			err, layers.HasContractTier, layers.HasProductBase)
	}

	layers, err = fixture.reader.LoadCustomerServiceRuleLayers(
		ctx, pcTenant(t, "tenant-1"), pcScope(t), mine, layerAnchor(t), noDeclaredProduct)
	if err != nil {
		t.Fatalf("层次读口：%v", err)
	}
	if layers.ProductBaseOutcome != domain.NoApplicableBasis || layers.HasProductBase {
		t.Fatalf("他租户的产品版成了本租户的底座：选法 = %q，在场 = %v", layers.ProductBaseOutcome, layers.HasProductBase)
	}
	if !layers.HasContractTier {
		t.Fatal("本租户的合同版没读回")
	}
}

// unreadableAuthority 是读不到的权威视图：LoadScope 一律报错。
type unreadableAuthority struct{}

func (unreadableAuthority) LoadScope(
	context.Context,
	domain.TenantID,
	domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	return nil, errors.New("authority view unavailable")
}

// Covers: 读口头注「锚点立不住与权威读不到同答`解析未决`，不带原因」——两者都不是「这个范围没有产品版」，压成`无适用
// 依据`会让一次读不到被当成租户没登记；也不是 error：与第一阶段「读不到权威不向上抛技术错误」同一条分界。合同版
// 照常交回。
func TestAnUnreadableAuthorityAndAMissingAnchorBothLeaveTheProductBasePending(t *testing.T) {
	fixture := newLayerFixture(t)
	repository, transactor := fixture.repository, fixture.transactor
	ctx := t.Context()

	contract := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
	base := serviceRuleShellIn(t, "tenant-1", "csr-product-a", shellNamesProduct("product-a"))
	mustSaveVersion(t, transactor, ctx, repository, contract)
	mustSaveVersion(t, transactor, ctx, repository, base)
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, contractTierBody(t, contract, "contract-02", 10))
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, productBaseBody(t, base, "product-a", 30))

	unreadable, err := application.NewCustomerServiceRuleLayerReader(unreadableAuthority{}, fixture.contents)
	if err != nil {
		t.Fatalf("构造层次读口：%v", err)
	}
	for name, ask := range map[string]func() (ports.CustomerServiceRuleLayers, error){
		"权威读不到": func() (ports.CustomerServiceRuleLayers, error) {
			return unreadable.LoadCustomerServiceRuleLayers(
				ctx, pcTenant(t, "tenant-1"), pcScope(t), contract, layerAnchor(t), noDeclaredProduct)
		},
		"锚点缺席": func() (ports.CustomerServiceRuleLayers, error) {
			return fixture.reader.LoadCustomerServiceRuleLayers(
				ctx, pcTenant(t, "tenant-1"), pcScope(t), contract, domain.SelectionAnchor{}, noDeclaredProduct)
		},
	} {
		t.Run(name, func(t *testing.T) {
			layers, err := ask()
			if err != nil {
				t.Fatalf("未决被当成了错误：%v", err)
			}
			if layers.ProductBaseOutcome != domain.ResolutionPending || layers.HasProductBase {
				t.Fatalf("底座选法 = %q，在场 = %v, want RESOLUTION_PENDING 且不在场",
					layers.ProductBaseOutcome, layers.HasProductBase)
			}
			if !layers.HasContractTier || firstClaimDays(layers.ContractTier) != 10 {
				t.Fatalf("底座未决连带丢了合同版：在场=%v", layers.HasContractTier)
			}
		})
	}
}

// Covers: 读口头注「范围必须是合同版自己的范围」与类别——问法立不住是调用方违约，走 error，不是任何一格答案：拿别的
// 范围去配底座就是跨范围套条款，拿一份不是客户服务规则的版本当合同版则根本无从读起。
func TestTheLayeredReadRefusesAnAskItCannotAnswer(t *testing.T) {
	fixture := newLayerFixture(t)
	repository, transactor := fixture.repository, fixture.transactor
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	contract := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
	base := serviceRuleShellIn(t, "tenant-1", "csr-product-a", shellNamesProduct("product-a"))
	mustSaveVersion(t, transactor, ctx, repository, contract)
	mustSaveVersion(t, transactor, ctx, repository, base)
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, contractTierBody(t, contract, "contract-02", 10))
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, productBaseBody(t, base, "product-a", 30))

	otherScope := pcValue(t, domain.NewCommercialScopeReference, "scope-2")
	if layers, err := fixture.reader.LoadCustomerServiceRuleLayers(
		ctx, tenant, otherScope, contract, layerAnchor(t), noDeclaredProduct); err == nil || layers.HasContractTier || layers.HasProductBase {
		t.Fatalf("合同版不在所问范围：err=%v 合同版在场=%v 底座在场=%v，应是 error 且不交内容",
			err, layers.HasContractTier, layers.HasProductBase)
	}

	notARule := effectiveContract(t, "contract-02", "v1", "digest-contract-02")
	if layers, err := fixture.reader.LoadCustomerServiceRuleLayers(
		ctx, tenant, pcScope(t), notARule, layerAnchor(t), noDeclaredProduct); err == nil || layers.HasContractTier || layers.HasProductBase {
		t.Fatalf("拿客户合同版本当合同版：err=%v 合同版在场=%v 底座在场=%v，应是 error",
			err, layers.HasContractTier, layers.HasProductBase)
	}
}

// Covers: 底座正文的坏数据走 error，不折成「底座不在场」：壳指名 product-1 而正文挂 product-2，点读口那道核在底座这一层
// 照样把守——折成不在场，消费方会把一份写坏的产品版当成「无继承物」，各行落未登记，而该去修的是那份正文。
func TestABadProductBaseBodyIsAnErrorNotAnAbsentBase(t *testing.T) {
	fixture := newLayerFixture(t)
	repository, transactor, pool := fixture.repository, fixture.transactor, fixture.pool
	ctx := t.Context()

	contract := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
	mustSaveVersion(t, transactor, ctx, repository, contract)
	mustSaveCustomerServiceRule(t, transactor, ctx, repository, contractTierBody(t, contract, "contract-02", 10))
	named := customerServiceRuleVersionNaming(t, "csr-named", "product-1")
	mustSaveVersion(t, transactor, ctx, repository, named)
	insertCustomerServiceRuleRows(t, pool, "csr-named", "product-2", true)

	layers, err := fixture.reader.LoadCustomerServiceRuleLayers(
		ctx, pcTenant(t, "tenant-1"), pcScope(t), contract, layerAnchor(t), noDeclaredProduct)
	if !errors.Is(err, domain.ErrCustomerServiceRuleApplicabilityMismatch) {
		t.Fatalf("err = %v, want ErrCustomerServiceRuleApplicabilityMismatch", err)
	}
	if layers.HasContractTier || layers.HasProductBase {
		t.Fatalf("坏底座仍交回了内容：合同版在场=%v 底座在场=%v", layers.HasContractTier, layers.HasProductBase)
	}
}
