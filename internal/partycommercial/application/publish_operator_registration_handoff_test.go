package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件钉的是 ADR-0094 决定四在 party-commercial 这一半的落点：时点策略声明随发布落库时，
// 发布编排在同一次 Handle 里向交接口交一份「参数已登记」意图（票 first-tenant-runway/07 D4
// 第二笔第 1 项）。停在`等待运营登记`的委托靠这封信被重驱；没有它，登记落了库也没人知道。

// operatorRegistrationHandoffDouble 记录发布编排交出的意图。fail 非空时交接失败，用例借它证
// 「信封入不了队则整项出错」。
type operatorRegistrationHandoffDouble struct {
	intents []ports.OperatorRegistrationCompletedIntent
	fail    error
}

func (double *operatorRegistrationHandoffDouble) HandOffOperatorRegistrationCompleted(
	_ context.Context,
	intent ports.OperatorRegistrationCompletedIntent,
) error {
	if double.fail != nil {
		return double.fail
	}
	double.intents = append(double.intents, intent)
	return nil
}

func reachabilityAsOfPolicy(t *testing.T) domain.AsOfPolicy {
	t.Helper()
	policy, err := domain.NewAsOfPolicy(domain.NetworkReachabilityJudgment,
		pcValue(t, domain.NewAsOfSemanticsReference, "AT_ACCEPTANCE"),
		pcValue(t, domain.NewAsOfPolicyVersion, "asof-policy/v1"))
	if err != nil {
		t.Fatalf("时点锚：%v", err)
	}
	return policy
}

func publishWithAsOf(t *testing.T, handler *application.PublishCommercialAuthorityHandler) application.PublishCommercialAuthorityResult {
	t.Helper()
	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
		Approval:     publishApproval(t, "rules-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			AsOfPolicies: []domain.AsOfPolicy{reachabilityAsOfPolicy(t)},
		},
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	return result
}

// Covers: ADR-0094 决定四 —— 时点策略声明落库，同一次 Handle 交出一份「参数已登记」意图：
// 种类是时点策略、承载版本是取效后的那一版、发生时刻是发布时刻。
func TestPublishingAsOfPoliciesHandsOffAnOperatorRegistrationIntent(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handoff := &operatorRegistrationHandoffDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, handoff)

	result := publishWithAsOf(t, handler)
	if result.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
	}
	if len(handoff.intents) != 1 {
		t.Fatalf("交接了 %d 份意图，want 1", len(handoff.intents))
	}
	intent := handoff.intents[0]
	if intent.Kind != ports.AsOfPolicyRegistered {
		t.Fatalf("种类 = %q, want AS_OF_POLICY", intent.Kind)
	}
	version, published := result.Version()
	if !published {
		t.Fatal("结果没有版本")
	}
	if intent.Registration.Tenant() != version.Tenant() ||
		intent.Registration.ObjectID() != version.ObjectID() ||
		intent.Registration.Version() != version.Version() {
		t.Fatalf("承载版本 = %s/%s@%s, want %s/%s@%s",
			intent.Registration.Tenant(), intent.Registration.ObjectID(), intent.Registration.Version(),
			version.Tenant(), version.ObjectID(), version.Version())
	}
	if intent.Registration.Status() != domain.CommercialVersionEffective {
		t.Fatal("承载版本不是取效后的那一版")
	}
	if !intent.RegisteredAt.Equal(pubNow) {
		t.Fatalf("发生时刻 = %s, want 发布时刻 %s", intent.RegisteredAt, pubNow)
	}
}

// Covers: 没有时点声明的发布不发信封——信封说的是「这一类参数落库了」，没落就没得说；发一封
// 空的会让消费门白跑一轮重驱。
func TestPublishingWithoutAsOfPoliciesHandsOffNothing(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handoff := &operatorRegistrationHandoffDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, handoff)

	if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
		Approval:     publishApproval(t, "rules-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
	}); err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if len(handoff.intents) != 0 {
		t.Fatalf("没有时点声明却交接了 %d 份意图", len(handoff.intents))
	}
}

// Covers: ADR-0043 —— 重放重发同一份。时点通道答`已登记`（同内容重放）照样交意图，认领由
// 适配器按同一个 ID 去重；答`内容冲突`则一行没写，不交——冲突不是登记。
func TestReplayedAsOfPoliciesResendWhileConflictsStaySilent(t *testing.T) {
	t.Run("replayed", func(t *testing.T) {
		registry := &publicationRegistryDouble{declarationOutcome: ports.DeclarationAlreadyRegistered}
		handoff := &operatorRegistrationHandoffDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, handoff)
		publishWithAsOf(t, handler)
		if len(handoff.intents) != 1 {
			t.Fatalf("重放交接了 %d 份意图，want 1", len(handoff.intents))
		}
	})
	t.Run("conflict", func(t *testing.T) {
		registry := &publicationRegistryDouble{declarationOutcome: ports.DeclarationContentConflict}
		handoff := &operatorRegistrationHandoffDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, handoff)
		publishWithAsOf(t, handler)
		if len(handoff.intents) != 0 {
			t.Fatalf("内容冲突却交接了 %d 份意图", len(handoff.intents))
		}
	})
}

// Covers: ADR-0094 决定四「否则不许落地」—— 信封入不了队，整项出错让调用方的事务回滚，登记
// 不得单独落库。
func TestAFailedHandoffFailsThePublication(t *testing.T) {
	registry := &publicationRegistryDouble{}
	boom := errors.New("outbox unavailable")
	handoff := &operatorRegistrationHandoffDouble{fail: boom}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, handoff)

	_, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
		Approval:     publishApproval(t, "rules-1"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: application.CommercialDeclarations{
			AsOfPolicies: []domain.AsOfPolicy{reachabilityAsOfPolicy(t)},
		},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("交接失败应上抛让事务回滚，实得：%v", err)
	}
}
