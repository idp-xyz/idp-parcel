package accessidentity_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// 本文件证首方客户命令五口的共用铸造底座（票 first-party-api/06；ADR-0310）：一口一词的授予、铸造侧判准入范围、按命令分型的信封，
// 与五个来源请求键推导（取舍 A）。各口的领域 Intake 不在这里。合成主体只记 `S`。

type commandFace struct {
	capability accessidentity.CustomerCapability
	literal    string
	prefix     string
	mint       func(*accessidentity.CustomerClientMinter, *requestReader) (accessidentity.SourceEnvelope, error)
}

func commandFaces() map[string]commandFace {
	credential := accessidentity.NewIntegrationClientCredential("presented.customer.token", nil)
	ctx := context.Background()
	return map[string]commandFace{
		"withdrawal": {accessidentity.CustomerCapabilityShipmentRequestWithdrawal, "SHIPMENT_REQUEST_WITHDRAWAL", "withdrawal:",
			func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
				envelope, err := minter.MintWithdrawal(ctx, credential, reader.channelRequest)
				return envelope.Envelope(), err
			}},
		"parcel cancellation": {accessidentity.CustomerCapabilityParcelCancellation, "PARCEL_CANCELLATION", "parcel-cancellation:",
			func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
				envelope, err := minter.MintParcelCancellation(ctx, credential, reader.channelRequest)
				return envelope.Envelope(), err
			}},
		"source data amendment": {accessidentity.CustomerCapabilitySourceDataAmendment, "SOURCE_DATA_AMENDMENT", "source-data-amendment:",
			func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
				envelope, err := minter.MintSourceDataAmendment(ctx, credential, reader.channelRequest)
				return envelope.Envelope(), err
			}},
		"supplement": {accessidentity.CustomerCapabilityShipmentRequestSupplement, "SHIPMENT_REQUEST_SUPPLEMENT", "supplement:",
			func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
				envelope, err := minter.MintSupplement(ctx, credential, reader.channelRequest)
				return envelope.Envelope(), err
			}},
		"claim": {accessidentity.CustomerCapabilityClaimSubmission, "CLAIM_SUBMISSION", "claim:",
			func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
				envelope, err := minter.MintClaim(ctx, credential, reader.channelRequest)
				return envelope.Envelope(), err
			}},
	}
}

// Covers: AT-PS-120——五口各一个词，字面量在封闭词表里、大小写不折叠；持这一口授予的客户端铸出这一口的信封，租户与客户账户只取自
// 册上绑定，渠道请求里自报的进不了信封；准入那一步只问（CUSTOMER_COMMAND，这一口的词）。
func TestCommandFacesMintTheirEnvelopesFromTheBinding(t *testing.T) {
	for name, face := range commandFaces() {
		t.Run(name, func(t *testing.T) {
			if got, err := accessidentity.ParseCustomerCapability(face.literal); err != nil || got != face.capability {
				t.Fatalf("%s：%q, %v", face.literal, got, err)
			}
			var asked []accessidentity.AdmissionRequirement
			minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
				customerRegistryFake{standing: customerStandingHolding(t, face.capability), found: true},
				admissionProbe{admitted: true, asked: &asked})
			reader := &requestReader{values: map[string]string{
				accessidentity.AccessRequestIdentifier: " SYN-ACCESS-REQUEST-CF-0001 ",
				"tenantId":                             "TENANT-PROD-9",
				"customerAccountId":                    "ACCOUNT-PROD-9",
			}}
			envelope, err := face.mint(minter, reader)
			if err != nil {
				t.Fatalf("持授予的客户端：%v", err)
			}
			if envelope.TenantID() != customerTenant || envelope.CustomerAccountID() != customerAccount {
				t.Fatalf("信封身份 = (%q, %q), want 册上绑定的", envelope.TenantID(), envelope.CustomerAccountID())
			}
			if want := face.prefix + "SYN-ACCESS-REQUEST-CF-0001"; envelope.RequestKey() != want {
				t.Fatalf("来源请求键 = %q, want %q", envelope.RequestKey(), want)
			}
			want := accessidentity.AdmissionRequirement{Capability: accessidentity.CustomerCommandAdmissionCapability, FactKind: face.literal}
			if len(asked) != 1 || asked[0] != want {
				t.Fatalf("准入要求 = %+v, want 只问 %+v", asked, want)
			}
			interval := clientIntervalOf(t, customerGrantStartsAt, customerGrantStartsAt.AddDate(1, 0, 0))
			if _, err := accessidentity.NewAdmissionScopeMapping(customerTenant, "SYN-MAP-1", want, "SYN-PILOT-SCOPE/customer-commands@v1", interval, "SYN-BASIS"); err != nil {
				t.Fatalf("这一口的词应登得进准入范围对照：%v", err)
			}
		})
	}
}

// Covers: AT-PS-120——未授予（不在册、只持别口或别的授予）答未授予、不问准入范围；授予在而准入范围不符答范围外、读不动答不可用；
// 认证没过之前一个字的请求都不读。
func TestCommandFacesAnswerTheirOwnGrades(t *testing.T) {
	faces := commandFaces()
	for name, face := range faces {
		t.Run(name, func(t *testing.T) {
			granted := customerRegistryFake{standing: customerStandingHolding(t, face.capability), found: true}
			unbound := customerVerifierFake{verified: verifiedCustomerToken(t, false)}
			mustNotAsk := admissionProbe{err: errors.New("admission must not be asked before the grant passes")}
			otherFace := faces["claim"].capability
			if face.capability == otherFace {
				otherFace = faces["supplement"].capability
			}
			for grade, testCase := range map[string]struct {
				verifier  accessidentity.CustomerClientCredentialVerifier
				registry  customerRegistryFake
				admission accessidentity.AdmissionScope
				want      string
			}{
				"issuer unset":           {accessidentity.UnconfiguredCustomerClientCredentialVerifier{}, granted, mustNotAsk, "not configured"},
				"token rejected":         {customerVerifierFake{err: accessidentity.ErrCredentialRejected}, granted, mustNotAsk, "credential rejected"},
				"not on the register":    {unbound, customerRegistryFake{}, mustNotAsk, "not granted"},
				"only another face":      {unbound, customerRegistryFake{standing: customerStandingHolding(t, otherFace), found: true}, mustNotAsk, "not granted"},
				"only shipment requests": {unbound, customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityShipmentRequestSubmission), found: true}, mustNotAsk, "not granted"},
				"outside admission":      {unbound, granted, accessidentity.UnconfiguredAdmissionScope{}, "outside admission"},
				"admission unreadable":   {unbound, granted, admissionProbe{err: errors.New("governance register unreachable")}, "admission unavailable"},
			} {
				t.Run(grade, func(t *testing.T) {
					minter := newPreAdviceMinter(t, testCase.verifier, testCase.registry, testCase.admission)
					reader := &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "SYN-ACCESS-REQUEST-CF-0001"}}
					_, err := face.mint(minter, reader)
					assertCustomerGrade(t, err, testCase.want)
					if reader.read {
						t.Fatal("认证没过却读了请求")
					}
				})
			}
		})
	}
}

// Covers: AT-PS-121（ADR-0310 取舍 A）——五个推导各有三格：带命令前缀、标识缺席或全空白答推导不出、去两端空白；读请求自己的错原样
// 交回；不给读请求的口答缺件。
func TestCommandFaceRequestKeysCarryThePrefixAndNeedAnIdentifier(t *testing.T) {
	for name, face := range commandFaces() {
		t.Run(name, func(t *testing.T) {
			minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
				customerRegistryFake{standing: customerStandingHolding(t, face.capability), found: true}, admissionProbe{admitted: true})
			envelope, err := face.mint(minter, &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "\t ID-7 \n"}})
			if err != nil || envelope.RequestKey() != face.prefix+"ID-7" {
				t.Fatalf("去空白：key = %q, err = %v, want %q", envelope.RequestKey(), err, face.prefix+"ID-7")
			}
			for absence, values := range map[string]map[string]string{
				"absent": {},
				"blank":  {accessidentity.AccessRequestIdentifier: " \t "},
			} {
				if _, err := face.mint(minter, &requestReader{values: values}); !errors.Is(err, accessidentity.ErrRequestKeyNotDerived) {
					t.Fatalf("%s：err = %v, want ErrRequestKeyNotDerived", absence, err)
				}
			}
			malformed := errors.New("payload is not JSON")
			if _, err := face.mint(minter, &requestReader{err: malformed}); !errors.Is(err, malformed) {
				t.Fatalf("读请求的错 = %v, want 原样交回", err)
			}
		})
	}
	minter := newPreAdviceMinter(t, customerVerifierFake{}, customerRegistryFake{}, admissionProbe{})
	credential := accessidentity.NewIntegrationClientCredential("t", nil)
	for name, mint := range map[string]func() error{
		"withdrawal": func() error { _, err := minter.MintWithdrawal(context.Background(), credential, nil); return err },
		"parcel cancellation": func() error {
			_, err := minter.MintParcelCancellation(context.Background(), credential, nil)
			return err
		},
		"source data amendment": func() error {
			_, err := minter.MintSourceDataAmendment(context.Background(), credential, nil)
			return err
		},
		"supplement": func() error { _, err := minter.MintSupplement(context.Background(), credential, nil); return err },
		"claim":      func() error { _, err := minter.MintClaim(context.Background(), credential, nil); return err },
	} {
		if err := mint(); !errors.Is(err, accessidentity.ErrNilDependency) {
			t.Fatalf("%s 不给读请求的口：err = %v", name, err)
		}
	}
}

// Covers: AT-PS-121——同一接入请求标识在五个命令与提交口的来源请求键两两不同（取舍 A：标识只在命令内唯一，跨命令不撞）。
func TestOneAccessRequestIdentifierGivesDistinctKeysAcrossCommands(t *testing.T) {
	keys := map[string]string{}
	for name, face := range commandFaces() {
		minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
			customerRegistryFake{standing: customerStandingHolding(t, face.capability), found: true}, admissionProbe{admitted: true})
		envelope, err := face.mint(minter, &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "SAME-ID"}})
		if err != nil {
			t.Fatal(err)
		}
		keys[name] = envelope.RequestKey()
	}
	keys["submission"] = "SAME-ID"
	seen := map[string]string{}
	for name, key := range keys {
		if other, clash := seen[key]; clash {
			t.Fatalf("%s 与 %s 的来源请求键同为 %q", name, other, key)
		}
		seen[key] = name
	}
}
