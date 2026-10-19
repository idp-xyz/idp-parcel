package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// 本文件证首方客户族在送站预报两口的铸造（票 ps-external-mark-relations/09；ADR-0212 决定二；ADR-0139 决定四、五、八）：
// 一口一词的授予、铸造侧判准入范围、按命令分型的两种信封，与预报两口的来源请求键推导。合成主体只记 `S`。

// admissionProbe 答准入范围并记下被问的要求：预报两口在铸造侧判准入，问的是哪一对（能力、事实类型）要钉住。
type admissionProbe struct {
	admitted bool
	err      error
	asked    *[]accessidentity.AdmissionRequirement
}

func (probe admissionProbe) Admits(_ context.Context, tenant string, requirement accessidentity.AdmissionRequirement, _ time.Time) (bool, error) {
	if probe.asked != nil {
		*probe.asked = append(*probe.asked, requirement)
	}
	if tenant != customerTenant {
		return false, errors.New("admission asked for a tenant the binding does not name")
	}
	return probe.admitted, probe.err
}

// customerStandingHolding 登一个绑在（customerTenant, customerAccount）上的客户端，名下每一项能力各授一笔，从
// customerMintAt 前一天起、不设终点。
func customerStandingHolding(t *testing.T, capabilities ...accessidentity.CustomerCapability) accessidentity.CustomerClientStanding {
	t.Helper()
	subject := customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01")
	interval := clientIntervalOf(t, customerMintAt.Add(-24*time.Hour), time.Time{})
	var grants []accessidentity.RecordedCustomerClientGrant
	for _, capability := range capabilities {
		grant, err := accessidentity.NewCustomerClientGrant(customerTenant, "SYN-CUSTOMER-GRANT-"+capability.String(), subject,
			capability, interval, "SYN-CUSTOMER-GRANT-BASIS-01")
		if err != nil {
			t.Fatalf("NewCustomerClientGrant(%s): %v", capability, err)
		}
		grants = append(grants, recordedCustomerGrantOf(t, grant, nil))
	}
	standing, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, false), grants)
	if err != nil {
		t.Fatal(err)
	}
	return standing
}

func newPreAdviceMinter(
	t *testing.T,
	verifier accessidentity.CustomerClientCredentialVerifier,
	registry accessidentity.CustomerClientRegistry,
	admission accessidentity.AdmissionScope,
) *accessidentity.CustomerClientMinter {
	t.Helper()
	minter, err := accessidentity.NewCustomerClientMinter(verifier, registry, admission, func() time.Time { return customerMintAt })
	if err != nil {
		t.Fatalf("NewCustomerClientMinter: %v", err)
	}
	return minter
}

// preAdviceLine 是预报两口之一：用哪一项能力、铸哪一种信封。两口的用例逐格相同，只差这两样。
type preAdviceLine struct {
	capability accessidentity.CustomerCapability
	other      accessidentity.CustomerCapability
	mint       func(*accessidentity.CustomerClientMinter, *requestReader) (accessidentity.SourceEnvelope, error)
}

var preAdviceLines = map[string]preAdviceLine{
	"submission": {
		capability: accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		other:      accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
		mint: func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
			envelope, err := minter.MintDeliveryPreAdviceSubmission(context.Background(),
				accessidentity.NewIntegrationClientCredential("presented.customer.token", nil), reader.channelRequest)
			return envelope.Envelope(), err
		},
	},
	"withdrawal": {
		capability: accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
		other:      accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		mint: func(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
			envelope, err := minter.MintDeliveryPreAdviceWithdrawal(context.Background(),
				accessidentity.NewIntegrationClientCredential("presented.customer.token", nil), reader.channelRequest)
			return envelope.Envelope(), err
		},
	},
}

// Covers: 票 ps-external-mark-relations/09 做什么「能力词」——一口一词：送站预报提交、送站预报撤回各一个字面量，与委托提交同在
// 一张封闭词表里，大小写不折叠。
func TestDeliveryPreAdviceCapabilitiesJoinTheClosedVocabulary(t *testing.T) {
	for literal, want := range map[string]accessidentity.CustomerCapability{
		"SHIPMENT_REQUEST_SUBMISSION":    accessidentity.CustomerCapabilityShipmentRequestSubmission,
		"DELIVERY_PRE_ADVICE_SUBMISSION": accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		"DELIVERY_PRE_ADVICE_WITHDRAWAL": accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
	} {
		if got, err := accessidentity.ParseCustomerCapability(literal); err != nil || got != want {
			t.Fatalf("%s：%q, %v", literal, got, err)
		}
	}
	if _, err := accessidentity.ParseCustomerCapability("delivery_pre_advice_submission"); !errors.Is(err, accessidentity.ErrCustomerCapabilityUnknown) {
		t.Fatalf("小写：err = %v", err)
	}
}

// Covers: AT-NO-069（铸造半边）与 ADR-0139 决定二、五——持这一口授予的客户端铸出这一口的信封：租户与客户账户只取自册上的
// 绑定，来源由客户端主体推出，来源请求键是载荷里的接入请求标识去两端空白、此外一字不动；渠道请求里自报的租户与账户进不了
// 信封。准入那一步问的是（CUSTOMER_COMMAND，这一口的能力词）。
func TestPreAdviceLinesMintTheirEnvelopesFromTheBinding(t *testing.T) {
	for name, line := range preAdviceLines {
		t.Run(name, func(t *testing.T) {
			var asked []accessidentity.AdmissionRequirement
			minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
				customerRegistryFake{standing: customerStandingHolding(t, line.capability), found: true},
				admissionProbe{admitted: true, asked: &asked})
			reader := &requestReader{values: map[string]string{
				accessidentity.AccessRequestIdentifier: "  SYN-ACCESS-REQUEST-PA-0001\t",
				"tenantId":                             "TENANT-PROD-9",
				"customerAccountId":                    "ACCOUNT-PROD-9",
			}}
			envelope, err := line.mint(minter, reader)
			if err != nil {
				t.Fatalf("持授予的客户端：%v", err)
			}
			if envelope.TenantID() != customerTenant || envelope.CustomerAccountID() != customerAccount {
				t.Fatalf("信封身份 = (%q, %q), want 册上绑定的 (%q, %q)", envelope.TenantID(), envelope.CustomerAccountID(), customerTenant, customerAccount)
			}
			if want := customerIssuer + "#SYN-CUSTOMER-CLIENT-01"; envelope.Source() != want {
				t.Fatalf("来源 = %q, want %q", envelope.Source(), want)
			}
			if envelope.RequestKey() != "SYN-ACCESS-REQUEST-PA-0001" {
				t.Fatalf("来源请求键 = %q, want 去两端空白的接入请求标识", envelope.RequestKey())
			}
			want := accessidentity.AdmissionRequirement{Capability: accessidentity.CustomerCommandAdmissionCapability, FactKind: line.capability.String()}
			if len(asked) != 1 || asked[0] != want {
				t.Fatalf("准入要求 = %+v, want 只问 %+v", asked, want)
			}
		})
	}
}

// Covers: AT-NO-069（铸造半边）、票 ps-external-mark-relations/09 完成判据「令牌无效 401、不在册或没有这一格授予 403、不在准入
// 范围 403、依赖读不动 503、发行方未配置 403」——各格只由自己的哨兵认出、互不顶替；只持另一口或委托提交的授予答未授予；授予
// 不过时不问准入范围；认证没过之前一个字的请求都不读。
func TestPreAdviceLinesAnswerTheirOwnGrades(t *testing.T) {
	for name, line := range preAdviceLines {
		t.Run(name, func(t *testing.T) {
			granted := customerRegistryFake{standing: customerStandingHolding(t, line.capability), found: true}
			unbound := customerVerifierFake{verified: verifiedCustomerToken(t, false)}
			mustNotAsk := admissionProbe{err: errors.New("admission must not be asked before the grant passes")}
			for grade, testCase := range map[string]struct {
				verifier  accessidentity.CustomerClientCredentialVerifier
				registry  customerRegistryFake
				admission accessidentity.AdmissionScope
				want      string
			}{
				"issuer unset":           {accessidentity.UnconfiguredCustomerClientCredentialVerifier{}, granted, mustNotAsk, "not configured"},
				"token rejected":         {customerVerifierFake{err: accessidentity.ErrCredentialRejected}, granted, mustNotAsk, "credential rejected"},
				"key set unreachable":    {customerVerifierFake{err: accessidentity.ErrCredentialVerifierUnavailable}, granted, mustNotAsk, "verifier unavailable"},
				"not on the register":    {unbound, customerRegistryFake{}, mustNotAsk, "not granted"},
				"register unreadable":    {unbound, customerRegistryFake{err: errors.New("connection refused")}, mustNotAsk, "registry unavailable"},
				"only the other line":    {unbound, customerRegistryFake{standing: customerStandingHolding(t, line.other), found: true}, mustNotAsk, "not granted"},
				"only shipment requests": {unbound, customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityShipmentRequestSubmission), found: true}, mustNotAsk, "not granted"},
				"outside admission":      {unbound, granted, accessidentity.UnconfiguredAdmissionScope{}, "outside admission"},
				"admission unreadable":   {unbound, granted, admissionProbe{err: errors.New("governance register unreachable")}, "admission unavailable"},
			} {
				t.Run(grade, func(t *testing.T) {
					minter := newPreAdviceMinter(t, testCase.verifier, testCase.registry, testCase.admission)
					reader := &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "SYN-ACCESS-REQUEST-PA-0001"}}
					_, err := line.mint(minter, reader)
					assertCustomerGrade(t, err, testCase.want)
					if reader.read {
						t.Fatal("认证没过却读了请求——未认证的调用方会看到载荷校验的结果")
					}
				})
			}
		})
	}
}

// Covers: ADR-0139 决定五与票 ps-external-mark-relations/09 做什么「铸造」末句——预报两口的来源请求键都取接入请求标识，缺席或空白
// 答推导不出，不折进令牌或授予那几格；读请求自己的错原样交回；不给读请求的口答缺件。
func TestPreAdviceLinesNeedAnAccessRequestIdentifier(t *testing.T) {
	for name, line := range preAdviceLines {
		t.Run(name, func(t *testing.T) {
			minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
				customerRegistryFake{standing: customerStandingHolding(t, line.capability), found: true}, admissionProbe{admitted: true})
			for absence, values := range map[string]map[string]string{
				"absent": {},
				"blank":  {accessidentity.AccessRequestIdentifier: " \t "},
			} {
				if _, err := line.mint(minter, &requestReader{values: values}); !errors.Is(err, accessidentity.ErrRequestKeyNotDerived) {
					t.Fatalf("%s：err = %v, want ErrRequestKeyNotDerived", absence, err)
				}
			}
			malformed := errors.New("payload is not JSON")
			if _, err := line.mint(minter, &requestReader{err: malformed}); !errors.Is(err, malformed) {
				t.Fatalf("读请求的错 = %v, want 原样交回", err)
			}
		})
	}
	minter := newPreAdviceMinter(t, customerVerifierFake{}, customerRegistryFake{}, admissionProbe{})
	credential := accessidentity.NewIntegrationClientCredential("t", nil)
	if _, err := minter.MintDeliveryPreAdviceSubmission(context.Background(), credential, nil); !errors.Is(err, accessidentity.ErrNilDependency) {
		t.Fatalf("提交口不给读请求的口：err = %v", err)
	}
	if _, err := minter.MintDeliveryPreAdviceWithdrawal(context.Background(), credential, nil); !errors.Is(err, accessidentity.ErrNilDependency) {
		t.Fatalf("撤回口不给读请求的口：err = %v", err)
	}
}

// Covers: ADR-0180 决定四末项、ADR-0139 决定八——首方族在铸造侧判准入的两口登得进准入范围对照（能力 CUSTOMER_COMMAND，事实
// 类型取这一口的能力词）；委托提交不在铸造侧判准入，它的词登不进；词表外的串、拿首方族的能力词顶能力格，也登不进。
func TestCustomerCommandAdmissionWordsAreOnlyThoseMintedLinesAskFor(t *testing.T) {
	interval := clientIntervalOf(t, customerGrantStartsAt, time.Time{})
	for _, capability := range []accessidentity.CustomerCapability{
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
	} {
		requirement := accessidentity.AdmissionRequirement{Capability: accessidentity.CustomerCommandAdmissionCapability, FactKind: capability.String()}
		if _, err := accessidentity.NewAdmissionScopeMapping(customerTenant, "SYN-MAP-1", requirement, "SYN-PILOT-SCOPE/customer-commands@v1", interval, "SYN-BASIS"); err != nil {
			t.Fatalf("%s：%v", capability, err)
		}
	}
	for name, requirement := range map[string]accessidentity.AdmissionRequirement{
		"shipment submission is judged by its orchestration": {Capability: accessidentity.CustomerCommandAdmissionCapability, FactKind: accessidentity.CustomerCapabilityShipmentRequestSubmission.String()},
		"unknown customer word":                              {Capability: accessidentity.CustomerCommandAdmissionCapability, FactKind: "CLAIM_SUBMISSION"},
		"customer word in the capability column":             {Capability: accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission.String(), FactKind: accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission.String()},
		"lower-cased capability":                             {Capability: "customer_command", FactKind: accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission.String()},
	} {
		if _, err := accessidentity.NewAdmissionScopeMapping(customerTenant, "SYN-MAP-1", requirement, "SYN-PILOT-SCOPE/customer-commands@v1", interval, "SYN-BASIS"); !errors.Is(err, accessidentity.ErrAdmissionRequirementUnknown) {
			t.Fatalf("%s：err = %v, want ErrAdmissionRequirementUnknown", name, err)
		}
	}
}
