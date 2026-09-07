package partycommercial_test

import (
	"context"
	"errors"
	"fmt"
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
	adapter        *adapter.PreAcceptanceControlPolicy
	pool           *pgxpool.Pool
	persistence    pcPersistence
	closure        pcdomain.CommercialClosure
	controlVersion pcdomain.CommercialVersion
	contractID     string
	contractLabe   string
}

// Covers: sa-preacceptance-policy-view/01 丙案主径与 /02 裁决 1 —— 凭商业解析回指取回已固定闭包，
// 从闭包里的客户合同读出`要求控制`，**控制项来自闭包里已采用的接受前财务控制策略版本的正文**
// （ADR-0115 / ADR-0122），结算方式与采用政策取自同一份闭包的结算政策但只作保存不再选路。这是
// ADR-0054 那份记录当年说「本记录不解决提供方表面」的那半边，也是 ADR-0115 决定五留给 SA 的那一票。
func TestARequiredDeclarationBecomesARequiredControlPolicyWithTheAdoptedMethod(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.PrepaidMethod)
	fixture.declare(t, "REQUIRED", nil)
	fixture.registerContent(t, controlItemFor(t, pcdomain.PrepaidFreezeControl, "charge-express", 1))

	policy, found, err := fixture.load(t)
	if err != nil || !found {
		t.Fatalf("读回控制策略：found=%v err=%v", found, err)
	}
	if !policy.ControlRequired() {
		t.Fatal("合同声明`要求`却答出了不要求控制")
	}
	items := policy.Items()
	if len(items) != 1 || items[0].Kind() != sadomain.PrepaidFreezeControl || items[0].Order() != 1 {
		t.Fatalf("items = %v, want one PREPAID_FREEZE at order 1——控制项来自策略正文", items)
	}
	if policy.ControlPolicy().String() != "control-1/v1" {
		t.Fatalf("control policy = %q, want control-1/v1——采用的控制策略版本要随答复带回", policy.ControlPolicy())
	}
	if policy.JointPassCondition() != sadomain.AllControlsPass {
		t.Fatalf("joint pass = %q, want ALL_CONTROLS_PASS", policy.JointPassCondition())
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

// Covers: pn-02-w03「结算模式不等于接受前财务控制策略：账期不能推导无需信用校验，预付不能推导冻结」
// 自本票起被结构守住——一份**账期**合同的策略正文同时要求预付冻结与信用校验，两项都原样译过来、
// 按判断顺序排，方式仍如实是 TERMS；旧形（方式即选路）在这个组合上说不出话。
func TestATermsClosureCarriesEveryControlItemThePolicyContentRequires(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.TermsMethod)
	fixture.declare(t, "REQUIRED", nil)
	fixture.registerContent(t,
		controlItemFor(t, pcdomain.CreditCheckControl, "charge-express", 2),
		controlItemFor(t, pcdomain.PrepaidFreezeControl, "charge-express", 1))

	policy, found, err := fixture.load(t)
	if err != nil || !found {
		t.Fatalf("读回控制策略：found=%v err=%v", found, err)
	}
	if policy.Method() != sadomain.TermsSettlement {
		t.Fatalf("method = %q, want TERMS", policy.Method())
	}
	items := policy.Items()
	if len(items) != 2 ||
		items[0].Kind() != sadomain.PrepaidFreezeControl || items[0].Order() != 1 ||
		items[1].Kind() != sadomain.CreditCheckControl || items[1].Order() != 2 {
		t.Fatalf("items = %v, want PREPAID_FREEZE@1 then CREDIT_CHECK@2——账期合同一样可以要求预付冻结", items)
	}
}

// Covers: ADR-0122 决定一「费用范围从已采用结算政策的适用范围上取」——正文里挂在别的费用范围上的
// 控制项不进本次答复；本范围一项都没有时是`未配置`（合同绑定把范围指到了这份策略，策略却没为它写
// 控制项，那要租户补，不是「无控制」——无控制得合同带依据声明）。
func TestOnlyTheItemsOnTheResolvedChargeScopeAreTranslated(t *testing.T) {
	fixture := newPolicyFixture(t, pcdomain.PrepaidMethod)
	fixture.declare(t, "REQUIRED", nil)
	fixture.registerContent(t,
		controlItemFor(t, pcdomain.PrepaidFreezeControl, "charge-express", 1),
		controlItemFor(t, pcdomain.CreditCheckControl, "charge-economy", 2))

	policy, found, err := fixture.load(t)
	if err != nil || !found {
		t.Fatalf("读回控制策略：found=%v err=%v", found, err)
	}
	items := policy.Items()
	if len(items) != 1 || items[0].Kind() != sadomain.PrepaidFreezeControl {
		t.Fatalf("items = %v, want only the charge-express item", items)
	}

	other := newPolicyFixture(t, pcdomain.PrepaidMethod)
	other.declare(t, "REQUIRED", nil)
	other.registerContent(t, controlItemFor(t, pcdomain.CreditCheckControl, "charge-economy", 1))
	policy, found, err = other.load(t)
	if err != nil {
		t.Fatalf("本范围无控制项被当成了错误：%v", err)
	}
	if found || policy.ControlRequired() {
		t.Fatalf("本范围一项控制都没写却答了 found=%v / %#v", found, policy)
	}
}

// Covers: 票 02 裁决 4 —— 合同版本级 `0007` 说`要求`而策略正文未登记：答`未配置`（found=false，SA 编排
// 据以停在 CONTROL_POLICY_NOT_CONFIGURED，恢复动作是租户补正文），**不**沿用「从结算政策推方式」
// 的过渡——那正是 pn-02-w03 禁的推导。闭包压根没采用控制策略同格：解析键没要求它或合同没绑，都是
// 租户配置未齐。
func TestARequiredDeclarationWithoutPolicyContentIsNotConfiguredNotDerived(t *testing.T) {
	t.Run("version published, content missing", func(t *testing.T) {
		fixture := newPolicyFixture(t, pcdomain.PrepaidMethod)
		fixture.declare(t, "REQUIRED", nil)

		policy, found, err := fixture.load(t)
		if err != nil {
			t.Fatalf("正文未登记被当成了错误：%v", err)
		}
		if found || policy.ControlRequired() {
			t.Fatalf("正文未登记却答了 found=%v / %#v——这是从结算方式推出来的控制", found, policy)
		}
	})

	t.Run("closure adopted no control policy", func(t *testing.T) {
		fixture := newPolicyFixtureWithClosure(t, fixedClosure(t, pcdomain.PrepaidMethod, false))
		fixture.declare(t, "REQUIRED", nil)
		fixture.registerContent(t, controlItemFor(t, pcdomain.PrepaidFreezeControl, "charge-express", 1))

		policy, found, err := fixture.load(t)
		if err != nil {
			t.Fatalf("闭包未采用控制策略被当成了错误：%v", err)
		}
		if found || policy.ControlRequired() {
			t.Fatalf("闭包里没有控制策略却答了 found=%v / %#v", found, policy)
		}
	})
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
	if _, err := adapter.NewPreAcceptanceControlPolicy(nil, nil, nil); err == nil {
		t.Fatal("三半都缺仍装配成功")
	}
	persistence := newPersistence(t)
	if _, err := adapter.NewPreAcceptanceControlPolicy(nil, persistence.declarations, persistence.contents); err == nil {
		t.Fatal("缺闭包读口仍装配成功")
	}
	if _, err := adapter.NewPreAcceptanceControlPolicy(persistence.resolutions, nil, persistence.contents); err == nil {
		t.Fatal("缺声明读口仍装配成功")
	}
	if _, err := adapter.NewPreAcceptanceControlPolicy(persistence.resolutions, persistence.declarations, nil); err == nil {
		t.Fatal("缺正文读口仍装配成功——少了它`要求`那一格就只能回到从结算方式推控制")
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

// registerContent 把接受前财务控制策略版本的壳与正文经 PC 的发布写口落库（正文表外键指向版本壳，
// 不能像 0007 那样直插）。正文的共同通过条件取首发唯一值。
func (fixture *policyFixture) registerContent(t *testing.T, items ...pcdomain.PreAcceptanceControlItem) {
	t.Helper()
	content, err := pcdomain.NewPreAcceptanceFinancialControlPolicy(fixture.controlVersion, pcdomain.AllControlsPass, items)
	if err != nil {
		t.Fatalf("构造策略正文：%v", err)
	}
	if err := fixture.persistence.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if outcome, err := fixture.persistence.publications.SaveVersion(txCtx, fixture.controlVersion); err != nil {
			return err
		} else if outcome != pcports.PublicationSaved {
			return fmt.Errorf("save version outcome = %s", outcome)
		}
		if outcome, err := fixture.persistence.publications.SavePreAcceptanceFinancialControlPolicy(txCtx, content); err != nil {
			return err
		} else if outcome != pcports.PreAcceptanceFinancialControlPolicySaved {
			return fmt.Errorf("save content outcome = %s", outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("登记策略正文：%v", err)
	}
}

func controlItemFor(
	t *testing.T,
	kind pcdomain.PreAcceptanceControlKind,
	scope string,
	order int,
) pcdomain.PreAcceptanceControlItem {
	t.Helper()
	item, err := pcdomain.NewPreAcceptanceControlItem(
		kind,
		pcValue(t, pcdomain.NewChargeScopeReference, scope),
		order,
		pcdomain.RejectOnControlFailure,
		pcValue(t, pcdomain.NewControlResponsibilityReference, "customer"),
	)
	if err != nil {
		t.Fatalf("构造控制项：%v", err)
	}
	return item
}

func newPolicyFixture(t *testing.T, method pcdomain.SettlementMethod) *policyFixture {
	t.Helper()
	return newPolicyFixtureWithClosure(t, fixedClosure(t, method, true))
}

func newPolicyFixtureWithClosure(t *testing.T, closure pcdomain.CommercialClosure) *policyFixture {
	t.Helper()

	persistence := newPersistence(t)
	var outcome pcports.ResolutionSaveOutcome
	if err := persistence.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = persistence.resolutions.Save(txCtx, closure)
		return err
	}); err != nil {
		t.Fatalf("固定解析：%v", err)
	}
	if outcome != pcports.ResolutionSaved {
		t.Fatalf("save outcome = %s, want SAVED", outcome)
	}

	built, err := adapter.NewPreAcceptanceControlPolicy(
		persistence.resolutions, persistence.declarations, persistence.contents)
	if err != nil {
		t.Fatalf("构造 SA→PC 控制策略适配器：%v", err)
	}
	return &policyFixture{
		adapter:        built,
		pool:           persistence.pool,
		persistence:    persistence,
		closure:        closure,
		controlVersion: controlPolicyVersion(t),
		contractID:     "contract-1",
		contractLabe:   "v1",
	}
}

// pcPersistence 是同一个池上的 PC 读写口。池一并交出去是必要的：声明行直插库（声明写口属发布链，
// 本票不经它），直插与适配器读到的必须是同一份数据；正文则经发布写口落，因为它外键指向版本壳。
type pcPersistence struct {
	declarations *pcpostgres.PreAcceptanceControlDeclarations
	resolutions  *pcpostgres.CommercialResolutions
	contents     *pcpostgres.PreAcceptanceFinancialControlPolicyContents
	publications *pcpostgres.CommercialPublications
	transactor   bentoapp.Transactor
	pool         *pgxpool.Pool
}

func newPersistence(t *testing.T) pcPersistence {
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
	contents, err := pcpostgres.NewPreAcceptanceFinancialControlPolicyContents(db)
	if err != nil {
		t.Fatalf("构造正文读口：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布写口：%v", err)
	}
	return pcPersistence{
		declarations: declarations,
		resolutions:  resolutions,
		contents:     contents,
		publications: publications,
		transactor:   db.Transactor(),
		pool:         pool,
	}
}

// controlPolicyVersion 是夹具里那一版接受前财务控制策略的壳，闭包与正文都用它。
func controlPolicyVersion(t *testing.T) pcdomain.CommercialVersion {
	t.Helper()
	return effectiveVersion(t, pcdomain.PreAcceptanceFinancialControlPolicyObject, "control-1", "v1", "digest-k1")
}

// fixedClosure 造一份含客户合同、接单规则包、结算政策与（可选）接受前财务控制策略的唯一已解析
// 闭包——正是接受控制目的下 PS 会固定的那个形状。withControlPolicy=false 模拟租户登记的解析键
// 没要求控制策略那一项。
func fixedClosure(t *testing.T, method pcdomain.SettlementMethod, withControlPolicy bool) pcdomain.CommercialClosure {
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

	required := []pcdomain.CommercialObjectKind{
		pcdomain.CustomerContractObject,
		pcdomain.AcceptanceRulePackageObject,
		pcdomain.SettlementPolicyObject,
	}
	if withControlPolicy {
		register(t, registry, controlPolicyVersion(t))
		required = append(required, pcdomain.PreAcceptanceFinancialControlPolicyObject)
	}

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
		RequiredBases: required,
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
