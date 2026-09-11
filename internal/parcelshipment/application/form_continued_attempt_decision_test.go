package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件覆盖票 label-channel/30 完成判据 1、2、4 的替身半边：关闭 / 重开两条命令一个 handler，
// 四件从授权答复取、`CutoffBoundary` = 决定标识、重开的四道门、`Save` 后的判断意图交接。
// 授权适配器四格与真库回滚各在自己的包里证。

var continuedAttemptDecidedAt = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

type continuedAttemptAuthorizerDouble struct {
	answer  ports.ContinuedAttemptDecisionAuthorization
	err     error
	queries []ports.ContinuedAttemptDecisionAuthorizationQuery
}

func (double *continuedAttemptAuthorizerDouble) AuthorizeContinuedAttemptDecision(
	_ context.Context,
	query ports.ContinuedAttemptDecisionAuthorizationQuery,
) (ports.ContinuedAttemptDecisionAuthorization, error) {
	double.queries = append(double.queries, query)
	if double.err != nil {
		return ports.ContinuedAttemptDecisionAuthorization{}, double.err
	}
	return double.answer, nil
}

func grantedContinuedAttemptAuthority(t *testing.T) ports.ContinuedAttemptDecisionAuthorization {
	t.Helper()
	return ports.ContinuedAttemptDecisionAuthorization{
		Outcome:       ports.AuthorizationGranted,
		Authority:     mustValue(t, domain.NewContinuedAttemptAuthoritySnapshot, "SYN-AUTH-RULE-1/v1"),
		AuthorityRole: mustValue(t, domain.NewContinuedAttemptAuthorityRoleReference, "SYN-LEVEL-OPS-2"),
		Decider:       mustValue(t, domain.NewDeciderReference, "SYN-OPERATOR-ROLE-7"),
	}
}

// continuedAttemptRegistersDouble 是登记册仓储的内存替身。Save 经 RehydrateContinuedAttemptRegister
// 把版本抬一格——聚合的 revision 不可从外部设，只有重建门能给它一个持久化版本，与真库读回同一条路。
type continuedAttemptRegistersDouble struct {
	stored   map[string]domain.ContinuedAttemptRegister
	inserts  int
	saves    int
	findErr  error
	insertAs ports.ContinuedAttemptRegisterInsertOutcome
	saveAs   ports.ContinuedAttemptRegisterSaveOutcome
}

func newContinuedAttemptRegisters() *continuedAttemptRegistersDouble {
	return &continuedAttemptRegistersDouble{stored: map[string]domain.ContinuedAttemptRegister{}}
}

func registerKey(tenant domain.TenantID, parcel domain.DeclaredParcelID) string {
	return tenant.String() + "/" + parcel.String()
}

func (double *continuedAttemptRegistersDouble) FindByParcel(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.ContinuedAttemptRegister, bool, error) {
	if double.findErr != nil {
		return domain.ContinuedAttemptRegister{}, false, double.findErr
	}
	register, found := double.stored[registerKey(tenant, parcel)]
	return register, found, nil
}

func (double *continuedAttemptRegistersDouble) Insert(
	_ context.Context,
	register domain.ContinuedAttemptRegister,
) (ports.ContinuedAttemptRegisterInsertOutcome, error) {
	double.inserts++
	if double.insertAs != ports.ContinuedAttemptRegisterInsertOutcomeInvalid {
		return double.insertAs, nil
	}
	key := registerKey(register.Tenant(), register.Parcel())
	if _, exists := double.stored[key]; exists {
		return ports.ContinuedAttemptRegisterAlreadyExists, nil
	}
	double.stored[key] = withRevision(register, 1)
	return ports.ContinuedAttemptRegisterInserted, nil
}

func (double *continuedAttemptRegistersDouble) Save(
	_ context.Context,
	register domain.ContinuedAttemptRegister,
) (ports.ContinuedAttemptRegisterSaveOutcome, error) {
	double.saves++
	if double.saveAs != ports.ContinuedAttemptRegisterSaveOutcomeInvalid {
		return double.saveAs, nil
	}
	key := registerKey(register.Tenant(), register.Parcel())
	current, exists := double.stored[key]
	if !exists || current.Revision() != register.Revision() {
		return ports.ContinuedAttemptRegisterRevisionConflict, nil
	}
	double.stored[key] = withRevision(register, register.Revision()+1)
	return ports.ContinuedAttemptRegisterSaved, nil
}

func (double *continuedAttemptRegistersDouble) decisions(tenant domain.TenantID, parcel domain.DeclaredParcelID) []domain.ContinuedAttemptDecision {
	return double.stored[registerKey(tenant, parcel)].Decisions()
}

func withRevision(register domain.ContinuedAttemptRegister, revision int64) domain.ContinuedAttemptRegister {
	specs := make([]domain.ContinuedAttemptDecisionSpec, 0, len(register.Decisions()))
	for _, decision := range register.Decisions() {
		specs = append(specs, domain.ContinuedAttemptDecisionSpec{
			ID:                          decision.ID(),
			Kind:                        decision.Kind(),
			Requester:                   decision.Requester(),
			Decider:                     decision.Decider(),
			AuthorityRole:               decision.AuthorityRole(),
			AuthoritySnapshot:           decision.AuthoritySnapshot(),
			Reason:                      decision.Reason(),
			EffectiveAt:                 decision.EffectiveAt(),
			CutoffBoundary:              decision.CutoffBoundary(),
			ClosureResponsibilitySource: decision.ClosureResponsibilitySource(),
			RelatedPriorClosure:         decision.RelatedPriorClosure(),
		})
	}
	rehydrated, err := domain.RehydrateContinuedAttemptRegister(domain.RehydrateContinuedAttemptRegisterSpec{
		Revision:  revision,
		Tenant:    register.Tenant(),
		Parcel:    register.Parcel(),
		Decisions: specs,
	})
	if err != nil {
		panic(err)
	}
	return rehydrated
}

type continuedAttemptIdentityDouble struct{ next int }

func (double *continuedAttemptIdentityDouble) NextContinuedAttemptDecisionID(context.Context) (domain.ContinuedAttemptDecisionID, error) {
	double.next++
	return domain.NewContinuedAttemptDecisionID("SYN-CADN-" + strings.Repeat("0", 3-len(itoa(double.next))) + itoa(double.next))
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

type continuedAttemptHandoffDouble struct {
	intents []ports.ContinuedAttemptDecisionHandoffIntent
	err     error
}

func (double *continuedAttemptHandoffDouble) HandOffContinuedAttemptDecision(
	_ context.Context,
	intent ports.ContinuedAttemptDecisionHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

var (
	_ ports.ContinuedAttemptDecisionAuthorizer = (*continuedAttemptAuthorizerDouble)(nil)
	_ ports.ContinuedAttemptRegisterRepository = (*continuedAttemptRegistersDouble)(nil)
	_ ports.ContinuedAttemptDecisionIdentity   = (*continuedAttemptIdentityDouble)(nil)
	_ ports.ContinuedAttemptDecisionHandoff    = (*continuedAttemptHandoffDouble)(nil)
)

type continuedAttemptFixture struct {
	authorizer *continuedAttemptAuthorizerDouble
	finals     *finalStoreDouble
	registers  *continuedAttemptRegistersDouble
	identities *continuedAttemptIdentityDouble
	handoff    *continuedAttemptHandoffDouble
	handler    *application.FormContinuedAttemptDecisionHandler
	identity   domain.SourceIdentity
	parcel     domain.DeclaredParcelID
}

func newContinuedAttemptFixture(t *testing.T) *continuedAttemptFixture {
	t.Helper()
	fixture := &continuedAttemptFixture{
		authorizer: &continuedAttemptAuthorizerDouble{answer: grantedContinuedAttemptAuthority(t)},
		finals:     newFinalStore(),
		registers:  newContinuedAttemptRegisters(),
		identities: &continuedAttemptIdentityDouble{},
		handoff:    &continuedAttemptHandoffDouble{},
		identity:   sourceIdentity(t, "SYN-TENANT-1", "SYN-CUSTOMER-1", "SYN-SOURCE-A", "SYN-SUBMIT-KEY-CA-1"),
		parcel:     mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-CA-1"),
	}
	handler, err := application.NewFormContinuedAttemptDecisionHandler(application.FormContinuedAttemptDecisionDeps{
		Authorizer: fixture.authorizer,
		Finals:     fixture.finals,
		Registers:  fixture.registers,
		Identities: fixture.identities,
		Handoff:    fixture.handoff,
		Clock:      fixedClock{at: continuedAttemptDecidedAt},
	})
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	fixture.handler = handler
	return fixture
}

func (fixture *continuedAttemptFixture) closureCommand(t *testing.T) application.FormControlledClosureCommand {
	t.Helper()
	return application.FormControlledClosureCommand{
		Identity:                    fixture.identity,
		Parcel:                      fixture.parcel,
		Requester:                   mustValue(t, domain.NewRequesterReference, "SYN-SHIPPER-ACCOUNT-1"),
		Reason:                      mustValue(t, domain.NewContinuedAttemptReasonReference, "SYN-REASON-SHIPPER-STOP"),
		EffectiveAt:                 continuedAttemptDecidedAt.Add(time.Hour),
		ResponsibilitySourceKind:    application.ShipperInstructionResponsibilitySource,
		ResponsibilitySourceSubject: "SYN-SHIPPER-INSTRUCTION-9",
	}
}

func (fixture *continuedAttemptFixture) reopeningCommand(t *testing.T, priorClosure domain.ContinuedAttemptDecisionID) application.FormReopeningCommand {
	t.Helper()
	return application.FormReopeningCommand{
		Identity:                     fixture.identity,
		Parcel:                       fixture.parcel,
		Requester:                    mustValue(t, domain.NewRequesterReference, "SYN-SHIPPER-ACCOUNT-1"),
		Reason:                       mustValue(t, domain.NewContinuedAttemptReasonReference, "SYN-REASON-RESUME"),
		EffectiveAt:                  continuedAttemptDecidedAt.Add(2 * time.Hour),
		RelatedPriorClosure:          priorClosure,
		ShipperAccount:               mustValue(t, domain.NewRequesterReference, "SYN-SHIPPER-ACCOUNT-1"),
		ShipperAuthorizationEvidence: "SYN-SHIPPER-REAUTH-EVIDENCE-3",
	}
}

func (fixture *continuedAttemptFixture) closed(t *testing.T) domain.ContinuedAttemptDecision {
	t.Helper()
	result, err := fixture.handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
	if err != nil {
		t.Fatalf("先关闭：%v", err)
	}
	if result.Outcome() != application.ContinuedAttemptDecisionFormed {
		t.Fatalf("先关闭 outcome = %v, want FORMED", result.Outcome())
	}
	decision, _ := result.Decision()
	return decision
}

func TestAnAuthorizedControlledClosureIsAppendedWithTheProvidersFourItems(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)

	result, err := fixture.handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
	if err != nil {
		t.Fatalf("形成关闭：%v", err)
	}
	if got := result.Outcome(); got != application.ContinuedAttemptDecisionFormed {
		t.Fatalf("outcome = %v, want FORMED", got)
	}
	decision, present := result.Decision()
	if !present {
		t.Fatal("已形成却没交回决定")
	}
	if decision.Kind() != domain.ControlledClosureDecision {
		t.Fatalf("kind = %v, want CONTROLLED_CLOSURE", decision.Kind())
	}
	// 四件从 PC 答复取，不从命令取：命令里根本没有这四格。
	granted := grantedContinuedAttemptAuthority(t)
	if decision.Decider() != granted.Decider ||
		decision.AuthorityRole() != granted.AuthorityRole ||
		decision.AuthoritySnapshot() != granted.Authority {
		t.Fatalf("决定方 / 授权角色 / 授权依据快照 = %v / %v / %v，不是 PC 交回的那三项",
			decision.Decider(), decision.AuthorityRole(), decision.AuthoritySnapshot())
	}
	if decision.Requester().String() != "SYN-SHIPPER-ACCOUNT-1" {
		t.Fatalf("requester = %q，请求方是命令自带的事实", decision.Requester())
	}
	// 裁决：`AuthoritativeCutoffBoundary` = 关闭决定标识，一个事一个名。
	if decision.CutoffBoundary().String() != decision.ID().String() {
		t.Fatalf("cutoff boundary = %q, decision id = %q，两者必须同串", decision.CutoffBoundary(), decision.ID())
	}
	if !strings.HasPrefix(decision.ClosureResponsibilitySource().String(), "SHIPPER-INSTRUCTION/") {
		t.Fatalf("closure responsibility source = %q，缺种类段", decision.ClosureResponsibilitySource())
	}
	if !decision.EffectiveAt().Equal(continuedAttemptDecidedAt.Add(time.Hour)) {
		t.Fatalf("effectiveAt = %v，生效时间由命令给", decision.EffectiveAt())
	}

	// 册是本次开的：Insert 一次、Save 零次；册上恰一条。
	if fixture.registers.inserts != 1 || fixture.registers.saves != 0 {
		t.Fatalf("inserts / saves = %d / %d, want 1 / 0", fixture.registers.inserts, fixture.registers.saves)
	}
	if got := len(fixture.registers.decisions(fixture.identity.TenantID(), fixture.parcel)); got != 1 {
		t.Fatalf("册上 %d 条决定, want 1", got)
	}

	// 判断意图一封一决定：决定标识、种类、生效时间原样带出。
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("交接 %d 封, want 1", len(fixture.handoff.intents))
	}
	intent := fixture.handoff.intents[0]
	if intent.Decision != decision.ID() || intent.Kind != domain.ControlledClosureDecision ||
		!intent.OccurredAt.Equal(decision.EffectiveAt()) || intent.Parcel != fixture.parcel ||
		intent.Tenant != fixture.identity.TenantID() {
		t.Fatalf("intent = %+v，与落册的决定对不上", intent)
	}

	// 授权问的是关闭、在决定时刻、带请求方与原因。
	if len(fixture.authorizer.queries) != 1 {
		t.Fatalf("问授权 %d 次, want 1", len(fixture.authorizer.queries))
	}
	query := fixture.authorizer.queries[0]
	if query.Kind != domain.ControlledClosureDecision || !query.At.Equal(continuedAttemptDecidedAt) ||
		query.Parcel != fixture.parcel || query.Requester.String() != "SYN-SHIPPER-ACCOUNT-1" {
		t.Fatalf("query = %+v", query)
	}
}

func TestARefusedOrUnconfiguredAuthorizationWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  ports.ContinuedAttemptDecisionAuthorization
		err     error
		outcome application.ContinuedAttemptDecisionOutcome
		pending application.ContinuedAttemptPendingReason
	}{
		{
			name:    "拒绝是确定的业务答案",
			answer:  ports.ContinuedAttemptDecisionAuthorization{Outcome: ports.AuthorizationRefused},
			outcome: application.ContinuedAttemptDecisionNotAuthorized,
		},
		{
			name:    "没有规则不是拒绝",
			answer:  ports.ContinuedAttemptDecisionAuthorization{Outcome: ports.AuthorizationRulesNotConfigured},
			outcome: application.ContinuedAttemptDecisionUndecided,
			pending: application.ContinuedAttemptAuthorityRulesNotConfigured,
		},
		{
			name:    "授权口答不出是未决",
			err:     errors.New("mapping not configured"),
			outcome: application.ContinuedAttemptDecisionUndecided,
			pending: application.ContinuedAttemptAuthorityUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newContinuedAttemptFixture(t)
			fixture.authorizer.answer = tc.answer
			fixture.authorizer.err = tc.err

			result, err := fixture.handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
			if err != nil {
				t.Fatalf("形成关闭：%v", err)
			}
			if result.Outcome() != tc.outcome {
				t.Fatalf("outcome = %v, want %v", result.Outcome(), tc.outcome)
			}
			if result.PendingReason() != tc.pending {
				t.Fatalf("pending = %v, want %v", result.PendingReason(), tc.pending)
			}
			if _, present := result.Decision(); present {
				t.Fatal("没形成却交回了决定")
			}
			if fixture.registers.inserts != 0 || fixture.registers.saves != 0 || len(fixture.handoff.intents) != 0 {
				t.Fatalf("不写册也不交接：inserts %d / saves %d / intents %d",
					fixture.registers.inserts, fixture.registers.saves, len(fixture.handoff.intents))
			}
			if fixture.identities.next != 0 {
				t.Fatal("未获授权不该消耗决定标识")
			}
		})
	}
}

func TestAClosureMayBeFormedWithoutARequester(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)
	command := fixture.closureCommand(t)
	command.Requester = domain.RequesterReference{}

	result, err := fixture.handler.FormControlledClosure(t.Context(), command)
	if err != nil {
		t.Fatalf("形成关闭：%v", err)
	}
	if result.Outcome() != application.ContinuedAttemptDecisionFormed {
		t.Fatalf("outcome = %v, want FORMED——运营企业自行发起的关闭没有外部请求方", result.Outcome())
	}
}

func TestAnUnrecognisedResponsibilitySourceKindIsNotAccepted(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)
	command := fixture.closureCommand(t)
	command.ResponsibilitySourceKind = 0

	result, err := fixture.handler.FormControlledClosure(t.Context(), command)
	if err != nil {
		t.Fatalf("形成关闭：%v", err)
	}
	if result.Outcome() != application.ContinuedAttemptDecisionNotAccepted {
		t.Fatalf("outcome = %v, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
	if result.Refusal() != application.ContinuedAttemptResponsibilitySourceUnknown {
		t.Fatalf("refusal = %v, want RESPONSIBILITY_SOURCE_UNKNOWN", result.Refusal())
	}
	if fixture.registers.inserts != 0 {
		t.Fatal("输入未受理不写册")
	}
}

// ---- 重开（完成判据 2） ----

func TestAnAuthorizedReopeningIsAppendedAfterAStandingClosure(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)
	closure := fixture.closed(t)
	// 重开的授权答复换一个授权角色：编排不比等级（pc-gaps/13 裁决 ①），角色不同照样形成——等级集合归 PC 登进 REOPENING 那一格。
	reopeningGrant := grantedContinuedAttemptAuthority(t)
	reopeningGrant.AuthorityRole = mustValue(t, domain.NewContinuedAttemptAuthorityRoleReference, "SYN-LEVEL-OPS-1")
	fixture.authorizer.answer = reopeningGrant

	result, err := fixture.handler.FormReopening(t.Context(), fixture.reopeningCommand(t, closure.ID()))
	if err != nil {
		t.Fatalf("形成重开：%v", err)
	}
	if result.Outcome() != application.ContinuedAttemptDecisionFormed {
		t.Fatalf("outcome = %v, want FORMED", result.Outcome())
	}
	decision, present := result.Decision()
	if !present || decision.Kind() != domain.ReopeningDecision {
		t.Fatalf("decision = %+v / %v, want a REOPENING", decision, present)
	}
	if decision.RelatedPriorClosure() != closure.ID() {
		t.Fatalf("relatedPriorClosure = %q, want %q", decision.RelatedPriorClosure(), closure.ID())
	}
	if decision.AuthorityRole() != reopeningGrant.AuthorityRole || decision.Decider() != reopeningGrant.Decider ||
		decision.AuthoritySnapshot() != reopeningGrant.Authority {
		t.Fatalf("重开的三件要取自重开那次授权答复：%v / %v / %v", decision.AuthorityRole(), decision.Decider(), decision.AuthoritySnapshot())
	}
	if decision.CutoffBoundary().String() != "" || decision.ClosureResponsibilitySource().String() != "" {
		t.Fatal("重开不带截断边界与关闭责任来源")
	}

	// 册是读回的：Insert 仍是关闭那一次、Save 一次；册上两条，判断在册上派生为开放。
	if fixture.registers.inserts != 1 || fixture.registers.saves != 1 {
		t.Fatalf("inserts / saves = %d / %d, want 1 / 1", fixture.registers.inserts, fixture.registers.saves)
	}
	decisions := fixture.registers.decisions(fixture.identity.TenantID(), fixture.parcel)
	if len(decisions) != 2 {
		t.Fatalf("册上 %d 条决定, want 2", len(decisions))
	}
	register, _, _ := fixture.registers.FindByParcel(t.Context(), fixture.identity.TenantID(), fixture.parcel)
	if register.Judge(false) != domain.ContinuedAttemptOpen {
		t.Fatalf("judge = %v, want OPEN", register.Judge(false))
	}

	// 一封一决定：第二封是重开的。
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("交接 %d 封, want 2", len(fixture.handoff.intents))
	}
	intent := fixture.handoff.intents[1]
	if intent.Decision != decision.ID() || intent.Kind != domain.ReopeningDecision || !intent.OccurredAt.Equal(decision.EffectiveAt()) {
		t.Fatalf("intent = %+v", intent)
	}
	// 第二次问的是重开。
	if len(fixture.authorizer.queries) != 2 || fixture.authorizer.queries[1].Kind != domain.ReopeningDecision {
		t.Fatalf("queries = %+v", fixture.authorizer.queries)
	}
}

func TestReopeningIsNotAdmittedWhileACurrentFinalStands(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)
	closure := fixture.closed(t)
	key := ports.FinalAdoptionKey{TenantID: fixture.identity.TenantID(), Parcel: fixture.parcel}
	fixture.finals.byKey[key] = ports.FinalOutcomeRecord{Key: key, Finalized: true, AdoptedAt: continuedAttemptDecidedAt}

	result, err := fixture.handler.FormReopening(t.Context(), fixture.reopeningCommand(t, closure.ID()))
	if err != nil {
		t.Fatalf("形成重开：%v", err)
	}
	if result.Outcome() != application.ContinuedAttemptDecisionNotAdmitted {
		t.Fatalf("outcome = %v, want NOT_ADMITTED——「只有当前不存在有效终局服务结果时，才能……追加重开决定」", result.Outcome())
	}
	assertNothingAppendedAfterClosure(t, fixture)
}

func TestReopeningNotAfterTheClosureIsNotAccepted(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)
	closure := fixture.closed(t)
	command := fixture.reopeningCommand(t, closure.ID())
	command.EffectiveAt = closure.EffectiveAt()

	result, err := fixture.handler.FormReopening(t.Context(), command)
	if err != nil {
		t.Fatalf("形成重开：%v", err)
	}
	if result.Outcome() != application.ContinuedAttemptDecisionNotAccepted || result.Refusal() != application.ContinuedAttemptDecisionInvalid {
		t.Fatalf("outcome / refusal = %v / %v, want INPUT_NOT_ACCEPTED / DECISION_INVALID", result.Outcome(), result.Refusal())
	}
	assertNothingAppendedAfterClosure(t, fixture)
}

func TestReopeningAShipperInstructedClosureRequiresTheSameShippersNewAuthorization(t *testing.T) {
	t.Run("证据缺席即拒", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		closure := fixture.closed(t)
		command := fixture.reopeningCommand(t, closure.ID())
		command.ShipperAuthorizationEvidence = "   "

		result, err := fixture.handler.FormReopening(t.Context(), command)
		if err != nil {
			t.Fatalf("形成重开：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionNotAccepted ||
			result.Refusal() != application.ContinuedAttemptShipperReauthorizationMissing {
			t.Fatalf("outcome / refusal = %v / %v", result.Outcome(), result.Refusal())
		}
		assertNothingAppendedAfterClosure(t, fixture)
		if fixture.identities.next != 1 {
			t.Fatal("门拒不该消耗第二个决定标识")
		}
	})
	t.Run("账户与原关闭请求方不同即拒", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		closure := fixture.closed(t)
		command := fixture.reopeningCommand(t, closure.ID())
		command.ShipperAccount = mustValue(t, domain.NewRequesterReference, "SYN-SHIPPER-ACCOUNT-2")

		result, err := fixture.handler.FormReopening(t.Context(), command)
		if err != nil {
			t.Fatalf("形成重开：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionNotAccepted ||
			result.Refusal() != application.ContinuedAttemptShipperAccountMismatch {
			t.Fatalf("outcome / refusal = %v / %v", result.Outcome(), result.Refusal())
		}
		assertNothingAppendedAfterClosure(t, fixture)
	})
	t.Run("原关闭没记请求方就核不出同一账户", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		closureCommand := fixture.closureCommand(t)
		closureCommand.Requester = domain.RequesterReference{}
		closed, err := fixture.handler.FormControlledClosure(t.Context(), closureCommand)
		if err != nil || closed.Outcome() != application.ContinuedAttemptDecisionFormed {
			t.Fatalf("先关闭：%v / %v", closed.Outcome(), err)
		}
		closure, _ := closed.Decision()

		result, err := fixture.handler.FormReopening(t.Context(), fixture.reopeningCommand(t, closure.ID()))
		if err != nil {
			t.Fatalf("形成重开：%v", err)
		}
		if result.Refusal() != application.ContinuedAttemptShipperAccountMismatch {
			t.Fatalf("refusal = %v, want SHIPPER_ACCOUNT_MISMATCH", result.Refusal())
		}
	})
	t.Run("运营企业操作形成的关闭不要货主证据", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		closureCommand := fixture.closureCommand(t)
		closureCommand.Requester = domain.RequesterReference{}
		closureCommand.ResponsibilitySourceKind = application.OperatorActionResponsibilitySource
		closureCommand.ResponsibilitySourceSubject = "SYN-OPS-DECISION-4"
		closed, err := fixture.handler.FormControlledClosure(t.Context(), closureCommand)
		if err != nil || closed.Outcome() != application.ContinuedAttemptDecisionFormed {
			t.Fatalf("先关闭：%v / %v", closed.Outcome(), err)
		}
		closure, _ := closed.Decision()
		command := fixture.reopeningCommand(t, closure.ID())
		command.ShipperAccount = domain.RequesterReference{}
		command.ShipperAuthorizationEvidence = ""

		result, err := fixture.handler.FormReopening(t.Context(), command)
		if err != nil {
			t.Fatalf("形成重开：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionFormed {
			t.Fatalf("outcome = %v, want FORMED——原关闭不是货主指令形成的，两格不读", result.Outcome())
		}
	})
}

// seedClosureWithRawResponsibilitySource 绕过写面直接把一条关闭放进册。写面合成的责任来源一定带种类段，
// 「原关闭的种类段读不出」这一格只能这样造——它模拟的正是那条不经写面落下的坏数据。
func (fixture *continuedAttemptFixture) seedClosureWithRawResponsibilitySource(t *testing.T, source string) domain.ContinuedAttemptDecision {
	t.Helper()
	granted := grantedContinuedAttemptAuthority(t)
	id := mustValue(t, domain.NewContinuedAttemptDecisionID, "SYN-CADN-SEEDED")
	register, err := domain.RehydrateContinuedAttemptRegister(domain.RehydrateContinuedAttemptRegisterSpec{
		Revision: 1,
		Tenant:   fixture.identity.TenantID(),
		Parcel:   fixture.parcel,
		Decisions: []domain.ContinuedAttemptDecisionSpec{{
			ID:                          id,
			Kind:                        domain.ControlledClosureDecision,
			Requester:                   mustValue(t, domain.NewRequesterReference, "SYN-SHIPPER-ACCOUNT-1"),
			Decider:                     granted.Decider,
			AuthorityRole:               granted.AuthorityRole,
			AuthoritySnapshot:           granted.Authority,
			Reason:                      mustValue(t, domain.NewContinuedAttemptReasonReference, "SYN-REASON-SHIPPER-STOP"),
			EffectiveAt:                 continuedAttemptDecidedAt.Add(time.Hour),
			CutoffBoundary:              mustValue(t, domain.NewAuthoritativeCutoffBoundary, id.String()),
			ClosureResponsibilitySource: mustValue(t, domain.NewClosureResponsibilitySourceReference, source),
		}},
	})
	if err != nil {
		t.Fatalf("造册：%v", err)
	}
	fixture.registers.stored[registerKey(fixture.identity.TenantID(), fixture.parcel)] = register
	return register.Decisions()[0]
}

func TestReopeningAClosureWhoseResponsibilitySourceKindCannotBeReadIsNotAccepted(t *testing.T) {
	// 裁决 ②：认不出种类段 → 不放行、不默认「其他」。认不出不等于「不是货主指令」——当非货主指令放过去，
	// 「同一货主账户新的有效授权」那道门就被一条坏数据绕开了；命令带不带证据都一样，证据是货主指令那一格才读的。
	for _, tc := range []struct {
		name     string
		source   string
		evidence string
	}{
		{name: "无分隔符·命令带证据", source: "SYN-LEGACY-SOURCE", evidence: "SYN-SHIPPER-REAUTH-EVIDENCE-3"},
		{name: "无分隔符·命令不带证据", source: "SYN-LEGACY-SOURCE", evidence: ""},
		{name: "种类不在封闭集·命令带证据", source: "OTHER/SYN-SUBJECT-1", evidence: "SYN-SHIPPER-REAUTH-EVIDENCE-3"},
		{name: "种类不在封闭集·命令不带证据", source: "OTHER/SYN-SUBJECT-1", evidence: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newContinuedAttemptFixture(t)
			closure := fixture.seedClosureWithRawResponsibilitySource(t, tc.source)
			command := fixture.reopeningCommand(t, closure.ID())
			command.ShipperAuthorizationEvidence = tc.evidence

			result, err := fixture.handler.FormReopening(t.Context(), command)
			if err != nil {
				t.Fatalf("形成重开：%v", err)
			}
			if result.Outcome() != application.ContinuedAttemptDecisionNotAccepted ||
				result.Refusal() != application.ContinuedAttemptResponsibilitySourceUnknown {
				t.Fatalf("outcome / refusal = %v / %v, want INPUT_NOT_ACCEPTED / RESPONSIBILITY_SOURCE_UNKNOWN",
					result.Outcome(), result.Refusal())
			}
			if _, present := result.Decision(); present {
				t.Fatal("没形成却交回了决定")
			}
			// 册是直接造进去的、没经写面：标识签发器、写口、交接都必须是零，册上仍只那一条。
			if fixture.identities.next != 0 {
				t.Fatalf("消耗了 %d 个决定标识, want 0", fixture.identities.next)
			}
			if fixture.registers.inserts != 0 || fixture.registers.saves != 0 || len(fixture.handoff.intents) != 0 {
				t.Fatalf("不写册也不交接：inserts %d / saves %d / intents %d",
					fixture.registers.inserts, fixture.registers.saves, len(fixture.handoff.intents))
			}
			if got := len(fixture.registers.decisions(fixture.identity.TenantID(), fixture.parcel)); got != 1 {
				t.Fatalf("册上 %d 条决定, want 1", got)
			}
			// 授权口问过恰一次：共用路径上授权先于读册与这道门（`FormControlledClosure` 头注「授权先于一切写动作与标识签发」），
			// 与证据缺席 / 账户不符两格同一顺序；这道门不再多问 PC。
			if len(fixture.authorizer.queries) != 1 {
				t.Fatalf("问授权 %d 次, want 1", len(fixture.authorizer.queries))
			}
		})
	}
}

func TestReopeningWithoutARegisterOrAStandingClosureIsNotAdmitted(t *testing.T) {
	t.Run("没开过册", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		prior := mustValue(t, domain.NewContinuedAttemptDecisionID, "SYN-CADN-NEVER")

		result, err := fixture.handler.FormReopening(t.Context(), fixture.reopeningCommand(t, prior))
		if err != nil {
			t.Fatalf("形成重开：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionNotAdmitted {
			t.Fatalf("outcome = %v, want NOT_ADMITTED", result.Outcome())
		}
		if fixture.registers.inserts != 0 || fixture.identities.next != 0 || len(fixture.handoff.intents) != 0 {
			t.Fatal("没有册可追加：不开册、不消耗标识、不交接")
		}
	})
	t.Run("指向的关闭不是生效的那份", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		fixture.closed(t)
		unknown := mustValue(t, domain.NewContinuedAttemptDecisionID, "SYN-CADN-UNKNOWN")

		result, err := fixture.handler.FormReopening(t.Context(), fixture.reopeningCommand(t, unknown))
		if err != nil {
			t.Fatalf("形成重开：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionNotAdmitted {
			t.Fatalf("outcome = %v, want NOT_ADMITTED（领域：指不到一份仍然生效的关闭）", result.Outcome())
		}
		assertNothingAppendedAfterClosure(t, fixture)
	})
}

// ---- 写侧：冲突、交接失败、读不回（完成判据 4 的替身半边） ----

func TestAWriteConflictIsABusinessAnswerAndHandsOffNothing(t *testing.T) {
	t.Run("开册撞上别人刚开的", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		fixture.registers.insertAs = ports.ContinuedAttemptRegisterAlreadyExists

		result, err := fixture.handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
		if err != nil {
			t.Fatalf("形成关闭：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionWriteConflict {
			t.Fatalf("outcome = %v, want REVISION_CONFLICT", result.Outcome())
		}
		if _, present := result.Decision(); present {
			t.Fatal("冲突时不交回手上这份陈旧的")
		}
		if len(fixture.handoff.intents) != 0 {
			t.Fatal("没落库不交接")
		}
	})
	t.Run("追加时预期版本对不上", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		closure := fixture.closed(t)
		fixture.registers.saveAs = ports.ContinuedAttemptRegisterRevisionConflict

		result, err := fixture.handler.FormReopening(t.Context(), fixture.reopeningCommand(t, closure.ID()))
		if err != nil {
			t.Fatalf("形成重开：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionWriteConflict {
			t.Fatalf("outcome = %v, want REVISION_CONFLICT", result.Outcome())
		}
		if len(fixture.handoff.intents) != 1 {
			t.Fatalf("交接 %d 封, want 1（只有关闭那一封）", len(fixture.handoff.intents))
		}
	})
}

func TestAFailedHandoffSurfacesAsAnErrorSoTheShellRollsBack(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)
	fixture.handoff.err = errors.New("outbox unavailable")

	_, err := fixture.handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
	if err == nil {
		t.Fatal("入队失败必须以 error 交回——整步随事务回滚（ADR-0134 决定三），不留「决定已落、判断意图丢了」")
	}
}

type failingFinals struct {
	*finalStoreDouble
	err error
}

func (double failingFinals) FindCurrentFinal(
	context.Context, domain.TenantID, domain.DeclaredParcelID,
) (ports.FinalOutcomeRecord, bool, error) {
	return ports.FinalOutcomeRecord{}, false, double.err
}

func TestUnreadableFinalOrRegisterIsUndecidedAndWritesNothing(t *testing.T) {
	t.Run("当前有效终局读不回", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		handler, err := application.NewFormContinuedAttemptDecisionHandler(application.FormContinuedAttemptDecisionDeps{
			Authorizer: fixture.authorizer,
			Finals:     failingFinals{finalStoreDouble: fixture.finals, err: errors.New("final store down")},
			Registers:  fixture.registers,
			Identities: fixture.identities,
			Handoff:    fixture.handoff,
			Clock:      fixedClock{at: continuedAttemptDecidedAt},
		})
		if err != nil {
			t.Fatalf("构造编排：%v", err)
		}

		result, err := handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
		if err != nil {
			t.Fatalf("形成关闭：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionUndecided ||
			result.PendingReason() != application.ContinuedAttemptFinalOutcomeUnavailable {
			t.Fatalf("outcome / pending = %v / %v", result.Outcome(), result.PendingReason())
		}
		if fixture.registers.inserts != 0 || fixture.identities.next != 0 {
			t.Fatal("未决不写册、不消耗标识")
		}
	})
	t.Run("登记册读不回", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		fixture.registers.findErr = errors.New("register store down")

		result, err := fixture.handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
		if err != nil {
			t.Fatalf("形成关闭：%v", err)
		}
		if result.Outcome() != application.ContinuedAttemptDecisionUndecided ||
			result.PendingReason() != application.ContinuedAttemptDecisionRegisterUnavailable {
			t.Fatalf("outcome / pending = %v / %v", result.Outcome(), result.PendingReason())
		}
	})
	t.Run("标识签不出", func(t *testing.T) {
		fixture := newContinuedAttemptFixture(t)
		handler, err := application.NewFormContinuedAttemptDecisionHandler(application.FormContinuedAttemptDecisionDeps{
			Authorizer: fixture.authorizer,
			Finals:     fixture.finals,
			Registers:  fixture.registers,
			Identities: failingIdentity{},
			Handoff:    fixture.handoff,
			Clock:      fixedClock{at: continuedAttemptDecidedAt},
		})
		if err != nil {
			t.Fatalf("构造编排：%v", err)
		}
		result, err := handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
		if err != nil {
			t.Fatalf("形成关闭：%v", err)
		}
		if result.PendingReason() != application.ContinuedAttemptDecisionIdentityUnavailable {
			t.Fatalf("pending = %v, want DECISION_IDENTITY_UNAVAILABLE", result.PendingReason())
		}
	})
}

type failingIdentity struct{}

func (failingIdentity) NextContinuedAttemptDecisionID(context.Context) (domain.ContinuedAttemptDecisionID, error) {
	return domain.ContinuedAttemptDecisionID{}, errors.New("identity issuer down")
}

func TestTheHandlerRefusesToBeBuiltWithAMissingPort(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)
	deps := application.FormContinuedAttemptDecisionDeps{
		Authorizer: fixture.authorizer,
		Finals:     fixture.finals,
		Registers:  fixture.registers,
		Identities: fixture.identities,
		Clock:      fixedClock{at: continuedAttemptDecidedAt},
	}
	if _, err := application.NewFormContinuedAttemptDecisionHandler(deps); err == nil {
		t.Fatal("漏装交接口必须在构造期就响，而不是落册之后 panic")
	}
}

// assertNothingAppendedAfterClosure 断言先关闭之后这一轮什么都没再写：册上仍一条、Save 零次、只有关闭那一封。
func assertNothingAppendedAfterClosure(t *testing.T, fixture *continuedAttemptFixture) {
	t.Helper()
	if got := len(fixture.registers.decisions(fixture.identity.TenantID(), fixture.parcel)); got != 1 {
		t.Fatalf("册上 %d 条决定, want 1", got)
	}
	if fixture.registers.saves != 0 {
		t.Fatalf("saves = %d, want 0", fixture.registers.saves)
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("交接 %d 封, want 1", len(fixture.handoff.intents))
	}
}
