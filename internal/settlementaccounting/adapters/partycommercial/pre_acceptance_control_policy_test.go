package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证 SA→PC 控制策略适配器的三格。走真库而不是替身，是因为这
// 一段翻译的要害恰在两侧持久化面之间：闭包按标识落库、声明按合同版本落库，替身能让它们
// 「对上」，而对不上正是这一票要防的那件事。
//
// 夹具刻意不复用 PC 侧 postgres_test 的那套 helper——它们在另一个包里，抄一份是有意的：
// 跨包共享夹具会让本票的用例随那边的夹具演进而变形。

var (
	publishedAtRow    = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	effectiveAtRow    = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	approvedAtFixture = time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC)
)

type policyFixture struct {
	adapter      *adapter.PreAcceptanceControlPolicy
	pool         *pgxpool.Pool
	closure      pcdomain.CommercialClosure
	contractID   string
	contractLabe string
}

// Covers: sa-preacceptance-policy-view/01 丙案主径 —— 凭商业解析回指取回已固定闭包，
// 从闭包里的客户合同读出`要求控制`，方式与采用政策取自同一份闭包的结算政策（ADR-0044
// 的分工：声明只答「要不要」）。这是 ADR-0054 那份记录当年说「本记录不解决提供方表面」
// 的那半边，今天补上。
func TestARequiredDeclarationBecomesARequiredControlPolicyWithTheAdoptedMethod(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.PrepaidMethod)
	fixture.declare(t, "REQUIRED", nil)

	policy, found, err := fixture.load(t)
	if err != nil || !found {
		t.Fatalf("读回控制策略：found=%v err=%v", found, err)
	}
	if !policy.ControlRequired() {
		t.Fatal("合同声明`要求`却答出了不要求控制")
	}
	if policy.Method() != sadomain.PrepaidSettlement {
		t.Fatalf("method = %q, want PREPAID——方式取自闭包里已采用的结算政策", policy.Method())
	}
	if policy.AdoptedPolicy().String() != "policy-1/v1" {
		t.Fatalf("adopted policy = %q, want policy-1/v1", policy.AdoptedPolicy())
	}
	if policy.Basis().String() != "" {
		t.Fatal("`要求控制`带出了一份不适用依据——那条依据的含义是「凭什么不控制」")
	}
}

// Covers: 账期分支同径（ADR-0047）。方式必须原样来自闭包，不得由本适配器从声明反推——
// 从账期倒推「无需信用校验」正是 pn-02-w03 明禁的那条推导。
func TestATermsClosureYieldsTheTermsMethodRatherThanADefault(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.TermsMethod)
	fixture.declare(t, "REQUIRED", nil)

	policy, found, err := fixture.load(t)
	if err != nil || !found {
		t.Fatalf("读回控制策略：found=%v err=%v", found, err)
	}
	if policy.Method() != sadomain.TermsSettlement {
		t.Fatalf("method = %q, want TERMS", policy.Method())
	}
}

// Covers: ADR-0054 第一格与 CONTEXT「合同明确无接受前财务控制时必须保存商业不适用依据，
// 不能用缺失结果或默认通过代替」——`不适用`必须带着合同给的那条依据过来。依据丢在翻译
// 途中，SA 就会形成一次没有依据的`无控制`，那与默认信用通过分不开。
func TestANotApplicableDeclarationCarriesItsCommercialBasisAcross(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.PrepaidMethod)
	basis := "CONTRACT-CLAUSE-7/NO-PREPAY"
	fixture.declare(t, "NOT_APPLICABLE", &basis)

	policy, found, err := fixture.load(t)
	if err != nil || !found {
		t.Fatalf("读回控制策略：found=%v err=%v", found, err)
	}
	if policy.ControlRequired() {
		t.Fatal("合同声明`不适用`却答出了要求控制")
	}
	if policy.Basis().String() != basis {
		t.Fatalf("basis = %q, want %q", policy.Basis(), basis)
	}
	if policy.Method() != sadomain.SettlementMethodInvalid {
		t.Fatalf("method = %q；`不适用`不该带方式，闭包里有结算政策也不带", policy.Method())
	}
}

// Covers: ADR-0054 第二格 —— 合同在、声明没写 = found=false（实例未配置），且必须与
// 「调不通」分开。这一格在首发没有租户时是唯一走得到的真实分支；把它读成`无控制`，一个
// 没人登记过的范围就会拿到一份「合同说不用控制」的结论。
func TestAnUndeclaredContractIsNotConfiguredRatherThanNoControl(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.PrepaidMethod)

	policy, found, err := fixture.load(t)
	if err != nil {
		t.Fatalf("未配置被当成了错误：%v", err)
	}
	if found {
		t.Fatal("没有任何声明却答 found=true")
	}
	if policy.ControlRequired() || policy.Basis().String() != "" {
		t.Fatalf("未配置带出了内容：%#v", policy)
	}
}

// Covers: 坏回指与「未登记」分格。查无闭包不是「商业侧没登记过这份合同」——答 found=false
// 会把租户支去补一份声明，而真正坏的是调用方给的那条标识；他租户拿本租户的回指同理，
// 那是一次跨租户读取，不得因为「反正读不到」就折成未配置。
func TestABadResolutionReferenceIsAnErrorNotAnUnconfiguredGrade(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.PrepaidMethod)
	fixture.declare(t, "REQUIRED", nil)

	t.Run("unknown resolution", func(t *testing.T) {
		_, found, err := fixture.loadWith(t, saTenant(t, "tenant-1"), saResolution(t, "RES-NOBODY"))
		if err == nil {
			t.Fatal("查无闭包被答成了业务答案")
		}
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
		}
		if found {
			t.Fatal("出错还答 found=true")
		}
	})

	t.Run("another tenant cannot borrow the reference", func(t *testing.T) {
		_, found, err := fixture.loadWith(t, saTenant(t, "tenant-b"),
			saResolution(t, fixture.closure.ResolutionID().String()))
		if err == nil || found {
			t.Fatalf("他租户凭本租户回指读到了东西：found=%v err=%v", found, err)
		}
	})

	t.Run("empty reference", func(t *testing.T) {
		_, _, err := fixture.loadWith(t, saTenant(t, "tenant-1"), sadomain.CommercialResolutionReference{})
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("err = %v；空回指译不动是编程错误，不是未配置", err)
		}
	})
}

// Covers: 声明按（租户 + 合同版本）圈定这条纪律要贯穿到本适配器 —— 声明挂在他租户同号
// 合同上时，本租户读不到。它证的是本适配器把租户一路带到了声明读口，而不是取了闭包就
// 以为身份已经核过。
func TestADeclarationOnAnotherTenantsContractIsNotVisible(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.PrepaidMethod)
	fixture.declareFor(t, "tenant-b", "REQUIRED", nil)

	_, found, err := fixture.load(t)
	if err != nil {
		t.Fatalf("读回控制策略：%v", err)
	}
	if found {
		t.Fatal("读到了他租户挂在同号合同上的声明")
	}
}

// Covers: 装配疏漏不得伪装成未配置 —— 少给任一只读半边，构造期就拒。缺一半而静默答
// 「未配置」会让一次接线遗漏与租户没登记长得一模一样。
func TestTheAdapterRefusesToAssembleWithAMissingHalf(t *testing.T) {
	if _, err := adapter.NewPreAcceptanceControlPolicy(nil, nil); err == nil {
		t.Fatal("两半都缺仍装配成功")
	}
	declarations, resolutions, _, _ := newPersistence(t)
	if _, err := adapter.NewPreAcceptanceControlPolicy(nil, declarations); err == nil {
		t.Fatal("缺闭包读口仍装配成功")
	}
	if _, err := adapter.NewPreAcceptanceControlPolicy(resolutions, nil); err == nil {
		t.Fatal("缺声明读口仍装配成功")
	}
}

func (fixture *policyFixture) load(t *testing.T) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	t.Helper()
	return fixture.loadWith(t, saTenant(t, "tenant-1"),
		saResolution(t, fixture.closure.ResolutionID().String()))
}

func (fixture *policyFixture) loadWith(
	t *testing.T,
	tenant sadomain.TenantID,
	resolution sadomain.CommercialResolutionReference,
) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	t.Helper()
	return fixture.adapter.LoadControlPolicy(t.Context(), tenant, settlementScope(t), resolution)
}

func (fixture *policyFixture) declare(t *testing.T, requirement string, basis *string) {
	t.Helper()
	fixture.declareFor(t, "tenant-1", requirement, basis)
}

func (fixture *policyFixture) declareFor(t *testing.T, tenant, requirement string, basis *string) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pre_acceptance_control_declaration
			(tenant_id, object_kind, object_id, version_label, requirement, not_applicable_basis)
		 VALUES ($1, 2, $2, $3, $4, $5)`,
		tenant, fixture.contractID, fixture.contractLabe, requirement, basis); err != nil {
		t.Fatalf("登记接受前财务控制声明：%v", err)
	}
}

func newPolicyFixture(t *testing.T, method pcdomain.SettlementMethod) *policyFixture {
	t.Helper()

	declarations, resolutions, transactor, pool := newPersistence(t)
	closure := fixedClosure(t, method)
	var outcome pcports.ResolutionSaveOutcome
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = resolutions.Save(txCtx, closure)
		return err
	}); err != nil {
		t.Fatalf("固定解析：%v", err)
	}
	if outcome != pcports.ResolutionSaved {
		t.Fatalf("save outcome = %s, want SAVED", outcome)
	}

	built, err := adapter.NewPreAcceptanceControlPolicy(resolutions, declarations)
	if err != nil {
		t.Fatalf("构造 SA→PC 控制策略适配器：%v", err)
	}
	return &policyFixture{
		adapter:      built,
		pool:         pool,
		closure:      closure,
		contractID:   "contract-1",
		contractLabe: "v1",
	}
}

// newPersistence 交回同一个池上的两个 PC 读写口。池一并交出去是必要的：声明行直插库
// （声明写口属发布链，本票不经它），直插与适配器读到的必须是同一份数据。
func newPersistence(t *testing.T) (
	*pcpostgres.PreAcceptanceControlDeclarations,
	*pcpostgres.CommercialResolutions,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	declarations, err := pcpostgres.NewPreAcceptanceControlDeclarations(db)
	if err != nil {
		t.Fatalf("构造声明读口：%v", err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	return declarations, resolutions, db.Transactor(), pool
}

// fixedClosure 造一份含客户合同、接单规则包与结算政策的唯一已解析闭包——正是接受控制
// 目的下 PS 会固定的那个形状。
func fixedClosure(t *testing.T, method pcdomain.SettlementMethod) pcdomain.CommercialClosure {
	t.Helper()

	registry := pcdomain.NewCommercialRegistry()
	register(t, registry, effectiveVersion(t, pcdomain.CustomerContractObject, "contract-1", "v1", "digest-c1"))
	register(t, registry, effectiveVersion(t, pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "digest-r1"))

	policyVersion := effectiveVersion(t, pcdomain.SettlementPolicyObject, "policy-1", "v1", "digest-p1")
	register(t, registry, policyVersion)
	policy, err := pcdomain.NewSettlementPolicy(policyVersion, method, settlementApplicability(t))
	if err != nil {
		t.Fatalf("构造结算政策：%v", err)
	}
	registry.RegisterSettlementPolicy(policy)

	anchor, err := pcdomain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	closure := pcdomain.ResolveCommercialClosure(registry, pcdomain.ClosureResolutionKey{
		TenantID:             pcValue(t, pcdomain.NewTenantID, "tenant-1"),
		CustomerAccountID:    pcValue(t, pcdomain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		Scope:                pcValue(t, pcdomain.NewCommercialScopeReference, "scope-1"),
		Purpose:              pcdomain.AcceptanceControlPurpose,
		Anchor:               anchor,
		// 合同维不上键：它由闭包解出的 contract-1/v1 填（ADR-0080），正是 settlementApplicability
		// 指名的那一版。
		Settlement: pcdomain.SettlementSelector{
			Counterparty: pcValue(t, pcdomain.NewCounterpartyReference, "customer-1"),
			ChargeScope:  pcValue(t, pcdomain.NewChargeScopeReference, "charge-express"),
			Currency:     pcValue(t, pcdomain.NewCurrencyCode, "CNY"),
		},
		RequiredBases: []pcdomain.CommercialObjectKind{
			pcdomain.CustomerContractObject,
			pcdomain.AcceptanceRulePackageObject,
			pcdomain.SettlementPolicyObject,
		},
	}, nil)
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	return closure
}

func settlementApplicability(t *testing.T) pcdomain.SettlementApplicability {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	applicability, err := pcdomain.NewSettlementApplicability(
		pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		pcValue(t, pcdomain.NewCounterpartyReference, "customer-1"),
		pcValue(t, pcdomain.NewCommercialVersionLabel, "contract-1/v1"),
		pcValue(t, pcdomain.NewChargeScopeReference, "charge-express"),
		pcValue(t, pcdomain.NewCurrencyCode, "CNY"),
		interval,
	)
	if err != nil {
		t.Fatalf("结算适用范围：%v", err)
	}
	return applicability
}

func register(t *testing.T, registry *pcdomain.CommercialRegistry, version pcdomain.CommercialVersion) {
	t.Helper()
	if _, err := registry.Register(version); err != nil {
		t.Fatalf("登记 %s：%v", version.ObjectID(), err)
	}
}

func effectiveVersion(
	t *testing.T,
	kind pcdomain.CommercialObjectKind,
	objectID, label, digest string,
) pcdomain.CommercialVersion {
	t.Helper()

	interval, err := pcdomain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		pcValue(t, pcdomain.NewApprovalReference, "approval-"+objectID),
		pcValue(t, pcdomain.NewCommercialSourceReference, "source-"+objectID),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      pcValue(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          kind,
		ObjectID:      pcValue(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, pcdomain.NewCommercialVersionLabel, label),
		Scope:         pcValue(t, pcdomain.NewCommercialScopeReference, "scope-1"),
		ContentDigest: pcValue(t, pcdomain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建 %s：%v", objectID, err)
	}
	return version
}

func settlementScope(t *testing.T) sadomain.SettlementScope {
	t.Helper()
	scope, err := sadomain.NewSettlementScope(
		saValue(t, sadomain.NewLegalEntityReference, "legal-1"),
		saValue(t, sadomain.NewSettlementAccountID, "account-1"),
		saValue(t, sadomain.NewCurrencyCode, "CNY"),
	)
	if err != nil {
		t.Fatalf("结算作用域：%v", err)
	}
	return scope
}

func saTenant(t *testing.T, raw string) sadomain.TenantID {
	t.Helper()
	return saValue(t, sadomain.NewTenantID, raw)
}

func saResolution(t *testing.T, raw string) sadomain.CommercialResolutionReference {
	t.Helper()
	return saValue(t, sadomain.NewCommercialResolutionReference, raw)
}

func pcValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func saValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}
