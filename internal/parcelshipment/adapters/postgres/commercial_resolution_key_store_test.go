package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证解析键登记行的持久化面在必需依据名集上的一格（票 ps-port-remainder/09；
// ADR-0136 决定三「客户合同版本与客户服务规则版本都必须在接受时闭包的必需依据里」）：含 CUSTOMER_SERVICE_RULE
// 的一行登得进去——迁移 0022 重加的 `..._bases_closed` 白名单收它——读回逐字同。名集的翻译半边（译回领域类别）
// 由 adapters/partycommercial 证，这里只证行本身能落库、能原样读回。

// Covers: 票面完成判据 3 前半——必需依据含 CUSTOMER_SERVICE_RULE 与 CUSTOMER_CONTRACT 的一行经持久化面登入、
// 按（租户，客户）读回，required_bases 逐字、按登记顺序相同；其余各维照登记值。
func TestAResolutionKeyRowRequiringACustomerServiceRuleRoundTrips(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewCommercialResolutionKeyStore(db)
	if err != nil {
		t.Fatalf("构造解析键持久化面：%v", err)
	}
	row := pspartycommercial.ResolutionKeyRow{
		TenantID:          "tenant-1",
		CustomerAccountID: "customer-1",
		Scope:             "scope-1",
		LegalEntity:       "legal-1",
		AnchorPolicy:      "anchor-policy/v1",
		AnchorAt:          time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		RequiredBases:     []string{"CUSTOMER_CONTRACT", "CUSTOMER_SERVICE_RULE"},
	}

	var outcome pspartycommercial.ResolutionKeySaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var registerErr error
		outcome, registerErr = store.RegisterResolutionKey(txCtx, row)
		return registerErr
	}); err != nil {
		t.Fatalf("登记含客户服务规则的解析键行：%v", err)
	}
	if outcome != pspartycommercial.ResolutionKeySaved {
		t.Fatalf("register outcome = %s, want SAVED", outcome)
	}

	found, present, err := store.FindResolutionKey(t.Context(), "tenant-1", "customer-1")
	if err != nil || !present {
		t.Fatalf("读回解析键行：present=%v err=%v", present, err)
	}
	if !slices.Equal(found.RequiredBases, row.RequiredBases) {
		t.Fatalf("required_bases 读回 = %v, want %v 逐字同", found.RequiredBases, row.RequiredBases)
	}
	if found.Scope != row.Scope || found.LegalEntity != row.LegalEntity || found.AnchorPolicy != row.AnchorPolicy ||
		!found.AnchorAt.Equal(row.AnchorAt) {
		t.Fatalf("其余各维读回变形：%+v", found)
	}
	if found.SettlementCounterparty != "" || found.SettlementChargeScope != "" || found.SettlementCurrency != "" ||
		found.CreditLevel != "" || found.CreditChargeType != "" {
		t.Fatalf("没登结算 / 信用维却读回了值：%+v", found)
	}
}
