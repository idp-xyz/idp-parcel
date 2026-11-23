package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var errNotSaved = errPublication("requirement was not saved")

type publicationTestError string

func errPublication(text string) error { return publicationTestError(text) }

func (err publicationTestError) Error() string { return string(err) }

// 写口在无事务上下文必须被 RequireExecutor 拒绝。拒绝先于任何入参解读，所以传零值就够。
func TestQuotationPublicationRequirementWritesRefuseToRunOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	view, err := adapter.NewQuotationPublication(db)
	if err != nil {
		t.Fatalf("构造发布前提册：%v", err)
	}
	if _, _, err := view.SaveQuotationPublicationRequirement(t.Context(), domain.QuotationPublicationRequirement{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记发布前提应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestAnUnconfiguredPublicationRuleDoesNotNameAScope(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	view, err := adapter.NewQuotationPublication(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := view.LoadQuotationPublicationRequirement(t.Context(), tenant)
	if err != nil || found {
		t.Fatalf("未登记 = found %v %v", found, err)
	}
	named, err := view.ContractNamesPricingScope(t.Context(), tenant, "SYN-PARTY-SHIPPER-01", "SYN-SCOPE-NONE")
	if err != nil || named {
		t.Fatalf("没指名 = %v %v", named, err)
	}
	requirement, err := domain.NewQuotationPublicationRequirement(tenant, true)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		same, saved, saveErr := view.SaveQuotationPublicationRequirement(ctx, requirement)
		if saveErr != nil {
			return saveErr
		}
		if same || !saved {
			return errNotSaved
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	requires, found, err := view.LoadQuotationPublicationRequirement(t.Context(), tenant)
	if err != nil || !found || !requires {
		t.Fatalf("读回 = %v %v %v", requires, found, err)
	}
}
