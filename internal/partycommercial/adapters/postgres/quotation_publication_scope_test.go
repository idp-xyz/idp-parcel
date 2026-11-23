package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 读「这位客户此刻适用的合同有没有指名这个计价范围」。判法与解析选用同一套：快照里已生效、选用区间含读的时刻、
// 退役与截止折进来；账户最新修订已停用的不算。夹具全部合成。

func TestContractNamesPricingScopeUsesTheVersionInForce(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	view, err := adapter.NewQuotationPublication(db)
	if err != nil {
		t.Fatal(err)
	}
	registrations, err := adapter.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	const scope = "SYN-SCOPE-SELL"

	live := effectiveContract(t, "contract-live", "v1", "digest-live")
	plantNamedContract(t, transactor, repository, registrations, live, "customer-live", "party-live", scope)

	stillCut := effectiveContract(t, "contract-cut-open", "v1", "digest-cut-open")
	plantNamedContract(t, transactor, repository, registrations, stillCut, "customer-cut-open", "party-cut-open", scope)
	narrowStillOpen, err := domain.NewEffectiveInterval(effectiveAtRow, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	saveCorrection(t, transactor, repository, stillCut, narrowStillOpen)

	named := func(party, wantScope string) bool {
		t.Helper()
		got, nameErr := view.ContractNamesPricingScope(t.Context(), live.Tenant(), party, wantScope, at)
		if nameErr != nil {
			t.Fatal(nameErr)
		}
		return got
	}
	if !named("party-live", scope) {
		t.Fatal("此刻适用且来源指名了这个范围，应读到指名")
	}
	if named("party-live", "SYN-SCOPE-OTHER") {
		t.Fatal("别的计价范围不应读成指名")
	}
	if named("party-other", scope) {
		t.Fatal("别的相对方不应读成指名")
	}
	if !named("party-cut-open", scope) {
		t.Fatal("截止点还在读的时刻之后，这一版仍适用，应读到指名")
	}

	expired := contractVersionBetween(t, "contract-expired", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	plantNamedContract(t, transactor, repository, registrations, expired, "customer-expired", "party-expired", scope)
	if named("party-expired", scope) {
		t.Fatal("选用区间已经结束，不应读成此刻指名")
	}

	future := contractVersionBetween(t, "contract-future", at.Add(24*time.Hour), at.Add(90*24*time.Hour))
	plantNamedContract(t, transactor, repository, registrations, future, "customer-future", "party-future", scope)
	if named("party-future", scope) {
		t.Fatal("选用区间还没开始，不应读成此刻指名")
	}

	retired := effectiveContract(t, "contract-retired", "v1", "digest-retired")
	plantNamedContract(t, transactor, repository, registrations, retired, "customer-retired", "party-retired", scope)
	successor := contractVersionStatus(t, "contract-retired", "v2", domain.CommercialVersionEffective, effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour), effectiveAtRow)
	err = transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, saveErr := repository.SaveVersion(ctx, successor)
		if saveErr != nil {
			return saveErr
		}
		if outcome != ports.PublicationSaved {
			return fmt.Errorf("save successor %s", outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	saveRetirement(t, transactor, repository, retired)
	if named("party-retired", scope) {
		t.Fatal("已退役的版本不应读成此刻指名")
	}

	cut := effectiveContract(t, "contract-cut", "v1", "digest-cut")
	plantNamedContract(t, transactor, repository, registrations, cut, "customer-cut", "party-cut", scope)
	ended, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	saveCorrection(t, transactor, repository, cut, ended)
	if named("party-cut", scope) {
		t.Fatal("后继截止已经把选用区间收到读的时刻之前，不应读成此刻指名")
	}

	stopped := effectiveContract(t, "contract-stopped", "v1", "digest-stopped")
	plantNamedContract(t, transactor, repository, registrations, stopped, "customer-stopped", "party-stopped", scope)
	deactivateAccount(t, transactor, registrations, "customer-stopped", "party-stopped")
	if named("party-stopped", scope) {
		t.Fatal("货主客户账户已停用，不应读成这位客户此刻指名")
	}

	published := contractVersionStatus(t, "contract-published", "v1", domain.CommercialVersionPublished, effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour), time.Time{})
	plantContractRows(t, transactor, repository, registrations, published, "customer-published", "party-published", scope)
	if named("party-published", scope) {
		t.Fatal("发布时没取效、状态列停在已发布的版本，解析也不选它，不应读成此刻指名")
	}
}

func plantNamedContract(
	t *testing.T,
	transactor bentoapp.Transactor,
	repository *adapter.CommercialPublications,
	registrations *adapter.PartyIdentityRegistrations,
	version domain.CommercialVersion,
	accountID, partyID, scope string,
) {
	t.Helper()
	plantContractRows(t, transactor, repository, registrations, version, accountID, partyID, scope)
}

func plantContractRows(
	t *testing.T,
	transactor bentoapp.Transactor,
	repository *adapter.CommercialPublications,
	registrations *adapter.PartyIdentityRegistrations,
	version domain.CommercialVersion,
	accountID, partyID, scope string,
) {
	t.Helper()
	account := customerAccountRegistrationFixture(t, version.Tenant().String(), accountID, partyID)
	contentVersion := version
	if version.Status() != domain.CommercialVersionEffective {
		// 正文构造只收已生效的壳。库上的版本行仍按传入的状态保存，用来证「已发布」进不了此刻适用。
		contentVersion = contractVersionStatus(t, version.ObjectID().String(), version.Version().String(), domain.CommercialVersionEffective, version.Effective().StartsAt(), mustEnd(t, version), version.Effective().StartsAt())
	}
	err := transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if outcome, saveErr := registrations.SaveCustomerAccount(ctx, account); saveErr != nil || outcome != ports.PartyRegistrySaved {
			return fmt.Errorf("save account %s: %v %s", accountID, saveErr, outcome)
		}
		if outcome, saveErr := repository.SaveVersion(ctx, version); saveErr != nil || outcome != ports.PublicationSaved {
			return fmt.Errorf("save version %s: %v %s", version.ObjectID(), saveErr, outcome)
		}
		if outcome, saveErr := repository.SaveCustomerContractContent(ctx, contractContentFor(t, contentVersion, accountID)); saveErr != nil || outcome != ports.DeclarationSaved {
			return fmt.Errorf("save contract %s: %v %s", version.ObjectID(), saveErr, outcome)
		}
		if outcome, saveErr := repository.SaveContractSellPriceArrangement(ctx, sellPriceOf(t, contentVersion, "", scope)); saveErr != nil || outcome != ports.DeclarationSaved {
			return fmt.Errorf("save sources %s: %v %s", version.ObjectID(), saveErr, outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func saveCorrection(
	t *testing.T,
	transactor bentoapp.Transactor,
	repository *adapter.CommercialPublications,
	version domain.CommercialVersion,
	corrected domain.EffectiveInterval,
) {
	t.Helper()
	correction, err := version.CorrectEffectiveInterval(corrected, pcValue(t, domain.NewValidityCorrectionReference, "SYN-CUT-"+version.ObjectID().String()), effectiveAtRow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	err = transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, saveErr := repository.SaveValidityCorrection(ctx, correction)
		if saveErr != nil {
			return saveErr
		}
		if outcome != ports.ValidityCorrectionSaved {
			return fmt.Errorf("save correction %s", outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func saveRetirement(
	t *testing.T,
	transactor bentoapp.Transactor,
	repository *adapter.CommercialPublications,
	version domain.CommercialVersion,
) {
	t.Helper()
	retirement, err := version.RetireInFavourOf(
		pcValue(t, domain.NewCommercialVersionLabel, "v2"),
		pcValue(t, domain.NewRetirementReference, "SYN-RETIRE-"+version.ObjectID().String()),
		versionEffectiveAt(t, version).Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, saveErr := repository.SaveVersionRetirement(ctx, retirement)
		if saveErr != nil {
			return saveErr
		}
		if outcome != ports.VersionRetirementSaved {
			return fmt.Errorf("save retirement %s", outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func deactivateAccount(
	t *testing.T,
	transactor bentoapp.Transactor,
	registrations *adapter.PartyIdentityRegistrations,
	accountID, partyID string,
) {
	t.Helper()
	account := customerAccountRegistrationFixture(t, "tenant-1", accountID, partyID)
	stopped, err := account.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "basis-stop-"+accountID), identityDeactivateAt)
	if err != nil {
		t.Fatal(err)
	}
	err = transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, saveErr := registrations.SaveCustomerAccount(ctx, stopped)
		if saveErr != nil {
			return saveErr
		}
		if outcome != ports.PartyRegistrySaved {
			return fmt.Errorf("deactivate %s", outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func contractContentFor(t *testing.T, contract domain.CommercialVersion, accountID string) domain.CustomerContract {
	t.Helper()
	applied, err := domain.NewAppliedFinancialControl(
		pcValue(t, domain.NewChargeScopeReference, "charge-scope-1"),
		pcValue(t, domain.NewCommercialObjectID, "control-policy-1"))
	if err != nil {
		t.Fatal(err)
	}
	inapplicable, err := domain.NewInapplicableFinancialControl(
		pcValue(t, domain.NewChargeScopeReference, "charge-scope-2"),
		pcValue(t, domain.NewInapplicabilityBasis, "CONTRACT-CLAUSE/NO-CONTROL"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := domain.NewCustomerContract(contract, pcValue(t, domain.NewCustomerAccountID, accountID),
		pcValue(t, domain.NewCommercialObjectID, "rules-1"),
		[]domain.FinancialControlBinding{applied, inapplicable})
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func contractVersionBetween(t *testing.T, objectID string, start, end time.Time) domain.CommercialVersion {
	t.Helper()
	return contractVersionStatus(t, objectID, "v1", domain.CommercialVersionEffective, start, end, start)
}

func contractVersionStatus(
	t *testing.T,
	objectID, label string,
	status domain.CommercialVersionStatus,
	start, end, effectiveAt time.Time,
) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(start, end)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-"+objectID),
		pcValue(t, domain.NewCommercialSourceReference, "source-1"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          domain.CustomerContractObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "digest-"+objectID),
		Effective:     interval,
		Status:        status,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAt,
		References: map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.AcceptanceRulePackageObject: pcValue(t, domain.NewCommercialObjectID, "rules-1"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func mustEnd(t *testing.T, version domain.CommercialVersion) time.Time {
	t.Helper()
	end, bounded := version.Effective().EndsAt()
	if !bounded {
		t.Fatal("夹具区间应当有终点")
	}
	return end
}

func versionEffectiveAt(t *testing.T, version domain.CommercialVersion) time.Time {
	t.Helper()
	at, ok := version.EffectiveAt()
	if !ok {
		t.Fatal("已生效的夹具缺生效时刻")
	}
	return at
}
