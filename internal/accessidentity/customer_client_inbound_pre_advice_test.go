package accessidentity_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// 本文件证入库预报货主写口、收货差异结论口与仓配查阅的铸造（ADR-0384 决定八）。合成主体只记 `S`。

type inboundPreAdviceLine struct {
	capability accessidentity.CustomerCapability
	prefix     string
	mint       func(*accessidentity.CustomerClientMinter, *requestReader) (accessidentity.SourceEnvelope, error)
}

var inboundPreAdviceLines = map[string]inboundPreAdviceLine{
	"submission": {
		capability: accessidentity.CustomerCapabilityInboundPreAdviceSubmission,
		prefix:     "inbound-pre-advice:",
		mint: func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
			envelope, err := minter.MintInboundPreAdviceSubmission(context.Background(),
				accessidentity.NewIntegrationClientCredential("presented", nil), reader.channelRequest)
			if err != nil {
				return accessidentity.SourceEnvelope{}, err
			}
			return envelope.Envelope(), nil
		},
	},
	"arrival notice": {
		capability: accessidentity.CustomerCapabilityInboundPreAdviceArrivalNotice,
		prefix:     "inbound-arrival-notice:",
		mint: func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
			envelope, err := minter.MintInboundPreAdviceArrivalNotice(context.Background(),
				accessidentity.NewIntegrationClientCredential("presented", nil), reader.channelRequest)
			if err != nil {
				return accessidentity.SourceEnvelope{}, err
			}
			return envelope.Envelope(), nil
		},
	},
	"withdrawal": {
		capability: accessidentity.CustomerCapabilityInboundPreAdviceWithdrawal,
		prefix:     "inbound-withdrawal:",
		mint: func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
			envelope, err := minter.MintInboundPreAdviceWithdrawal(context.Background(),
				accessidentity.NewIntegrationClientCredential("presented", nil), reader.channelRequest)
			if err != nil {
				return accessidentity.SourceEnvelope{}, err
			}
			return envelope.Envelope(), nil
		},
	},
	"receipt discrepancy conclusion": {
		capability: accessidentity.CustomerCapabilityReceiptDiscrepancyConclusion,
		prefix:     "receipt-discrepancy:",
		mint: func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
			envelope, err := minter.MintReceiptDiscrepancyConclusion(context.Background(),
				accessidentity.NewIntegrationClientCredential("presented", nil), reader.channelRequest)
			if err != nil {
				return accessidentity.SourceEnvelope{}, err
			}
			return envelope.Envelope(), nil
		},
	},
}

// Covers: ADR-0384 决定八——货主写口问的准入词两格都是该口字面量，不是 CUSTOMER_COMMAND；来源请求键是前缀加去两端空白的标识。
func TestInboundPreAdviceWritesAskForTheirOwnAdmissionWords(t *testing.T) {
	for name, line := range inboundPreAdviceLines {
		t.Run(name, func(t *testing.T) {
			var asked []accessidentity.AdmissionRequirement
			minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
				customerRegistryFake{standing: customerStandingHolding(t, line.capability), found: true},
				admissionProbe{admitted: true, asked: &asked})
			reader := &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "  SYN-ACCESS-IPA-0001\t"}}
			envelope, err := line.mint(minter, reader)
			if err != nil {
				t.Fatal(err)
			}
			want := accessidentity.AdmissionRequirement{Capability: line.capability.String(), FactKind: line.capability.String()}
			if len(asked) != 1 || asked[0] != want {
				t.Fatalf("准入要求 = %+v, want 只问 %+v", asked, want)
			}
			if asked[0].Capability == accessidentity.CustomerCommandAdmissionCapability {
				t.Fatal("这一口走了 CUSTOMER_COMMAND")
			}
			if envelope.RequestKey() != line.prefix+"SYN-ACCESS-IPA-0001" {
				t.Fatalf("来源请求键 = %q, want %s", envelope.RequestKey(), line.prefix+"SYN-ACCESS-IPA-0001")
			}
			if envelope.TenantID() != customerTenant || envelope.CustomerAccountID() != customerAccount {
				t.Fatalf("信封身份 = (%q, %q)", envelope.TenantID(), envelope.CustomerAccountID())
			}
			if wantSource := customerIssuer + "#SYN-CUSTOMER-CLIENT-01"; envelope.Source() != wantSource {
				t.Fatalf("来源 = %q, want %q", envelope.Source(), wantSource)
			}
		})
	}
}

// Covers: ADR-0384 决定八——WAREHOUSE_READ 不判准入，铸造不问准入范围。
func TestWarehouseReadDoesNotAskAdmission(t *testing.T) {
	var asked []accessidentity.AdmissionRequirement
	minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
		customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityWarehouseRead), found: true},
		admissionProbe{err: errors.New("admission must not be asked"), asked: &asked})
	envelope, err := minter.MintWarehouseRead(context.Background(), accessidentity.NewIntegrationClientCredential("presented", nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 0 {
		t.Fatalf("查阅问了准入：%+v", asked)
	}
	if !envelope.Minted() || envelope.TenantID() != customerTenant || envelope.CustomerAccountID() != customerAccount {
		t.Fatalf("查阅身份 = (%q, %q) minted %t", envelope.TenantID(), envelope.CustomerAccountID(), envelope.Minted())
	}
}

// Covers: AT-WF-035——只持提交授予去撤回，答未授予，不问准入范围，也不读请求。
func TestInboundPreAdviceWithdrawalRefusesASubmissionOnlyGrant(t *testing.T) {
	var asked []accessidentity.AdmissionRequirement
	minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
		customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityInboundPreAdviceSubmission), found: true},
		admissionProbe{err: errors.New("admission must not be asked"), asked: &asked})
	reader := &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "SYN-ACCESS-IPA-0001"}}
	_, err := minter.MintInboundPreAdviceWithdrawal(context.Background(),
		accessidentity.NewIntegrationClientCredential("presented", nil), reader.channelRequest)
	assertCustomerGrade(t, err, "not granted")
	if len(asked) != 0 {
		t.Fatalf("未授予仍问了准入：%+v", asked)
	}
	if reader.read {
		t.Fatal("未授予仍读了请求")
	}
}

// Covers: ADR-0384 决定六——仓配三口的标识以保留前缀开头就推不出键，不落成来源请求键。
func TestInboundPreAdviceRequestKeyRejectsReservedPrefixes(t *testing.T) {
	minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
		customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityInboundPreAdviceSubmission), found: true},
		admissionProbe{admitted: true})
	for _, identifier := range []string{"withdrawal:abc", "inbound-pre-advice:abc"} {
		reader := &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: identifier}}
		_, err := minter.MintInboundPreAdviceSubmission(context.Background(),
			accessidentity.NewIntegrationClientCredential("presented", nil), reader.channelRequest)
		if !errors.Is(err, accessidentity.ErrRequestKeyNotDerived) {
			t.Fatalf("%s：err = %v, want ErrRequestKeyNotDerived", identifier, err)
		}
	}
}
