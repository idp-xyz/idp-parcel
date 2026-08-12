package settlementaccounting_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type resolverDouble struct {
	resolution psports.CommercialBasisResolution
	err        error
	asked      int
}

func (double *resolverDouble) ResolveCommercialBasis(
	_ context.Context,
	_ psports.CommercialBasisQuery,
) (psports.CommercialBasisResolution, error) {
	double.asked++
	if double.err != nil {
		return psports.CommercialBasisResolution{}, double.err
	}
	return double.resolution, nil
}

func (double *resolverDouble) FormJudgmentAsOf(
	_ context.Context,
	_ psports.JudgmentAsOfQuery,
) (psports.JudgmentAsOfFormation, error) {
	return psports.JudgmentAsOfFormation{}, errors.New("not part of this seam")
}

func (double *resolverDouble) RevalidateCommercialBasis(
	_ context.Context,
	_ psports.CommercialRevalidationQuery,
) (psports.CommercialRevalidation, error) {
	return psports.CommercialRevalidation{}, errors.New("not part of this seam")
}

type directoryDouble struct {
	account    sadomain.SettlementAccountID
	found      bool
	err        error
	askedTerms psdomain.AdoptedSettlementTerms
	asked      int
}

func (double *directoryDouble) FindSettlementAccount(
	_ context.Context,
	_ psdomain.SourceIdentity,
	terms psdomain.AdoptedSettlementTerms,
) (sadomain.SettlementAccountID, bool, error) {
	double.asked++
	double.askedTerms = terms
	if double.err != nil {
		return sadomain.SettlementAccountID{}, false, double.err
	}
	return double.account, double.found, nil
}

func adoptedTerms(t *testing.T) psdomain.AdoptedSettlementTerms {
	t.Helper()
	terms, err := psdomain.NewAdoptedSettlementTerms(psdomain.AdoptedSettlementTermsSpec{
		Policy:       value(t, psdomain.NewSettlementPolicyEcho, "settlement-policy-1/v3"),
		Method:       value(t, psdomain.NewSettlementMethodEcho, "TERMS"),
		LegalEntity:  value(t, psdomain.NewSettlementLegalEntityEcho, "legal-1"),
		Counterparty: value(t, psdomain.NewSettlementCounterpartyEcho, "counterparty-1"),
		Currency:     value(t, psdomain.NewSettlementCurrencyEcho, "CNY"),
	})
	if err != nil {
		t.Fatalf("new adopted settlement terms: %v", err)
	}
	return terms
}

func snapshotWithTerms(t *testing.T, terms psdomain.AdoptedSettlementTerms) psdomain.CommercialBasisSnapshot {
	t.Helper()
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID:    value(t, psdomain.NewCommercialResolutionID, "RES-1"),
		RulePackage:     value(t, psdomain.NewRulePackageReference, "rules-1/v1"),
		ViewRevision:    value(t, psdomain.NewCommercialViewRevision, "VIEW-1"),
		SettlementTerms: terms,
	})
	if err != nil {
		t.Fatalf("new commercial basis snapshot: %v", err)
	}
	return snapshot
}

func scopeIdentity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

// Covers: ADR-0047 作用域缝的机制半边——法人与币种取自解析采用的结算政策回显
// （ADR-0044），账户由目录按同一份回显换取；三维拼成的作用域正是 SA 要求的
// 「责任法人/结算账户/币种」。
func TestAControlScopeIsDerivedFromTheAdoptedSettlementPolicy(t *testing.T) {
	terms := adoptedTerms(t)
	resolver := &resolverDouble{resolution: psports.CommercialBasisResolution{
		Snapshot: snapshotWithTerms(t, terms),
	}}
	directory := &directoryDouble{
		account: value(t, sadomain.NewSettlementAccountID, "account-7"),
		found:   true,
	}
	source := adapter.NewPolicyBackedControlScopeSource(resolver, directory)

	scope, formed, err := source.FormControlScope(context.Background(), scopeIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		value(t, psdomain.NewSubmissionVersionID, "version-1"))
	if err != nil {
		t.Fatalf("form control scope: %v", err)
	}

	if !formed {
		t.Fatal("回显与目录都在，作用域却没形成")
	}
	if scope.LegalEntity().String() != "legal-1" ||
		scope.Account().String() != "account-7" ||
		scope.Currency().String() != "CNY" {
		t.Fatalf("scope = %s/%s/%s; 三维必须来自回显与目录", scope.LegalEntity(), scope.Account(), scope.Currency())
	}
	if directory.askedTerms.Policy().String() != "settlement-policy-1/v3" {
		t.Fatal("目录没拿到解析回显的那份政策——它换出的账户无从对上采用依据")
	}
}

// Covers: 停在未形成的三格——解析未含结算政策、目录未配置（nil）、映射查无。三者都是
// 「显式未配置」不是错误：编排把它记成 CONTROL_SCOPE_NOT_CONFIGURED 等配置补齐，而不是
// 当故障重试。解析器答不出才是错误（依赖失败另有续办路径）。
func TestAScopeWithoutPolicyDirectoryOrMappingStaysUnformed(t *testing.T) {
	requestID := value(t, psdomain.NewShipmentRequestID, "request-1")
	versionID := value(t, psdomain.NewSubmissionVersionID, "version-1")

	t.Run("resolution without settlement terms", func(t *testing.T) {
		resolver := &resolverDouble{resolution: psports.CommercialBasisResolution{}}
		directory := &directoryDouble{found: true}
		source := adapter.NewPolicyBackedControlScopeSource(resolver, directory)

		_, formed, err := source.FormControlScope(context.Background(), scopeIdentity(t), requestID, versionID)
		if err != nil || formed {
			t.Fatalf("formed = %v err = %v, want unformed without error", formed, err)
		}
		if directory.asked != 0 {
			t.Fatal("没有回显还去问了目录")
		}
	})

	t.Run("nil directory", func(t *testing.T) {
		resolver := &resolverDouble{resolution: psports.CommercialBasisResolution{
			Snapshot: snapshotWithTerms(t, adoptedTerms(t)),
		}}
		source := adapter.NewPolicyBackedControlScopeSource(resolver, nil)

		_, formed, err := source.FormControlScope(context.Background(), scopeIdentity(t), requestID, versionID)
		if err != nil || formed {
			t.Fatalf("formed = %v err = %v, want unformed without error", formed, err)
		}
		if resolver.asked != 0 {
			t.Fatal("目录未配置还消耗了一次解析")
		}
	})

	t.Run("mapping not found", func(t *testing.T) {
		resolver := &resolverDouble{resolution: psports.CommercialBasisResolution{
			Snapshot: snapshotWithTerms(t, adoptedTerms(t)),
		}}
		source := adapter.NewPolicyBackedControlScopeSource(resolver, &directoryDouble{})

		_, formed, err := source.FormControlScope(context.Background(), scopeIdentity(t), requestID, versionID)
		if err != nil || formed {
			t.Fatalf("formed = %v err = %v, want unformed without error", formed, err)
		}
	})

	t.Run("resolver unavailable is an error", func(t *testing.T) {
		resolver := &resolverDouble{err: errors.New("commercial authority down")}
		source := adapter.NewPolicyBackedControlScopeSource(resolver, &directoryDouble{found: true})

		_, formed, err := source.FormControlScope(context.Background(), scopeIdentity(t), requestID, versionID)
		if err == nil || formed {
			t.Fatalf("formed = %v err = %v, want a propagated error——依赖失败不是未配置", formed, err)
		}
	})
}
