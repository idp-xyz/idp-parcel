package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证客户合同版本的销售价格约定（0049；ADR-0380 决定一，票
// commercial-four-module-requirements/16）：来源按声明的顺序保存与读回，不计销售价的约定带依据读回，没有这一层的
// 合同那一格为 nil；同内容重放答`已登记`，换顺序或换格答`内容冲突`且一行不改。夹具全部合成。

func sellPriceOf(t *testing.T, contract domain.CommercialVersion, basis string, scopes ...string) domain.ContractSellPriceArrangement {
	t.Helper()
	sources := make([]domain.PricingScopeReference, 0, len(scopes))
	for _, scope := range scopes {
		sources = append(sources, pcValue(t, domain.NewPricingScopeReference, scope))
	}
	var noSellPrice domain.NoSellPriceBasis
	if basis != "" {
		noSellPrice = pcValue(t, domain.NewNoSellPriceBasis, basis)
	}
	arrangement, err := domain.DeclareContractSellPrice(contract, sources, noSellPrice)
	if err != nil {
		t.Fatalf("销售价格约定：%v", err)
	}
	return arrangement
}

// Covers: 票面完成判据「发布后读回，列表顺序与录入相同」与「旧规范化版本的合同读回不变」在持久化面——来源按合同
// 声明的位置读回，不按范围名排（夹具的顺序既不是升序也不是降序）；不计销售价的约定带依据、没有来源；没有这一层的
// 合同那一格为 nil，不拿空列表顶；约定行与来源行各聚各的，互不扇出；按身份读回与上列同一份；他租户不可见。
func TestCustomerContractCatalogueReadsSellPriceSourcesBackInTheDeclaredOrder(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	priced := effectiveContract(t, "contract-1", "v1", "digest-priced")
	notPriced := effectiveContract(t, "contract-2", "v1", "digest-not-priced")
	silent := effectiveContract(t, "contract-3", "v1", "digest-silent")
	for _, contract := range []domain.CommercialVersion{priced, notPriced, silent} {
		mustSaveVersion(t, transactor, ctx, repository, contract)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveCustomerContractContent(txCtx, contractContentOf(t, contract, "control-policy-1"))
		}, ports.DeclarationSaved)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveContractSellPriceArrangement(txCtx, sellPriceOf(t, priced, "", "SYN-SCOPE-M", "SYN-SCOPE-Z", "SYN-SCOPE-A"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveContractSellPriceArrangement(txCtx, sellPriceOf(t, notPriced, "SYN-NO-SELL-PRICE-01"))
	}, ports.DeclarationSaved)

	rows, err := rowsOf(catalogue.ListCustomerContracts(ctx, pcTenant(t, "tenant-1"), 10, firstPage(t, ports.CustomerContractPages)))
	if err != nil {
		t.Fatalf("上列客户合同：%v", err)
	}
	// 三版同刻发布，按对象标识降序收尾（ADR-0144）。
	if len(rows) != 3 || rows[0].ObjectID != "contract-3" || rows[1].ObjectID != "contract-2" || rows[2].ObjectID != "contract-1" {
		t.Fatalf("上列 = %d 行，次序或数量不对", len(rows))
	}

	if rows[2].SellPrice == nil {
		t.Fatal("声明过销售价格来源的合同那一格为 nil")
	}
	if got := strings.Join(rows[2].SellPrice.Sources, ","); got != "SYN-SCOPE-M,SYN-SCOPE-Z,SYN-SCOPE-A" {
		t.Fatalf("来源 = %s，want 合同声明的顺序 SYN-SCOPE-M,SYN-SCOPE-Z,SYN-SCOPE-A", got)
	}
	if rows[2].SellPrice.NoSellPriceBasis != "" || rows[2].SellPrice.DeclaredAt.IsZero() {
		t.Fatalf("有来源的约定：basis=%q declaredAt=%v", rows[2].SellPrice.NoSellPriceBasis, rows[2].SellPrice.DeclaredAt)
	}
	if len(rows[2].Bindings) != 2 {
		t.Fatalf("来源行把约定行数重了：bindings=%d", len(rows[2].Bindings))
	}

	if rows[1].SellPrice == nil || rows[1].SellPrice.NoSellPriceBasis != "SYN-NO-SELL-PRICE-01" || len(rows[1].SellPrice.Sources) != 0 {
		t.Fatalf("不计销售价的约定 = %#v", rows[1].SellPrice)
	}
	if rows[0].SellPrice != nil {
		t.Fatalf("没有销售价格约定的合同那一格 = %#v，want nil", rows[0].SellPrice)
	}

	found, present, err := catalogue.FindCustomerContract(ctx, pcTenant(t, "tenant-1"), priced.ObjectID(), priced.Version())
	if err != nil || !present {
		t.Fatalf("按身份读回：present=%v err=%v", present, err)
	}
	if found.SellPrice == nil || strings.Join(found.SellPrice.Sources, ",") != "SYN-SCOPE-M,SYN-SCOPE-Z,SYN-SCOPE-A" {
		t.Fatalf("按身份读回的约定 = %#v", found.SellPrice)
	}

	foreign, err := rowsOf(catalogue.ListCustomerContracts(ctx, pcTenant(t, "tenant-2"), 10, firstPage(t, ports.CustomerContractPages)))
	if err != nil {
		t.Fatalf("他租户上列：%v", err)
	}
	if len(foreign) != 0 {
		t.Fatalf("他租户看见了 %d 行", len(foreign))
	}
}

// Covers: ADR-0126 Decision 三在载体册的落法——带销售价格约定的客户合同载体（PCC-3 的正文快照）经真库往返：读回
// 的正文里来源顺序与录入相同，规范化版本与摘要原样。载体册不认版本名单，只要求摘要自带它的规范化版本。
func TestAContractDraftWithSellPriceSourcesRoundTripsThroughTheRehydrationGate(t *testing.T) {
	drafts, _, transactor, _ := newDraftRegistry(t)
	ctx := t.Context()
	shell := draftShellIn(t, "tenant-1", "scope-1", "contract-1", "v1")
	shell.Kind = domain.CustomerContractObject
	shell.References = nil
	content := domain.PublicationContent{Kind: domain.CustomerContractObject, CustomerContract: &domain.CustomerContractBody{
		CustomerAccount: pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		RulePackage:     pcValue(t, domain.NewCommercialObjectID, "rules-1"),
		SellPrice: &domain.SellPriceArrangementBody{Sources: []domain.PricingScopeReference{
			pcValue(t, domain.NewPricingScopeReference, "SYN-SCOPE-M"),
			pcValue(t, domain.NewPricingScopeReference, "SYN-SCOPE-Z"),
			pcValue(t, domain.NewPricingScopeReference, "SYN-SCOPE-A"),
		}},
	}}
	submitted, err := domain.SubmitPublicationDraft(shell, content,
		pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"), draftSubmittedAt)
	if err != nil {
		t.Fatalf("录入载体：%v", err)
	}
	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, submitted); outcome != ports.PublicationDraftSaved {
		t.Fatalf("submit outcome = %s, want SAVED", outcome)
	}

	stored, found := loadDraft(t, ctx, drafts, "tenant-1", shell)
	if !found {
		t.Fatal("录入的载体读不回来")
	}
	if stored.Canonical().Canonicalization() != "PCC-3" || stored.Canonical().Digest() != submitted.Canonical().Digest() {
		t.Fatalf("读回的载体 = %s，want 录入的那份 %s", stored.Canonical().Digest(), submitted.Canonical().Digest())
	}
	arrangement := stored.Content().CustomerContract.SellPrice
	if arrangement == nil || len(arrangement.Sources) != 3 {
		t.Fatalf("读回的销售价格约定 = %#v", arrangement)
	}
	got := []string{arrangement.Sources[0].String(), arrangement.Sources[1].String(), arrangement.Sources[2].String()}
	if strings.Join(got, ",") != "SYN-SCOPE-M,SYN-SCOPE-Z,SYN-SCOPE-A" {
		t.Fatalf("读回的来源 = %v，want 录入的顺序", got)
	}
}

// Covers: 声明写口的共同纪律——销售价格约定的写入不在事务里即拒，不自己开一笔：约定与版本壳、正文同一事务落库，
// 半份发布不该存在。
func TestContractSellPriceArrangementWritesRefuseToRunOutsideATransaction(t *testing.T) {
	_, repository, _ := newCatalogue(t)
	arrangement := sellPriceOf(t, effectiveContract(t, "contract-1", "v1", "digest-priced"), "", "SYN-SCOPE-STANDARD")
	if _, err := repository.SaveContractSellPriceArrangement(t.Context(), arrangement); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记销售价格约定应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 声明写口的共同纪律（绝不覆盖）在本册的落法——同一份约定重放答`已登记`；来源换顺序、增减一项、换成不计
// 销售价，或不计销售价的约定换依据、换成来源，都答`内容冲突`，册上仍是先登的那一份。顺序是正文：同一组计价范围
// 换了先后就是另一份约定，不是重放。
func TestAContractSellPriceArrangementIsNeverOverwritten(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	priced := effectiveContract(t, "contract-1", "v1", "digest-priced")
	notPriced := effectiveContract(t, "contract-2", "v1", "digest-not-priced")
	mustSaveVersion(t, transactor, ctx, repository, priced)
	mustSaveVersion(t, transactor, ctx, repository, notPriced)

	save := func(arrangement domain.ContractSellPriceArrangement, want ports.DeclarationSaveOutcome) {
		t.Helper()
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveContractSellPriceArrangement(txCtx, arrangement)
		}, want)
	}

	save(sellPriceOf(t, priced, "", "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD"), ports.DeclarationSaved)
	save(sellPriceOf(t, priced, "", "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD"), ports.DeclarationAlreadyRegistered)
	save(sellPriceOf(t, priced, "", "SYN-SCOPE-STANDARD", "SYN-SCOPE-EXCLUSIVE"), ports.DeclarationContentConflict)
	save(sellPriceOf(t, priced, "", "SYN-SCOPE-EXCLUSIVE"), ports.DeclarationContentConflict)
	save(sellPriceOf(t, priced, "", "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD", "SYN-SCOPE-OTHER"), ports.DeclarationContentConflict)
	save(sellPriceOf(t, priced, "SYN-NO-SELL-PRICE-01"), ports.DeclarationContentConflict)

	save(sellPriceOf(t, notPriced, "SYN-NO-SELL-PRICE-01"), ports.DeclarationSaved)
	save(sellPriceOf(t, notPriced, "SYN-NO-SELL-PRICE-01"), ports.DeclarationAlreadyRegistered)
	save(sellPriceOf(t, notPriced, "SYN-NO-SELL-PRICE-02"), ports.DeclarationContentConflict)
	save(sellPriceOf(t, notPriced, "", "SYN-SCOPE-STANDARD"), ports.DeclarationContentConflict)

	first, present, err := catalogue.FindCustomerContract(ctx, pcTenant(t, "tenant-1"), priced.ObjectID(), priced.Version())
	if err != nil || !present || first.SellPrice == nil {
		t.Fatalf("读回先登的那一份：present=%v err=%v row=%#v", present, err, first.SellPrice)
	}
	if got := strings.Join(first.SellPrice.Sources, ","); got != "SYN-SCOPE-EXCLUSIVE,SYN-SCOPE-STANDARD" || first.SellPrice.NoSellPriceBasis != "" {
		t.Fatalf("冲突的写入改了册上的约定：sources=%s basis=%q", got, first.SellPrice.NoSellPriceBasis)
	}
	second, present, err := catalogue.FindCustomerContract(ctx, pcTenant(t, "tenant-1"), notPriced.ObjectID(), notPriced.Version())
	if err != nil || !present || second.SellPrice == nil {
		t.Fatalf("读回不计销售价的那一份：present=%v err=%v row=%#v", present, err, second.SellPrice)
	}
	if second.SellPrice.NoSellPriceBasis != "SYN-NO-SELL-PRICE-01" || len(second.SellPrice.Sources) != 0 {
		t.Fatalf("冲突的写入改了册上的约定：%#v", second.SellPrice)
	}
}
