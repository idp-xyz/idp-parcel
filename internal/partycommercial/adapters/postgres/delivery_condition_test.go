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

// newDeliveryConditionFixture 一库四件：写口所在的发布登记册、事务器、框架 DB（供构造读口与解析库）与裸池（供数行）。
func newDeliveryConditionFixture(t *testing.T) (*adapter.CommercialPublications, bentoapp.Transactor, *bentopg.DB, *pgxpool.Pool) {
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
	return repository, db.Transactor(), db, pool
}

// 本文件对真实 PostgreSQL 16 证交付条件声明族（0030，ADR-0133 决定二 / 四，票 party-commercial-context-gaps/11）：
// 产品层与合同层经具名 Save 写入、按回指的读口读回；合同层写前对着它指名的那一版产品层核「只能在其内收紧」；
// 重放 / 冲突判据含方式集合、两条规则引用与所收紧的产品版本；读口四格里能在库上摆出来的三格各一例。夹具全部合成，
// 不写任何租户的交付方式或规则取值。

func deliveryTermsOf(t *testing.T, methods ...string) domain.DeliveryConditionTerms {
	t.Helper()
	terms := domain.DeliveryConditionTerms{
		RecipientScopeRule:  pcValue(t, domain.NewDeliveryRuleReference, "RULE/recipient-scope"),
		ProofOfDeliveryRule: pcValue(t, domain.NewDeliveryRuleReference, "RULE/proof-of-delivery"),
	}
	for _, method := range methods {
		terms.Methods = append(terms.Methods, pcValue(t, domain.NewDeliveryMethodReference, method))
	}
	return terms
}

func productDeliveryConditionsOf(t *testing.T, product domain.CommercialVersion, methods ...string) domain.DeliveryConditionContent {
	t.Helper()
	content, err := domain.DeclareProductDeliveryConditions(product, deliveryTermsOf(t, methods...))
	if err != nil {
		t.Fatalf("组产品层交付条件：%v", err)
	}
	return content
}

func contractDeliveryConditionsOf(t *testing.T, contract domain.CommercialVersion, productID, productVersion string, methods ...string) domain.DeliveryConditionContent {
	t.Helper()
	tightens, err := domain.NewTightenedProductVersion(
		pcValue(t, domain.NewCommercialObjectID, productID),
		pcValue(t, domain.NewCommercialVersionLabel, productVersion),
	)
	if err != nil {
		t.Fatalf("组所收紧的产品版本：%v", err)
	}
	content, err := domain.DeclareContractDeliveryConditions(contract, tightens, deliveryTermsOf(t, methods...))
	if err != nil {
		t.Fatalf("组合同层交付条件：%v", err)
	}
	return content
}

// saveDeliveryConditionsExpectingError 在一个会回滚的事务里跑 Save，交回它报的 error（无 error 即失败）。
func saveDeliveryConditionsExpectingError(
	t *testing.T,
	transactor bentoapp.Transactor,
	repository *adapter.CommercialPublications,
	content domain.DeliveryConditionContent,
) error {
	t.Helper()
	err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := repository.SaveDeliveryConditions(txCtx, content)
		if err == nil {
			t.Fatalf("本该被拒的合同层写成了 %s", outcome)
		}
		return err
	})
	if err == nil {
		t.Fatal("事务没把 Save 的 error 带出来")
	}
	return err
}

// Covers: 产品层往返与重放 / 冲突判据——同方式集合同规则引用是重放；方式多一种、规则引用换一条都是内容冲突且原正文
// 一行不动；合同层在指名的产品层之内收紧写得进去，与产品层同键不同层互不遮蔽。
func TestDeliveryConditionsRoundTripThroughTheProductionWritePort(t *testing.T) {
	repository, transactor, _, pool := newDeliveryConditionFixture(t)
	ctx := t.Context()

	product := effectiveServiceProductVersion(t, "product-1", "v1", "digest-p1")
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, product)
	mustSaveVersion(t, transactor, ctx, repository, contract)

	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, productDeliveryConditionsOf(t, product, "method-c", "method-a", "method-b"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, productDeliveryConditionsOf(t, product, "method-a", "method-b", "method-c"))
	}, ports.DeclarationAlreadyRegistered)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, productDeliveryConditionsOf(t, product, "method-a", "method-b"))
	}, ports.DeclarationContentConflict)
	otherRuleTerms := deliveryTermsOf(t, "method-a", "method-b", "method-c")
	otherRuleTerms.ProofOfDeliveryRule = pcValue(t, domain.NewDeliveryRuleReference, "RULE/other-proof")
	otherRule, err := domain.DeclareProductDeliveryConditions(product, otherRuleTerms)
	if err != nil {
		t.Fatalf("组换了规则引用的产品层：%v", err)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, otherRule)
	}, ports.DeclarationContentConflict)

	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, contractDeliveryConditionsOf(t, contract, "product-1", "v1", "method-b"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, contractDeliveryConditionsOf(t, contract, "product-1", "v1", "method-b"))
	}, ports.DeclarationAlreadyRegistered)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, contractDeliveryConditionsOf(t, contract, "product-1", "v1", "method-a"))
	}, ports.DeclarationContentConflict)

	var productMethods, contractMethods int
	if err := pool.QueryRow(ctx,
		`SELECT
		   (SELECT count(*) FROM party_commercial.delivery_condition_method WHERE object_kind = 1 AND object_id = 'product-1'),
		   (SELECT count(*) FROM party_commercial.delivery_condition_method WHERE object_kind = 2 AND object_id = 'contract-1')`,
	).Scan(&productMethods, &contractMethods); err != nil {
		t.Fatalf("数子行：%v", err)
	}
	if productMethods != 3 || contractMethods != 1 {
		t.Fatalf("子行数 = 产品 %d / 合同 %d，want 3 / 1——冲突路径写进了东西", productMethods, contractMethods)
	}
}

// Covers: 合同层写前核收紧（票面完成判据 1 在持久化面的那一半）——出现产品层没有的方式是放宽，Save 报 error 且事务回滚
// 一行不写；指名的产品版本不在本范围册上、或在册却没有产品层声明，同样拒：收紧是对着一份具体的产品层说的话。
func TestAContractLayerIsRefusedWhenItWidensOrNamesAnAbsentProductLayer(t *testing.T) {
	repository, transactor, _, pool := newDeliveryConditionFixture(t)
	ctx := t.Context()

	product := effectiveServiceProductVersion(t, "product-1", "v1", "digest-p1")
	bare := effectiveServiceProductVersion(t, "product-bare", "v1", "digest-bare")
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, product)
	mustSaveVersion(t, transactor, ctx, repository, bare)
	mustSaveVersion(t, transactor, ctx, repository, contract)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, productDeliveryConditionsOf(t, product, "method-a", "method-b"))
	}, ports.DeclarationSaved)

	widened := saveDeliveryConditionsExpectingError(t, transactor, repository,
		contractDeliveryConditionsOf(t, contract, "product-1", "v1", "method-a", "method-z"))
	if !errors.Is(widened, domain.ErrDeliveryConditionWidened) {
		t.Fatalf("放宽：err = %v，want ErrDeliveryConditionWidened", widened)
	}
	absent := saveDeliveryConditionsExpectingError(t, transactor, repository,
		contractDeliveryConditionsOf(t, contract, "product-9", "v1", "method-a"))
	if !errors.Is(absent, domain.ErrDeliveryConditionTighteningTarget) {
		t.Fatalf("产品版本不在册：err = %v，want ErrDeliveryConditionTighteningTarget", absent)
	}
	undeclared := saveDeliveryConditionsExpectingError(t, transactor, repository,
		contractDeliveryConditionsOf(t, contract, "product-bare", "v1", "method-a"))
	if !errors.Is(undeclared, domain.ErrDeliveryConditionWidened) {
		t.Fatalf("产品版本在册却无产品层：err = %v，want ErrDeliveryConditionWidened", undeclared)
	}

	var contractRows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM party_commercial.delivery_condition WHERE object_kind = 2`).Scan(&contractRows); err != nil {
		t.Fatalf("数合同层：%v", err)
	}
	if contractRows != 0 {
		t.Fatalf("被拒的合同层写进了 %d 行", contractRows)
	}
}

// Covers: 按回指答「有没有交付条件」的读口（票面完成判据 2）——「产品层有、合同层无」答引用且引用就是回指本身；
// 「两层都无」答没有（found=false, err=nil）；「闭包不在场」答 ErrDeliveryConditionClosureAbsent，他租户拿同一个回指同判
// （租户条件进语句，ADR-0003）；闭包在场却未采用客户合同版本答 ErrDeliveryConditionContractNotAdopted。
func TestDeliveryConditionReferenceIsAnsweredByResolutionReference(t *testing.T) {
	repository, transactor, db, _ := newDeliveryConditionFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	resolutions, err := adapter.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	reader, err := adapter.NewDeliveryConditions(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}

	product := effectiveServiceProductVersion(t, "product-1", "v1", "digest-p1")
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	mustSaveVersion(t, transactor, ctx, repository, product)
	mustSaveVersion(t, transactor, ctx, repository, contract)
	closure := uniqueClosureWithServiceProduct(t)
	mustSaveResolution(t, transactor, ctx, resolutions, closure)

	if _, found, err := reader.LoadDeliveryConditionReference(ctx, tenant, closure.ResolutionID()); err != nil || found {
		t.Fatalf("两层都无：found=%v err=%v，want false, nil", found, err)
	}

	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, productDeliveryConditionsOf(t, product, "method-a", "method-b"))
	}, ports.DeclarationSaved)
	reference, found, err := reader.LoadDeliveryConditionReference(ctx, tenant, closure.ResolutionID())
	if err != nil || !found {
		t.Fatalf("产品层有、合同层无：found=%v err=%v", found, err)
	}
	if reference.Resolution() != closure.ResolutionID() || reference.String() != closure.ResolutionID().String() {
		t.Fatalf("引用 = %s，want 回指本身 %s", reference, closure.ResolutionID())
	}

	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, contractDeliveryConditionsOf(t, contract, "product-1", "v1", "method-a"))
	}, ports.DeclarationSaved)
	if _, found, err := reader.LoadDeliveryConditionReference(ctx, tenant, closure.ResolutionID()); err != nil || !found {
		t.Fatalf("两层都有：found=%v err=%v", found, err)
	}

	unknown := pcValue(t, domain.NewResolutionID, "resolution-unknown")
	if _, _, err := reader.LoadDeliveryConditionReference(ctx, tenant, unknown); !errors.Is(err, domain.ErrDeliveryConditionClosureAbsent) {
		t.Fatalf("闭包不在场：err = %v，want ErrDeliveryConditionClosureAbsent", err)
	}
	other := pcTenant(t, "tenant-b")
	if _, _, err := reader.LoadDeliveryConditionReference(ctx, other, closure.ResolutionID()); !errors.Is(err, domain.ErrDeliveryConditionClosureAbsent) {
		t.Fatalf("他租户拿同一个回指：err = %v，want ErrDeliveryConditionClosureAbsent", err)
	}

	withoutContract := uniqueClosureWithoutContract(t)
	mustSaveResolution(t, transactor, ctx, resolutions, withoutContract)
	if _, _, err := reader.LoadDeliveryConditionReference(ctx, tenant, withoutContract.ResolutionID()); !errors.Is(err, domain.ErrDeliveryConditionContractNotAdopted) {
		t.Fatalf("未采用客户合同版本：err = %v，want ErrDeliveryConditionContractNotAdopted", err)
	}
}

// uniqueClosureWithoutContract 造一份只要求服务产品的唯一解析闭包：解析键不要求合同，闭包就不采用合同——读口对它
// 答 error 那一格靠它摆出来。
func uniqueClosureWithoutContract(t *testing.T) domain.CommercialClosure {
	t.Helper()
	registry := domain.NewCommercialRegistry()
	productVersion := effectiveServiceProductVersion(t, "product-1", "v1", "digest-p1")
	if _, err := registry.Register(productVersion); err != nil {
		t.Fatalf("登记产品版本：%v", err)
	}
	anchor, err := domain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	closure := domain.ResolveCommercialClosure(registry, domain.ClosureResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases:        []domain.CommercialObjectKind{domain.ServiceProductObject},
	}, nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	return closure
}
