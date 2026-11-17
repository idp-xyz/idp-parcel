package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
)

// 本文件对真实 PostgreSQL 16 证结算按已固定闭包采用的那一版合同点读销售价格约定（ADR-0380 决定二，票
// commercial-four-module-requirements/17）。三种答复各走一条；合同发了新版，按原回指读到的仍是旧列表。

func newSellPriceAdapter(t *testing.T, closure pcdomain.CommercialClosure) (*adapter.AdoptedSellPriceArrangement, pcPersistence) {
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
	built, err := adapter.NewAdoptedSellPriceArrangement(persistence.resolutions, persistence.arrangements)
	if err != nil {
		t.Fatalf("构造销售价格约定适配器：%v", err)
	}
	return built, persistence
}

func saveSellPrice(t *testing.T, persistence pcPersistence, contract pcdomain.CommercialVersion, basis string, scopes ...string) {
	t.Helper()
	sources := make([]pcdomain.PricingScopeReference, 0, len(scopes))
	for _, scope := range scopes {
		sources = append(sources, pcValue(t, pcdomain.NewPricingScopeReference, scope))
	}
	var noSellPrice pcdomain.NoSellPriceBasis
	if basis != "" {
		noSellPrice = pcValue(t, pcdomain.NewNoSellPriceBasis, basis)
	}
	arrangement, err := pcdomain.DeclareContractSellPriceArrangement(contract, sources, noSellPrice)
	if err != nil {
		t.Fatalf("销售价格约定：%v", err)
	}
	var outcome pcports.DeclarationSaveOutcome
	if err := persistence.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = persistence.publications.SaveContractSellPriceArrangement(txCtx, arrangement)
		return err
	}); err != nil {
		t.Fatalf("登记销售价格约定：%v", err)
	}
	if outcome != pcports.DeclarationSaved {
		t.Fatalf("save outcome = %s, want SAVED", outcome)
	}
}

func adoptedContract(t *testing.T, closure pcdomain.CommercialClosure) pcdomain.CommercialVersion {
	t.Helper()
	adopted, ok := closure.AdoptedFor(pcdomain.CustomerContractObject)
	if !ok {
		t.Fatal("闭包没有采用客户合同")
	}
	return adopted.Version()
}

// Covers: 票面完成判据「三种答复各有一条测试」之一——已固定的解析采用的那一版合同声明了有序来源，读回的顺序就是
// 合同声明的顺序（夹具的顺序既不是升序也不是降序），名字照原串。
func TestTheFixedResolutionReadsOrderedSellPriceSources(t *testing.T) {
	closure := fixedClosure(t, pcdomain.TermsMethod, false)
	reader, persistence := newSellPriceAdapter(t, closure)
	saveSellPrice(t, persistence, adoptedContract(t, closure), "", "SYN-SCOPE-M", "SYN-SCOPE-Z", "SYN-SCOPE-A")

	arrangement, err := reader.LoadAdoptedSellPriceArrangement(
		t.Context(), saTenant(t, "tenant-1"), saResolution(t, closure.ResolutionID().String()))
	if err != nil {
		t.Fatalf("读回销售价格约定：%v", err)
	}
	sources, ok := arrangement.Sources()
	if !ok || arrangement.NotDeclared() {
		t.Fatalf("有序来源被读成别的答复：ok=%v notDeclared=%v", ok, arrangement.NotDeclared())
	}
	got := sources[0].String() + "," + sources[1].String() + "," + sources[2].String()
	if got != "SYN-SCOPE-M,SYN-SCOPE-Z,SYN-SCOPE-A" {
		t.Fatalf("来源 = %s，want 合同声明的顺序", got)
	}
}

// Covers: 三种答复之二——合同显式声明不计销售价，读回带依据，不读成空列表也不读成没有约定。
func TestTheFixedResolutionReadsAnExplicitNoSellPrice(t *testing.T) {
	closure := fixedClosure(t, pcdomain.TermsMethod, false)
	reader, persistence := newSellPriceAdapter(t, closure)
	saveSellPrice(t, persistence, adoptedContract(t, closure), "SYN-NO-SELL-PRICE-01")

	arrangement, err := reader.LoadAdoptedSellPriceArrangement(
		t.Context(), saTenant(t, "tenant-1"), saResolution(t, closure.ResolutionID().String()))
	if err != nil {
		t.Fatalf("读回不计销售价：%v", err)
	}
	basis, ok := arrangement.NoSellPriceBasis()
	if !ok || basis.String() != "SYN-NO-SELL-PRICE-01" {
		t.Fatalf("依据 = %q ok=%v", basis.String(), ok)
	}
	if _, ordered := arrangement.Sources(); ordered || arrangement.NotDeclared() {
		t.Fatal("不计销售价被读成来源或没有约定")
	}
}

// Covers: 三种答复之三，与票面「旧快照」一条——采用的那一版合同没有销售价格约定，答第三种且不报错；不拿空列表顶。
func TestAContractVersionWithoutAnArrangementAnswersNotDeclared(t *testing.T) {
	closure := fixedClosure(t, pcdomain.TermsMethod, false)
	reader, _ := newSellPriceAdapter(t, closure)

	arrangement, err := reader.LoadAdoptedSellPriceArrangement(
		t.Context(), saTenant(t, "tenant-1"), saResolution(t, closure.ResolutionID().String()))
	if err != nil {
		t.Fatalf("没有约定：%v", err)
	}
	if !arrangement.NotDeclared() {
		t.Fatal("无父行没有被读成第三种")
	}
	if sources, ordered := arrangement.Sources(); ordered || len(sources) != 0 {
		t.Fatalf("没有约定被读成列表 %#v", sources)
	}
}

// Covers: 票面完成判据「委托接受之后合同发了新一版、列表不同：按原回指读到的仍是旧列表」——点读的是闭包里固定
// 下来的那一版，同一合同对象的新版登了另一份约定也不改口。
func TestALaterContractVersionDoesNotChangeTheFixedResolution(t *testing.T) {
	closure := fixedClosure(t, pcdomain.TermsMethod, false)
	reader, persistence := newSellPriceAdapter(t, closure)
	saveSellPrice(t, persistence, adoptedContract(t, closure), "", "SYN-SCOPE-M", "SYN-SCOPE-Z")
	later := effectiveVersion(t, pcdomain.CustomerContractObject, "contract-1", "v2", "digest-c2")
	saveSellPrice(t, persistence, later, "", "SYN-SCOPE-ONLY-NEW")

	arrangement, err := reader.LoadAdoptedSellPriceArrangement(
		t.Context(), saTenant(t, "tenant-1"), saResolution(t, closure.ResolutionID().String()))
	if err != nil {
		t.Fatalf("新版发布后再读：%v", err)
	}
	sources, ok := arrangement.Sources()
	if !ok {
		t.Fatal("原回指不再是有序来源")
	}
	got := sources[0].String() + "," + sources[1].String()
	if got != "SYN-SCOPE-M,SYN-SCOPE-Z" {
		t.Fatalf("来源 = %s，want 固定时的旧列表", got)
	}
}

// Covers: 票面完成判据「跨租户读不到」——他租户借用回指与查无闭包都是 error，不折成「没有约定」。
func TestAForeignTenantCannotReadTheFixedSellPriceArrangement(t *testing.T) {
	closure := fixedClosure(t, pcdomain.TermsMethod, false)
	reader, persistence := newSellPriceAdapter(t, closure)
	saveSellPrice(t, persistence, adoptedContract(t, closure), "", "SYN-SCOPE-M")

	if _, err := reader.LoadAdoptedSellPriceArrangement(t.Context(), saTenant(t, "tenant-b"),
		saResolution(t, closure.ResolutionID().String())); err == nil {
		t.Fatal("他租户借用回指读到了销售价格约定")
	}
	if _, err := reader.LoadAdoptedSellPriceArrangement(t.Context(), saTenant(t, "tenant-1"),
		saResolution(t, "RES-NOBODY")); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("查无闭包 err = %v, want ErrUntranslatableAnswer", err)
	}
}

// Covers: 闭包没采用客户合同不是三种答复之一——那一版合同无从点读，答 error，不折成「没有约定」让结算去读采用册。
func TestAClosureWithoutACustomerContractIsNotASellPriceAnswer(t *testing.T) {
	registry := pcdomain.NewCommercialRegistry()
	register(t, registry, effectiveVersion(t, pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "digest-r1"))
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
		RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.AcceptanceRulePackageObject},
	}, nil)
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（reason=%q）", closure.Outcome(), closure.Reason())
	}
	reader, _ := newSellPriceAdapter(t, closure)

	if _, err := reader.LoadAdoptedSellPriceArrangement(t.Context(), saTenant(t, "tenant-1"),
		saResolution(t, closure.ResolutionID().String())); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("没采用客户合同 err = %v, want ErrUntranslatableAnswer", err)
	}
}

// Covers: 装配缺一口即拒——缺口而静默答「没有约定」会让一次装配疏漏与旧规范化版本长得一样。
func TestTheSellPriceAdapterRefusesToAssembleWithoutEitherView(t *testing.T) {
	persistence := newPersistence(t)
	if _, err := adapter.NewAdoptedSellPriceArrangement(nil, persistence.arrangements); err == nil {
		t.Fatal("缺闭包读口仍装配成功")
	}
	if _, err := adapter.NewAdoptedSellPriceArrangement(persistence.resolutions, nil); err == nil {
		t.Fatal("缺约定点读口仍装配成功")
	}
}
