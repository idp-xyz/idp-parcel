package partycommercial_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 PS→PC 处置读口（ADR-0132 决定三，票 sa-preacceptance-policy-view/04 第 2 步）。
// 走真库而不是替身，理由与 SA 那只控制策略适配器的用例一字不差：这一段翻译的要害在两侧持久化面之间——
// 闭包按标识落库、正文按版本落库，替身能让它们「对上」，而对不上正是要防的那件事。夹具刻意不复用 SA 侧
// 那套 helper（另一个包，且跨包共享夹具会让本票用例随那边演进而变形）。

var (
	dispositionEffectiveAt = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	dispositionApprovedAt  = time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC)
	dispositionPublishedAt = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
)

// Covers: 一正——策略正文为本费用范围登记的行逐种类译成采用引用（处置 × 责任引用成对），挂在别的费用范围上的
// 行不进答复；译出的引用采用到 SA 交回的受限项上之后，`FinancialControlCheckFor` 走到`无法判定`+ 续办路径
// 「授权处置」——正文登 AUTHORIZED_DISPOSITION 的受限控制不再自动拒绝。
func TestAnAuthorizedDispositionRowIsTranslatedIntoAnAdoptedReference(t *testing.T) {
	fixture := newDispositionFixture(t)
	fixture.registerContent(t,
		dispositionItem(t, pcdomain.PrepaidFreezeControl, "charge-express", 1, pcdomain.RejectOnControlFailure, "operator"),
		dispositionItem(t, pcdomain.CreditCheckControl, "charge-express", 2, pcdomain.AuthorizedDispositionOnControlFailure, "customer"),
		dispositionItem(t, pcdomain.CreditCheckControl, "charge-economy", 3, pcdomain.RejectOnControlFailure, "elsewhere"),
	)

	dispositions, found, err := fixture.load(t)
	if err != nil || !found {
		t.Fatalf("读回处置：found=%v err=%v", found, err)
	}
	if len(dispositions) != 2 {
		t.Fatalf("dispositions = %v, want the two kinds on charge-express only", dispositions)
	}
	credit, present := dispositions[psdomain.CreditCheckControlItem]
	if !present || credit.FailureDisposition() != psdomain.AuthorizedDispositionOnControlFailure ||
		credit.Responsibility().String() != "customer" {
		t.Fatalf("CREDIT_CHECK disposition = %+v (present=%v), want AUTHORIZED_DISPOSITION × customer", credit, present)
	}
	freeze, present := dispositions[psdomain.PrepaidFreezeControlItem]
	if !present || freeze.FailureDisposition() != psdomain.RejectOnControlFailure ||
		freeze.Responsibility().String() != "operator" {
		t.Fatalf("PREPAID_FREEZE disposition = %+v (present=%v), want REJECT × operator", freeze, present)
	}

	adopted, err := creditRestrictedResult(t).AdoptControlDispositions(dispositions)
	if err != nil {
		t.Fatalf("adopt control dispositions: %v", err)
	}
	check, err := psdomain.FinancialControlCheckFor(adopted)
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}
	if check.Outcome() != psdomain.CheckUndetermined || check.ResumePath() != psdomain.ResumeByAuthorizedDisposition {
		t.Fatalf("check = %s/%s, want UNDETERMINED waiting on AUTHORIZED_DISPOSITION", check.Outcome(), check.ResumePath())
	}
}

// Covers: 一反——本费用范围下没有受限项那一种类的行：适配器如实交回范围下有的行，领域采用时对不上、整份不成立
// （ErrControlDispositionNotAdopted），**不折成 REJECT 也不折成任一去向**——对不上是换版竞争或坏数据，恢复动作
// 是内部续办重读（ADR-0132 决定三）。
func TestAMissingKindOnTheChargeScopeIsNotFoldedIntoRejection(t *testing.T) {
	fixture := newDispositionFixture(t)
	fixture.registerContent(t,
		dispositionItem(t, pcdomain.PrepaidFreezeControl, "charge-express", 1, pcdomain.RejectOnControlFailure, "operator"),
	)

	dispositions, found, err := fixture.load(t)
	if err != nil || !found {
		t.Fatalf("读回处置：found=%v err=%v", found, err)
	}
	if _, present := dispositions[psdomain.CreditCheckControlItem]; present {
		t.Fatal("正文没为 CREDIT_CHECK 写行，适配器却交出了一份处置——那是替租户选去向")
	}
	if _, err := creditRestrictedResult(t).AdoptControlDispositions(dispositions); !errors.Is(err, psdomain.ErrControlDispositionNotAdopted) {
		t.Fatalf("err = %v, want ErrControlDispositionNotAdopted——受限项对不上处置被折成了某个去向", err)
	}
}

// Covers: found=false 那一格——正文只为别的费用范围写了行、或闭包没采用控制策略：都是租户要补的配置，不是 error，
// 也不是空 map 冒充「读到了」。
func TestNoRowOnTheResolvedChargeScopeIsReportedAsNotFound(t *testing.T) {
	t.Run("rows on another scope only", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		fixture.registerContent(t,
			dispositionItem(t, pcdomain.CreditCheckControl, "charge-economy", 1, pcdomain.AuthorizedDispositionOnControlFailure, "customer"),
		)
		dispositions, found, err := fixture.load(t)
		if err != nil || found || dispositions != nil {
			t.Fatalf("got %v found=%v err=%v, want nothing found without error", dispositions, found, err)
		}
	})

	t.Run("closure without a control policy", func(t *testing.T) {
		fixture := newDispositionFixtureWithClosure(t, dispositionClosure(t, false))
		dispositions, found, err := fixture.load(t)
		if err != nil || found || dispositions != nil {
			t.Fatalf("got %v found=%v err=%v, want nothing found without error", dispositions, found, err)
		}
	})

	t.Run("content not registered", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		dispositions, found, err := fixture.load(t)
		if err != nil || found || dispositions != nil {
			t.Fatalf("got %v found=%v err=%v, want nothing found without error", dispositions, found, err)
		}
	})
}

// Covers: 答不出与「未登记」分开——空回指是译不动，查无此闭包是坏回指，两者都是 error 而不是 found=false：
// 答成未登记会把租户支去补一份其实已经存在的正文。
func TestABadResolutionReferenceIsAnErrorNotAnUnconfiguredAnswer(t *testing.T) {
	fixture := newDispositionFixture(t)

	if _, found, err := fixture.loadWith(t, psdomain.CommercialResolutionID{}); !errors.Is(err, adapter.ErrUntranslatableAnswer) || found {
		t.Fatalf("empty reference: found=%v err=%v, want ErrUntranslatableAnswer", found, err)
	}
	unknown := value(t, psdomain.NewCommercialResolutionID, "no-such-resolution")
	if _, found, err := fixture.loadWith(t, unknown); !errors.Is(err, adapter.ErrUntranslatableAnswer) || found {
		t.Fatalf("unknown reference: found=%v err=%v, want ErrUntranslatableAnswer", found, err)
	}
}

// Covers: 两个只读半边都不许为 nil——缺一半而静默答「未配置」会让装配疏漏与租户没登记长得一样。
func TestTheDispositionAdapterRefusesNilCollaborators(t *testing.T) {
	if _, err := adapter.NewControlDispositionAdapter(nil, nil); err == nil {
		t.Fatal("nil collaborators were accepted")
	}
}

type dispositionFixture struct {
	adapter     *adapter.ControlDispositionAdapter
	persistence dispositionPersistence
	closure     pcdomain.CommercialClosure
}

func (fixture *dispositionFixture) load(t *testing.T) (map[psdomain.ControlItemKind]psdomain.AdoptedControlDisposition, bool, error) {
	t.Helper()
	return fixture.loadWith(t, value(t, psdomain.NewCommercialResolutionID, fixture.closure.ResolutionID().String()))
}

func (fixture *dispositionFixture) loadWith(
	t *testing.T,
	resolution psdomain.CommercialResolutionID,
) (map[psdomain.ControlItemKind]psdomain.AdoptedControlDisposition, bool, error) {
	t.Helper()
	return fixture.adapter.LoadControlDispositions(t.Context(), psports.ControlDispositionQuery{
		Identity:   resolutionKeyIdentity(t, "tenant-1", "customer-1"),
		Resolution: resolution,
	})
}

// registerContent 把接受前财务控制策略版本的壳与正文经 PC 的发布写口落库（正文表外键指向版本壳）。
func (fixture *dispositionFixture) registerContent(t *testing.T, items ...pcdomain.PreAcceptanceControlItem) {
	t.Helper()
	version := dispositionControlVersion(t)
	content, err := pcdomain.NewPreAcceptanceFinancialControlPolicy(version, pcdomain.AllControlsPass, items)
	if err != nil {
		t.Fatalf("构造策略正文：%v", err)
	}
	if err := fixture.persistence.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if outcome, err := fixture.persistence.publications.SaveVersion(txCtx, version); err != nil {
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

func newDispositionFixture(t *testing.T) *dispositionFixture {
	t.Helper()
	return newDispositionFixtureWithClosure(t, dispositionClosure(t, true))
}

func newDispositionFixtureWithClosure(t *testing.T, closure pcdomain.CommercialClosure) *dispositionFixture {
	t.Helper()
	persistence := newDispositionPersistence(t)
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
	built, err := adapter.NewControlDispositionAdapter(persistence.resolutions, persistence.contents)
	if err != nil {
		t.Fatalf("构造 PS→PC 处置读口：%v", err)
	}
	return &dispositionFixture{adapter: built, persistence: persistence, closure: closure}
}

type dispositionPersistence struct {
	resolutions  *pcpostgres.CommercialResolutions
	contents     *pcpostgres.PreAcceptanceFinancialControlPolicyContents
	publications *pcpostgres.CommercialPublications
	transactor   bentoapp.Transactor
}

func newDispositionPersistence(t *testing.T) dispositionPersistence {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
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
	return dispositionPersistence{
		resolutions:  resolutions,
		contents:     contents,
		publications: publications,
		transactor:   db.Transactor(),
	}
}

// creditRestrictedResult 是 SA 交回的「预付冻结成立、信用受限」采用结果——处置读口要对上的正是它的受限项。
func creditRestrictedResult(t *testing.T) psdomain.FinancialControlResult {
	t.Helper()
	freeze, err := psdomain.NewControlItemResult(psdomain.PrepaidFreezeControlItem, 1, psdomain.ControlItemSatisfied, psdomain.ControlBasisReference{})
	if err != nil {
		t.Fatalf("new satisfied item: %v", err)
	}
	credit, err := psdomain.NewControlItemResult(psdomain.CreditCheckControlItem, 2, psdomain.ControlItemRestricted,
		value(t, psdomain.NewControlBasisReference, "CREDIT_CHECK_INSUFFICIENT"))
	if err != nil {
		t.Fatalf("new restricted item: %v", err)
	}
	echoed, err := psdomain.NewEchoedAsOfPolicy(
		psdomain.FinancialControlJudgmentKind,
		value(t, psdomain.NewAsOfSemanticsReference, "ASOF-SEMANTICS-FINANCIAL_CONTROL"),
		value(t, psdomain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new echoed as-of policy: %v", err)
	}
	asOf, err := psdomain.NewJudgmentAsOf(dispositionEffectiveAt.Add(time.Hour), echoed)
	if err != nil {
		t.Fatalf("new judgment as-of: %v", err)
	}
	result, err := psdomain.NewExecutedFinancialControlResult(psdomain.ExecutedFinancialControlSpec{
		ResultID:  value(t, psdomain.NewFinancialControlResultID, "request-1/version-1"),
		Items:     []psdomain.ControlItemResult{freeze, credit},
		JointPass: psdomain.AllControlsPass,
		AsOf:      asOf,
	})
	if err != nil {
		t.Fatalf("new executed financial control result: %v", err)
	}
	return result
}

func dispositionItem(
	t *testing.T,
	kind pcdomain.PreAcceptanceControlKind,
	scope string,
	order int,
	disposition pcdomain.ControlFailureDisposition,
	responsibility string,
) pcdomain.PreAcceptanceControlItem {
	t.Helper()
	item, err := pcdomain.NewPreAcceptanceControlItem(
		kind,
		value(t, pcdomain.NewChargeScopeReference, scope),
		order,
		disposition,
		value(t, pcdomain.NewControlResponsibilityReference, responsibility),
	)
	if err != nil {
		t.Fatalf("构造控制项：%v", err)
	}
	return item
}

// dispositionControlVersion 是夹具里那一版接受前财务控制策略的壳，闭包与正文都用它。
func dispositionControlVersion(t *testing.T) pcdomain.CommercialVersion {
	t.Helper()
	return dispositionVersion(t, pcdomain.PreAcceptanceFinancialControlPolicyObject, "control-1", "v1", "digest-k1")
}

// dispositionClosure 造一份含客户合同、接单规则包、结算政策与（可选）接受前财务控制策略的唯一已解析闭包——
// 接受控制目的下 PS 会固定的那个形状；结算政策的适用范围指到 charge-express，那就是本次委托的费用范围。
func dispositionClosure(t *testing.T, withControlPolicy bool) pcdomain.CommercialClosure {
	t.Helper()

	registry := pcdomain.NewCommercialRegistry()
	dispositionRegister(t, registry, dispositionVersion(t, pcdomain.CustomerContractObject, "contract-1", "v1", "digest-c1"))
	dispositionRegister(t, registry, dispositionVersion(t, pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "digest-r1"))

	policyVersion := dispositionVersion(t, pcdomain.SettlementPolicyObject, "policy-1", "v1", "digest-p1")
	dispositionRegister(t, registry, policyVersion)
	interval, err := pcdomain.NewEffectiveInterval(dispositionEffectiveAt, dispositionEffectiveAt.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	applicability, err := pcdomain.NewSettlementApplicability(
		value(t, pcdomain.NewLegalEntityReference, "legal-1"),
		value(t, pcdomain.NewCounterpartyReference, "customer-1"),
		value(t, pcdomain.NewCommercialVersionLabel, "contract-1/v1"),
		value(t, pcdomain.NewChargeScopeReference, "charge-express"),
		value(t, pcdomain.NewCurrencyCode, "CNY"),
		interval,
	)
	if err != nil {
		t.Fatalf("结算适用范围：%v", err)
	}
	policy, err := pcdomain.NewSettlementPolicy(policyVersion, pcdomain.TermsMethod, applicability)
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
		dispositionRegister(t, registry, dispositionControlVersion(t))
		required = append(required, pcdomain.PreAcceptanceFinancialControlPolicyObject)
	}

	anchor, err := pcdomain.NewSelectionAnchor(dispositionEffectiveAt.Add(24*time.Hour),
		value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	closure := pcdomain.ResolveCommercialClosure(registry, pcdomain.ClosureResolutionKey{
		TenantID:             value(t, pcdomain.NewTenantID, "tenant-1"),
		CustomerAccountID:    value(t, pcdomain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: value(t, pcdomain.NewLegalEntityReference, "legal-1"),
		Scope:                value(t, pcdomain.NewCommercialScopeReference, "scope-1"),
		Purpose:              pcdomain.AcceptanceControlPurpose,
		Anchor:               anchor,
		Settlement: pcdomain.SettlementSelector{
			Counterparty: value(t, pcdomain.NewCounterpartyReference, "customer-1"),
			ChargeScope:  value(t, pcdomain.NewChargeScopeReference, "charge-express"),
			Currency:     value(t, pcdomain.NewCurrencyCode, "CNY"),
		},
		RequiredBases: required,
	}, nil)
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	return closure
}

func dispositionRegister(t *testing.T, registry *pcdomain.CommercialRegistry, version pcdomain.CommercialVersion) {
	t.Helper()
	if _, err := registry.Register(version); err != nil {
		t.Fatalf("登记 %s：%v", version.ObjectID(), err)
	}
}

func dispositionVersion(
	t *testing.T,
	kind pcdomain.CommercialObjectKind,
	objectID, label, digest string,
) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(dispositionEffectiveAt, dispositionEffectiveAt.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		value(t, pcdomain.NewApprovalReference, "approval-"+objectID),
		value(t, pcdomain.NewCommercialSourceReference, "source-"+objectID),
		dispositionApprovedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      value(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          kind,
		ObjectID:      value(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       value(t, pcdomain.NewCommercialVersionLabel, label),
		Scope:         value(t, pcdomain.NewCommercialScopeReference, "scope-1"),
		ContentDigest: value(t, pcdomain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   dispositionPublishedAt,
		EffectiveAt:   dispositionEffectiveAt,
	})
	if err != nil {
		t.Fatalf("重建 %s：%v", objectID, err)
	}
	return version
}
