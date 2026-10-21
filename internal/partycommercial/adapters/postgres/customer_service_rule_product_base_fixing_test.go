package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 ADR-0268 决定一、三：闭包采纳合同版时随接受固定的继承底座，经快照写进去、读回来，层次
// 读口按它点读正文；底座那一版此后被同起点退役或有效性更正，已接受委托读到的仍是接受时那一版。没有底座一格的旧快照
// 照旧按锚点现选。整条路与生产同一：登记册与正文走 postgres，闭包由领域解析、经解析册固定再取回。

type fixedBaseFixture struct {
	publications *adapter.CommercialPublications
	resolutions  *adapter.CommercialResolutions
	reader       *application.CustomerServiceRuleLayerReader
	handler      *application.CloseCommercialVersionHandler
	transactor   bentoapp.Transactor
	pool         *pgxpool.Pool
}

func newFixedBaseFixture(t *testing.T) fixedBaseFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	publications, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	resolutions, err := adapter.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析册：%v", err)
	}
	contents, err := adapter.NewCustomerServiceRuleContents(db)
	if err != nil {
		t.Fatalf("构造客户服务规则读口：%v", err)
	}
	authority, err := adapter.NewCommercialAuthority(publications)
	if err != nil {
		t.Fatalf("构造权威视图：%v", err)
	}
	reader, err := application.NewCustomerServiceRuleLayerReader(authority, contents)
	if err != nil {
		t.Fatalf("构造层次读口：%v", err)
	}
	return fixedBaseFixture{
		publications: publications,
		resolutions:  resolutions,
		reader:       reader,
		handler:      application.NewCloseCommercialVersionHandler(publications, fixedClock{at: cutRowRegisteredAt}),
		transactor:   db.Transactor(),
		pool:         pool,
	}
}

// seed 登一份合同版委托接受时的世界：客户合同 contract-02、挂它的合同版 csr-contract-02（只写首次索赔期限 10 天）、
// 挂 product-a 的产品版 csr-product-a/v1（30 天加一份材料清单）。
func (fixture fixedBaseFixture) seed(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	mustSaveVersion(t, fixture.transactor, ctx, fixture.publications, effectiveContract(t, "contract-02", "v1", "digest-contract-02"))
	contractRule := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
	mustSaveVersion(t, fixture.transactor, ctx, fixture.publications, contractRule)
	mustSaveCustomerServiceRule(t, fixture.transactor, ctx, fixture.publications, contractTierBody(t, contractRule, "contract-02", 10))
	fixture.saveProductRule(t, "v1", 30)
}

func (fixture fixedBaseFixture) saveProductRule(t *testing.T, label string, days int) domain.CommercialVersion {
	t.Helper()
	version := serviceRuleShellLabelled(t, "csr-product-a", label, shellNamesProduct("product-a"))
	mustSaveVersion(t, fixture.transactor, t.Context(), fixture.publications, version)
	mustSaveCustomerServiceRule(t, fixture.transactor, t.Context(), fixture.publications, productBaseBody(t, version, "product-a", days))
	return version
}

func (fixture fixedBaseFixture) registry(t *testing.T) *domain.CommercialRegistry {
	t.Helper()
	registry, err := fixture.publications.LoadForScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	return registry
}

// accept 按此刻的登记册解一份合同加客户服务规则的闭包，经解析册固定，再按标识取回——取回的那份才是索赔时读到的。
func (fixture fixedBaseFixture) accept(t *testing.T) domain.CommercialClosure {
	t.Helper()
	closure := domain.ResolveCommercialClosure(fixture.registry(t), serviceRuleClosureKey(t), nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("接受时 outcome = %q（reason=%q，conflicting=%v）", closure.Outcome(), closure.Reason(), closure.ConflictingBases())
	}
	mustSaveResolution(t, fixture.transactor, t.Context(), fixture.resolutions, closure)
	return fixture.reload(t, closure.ResolutionID())
}

func (fixture fixedBaseFixture) reload(t *testing.T, id domain.ResolutionID) domain.CommercialClosure {
	t.Helper()
	held, found, err := fixture.resolutions.LoadResolution(t.Context(), pcTenant(t, "tenant-1"), id)
	if err != nil || !found {
		t.Fatalf("已固定的解析读不回：found=%v err=%v", found, err)
	}
	return held
}

func serviceRuleClosureKey(t *testing.T) domain.ClosureResolutionKey {
	t.Helper()
	anchor := layerAnchor(t)
	return domain.ClosureResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases:        []domain.CommercialObjectKind{domain.CustomerContractObject, domain.CustomerServiceRuleObject},
	}
}

// serviceRuleShellLabelled 同 serviceRuleShellIn，只是版本号由调用方给：同一产品版发第二版时要它。
func serviceRuleShellLabelled(
	t *testing.T,
	objectID, label string,
	names map[domain.CommercialObjectKind]string,
) domain.CommercialVersion {
	t.Helper()
	shell := serviceRuleShellIn(t, "tenant-1", objectID, names)
	approval, _ := shell.ApprovalBasis()
	references := make(map[domain.CommercialObjectKind]domain.CommercialObjectID, len(names))
	for kind := range names {
		named, _ := shell.ReferenceTo(kind)
		references[kind] = named
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      shell.Tenant(),
		Kind:          shell.Kind(),
		ObjectID:      shell.ObjectID(),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         shell.Scope(),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "digest-"+objectID+"-"+label),
		Effective:     shell.Effective(),
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
		References:    references,
	})
	if err != nil {
		t.Fatalf("重建客户服务规则版本 %s/%s：%v", objectID, label, err)
	}
	return version
}

// Covers: ADR-0268 决定一「已接受委托读底座，只读闭包上固定的那一版正文……那一版此后被截止、退役或有效性更正，都不改
// 已接受委托所读的那一行」与决定一快照「闭包快照记它的版本身份」——两条事后改动各走一遍真库：同起点发 v2 并退役 v1、
// 对 v1 登一条把锚点挪出区间的有效性更正。每一格先证「此刻现选已经不是 v1」，夹具才算咬到。
func TestAnAcceptedClosureKeepsReadingTheProductBaseFixedAtAcceptance(t *testing.T) {
	for name, afterAcceptance := range map[string]func(t *testing.T, fixture fixedBaseFixture){
		"同起点退役，v2 接替": func(t *testing.T, fixture fixedBaseFixture) {
			fixture.saveProductRule(t, "v2", 45)
			old := serviceRuleShellLabelled(t, "csr-product-a", "v1", shellNamesProduct("product-a"))
			var result application.VersionClosureResult
			mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
				var err error
				result, err = fixture.handler.RetireAtSameStart(txCtx, retireCommand(t, old, "v2", "SYN-RETIRE-BASE"))
				return err
			})
			if result.Outcome() != application.VersionClosureRegistered {
				t.Fatalf("退役 outcome = %q (cause %v)", result.Outcome(), result.Cause())
			}
		},
		"有效性更正把锚点挪出区间": func(t *testing.T, fixture fixedBaseFixture) {
			old := serviceRuleShellLabelled(t, "csr-product-a", "v1", shellNamesProduct("product-a"))
			end, _ := old.Effective().EndsAt()
			moved, err := domain.NewEffectiveInterval(layerAnchor(t).At().Add(24*time.Hour), end)
			if err != nil {
				t.Fatalf("有效区间：%v", err)
			}
			correction, err := old.CorrectEffectiveInterval(moved,
				pcValue(t, domain.NewValidityCorrectionReference, "SYN-CORRECT-BASE"), cutRowRegisteredAt)
			if err != nil {
				t.Fatalf("区间更正：%v", err)
			}
			mustSaveValidityCorrection(t, fixture.transactor, t.Context(), fixture.publications, correction)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newFixedBaseFixture(t)
			fixture.seed(t)
			held := fixture.accept(t)

			adopted, _ := held.AdoptedFor(domain.CustomerServiceRuleObject)
			base, hasBase, fixed := adopted.InheritedProductBase()
			if !fixed || !hasBase || base.ObjectID().String() != "csr-product-a" || base.Version().String() != "v1" {
				t.Fatalf("快照读回的底座 = %s/%s（在场=%v，已固定=%v），want csr-product-a/v1 已固定",
					base.ObjectID(), base.Version(), hasBase, fixed)
			}

			afterAcceptance(t, fixture)
			now, outcome := fixture.registry(t).CustomerServiceRuleProductBase(
				pcTenant(t, "tenant-1"), pcScope(t), held.Anchor(), domain.CommercialObjectID{})
			if outcome == domain.UniquelyResolved && now.Version().String() == "v1" {
				t.Fatal("事后改动之后此刻现选仍是 v1：夹具没咬到")
			}

			layers, err := fixture.reader.LoadCustomerServiceRuleLayers(t.Context(), pcTenant(t, "tenant-1"),
				fixture.reload(t, held.ResolutionID()))
			if err != nil {
				t.Fatalf("层次读口：%v", err)
			}
			if layers.ProductBaseOutcome != domain.UniquelyResolved || !layers.HasProductBase ||
				layers.ProductBase.Version().Version().String() != "v1" || firstClaimDays(layers.ProductBase) != 30 {
				t.Fatalf("索赔时读到的底座 = %q %s（在场=%v，首次索赔 %d 天），want 接受时的 v1、30 天",
					layers.ProductBaseOutcome, layers.ProductBase.Version().Version(), layers.HasProductBase,
					firstClaimDays(layers.ProductBase))
			}
			if !layers.HasContractTier || firstClaimDays(layers.ContractTier) != 10 {
				t.Fatalf("合同版没随之交回：在场=%v", layers.HasContractTier)
			}
		})
	}
}

// Covers: ADR-0268 决定一「零版：记没有底座」——快照记下「固定了、没有」，此后同范围再登一版产品版，层次读口照答`无适用
// 依据`，不现选出那一版：接受时没有的继承物，事后也不该冒出来。
func TestAClosureFixedWithoutAProductBaseDoesNotPickUpALaterOne(t *testing.T) {
	fixture := newFixedBaseFixture(t)
	ctx := t.Context()
	mustSaveVersion(t, fixture.transactor, ctx, fixture.publications, effectiveContract(t, "contract-02", "v1", "digest-contract-02"))
	contractRule := serviceRuleShellIn(t, "tenant-1", "csr-contract-02", shellNamesContract("contract-02"))
	mustSaveVersion(t, fixture.transactor, ctx, fixture.publications, contractRule)
	mustSaveCustomerServiceRule(t, fixture.transactor, ctx, fixture.publications, contractTierBody(t, contractRule, "contract-02", 10))

	held := fixture.accept(t)
	adopted, _ := held.AdoptedFor(domain.CustomerServiceRuleObject)
	if _, hasBase, fixed := adopted.InheritedProductBase(); !fixed || hasBase {
		t.Fatalf("快照读回：底座在场=%v 已固定=%v，want 已固定且没有", hasBase, fixed)
	}

	fixture.saveProductRule(t, "v1", 30)
	layers, err := fixture.reader.LoadCustomerServiceRuleLayers(ctx, pcTenant(t, "tenant-1"), held)
	if err != nil {
		t.Fatalf("层次读口：%v", err)
	}
	if layers.ProductBaseOutcome != domain.NoApplicableBasis || layers.HasProductBase {
		t.Fatalf("底座 = %q（在场=%v），want NO_APPLICABLE_BASIS 且不在场", layers.ProductBaseOutcome, layers.HasProductBase)
	}
}

// Covers: ADR-0268 决定一「多版：……在接受时就挡，不留到索赔时报」的真库一半——同范围两版产品版同期生效，合同版委托的
// 闭包在接受时就答`适用冲突`，解析册里没有可固定的那一份。
func TestTwoProductBasesStopTheAcceptanceAtAnApplicabilityConflict(t *testing.T) {
	fixture := newFixedBaseFixture(t)
	fixture.seed(t)
	bare := serviceRuleShellIn(t, "tenant-1", "csr-bare", nil)
	mustSaveVersion(t, fixture.transactor, t.Context(), fixture.publications, bare)

	closure := domain.ResolveCommercialClosure(fixture.registry(t), serviceRuleClosureKey(t), nil)
	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
	}
	if conflicting := closure.ConflictingBases(); len(conflicting) != 1 || conflicting[0] != domain.CustomerServiceRuleObject {
		t.Fatalf("conflicting = %v, want exactly the customer service rule", conflicting)
	}
}

// Covers: ADR-0268 决定三「本记录之前形成的闭包快照没有底座一格。读到这种快照时照今天的做法按锚点现选」——把一份新快照
// 里的底座一格删掉，读回即「未固定」，层次读口按锚点现选：同起点退役之后选到的是 v2。这一路只为演示库与测试夹具留着。
func TestASnapshotWithoutTheProductBaseCellStillSelectsAtTheAnchor(t *testing.T) {
	fixture := newFixedBaseFixture(t)
	fixture.seed(t)
	held := fixture.accept(t)

	tag, err := fixture.pool.Exec(t.Context(),
		`UPDATE party_commercial.commercial_resolution
		    SET snapshot = jsonb_set(snapshot, '{adopted}',
		        (SELECT jsonb_agg(item - 'productBase') FROM jsonb_array_elements(snapshot->'adopted') AS item))
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		"tenant-1", held.ResolutionID().String())
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("改写成旧快照：rows=%d err=%v", tag.RowsAffected(), err)
	}
	legacy := fixture.reload(t, held.ResolutionID())
	adopted, _ := legacy.AdoptedFor(domain.CustomerServiceRuleObject)
	if _, hasBase, fixed := adopted.InheritedProductBase(); fixed || hasBase {
		t.Fatalf("旧快照读回：底座在场=%v 已固定=%v，want 都不——缺这一格不补成「没有」", hasBase, fixed)
	}

	fixture.saveProductRule(t, "v2", 45)
	old := serviceRuleShellLabelled(t, "csr-product-a", "v1", shellNamesProduct("product-a"))
	mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		_, err := fixture.handler.RetireAtSameStart(txCtx, retireCommand(t, old, "v2", "SYN-RETIRE-BASE"))
		return err
	})

	layers, err := fixture.reader.LoadCustomerServiceRuleLayers(t.Context(), pcTenant(t, "tenant-1"), legacy)
	if err != nil {
		t.Fatalf("层次读口：%v", err)
	}
	if layers.ProductBaseOutcome != domain.UniquelyResolved || layers.ProductBase.Version().Version().String() != "v2" ||
		firstClaimDays(layers.ProductBase) != 45 {
		t.Fatalf("旧快照的底座 = %q %s（首次索赔 %d 天），want 按锚点现选的 v2、45 天",
			layers.ProductBaseOutcome, layers.ProductBase.Version().Version(), firstClaimDays(layers.ProductBase))
	}
}
