package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证供应商商业协议册（票 party-commercial-context-gaps/03）：正文
// 往返、缺正文是合法缺席、同内容重放、异内容冲突、租户是身份不是过滤器、目录上列壳并带正文。

func newSupplierAgreementContents(t *testing.T) (*adapter.CommercialPublications, *adapter.SupplierAgreementContents, *adapter.OperationsCatalogue, bentoapp.Transactor) {
	t.Helper()
	repository, transactor, db := newDeclarationFixture(t)
	contents, err := adapter.NewSupplierAgreementContents(db)
	if err != nil {
		t.Fatalf("构造供应商协议读口：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	return repository, contents, catalogue, transactor
}

func supplierAgreementOn(
	t *testing.T,
	version domain.CommercialVersion,
	supplier, purchasePlan string,
) domain.SupplierAgreement {
	t.Helper()
	agreement, err := domain.NewSupplierAgreement(
		version,
		pcValue(t, domain.NewPartyID, supplier),
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewCommercialScopeReference, "scope-procurement"),
		pcValue(t, domain.NewPricingPlanReference, purchasePlan),
		version.Effective(),
	)
	if err != nil {
		t.Fatalf("new supplier agreement: %v", err)
	}
	return agreement
}

func mustSaveSupplierAgreement(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	agreement domain.SupplierAgreement,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveSupplierAgreement(txCtx, agreement)
		if err != nil {
			return err
		}
		if outcome != ports.SupplierAgreementSaved {
			t.Fatalf("save outcome = %q, want SAVED", outcome)
		}
		return nil
	})
}

// Covers: CONTEXT「供应商商业协议在批准生效后，才能用于新的采购决定和供应商预期成本计算」
// ——正文往返后采购定价方案与供应商原样在场，方向恒为 BUY，生效区间内支持新的采购决定。
func TestSupplierAgreementRoundTripsThroughItsContentView(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-1", "v1", "digest-a1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveSupplierAgreement(t, transactor, ctx, repository,
		supplierAgreementOn(t, version, "supplier-1", "plan-buy-1"))

	agreement, found, err := contents.LoadSupplierAgreement(ctx, pcTenant(t, "tenant-1"), version)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if agreement.Supplier().String() != "supplier-1" || agreement.PurchasePricingPlan().String() != "plan-buy-1" {
		t.Fatalf("正文被改动了：%#v", agreement)
	}
	if agreement.Scope().String() != "scope-procurement" || agreement.LegalEntity().String() != "legal-1" {
		t.Fatalf("范围或法人被改动了：%#v", agreement)
	}
	if agreement.Direction() != domain.BuyDirection {
		t.Fatalf("方向 = %q", agreement.Direction())
	}
	if !agreement.SupportsProcurementAt(effectiveAtRow.Add(24 * time.Hour)) {
		t.Fatal("生效区间内的协议不支持新的采购决定")
	}
	if !agreement.Version().SameVersionAs(version) {
		t.Fatal("正文挂回了另一个版本")
	}
}

// Covers: 缺正文是合法缺席（found=false）——壳可先入册，正文随发布登记。
func TestASupplierAgreementVersionWithoutContentIsNotFound(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-bare", "v1", "digest-ab")
	mustSaveVersion(t, transactor, ctx, repository, version)

	if _, found, err := contents.LoadSupplierAgreement(ctx, pcTenant(t, "tenant-1"), version); err != nil || found {
		t.Fatalf("没正文：found=%v err=%v，应是 found=false 且无 error", found, err)
	}
}

// Covers: ADR-0031——同内容重放答`已登记`，换采购定价方案答`内容冲突`，原行不动。
func TestSavingASupplierAgreementTwiceIsAReplayAndAChangedPlanConflicts(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-1", "v1", "digest-a1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	original := supplierAgreementOn(t, version, "supplier-1", "plan-buy-1")
	mustSaveSupplierAgreement(t, transactor, ctx, repository, original)

	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveSupplierAgreement(txCtx, original)
		if err != nil {
			return err
		}
		if outcome != ports.SupplierAgreementAlreadyRegistered {
			t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
		}
		return nil
	})

	changed := supplierAgreementOn(t, version, "supplier-1", "plan-buy-2")
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveSupplierAgreement(txCtx, changed)
		if err != nil {
			return err
		}
		if outcome != ports.SupplierAgreementContentConflict {
			t.Fatalf("conflict outcome = %q, want CONTENT_CONFLICT", outcome)
		}
		return nil
	})

	agreement, found, err := contents.LoadSupplierAgreement(ctx, pcTenant(t, "tenant-1"), version)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if agreement.PurchasePricingPlan().String() != "plan-buy-1" {
		t.Fatalf("冲突写入改动了原行：%q", agreement.PurchasePricingPlan())
	}
}

// Covers: ADR-0003——拿他租户身份读本租户版本是 error 且不交内容；他租户的正文不进本租户读口。
func TestSupplierAgreementContentIsBoundToItsTenant(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	mine := policyVersionInTenant(t, "tenant-1", domain.SupplierAgreementObject, "agreement-1", "v1", "digest-mine")
	theirs := policyVersionInTenant(t, "tenant-2", domain.SupplierAgreementObject, "agreement-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveSupplierAgreement(t, transactor, ctx, repository,
		supplierAgreementOn(t, theirs, "supplier-2", "plan-buy-9"))

	if _, found, err := contents.LoadSupplierAgreement(ctx, pcTenant(t, "tenant-2"), mine); err == nil || found {
		t.Fatalf("拿他租户身份读本租户版本：found=%v err=%v", found, err)
	}
	if _, found, err := contents.LoadSupplierAgreement(ctx, pcTenant(t, "tenant-1"), mine); err != nil || found {
		t.Fatalf("他租户的正文进了本租户的读口：found=%v err=%v", found, err)
	}
}

// Covers: 目录上列的是版本壳，正文左连接——壳在正文缺是合法状态，HasContent 分它们
// （判据同 CustomerContractCatalogueRow.HasContent）。
func TestSupplierAgreementCatalogueListsShellsAndTheirContent(t *testing.T) {
	repository, _, catalogue, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	withContent := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-1", "v1", "digest-a1")
	bare := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-2", "v1", "digest-a2")
	mustSaveVersion(t, transactor, ctx, repository, withContent)
	mustSaveVersion(t, transactor, ctx, repository, bare)
	mustSaveSupplierAgreement(t, transactor, ctx, repository,
		supplierAgreementOn(t, withContent, "supplier-1", "plan-buy-1"))

	rows, err := catalogue.ListSupplierAgreements(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列供应商协议：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("上列 %d 行，want 2（壳在正文缺的版本照样在列）", len(rows))
	}
	byObject := map[string]ports.SupplierAgreementCatalogueRow{}
	for _, row := range rows {
		byObject[row.ObjectID] = row
	}
	full := byObject["agreement-1"]
	if !full.HasContent || full.Supplier != "supplier-1" || full.PurchasePlan != "plan-buy-1" ||
		full.LegalEntity != "legal-1" || full.AgreementScope != "scope-procurement" {
		t.Fatalf("带正文的行 = %#v", full)
	}
	if !full.AgreementEffectiveStartsAt.Equal(withContent.Effective().StartsAt()) || !full.HasAgreementEffectiveEnd {
		t.Fatalf("正文区间没有原样上列：%#v", full)
	}
	shell := byObject["agreement-2"]
	if shell.HasContent || shell.Supplier != "" || shell.PurchasePlan != "" {
		t.Fatalf("只有壳的行带了正文：%#v", shell)
	}
	if shell.Scope != "scope-1" || shell.Status != "EFFECTIVE" {
		t.Fatalf("壳字段没有照列：%#v", shell)
	}
}
