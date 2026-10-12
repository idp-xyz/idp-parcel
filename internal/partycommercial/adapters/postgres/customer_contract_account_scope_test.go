package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件对真实 PostgreSQL 证 ADR-0177：合同正文的客户账户随正文落库、经 LoadForScope 进登记册，闭包按键上的客户账户
// 收窄客户合同候选；迁移 0037 之前那种没有账户的旧行，点读口报错、不折成任何一户的合同。

func contractKey(t *testing.T, account string) domain.ResolutionKey {
	t.Helper()
	anchor, err := domain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour), pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	return domain.ResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, account),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		RequiredBasis:        domain.CustomerContractObject,
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
	}
}

func contractOfAccount(t *testing.T, contract domain.CommercialVersion, account string) domain.CustomerContract {
	t.Helper()
	content, err := domain.NewCustomerContract(contract, pcValue(t, domain.NewCustomerAccountID, account),
		pcValue(t, domain.NewCommercialObjectID, "rules-1"), nil)
	if err != nil {
		t.Fatalf("组合同正文：%v", err)
	}
	return content
}

// Covers: ADR-0177 决定二与票 07 完成判据一——同一范围两户各一份合同、各登正文，经真库 LoadForScope 读回的登记册上，
// 各户闭包解出自己那一份；读回的正文报出它属于哪一户；同版本换一户再登是内容冲突，不是重放。
func TestEachAccountResolvesItsOwnContractFromTheStoredScope(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()

	first := effectiveContract(t, "contract-1", "v1", "digest-1")
	second := effectiveContract(t, "contract-2", "v1", "digest-2")
	mustSaveVersion(t, transactor, ctx, repository, first)
	mustSaveVersion(t, transactor, ctx, repository, second)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractOfAccount(t, first, "customer-1"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractOfAccount(t, second, "customer-2"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractOfAccount(t, first, "customer-1"))
	}, ports.DeclarationAlreadyRegistered)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractOfAccount(t, first, "customer-2"))
	}, ports.DeclarationContentConflict)

	registry, err := repository.LoadForScope(ctx, pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("读登记册：%v", err)
	}
	for account, want := range map[string]string{"customer-1": "contract-1", "customer-2": "contract-2"} {
		result := domain.ResolveCommercialBasis(registry, contractKey(t, account), nil)
		adopted, _ := result.AdoptedVersion()
		if result.Outcome() != domain.UniquelyResolved || adopted.ObjectID().String() != want {
			t.Fatalf("%s：outcome = %q adopted = %q，要唯一解出 %s", account, result.Outcome(), adopted.ObjectID(), want)
		}
	}
	if result := domain.ResolveCommercialBasis(registry, contractKey(t, "customer-3"), nil); result.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("没有合同的一户：outcome = %q，要无适用依据", result.Outcome())
	}

	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	contents, err := adapter.NewCustomerContractContents(db)
	if err != nil {
		t.Fatalf("构造合同正文读口：%v", err)
	}
	read, found, err := contents.LoadCustomerContract(ctx, pcTenant(t, "tenant-1"), second)
	if err != nil || !found || read.CustomerAccount().String() != "customer-2" {
		t.Fatalf("读回正文：found = %v err = %v account = %q", found, err, read.CustomerAccount())
	}
}

// Covers: ADR-0177 决定一、五——迁移 0037 之前写下的、没有客户账户的正文行：点读口报错而不是答「未登记」或猜一户；
// 选法照「正文未登记」那一格让它参选，坏的那份由点读口报出来。
func TestALegacyContractRowWithoutAnAccountIsAnErrorNotAGuess(t *testing.T) {
	contents, pool := newCustomerContractContents(t)
	contract := contractVersionInTenant(t, "tenant-1", "contract-legacy", "v1", "digest-legacy")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_content
			(tenant_id, object_kind, object_id, version_label, rule_package_id)
		 VALUES ('tenant-1', 2, 'contract-legacy', 'v1', 'rules-1')`); err != nil {
		t.Fatalf("写旧形状的正文行：%v", err)
	}

	_, found, err := contents.LoadCustomerContract(t.Context(), pcTenant(t, "tenant-1"), contract)
	if err == nil || found || !strings.Contains(err.Error(), "names no customer account") {
		t.Fatalf("旧行：found = %v err = %v，要报错且不交内容", found, err)
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE party_commercial.customer_contract_content SET customer_account_id = '  '
		  WHERE object_id = 'contract-legacy'`); err == nil {
		t.Fatal("空白客户账户进了正文表")
	}
}
