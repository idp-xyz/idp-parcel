package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestAmountGrammarsRefuseToRunOutsideATransaction(t *testing.T) {
	grammars := newAmountGrammars(t)
	registration := amountGrammarRegistration(t, domain.AmountGrammarClaimRule, "claim-rule/v2", 5_000, 8_000, 1_000)

	if _, err := grammars.SaveAmountGrammar(t.Context(), registerTenant(t), registration, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestEmptyAmountGrammarsStayUnconfigured(t *testing.T) {
	grammars := newAmountGrammars(t)

	_, found, err := grammars.LoadAmountGrammar(t.Context(), registerTenant(t), domain.AmountGrammarClaimRule, "claim-rule/v2")
	if err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}
}

func TestAnAmountGrammarRegistersReplaysAndConflicts(t *testing.T) {
	db := catalogueDB(t)
	grammars := mustAmountGrammars(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	first := amountGrammarRegistration(t, domain.AmountGrammarClaimRule, "claim-rule/v2", 5_000, 8_000, 1_000)

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		effect, err := grammars.SaveAmountGrammar(ctx, tenant, first, at)
		if err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		effect, err = grammars.SaveAmountGrammar(ctx, tenant, first, at)
		if err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		other := amountGrammarRegistration(t, domain.AmountGrammarClaimRule, "claim-rule/v2", 9_000, 8_000, 1_000)
		effect, err = grammars.SaveAmountGrammar(ctx, tenant, other, at)
		if err != nil || effect != ports.CatalogueConflict {
			return errOrEffect(err, effect, ports.CatalogueConflict)
		}
		recovery := amountGrammarRegistration(t, domain.AmountGrammarRecoveryContract, "contract-clause-7", 4_000, 10_000, 0)
		effect, err = grammars.SaveAmountGrammar(ctx, tenant, recovery, at)
		if err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := grammars.LoadAmountGrammar(t.Context(), tenant, domain.AmountGrammarClaimRule, "claim-rule/v2")
	if err != nil || !found || loaded.LimitMinor() != 5_000 || loaded.RatioBasisPoints() != 8_000 || loaded.DeductibleMinor() != 1_000 {
		t.Fatalf("claim grammar found=%v err=%v limit=%d", found, err, loaded.LimitMinor())
	}
	_, found, err = grammars.LoadAmountGrammar(t.Context(), tenant, domain.AmountGrammarRecoveryContract, "claim-rule/v2")
	if err != nil || found {
		t.Fatalf("回收引用不该读到赔付那一行 found=%v err=%v", found, err)
	}
}

func newAmountGrammars(t *testing.T) *adapter.AmountGrammars {
	t.Helper()
	return mustAmountGrammars(t, catalogueDB(t))
}

func mustAmountGrammars(t *testing.T, db *bentopg.DB) *adapter.AmountGrammars {
	t.Helper()
	grammars, err := adapter.NewAmountGrammars(db)
	if err != nil {
		t.Fatal(err)
	}
	return grammars
}

func amountGrammarRegistration(
	t *testing.T,
	subject domain.AmountGrammarSubject,
	ref string,
	limit, ratio, deductible int64,
) domain.AmountGrammarRegistration {
	t.Helper()
	grammar, err := domain.NewAmountGrammar(limit, ratio, deductible)
	if err != nil {
		t.Fatalf("grammar: %v", err)
	}
	registration, err := domain.NewAmountGrammarRegistration(subject, ref, grammar)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}
