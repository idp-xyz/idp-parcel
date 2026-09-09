package productionhandoff_test

import (
	"testing"
	"time"

	pshandoff "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/productionhandoff"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: 票 wiring-baseline-remainder/01 判据 1 与 ADR-0128 决定三——通往他方权威的通道未配置时，
// 适配器对任何投递都答`通道未配置`那一格，且答复不随范围、对方或尝试身份变；不带任何对方给的引用
// （通道都没配置，手上不可能有对方给的东西），不报错（error 那格的恢复动作是重试，重试改不了一条
// 没人配置的通道）。答复随投递内容变化是它偷偷投递了的第一个征兆。
func TestUnconfiguredChannelAnswersChannelUnconfiguredIdenticallyForEveryDelivery(t *testing.T) {
	channel := pshandoff.UnconfiguredOtherProductionAuthorityChannel{}

	for name, delivery := range map[string]ports.ProductionHandoffDelivery{
		"first scope to authority A": {
			AttemptID:       value(t, domain.NewHandoffAttemptID, "PS-HANDOFF/decision-1"),
			Scope:           scope(t, "scope-ref-1", "scope-1"),
			TargetAuthority: value(t, domain.NewProductionAuthorityReference, "authority-a"),
		},
		"another scope to authority B": {
			AttemptID:       value(t, domain.NewHandoffAttemptID, "PS-HANDOFF/decision-2"),
			Scope:           scope(t, "scope-ref-2", "scope-2"),
			TargetAuthority: value(t, domain.NewProductionAuthorityReference, "authority-b"),
		},
		"zero delivery": {},
	} {
		observed, err := channel.DeliverAdmissionScope(t.Context(), delivery)
		if err != nil {
			t.Fatalf("%s：未配置通道不该报错：%v", name, err)
		}
		if observed.Observation != domain.HandoffObservationChannelUnconfigured {
			t.Fatalf("%s：observation = %s, want CHANNEL_UNCONFIGURED——没有通道时既不能答确认也不能答对方失败", name, observed.Observation)
		}
		if observed.ConfirmedScopeDigest.String() != "" || observed.ConfirmationRef.String() != "" || observed.QueryRef.String() != "" {
			t.Fatalf("%s：未配置却带出了对方给的引用 %q / %q / %q", name, observed.ConfirmedScopeDigest, observed.ConfirmationRef, observed.QueryRef)
		}
		if !observed.EffectiveAt.IsZero() {
			t.Fatalf("%s：未配置却带出了生效时刻 %v", name, observed.EffectiveAt)
		}
	}
}

// Covers: 同上判据的另一半——未配置适配器的答复必须过得了 AssessSafeHandoff 的证据形，形成的是
// 带`通道未配置`原因的未决评估而不是构造错误；否则编排拿到它会报错，「未配置」就从一格业务答案
// 退化成一次故障，恢复动作被指错（ADR-0029）。
func TestUnconfiguredChannelAnswerFormsAnUnresolvedAssessmentWithChannelUnconfigured(t *testing.T) {
	channel := pshandoff.UnconfiguredOtherProductionAuthorityChannel{}
	delivery := ports.ProductionHandoffDelivery{
		AttemptID:       value(t, domain.NewHandoffAttemptID, "PS-HANDOFF/decision-1"),
		Scope:           scope(t, "scope-ref-1", "scope-1"),
		TargetAuthority: value(t, domain.NewProductionAuthorityReference, "authority-a"),
	}
	observed, err := channel.DeliverAdmissionScope(t.Context(), delivery)
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}

	assessment, err := domain.AssessSafeHandoff(domain.SafeHandoffAssessmentSpec{
		AttemptID:            delivery.AttemptID,
		Scope:                delivery.Scope,
		TargetAuthority:      delivery.TargetAuthority,
		Observation:          observed.Observation,
		ConfirmedScopeDigest: observed.ConfirmedScopeDigest,
		ConfirmationRef:      observed.ConfirmationRef,
		QueryRef:             observed.QueryRef,
		ContinuationRef:      value(t, domain.NewOwnershipContinuationReference, "CONT-PS-HANDOFF/decision-1"),
		EffectiveAt:          observed.EffectiveAt,
		AssessedAt:           time.Date(2026, 9, 9, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("未配置通道的答复过不了评估的证据形：%v", err)
	}
	if assessment.Status() != domain.SafeHandoffUnresolved {
		t.Fatalf("status = %s, want UNRESOLVED", assessment.Status())
	}
	if reason, ok := assessment.UnresolvedReason(); !ok || reason != domain.HandoffUnresolvedChannelUnconfigured {
		t.Fatalf("unresolved reason = %s, %t; want CHANNEL_NOT_CONFIGURED", reason, ok)
	}
	if _, ok := assessment.ConfirmationReference(); ok {
		t.Fatal("未配置的交接冒出了确认引用")
	}
}

type stringValue interface{ String() string }

func value[T stringValue](t *testing.T, constructor func(string) (T, error), raw string) T {
	t.Helper()
	got, err := constructor(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return got
}

func scope(t *testing.T, reference, digest string) domain.AdmissionScope {
	t.Helper()
	got, err := domain.NewAdmissionScope(
		value(t, domain.NewAdmissionScopeReference, reference),
		value(t, domain.NewAdmissionScopeDigest, digest),
	)
	if err != nil {
		t.Fatalf("admission scope: %v", err)
	}
	return got
}
