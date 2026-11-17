package postgres_test

import (
	"context"
	"strings"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证按合同版本点读销售价格约定（ADR-0380 决定二，票
// commercial-four-module-requirements/17）：点的是问到的那一版，同对象的新版不改旧版的口；没有父行是
// 「没有约定」，不是空列表。

// Covers: 点读口的四格——问哪一版答哪一版（同一合同对象的两版各答各的列表，顺序同声明）；没有父行答没有且不
// 报错；不计销售价带依据、没有来源；显式租户不是合同版本的租户即拒。
func TestLoadContractSellPriceArrangementReadsTheVersionItIsAsked(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	reader, err := adapter.NewContractSellPriceArrangements(db)
	if err != nil {
		t.Fatalf("构造点读口：%v", err)
	}
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	first := effectiveContract(t, "contract-1", "v1", "digest-v1")
	second := effectiveContract(t, "contract-1", "v2", "digest-v2")
	silent := effectiveContract(t, "contract-2", "v1", "digest-silent")
	notPriced := effectiveContract(t, "contract-3", "v1", "digest-not-priced")
	for _, contract := range []domain.CommercialVersion{first, second, silent, notPriced} {
		mustSaveVersion(t, transactor, ctx, repository, contract)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveContractSellPriceArrangement(txCtx, sellPriceOf(t, first, "", "SYN-SCOPE-M", "SYN-SCOPE-Z", "SYN-SCOPE-A"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveContractSellPriceArrangement(txCtx, sellPriceOf(t, second, "", "SYN-SCOPE-ONLY-NEW"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveContractSellPriceArrangement(txCtx, sellPriceOf(t, notPriced, "SYN-NO-SELL-PRICE-01"))
	}, ports.DeclarationSaved)

	loaded, found, err := reader.LoadContractSellPriceArrangement(ctx, tenant, first)
	if err != nil || !found {
		t.Fatalf("点读 v1：found=%v err=%v", found, err)
	}
	sources, ordered := loaded.Sources()
	if !ordered || joinScopes(sources) != "SYN-SCOPE-M,SYN-SCOPE-Z,SYN-SCOPE-A" {
		t.Fatalf("v1 来源 = %s ordered=%v，want 旧列表", joinScopes(sources), ordered)
	}

	loaded, found, err = reader.LoadContractSellPriceArrangement(ctx, tenant, second)
	if err != nil || !found {
		t.Fatalf("点读 v2：found=%v err=%v", found, err)
	}
	sources, ordered = loaded.Sources()
	if !ordered || joinScopes(sources) != "SYN-SCOPE-ONLY-NEW" {
		t.Fatalf("v2 来源 = %s，want 新版自己的列表", joinScopes(sources))
	}

	if _, found, err = reader.LoadContractSellPriceArrangement(ctx, tenant, silent); err != nil || found {
		t.Fatalf("没有约定：found=%v err=%v，want 没有且不报错", found, err)
	}

	loaded, found, err = reader.LoadContractSellPriceArrangement(ctx, tenant, notPriced)
	if err != nil || !found {
		t.Fatalf("不计销售价：found=%v err=%v", found, err)
	}
	basis, notSell := loaded.NoSellPriceBasis()
	if !notSell || basis.String() != "SYN-NO-SELL-PRICE-01" {
		t.Fatalf("依据 = %q notSell=%v", basis.String(), notSell)
	}
	if _, ordered = loaded.Sources(); ordered {
		t.Fatal("不计销售价被读成来源列表")
	}

	if _, found, err = reader.LoadContractSellPriceArrangement(ctx, pcTenant(t, "tenant-2"), first); err == nil || found {
		t.Fatalf("他租户点读：found=%v err=%v", found, err)
	}
}

func joinScopes(sources []domain.PricingScopeReference) string {
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		parts = append(parts, source.String())
	}
	return strings.Join(parts, ",")
}
