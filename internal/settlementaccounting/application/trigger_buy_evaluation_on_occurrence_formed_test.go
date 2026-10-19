package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0208 的发起半边：发生项已形成之后，费用项目取触发册那一行、主要范围取协议的商业范围、发生项与协议原样取自
// 命令；没登原因或没登费用项目答未配置、不读协议、不铸请求；依赖答不出答未决并点名；协议在 PC 不认是矛盾、协议范围
// 答出确定性坏答案，都上抛；修订版不发起；交信封失败答未决让调用方整笔回滚。夹具全是合成串，证据只记 S。

// agreementScopeDouble 按协议指称答范围；err 非空时如实报错。calls 数被问了几次——未配置的格不该去问。
type agreementScopeDouble struct {
	scopes map[string]string
	err    error
	calls  int
}

func (double *agreementScopeDouble) LoadSupplierAgreementScope(
	_ context.Context,
	_ domain.TenantID,
	agreement domain.SupplierAgreementReference,
) (domain.PrimaryScopeReference, bool, error) {
	double.calls++
	if double.err != nil {
		return domain.PrimaryScopeReference{}, false, double.err
	}
	scope, found := double.scopes[agreement.String()]
	if !found {
		return domain.PrimaryScopeReference{}, false, nil
	}
	reference, err := domain.NewPrimaryScopeReference(scope)
	return reference, err == nil, err
}

const trunkAgreement = "SYN-SUPPLIER-TRUNK-CN-SG-01/v1"

func registeredFulfillmentRow() buyEvaluationTriggerViewDouble {
	return buyEvaluationTriggerViewDouble{
		moment:  domain.BuyEvaluationTriggerOnOccurrenceFormed,
		found:   true,
		reason:  "ACTUAL_FULFILLMENT",
		feeItem: "SYN-FEE-ITEM-TRUNK-COST-01",
	}
}

func trunkScope() *agreementScopeDouble {
	return &agreementScopeDouble{scopes: map[string]string{trunkAgreement: "SYN-SCOPE-01"}}
}

func occurrenceFormedCommand(t *testing.T) application.TriggerBuyEvaluationOnOccurrenceFormedCommand {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		value(t, domain.NewChargeOccurrenceID, "SYN-OCC-TRUNK-01"),
		value(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		value(t, domain.NewOccurrenceVersion, "V1"),
		time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("发生项引用：%v", err)
	}
	return application.TriggerBuyEvaluationOnOccurrenceFormedCommand{
		TenantID:   value(t, domain.NewTenantID, "SYN-TENANT-01"),
		Occurrence: occurrence,
		Agreement:  value(t, domain.NewSupplierAgreementReference, trunkAgreement),
	}
}

func newOccurrenceFormedTrigger(
	t *testing.T,
	view buyEvaluationTriggerViewDouble,
	scopes *agreementScopeDouble,
	fixture *evaluationRequestFixture,
) *application.TriggerBuyEvaluationOnOccurrenceFormedHandler {
	t.Helper()
	handler, err := application.NewTriggerBuyEvaluationOnOccurrenceFormedHandler(application.TriggerBuyEvaluationOnOccurrenceFormedDeps{
		Triggers:   view,
		Agreements: scopes,
		Requests:   fixture.handler,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// Covers: 正例——一份已形成的发生项发起恰一份 BUY 请求，费用项目取触发册、主要范围取协议的商业范围、发生项与协议取自
// 发生项、请求方是本路自己；同一份再来一次答已存在、不铸第二个 ID，仍算发起。
func TestAFormedOccurrenceFiresABuyEvaluationRequestFromItsOwnFacts(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	handler := newOccurrenceFormedTrigger(t, registeredFulfillmentRow(), trunkScope(), fixture)
	command := occurrenceFormedCommand(t)

	result, err := handler.Trigger(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.BuyEvaluationTriggerFired {
		t.Fatalf("outcome = %s（未配置 %s 未决 %s）", result.Outcome(), result.UnconfiguredReason(), result.UndecidedReason())
	}
	requested, fired := result.Request()
	record, recorded := requested.Request()
	if !fired || !recorded || requested.Outcome() != application.EvaluationRequested {
		t.Fatalf("请求 outcome = %s fired=%v recorded=%v", requested.Outcome(), fired, recorded)
	}
	sources := record.Request.Sources()
	if sources.FeeItem.String() != "SYN-FEE-ITEM-TRUNK-COST-01" || sources.Agreement.String() != trunkAgreement ||
		sources.Occurrence != command.Occurrence || record.Request.Scope().String() != "SYN-SCOPE-01" ||
		record.Request.RequestedBy().String() != "settlement-accounting/buy-evaluation-on-occurrence-formed" {
		t.Fatalf("请求 = 项目 %s 协议 %s 发生项 %+v 范围 %s 请求方 %s",
			sources.FeeItem, sources.Agreement, sources.Occurrence, record.Request.Scope(), record.Request.RequestedBy())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("信封 %d 封, want 1", len(fixture.handoff.intents))
	}

	replay, err := handler.Trigger(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := replay.Request()
	if replay.Outcome() != application.BuyEvaluationTriggerFired || again.Outcome() != application.EvaluationRequestExisting {
		t.Fatalf("重放 outcome = %s / %s", replay.Outcome(), again.Outcome())
	}
	if fixture.identity.next != 1 {
		t.Fatalf("重放铸了第 %d 个 ID", fixture.identity.next)
	}
}

// Covers: 未配置两格——没登这个原因、登了却没登费用项目——都入账：不问协议、不铸 ID、不交信封，并点名缺哪一格。
func TestAnUnregisteredReasonOrFeeItemStaysUnconfiguredWithoutAskingTheAgreement(t *testing.T) {
	withoutFeeItem := registeredFulfillmentRow()
	withoutFeeItem.feeItem = ""
	otherReason := registeredFulfillmentRow()
	otherReason.reason = "FAILED_ATTEMPT"
	for name, testCase := range map[string]struct {
		view buyEvaluationTriggerViewDouble
		want application.BuyEvaluationTriggerUnconfiguredReason
	}{
		"空册":      {buyEvaluationTriggerViewDouble{}, application.BuyEvaluationTriggerReasonNotRegistered},
		"只登了别的原因": {otherReason, application.BuyEvaluationTriggerReasonNotRegistered},
		"没登费用项目":  {withoutFeeItem, application.BuyEvaluationTriggerFeeItemNotRegistered},
	} {
		fixture := newEvaluationRequestFixture(t)
		scopes := trunkScope()
		result, err := newOccurrenceFormedTrigger(t, testCase.view, scopes, fixture).Trigger(t.Context(), occurrenceFormedCommand(t))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.Outcome() != application.BuyEvaluationTriggerUnconfigured || result.UnconfiguredReason() != testCase.want {
			t.Fatalf("%s: outcome = %s / %s, want 未配置 / %s", name, result.Outcome(), result.UnconfiguredReason(), testCase.want)
		}
		if scopes.calls != 0 || fixture.identity.next != 0 || len(fixture.handoff.intents) != 0 {
			t.Fatalf("%s: 未配置仍问了协议 %d 次、铸了 %d 个 ID、交了 %d 封信", name, scopes.calls, fixture.identity.next, len(fixture.handoff.intents))
		}
	}
}

// Covers: 依赖答不出才是未决，并点名停在哪一口：触发册、协议范围读口、请求册、交信封。
func TestADependencyThatCannotAnswerLeavesTheTriggerUndecided(t *testing.T) {
	down := errors.New("dependency down")
	for name, testCase := range map[string]struct {
		view    buyEvaluationTriggerViewDouble
		scopes  *agreementScopeDouble
		arrange func(*evaluationRequestFixture)
		want    application.BuyEvaluationTriggerUndecidedReason
	}{
		"触发册": {buyEvaluationTriggerViewDouble{err: down}, trunkScope(), nil,
			application.BuyEvaluationTriggerRegisterUnavailable},
		"协议范围": {registeredFulfillmentRow(), &agreementScopeDouble{err: down}, nil,
			application.BuyEvaluationTriggerAgreementScopeUnavailable},
		"请求册": {registeredFulfillmentRow(), trunkScope(), func(fixture *evaluationRequestFixture) { fixture.registry.findErr = down },
			application.BuyEvaluationTriggerRequestUndecided},
		"交信封": {registeredFulfillmentRow(), trunkScope(), func(fixture *evaluationRequestFixture) { fixture.handoff.err = down },
			application.BuyEvaluationTriggerRequestHandoffUnavailable},
	} {
		fixture := newEvaluationRequestFixture(t)
		if testCase.arrange != nil {
			testCase.arrange(fixture)
		}
		result, err := newOccurrenceFormedTrigger(t, testCase.view, testCase.scopes, fixture).Trigger(t.Context(), occurrenceFormedCommand(t))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.Outcome() != application.BuyEvaluationTriggerUndecided || result.UndecidedReason() != testCase.want {
			t.Fatalf("%s: outcome = %s / %s, want 未决 / %s", name, result.Outcome(), result.UndecidedReason(), testCase.want)
		}
	}
}

// Covers: 协议在 PC 不认是两侧矛盾——上抛 ErrOccurrenceAgreementNotRegistered，不发起请求、不当未决。
func TestAnAgreementUnknownToPartyCommercialIsAContradiction(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	handler := newOccurrenceFormedTrigger(t, registeredFulfillmentRow(), &agreementScopeDouble{scopes: map[string]string{}}, fixture)
	_, err := handler.Trigger(t.Context(), occurrenceFormedCommand(t))
	if !errors.Is(err, application.ErrOccurrenceAgreementNotRegistered) {
		t.Fatalf("err = %v, want ErrOccurrenceAgreementNotRegistered", err)
	}
	if fixture.identity.next != 0 {
		t.Fatal("矛盾仍铸了请求 ID")
	}
}

// Covers: 协议范围读口答出确定性坏答案（译不过去、指称歧义）不是未决——原样上抛，不铸请求；重投同一份永远同一个结果，
// 当未决只会耗尽失败预算（ADR-0208 决定五）。
func TestAnUntranslatableAgreementScopeIsRaisedNotLeftUndecided(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	untranslatable := fmt.Errorf("load supplier agreement scope: %w", ports.ErrSupplierAgreementScopeUntranslatable)
	handler := newOccurrenceFormedTrigger(t, registeredFulfillmentRow(), &agreementScopeDouble{err: untranslatable}, fixture)
	result, err := handler.Trigger(t.Context(), occurrenceFormedCommand(t))
	if !errors.Is(err, ports.ErrSupplierAgreementScopeUntranslatable) {
		t.Fatalf("outcome = %s / %s err = %v, want 上抛 ErrSupplierAgreementScopeUntranslatable",
			result.Outcome(), result.UndecidedReason(), err)
	}
	if fixture.identity.next != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatalf("坏答案仍铸了 %d 个 ID、交了 %d 封信", fixture.identity.next, len(fixture.handoff.intents))
	}
}

// Covers: 有效性修订产生的那一版不经本路发起，入账；残缺命令答未受理。两格都不读任何册。
func TestARevisedOccurrenceAndAMalformedCommandFireNothing(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	scopes := trunkScope()
	handler := newOccurrenceFormedTrigger(t, registeredFulfillmentRow(), scopes, fixture)

	revised := occurrenceFormedCommand(t)
	revised.Revised = true
	result, err := handler.Trigger(t.Context(), revised)
	if err != nil || result.Outcome() != application.BuyEvaluationTriggerOccurrenceRevised {
		t.Fatalf("修订版 outcome = %s err = %v", result.Outcome(), err)
	}

	malformed := occurrenceFormedCommand(t)
	malformed.Agreement = domain.SupplierAgreementReference{}
	result, err = handler.Trigger(t.Context(), malformed)
	if err != nil || result.Outcome() != application.BuyEvaluationTriggerNotAccepted {
		t.Fatalf("残缺命令 outcome = %s err = %v", result.Outcome(), err)
	}
	if scopes.calls != 0 || fixture.identity.next != 0 {
		t.Fatalf("问了协议 %d 次、铸了 %d 个 ID", scopes.calls, fixture.identity.next)
	}
}

func TestTheOccurrenceFormedTriggerRefusesMissingDependencies(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	for name, deps := range map[string]application.TriggerBuyEvaluationOnOccurrenceFormedDeps{
		"触发册":  {Agreements: trunkScope(), Requests: fixture.handler},
		"协议范围": {Triggers: registeredFulfillmentRow(), Requests: fixture.handler},
		"请求编排": {Triggers: registeredFulfillmentRow(), Agreements: trunkScope()},
	} {
		if _, err := application.NewTriggerBuyEvaluationOnOccurrenceFormedHandler(deps); !errors.Is(err, application.ErrNilDependency) {
			t.Fatalf("%s 缺席：err = %v", name, err)
		}
	}
}
