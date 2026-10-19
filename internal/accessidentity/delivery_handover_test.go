package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// 本文件证送站交接单在接入身份上的三处词（票 ps-external-mark-relations/12；ADR-0212 决定三、九）：运营决定多一种决定「送站交接
// 确认」，首方客户面多一项能力「送站交接确认」，作业事实登记面的交接单开立、归入、关闭各是一种准入事实类型。合成主体只记 `S`。

// Covers: 票 12 做什么第 2 项——「送站交接确认」是运营决定的一种，字面量大小写不折叠；按这一种授予的操作者不因此持有别的决定。
func TestDeliveryHandoverConfirmationIsAnOperationDecisionKind(t *testing.T) {
	kind, err := accessidentity.ParseDecisionKind("DELIVERY_HANDOVER_CONFIRMATION")
	if err != nil || kind != accessidentity.DecisionDeliveryHandoverConfirmation {
		t.Fatalf("ParseDecisionKind = %q, %v", kind, err)
	}
	if _, err := accessidentity.ParseDecisionKind("delivery_handover_confirmation"); !errors.Is(err, accessidentity.ErrDecisionKindUnknown) {
		t.Fatalf("小写：err = %v", err)
	}
	requirement := accessidentity.AdmissionRequirement{Capability: accessidentity.CapabilityOperationDecision.String(), FactKind: kind.String()}
	interval := clientIntervalOf(t, customerGrantStartsAt, time.Time{})
	if _, err := accessidentity.NewAdmissionScopeMapping(customerTenant, "SYN-MAP-1", requirement, "SYN-PILOT-SCOPE/operation-decisions@v1", interval, "SYN-BASIS"); err != nil {
		t.Fatalf("运营决定这一种登不进准入范围对照：%v", err)
	}
}

// Covers: 票 12 做什么第 3 项「一口一词」——客户面能力多一个词「送站交接确认」，与委托提交、预报两口同在一张封闭词表里。
func TestDeliveryHandoverConfirmationJoinsTheCustomerCapabilities(t *testing.T) {
	got, err := accessidentity.ParseCustomerCapability("DELIVERY_HANDOVER_CONFIRMATION")
	if err != nil || got != accessidentity.CustomerCapabilityDeliveryHandoverConfirmation {
		t.Fatalf("ParseCustomerCapability = %q, %v", got, err)
	}
	if _, err := accessidentity.ParseCustomerCapability("delivery_handover_confirmation"); !errors.Is(err, accessidentity.ErrCustomerCapabilityUnknown) {
		t.Fatalf("小写：err = %v", err)
	}
}

func mintHandoverConfirmation(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SourceEnvelope, error) {
	envelope, err := minter.MintDeliveryHandoverConfirmation(context.Background(),
		accessidentity.NewIntegrationClientCredential("presented.customer.token", nil), reader.channelRequest)
	return envelope.Envelope(), err
}

// Covers: `AT-NO-073`（铸造半边）、票 12 做什么第 3 项——持这一口授予的客户端铸出送站交接确认的信封：租户与客户账户只取自册上
// 的绑定，来源请求键取接入请求标识去两端空白；准入在铸造侧判，问的是（CUSTOMER_COMMAND，DELIVERY_HANDOVER_CONFIRMATION）。
func TestTheHandoverConfirmationLineMintsItsEnvelopeFromTheBinding(t *testing.T) {
	var asked []accessidentity.AdmissionRequirement
	minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
		customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityDeliveryHandoverConfirmation), found: true},
		admissionProbe{admitted: true, asked: &asked})
	reader := &requestReader{values: map[string]string{
		accessidentity.AccessRequestIdentifier: " SYN-ACCESS-REQUEST-HN-0001 ",
		"customerAccountId":                    "ACCOUNT-PROD-9",
	}}
	envelope, err := mintHandoverConfirmation(minter, reader)
	if err != nil {
		t.Fatalf("持授予的客户端：%v", err)
	}
	if envelope.TenantID() != customerTenant || envelope.CustomerAccountID() != customerAccount {
		t.Fatalf("信封身份 = (%q, %q), want 册上绑定的 (%q, %q)", envelope.TenantID(), envelope.CustomerAccountID(), customerTenant, customerAccount)
	}
	if envelope.RequestKey() != "SYN-ACCESS-REQUEST-HN-0001" {
		t.Fatalf("来源请求键 = %q", envelope.RequestKey())
	}
	want := accessidentity.AdmissionRequirement{
		Capability: accessidentity.CustomerCommandAdmissionCapability,
		FactKind:   accessidentity.CustomerCapabilityDeliveryHandoverConfirmation.String(),
	}
	if len(asked) != 1 || asked[0] != want {
		t.Fatalf("准入要求 = %+v, want 只问 %+v", asked, want)
	}
	if _, err := accessidentity.NewAdmissionScopeMapping(customerTenant, "SYN-MAP-1", want, "SYN-PILOT-SCOPE/customer-commands@v1",
		clientIntervalOf(t, customerGrantStartsAt, time.Time{}), "SYN-BASIS"); err != nil {
		t.Fatalf("这一口的准入要求登不进对照：%v", err)
	}
}

// Covers: 票 12 完成判据「信封各格」的客户面一族——令牌无效、不在册或没有这一口的授予（只持预报或委托提交的授予也算）、不在准入
// 范围、依赖读不动、发行方未配置各答其格；认证没过之前不读请求；接入请求标识缺席答推导不出。
func TestTheHandoverConfirmationLineAnswersItsOwnGrades(t *testing.T) {
	granted := customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityDeliveryHandoverConfirmation), found: true}
	unbound := customerVerifierFake{verified: verifiedCustomerToken(t, false)}
	mustNotAsk := admissionProbe{err: errors.New("admission must not be asked before the grant passes")}
	preAdviceOnly := customerRegistryFake{standing: customerStandingHolding(t,
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission, accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
		accessidentity.CustomerCapabilityShipmentRequestSubmission), found: true}
	for grade, testCase := range map[string]struct {
		verifier  accessidentity.CustomerClientCredentialVerifier
		registry  customerRegistryFake
		admission accessidentity.AdmissionScope
		want      string
	}{
		"issuer unset":          {accessidentity.UnconfiguredCustomerClientCredentialVerifier{}, granted, mustNotAsk, "not configured"},
		"token rejected":        {customerVerifierFake{err: accessidentity.ErrCredentialRejected}, granted, mustNotAsk, "credential rejected"},
		"not on the register":   {unbound, customerRegistryFake{}, mustNotAsk, "not granted"},
		"only the other lines":  {unbound, preAdviceOnly, mustNotAsk, "not granted"},
		"outside admission":     {unbound, granted, accessidentity.UnconfiguredAdmissionScope{}, "outside admission"},
		"admission unreadable":  {unbound, granted, admissionProbe{err: errors.New("governance register unreachable")}, "admission unavailable"},
		"register unreadable":   {unbound, customerRegistryFake{err: errors.New("connection refused")}, mustNotAsk, "registry unavailable"},
		"key set unreachable":   {customerVerifierFake{err: accessidentity.ErrCredentialVerifierUnavailable}, granted, mustNotAsk, "verifier unavailable"},
		"operator token is not": {customerVerifierFake{err: accessidentity.ErrCredentialRejected}, granted, mustNotAsk, "credential rejected"},
	} {
		t.Run(grade, func(t *testing.T) {
			minter := newPreAdviceMinter(t, testCase.verifier, testCase.registry, testCase.admission)
			reader := &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "SYN-ACCESS-REQUEST-HN-0001"}}
			_, err := mintHandoverConfirmation(minter, reader)
			assertCustomerGrade(t, err, testCase.want)
			if reader.read {
				t.Fatal("认证没过却读了请求")
			}
		})
	}
	minter := newPreAdviceMinter(t, unbound, granted, admissionProbe{admitted: true})
	if _, err := mintHandoverConfirmation(minter, &requestReader{values: map[string]string{}}); !errors.Is(err, accessidentity.ErrRequestKeyNotDerived) {
		t.Fatalf("没有接入请求标识：err = %v, want ErrRequestKeyNotDerived", err)
	}
	if _, err := minter.MintDeliveryHandoverConfirmation(context.Background(), accessidentity.NewIntegrationClientCredential("t", nil), nil); !errors.Is(err, accessidentity.ErrNilDependency) {
		t.Fatalf("不给读请求的口：err = %v", err)
	}
}

// Covers: 票 12 地盘「作业事实登记的事实类型加开立、归入、关闭」与 ADR-0180 决定四末项「一个能力词，事实类型逐口取」——作业事实
// 登记面判准入的三种事实各是一个字面量，准入要求取（WORK_FACT_REGISTRATION，事实类型），登得进准入范围对照；词表外的串、大小写
// 不同、拿事实类型顶能力格都登不进。
func TestHandoverNoteWorkFactsAreAdmissionRequirements(t *testing.T) {
	interval := clientIntervalOf(t, customerGrantStartsAt, time.Time{})
	for literal, kind := range map[string]accessidentity.WorkFactKind{
		"DELIVERY_HANDOVER_NOTE_OPENING":    accessidentity.WorkFactDeliveryHandoverNoteOpening,
		"DELIVERY_HANDOVER_NOTE_ASSIGNMENT": accessidentity.WorkFactDeliveryHandoverNoteAssignment,
		"DELIVERY_HANDOVER_NOTE_CLOSURE":    accessidentity.WorkFactDeliveryHandoverNoteClosure,
	} {
		parsed, err := accessidentity.ParseWorkFactKind(literal)
		if err != nil || parsed != kind {
			t.Fatalf("ParseWorkFactKind(%q) = %q, %v", literal, parsed, err)
		}
		requirement := accessidentity.WorkFactAdmission(kind)
		if requirement == nil || *requirement != (accessidentity.AdmissionRequirement{Capability: "WORK_FACT_REGISTRATION", FactKind: literal}) {
			t.Fatalf("WorkFactAdmission(%s) = %+v", kind, requirement)
		}
		if _, err := accessidentity.NewAdmissionScopeMapping(customerTenant, "SYN-MAP-1", *requirement, "SYN-PILOT-SCOPE/work-facts@v1", interval, "SYN-BASIS"); err != nil {
			t.Fatalf("%s 登不进准入范围对照：%v", kind, err)
		}
	}
	if _, err := accessidentity.ParseWorkFactKind("delivery_handover_note_opening"); !errors.Is(err, accessidentity.ErrWorkFactKindUnknown) {
		t.Fatalf("小写：err = %v", err)
	}
	if accessidentity.WorkFactAdmission("NODE_RECEPTION") != nil {
		t.Fatal("词表外的作业事实也交出了准入要求")
	}
	for name, requirement := range map[string]accessidentity.AdmissionRequirement{
		"unknown work fact":            {Capability: "WORK_FACT_REGISTRATION", FactKind: "NODE_RECEPTION"},
		"fact kind as the capability":  {Capability: "DELIVERY_HANDOVER_NOTE_OPENING", FactKind: "DELIVERY_HANDOVER_NOTE_OPENING"},
		"lower-cased capability":       {Capability: "work_fact_registration", FactKind: "DELIVERY_HANDOVER_NOTE_OPENING"},
		"work fact under the decision": {Capability: accessidentity.CapabilityOperationDecision.String(), FactKind: "DELIVERY_HANDOVER_NOTE_OPENING"},
	} {
		if _, err := accessidentity.NewAdmissionScopeMapping(customerTenant, "SYN-MAP-1", requirement, "SYN-PILOT-SCOPE/work-facts@v1", interval, "SYN-BASIS"); !errors.Is(err, accessidentity.ErrAdmissionRequirementUnknown) {
			t.Fatalf("%s：err = %v, want ErrAdmissionRequirementUnknown", name, err)
		}
	}
}
