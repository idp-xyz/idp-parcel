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

// 钉票 sa-cc/08 完成判据 2 的编排半边：请求 → 已登记 + 信封一封；同自然键重复 → `已存在`交回原 ID、
// 不重发（替身照 EnqueueOnce 口径按（租户 + 请求 ID）认领吞重）；成分不同 → 另一份请求、另一个 ID；
// 交接失败 → 结果不翻、留续办引用、重放补交（SA 各编排的既有形）；登记面 / 铸造口不可用 → 未决并
// 指名；构造期拒 nil。夹具全是合成串。

type evaluationRequestRegistryDouble struct {
	byID      map[string]ports.EvaluationRequestRecord
	byNatural map[string]ports.EvaluationRequestRecord
	findErr   error
	saveErr   error
	// beforeSave 在 Save 查重之前跑一次就清掉：用来在 FindByNaturalKey 与 Save 之间塞进另一位写入方——
	// 真库上自然键唯一约束把并发请求判成 AlreadyRequested 的正是这一格，替身不开这个口就到不了。
	beforeSave func()
}

func newEvaluationRequestRegistry() *evaluationRequestRegistryDouble {
	return &evaluationRequestRegistryDouble{
		byID:      map[string]ports.EvaluationRequestRecord{},
		byNatural: map[string]ports.EvaluationRequestRecord{},
	}
}

func evaluationRequestIDKey(key ports.EvaluationRequestKey) string {
	return key.TenantID.String() + "|" + key.Request.String()
}

func evaluationRequestNaturalKey(tenant domain.TenantID, key domain.EvaluationRequestNaturalKey) string {
	return tenant.String() + "|" + key.Scope.String() + "|" + key.Purpose.String() + "|" + key.SourceDigest
}

func (double *evaluationRequestRegistryDouble) Save(
	_ context.Context,
	record ports.EvaluationRequestRecord,
) (ports.EvaluationRequestSaveOutcome, error) {
	if double.beforeSave != nil {
		hook := double.beforeSave
		double.beforeSave = nil
		hook()
	}
	if double.saveErr != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, double.saveErr
	}
	natural := evaluationRequestNaturalKey(record.Key.TenantID, record.Request.NaturalKey())
	if _, exists := double.byID[evaluationRequestIDKey(record.Key)]; exists {
		return ports.EvaluationRequestAlreadyRequested, nil
	}
	if _, exists := double.byNatural[natural]; exists {
		return ports.EvaluationRequestAlreadyRequested, nil
	}
	double.byID[evaluationRequestIDKey(record.Key)] = record
	double.byNatural[natural] = record
	return ports.EvaluationRequestSaved, nil
}

func (double *evaluationRequestRegistryDouble) FindByNaturalKey(
	_ context.Context,
	tenant domain.TenantID,
	key domain.EvaluationRequestNaturalKey,
) (ports.EvaluationRequestRecord, bool, error) {
	if double.findErr != nil {
		return ports.EvaluationRequestRecord{}, false, double.findErr
	}
	record, found := double.byNatural[evaluationRequestNaturalKey(tenant, key)]
	return record, found, nil
}

// evaluationRequestIdentityDouble 按序签发合成 ID；err 非空时如实报错。
type evaluationRequestIdentityDouble struct {
	next int
	err  error
}

func (double *evaluationRequestIdentityDouble) MintEvaluationRequestID(context.Context) (domain.EvaluationRequestID, error) {
	if double.err != nil {
		return domain.EvaluationRequestID{}, double.err
	}
	double.next++
	return domain.NewEvaluationRequestID(fmt.Sprintf("EVREQ-SYN-%d", double.next))
}

// evaluationRequestHandoffDouble 照 EnqueueOnce 的口径按（租户 + 请求 ID）认领：同一份意图再交一次不翻倍。
type evaluationRequestHandoffDouble struct {
	intents map[string]ports.EvaluationRequestIntent
	err     error
}

func newEvaluationRequestHandoff() *evaluationRequestHandoffDouble {
	return &evaluationRequestHandoffDouble{intents: map[string]ports.EvaluationRequestIntent{}}
}

func (double *evaluationRequestHandoffDouble) HandOffEvaluationRequest(
	_ context.Context,
	intent ports.EvaluationRequestIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents[evaluationRequestIDKey(intent.Record.Key)] = intent
	return nil
}

type evaluationRequestClock struct{ at time.Time }

func (clock evaluationRequestClock) Now() time.Time { return clock.at }

type evaluationRequestFixture struct {
	registry *evaluationRequestRegistryDouble
	identity *evaluationRequestIdentityDouble
	handoff  *evaluationRequestHandoffDouble
	handler  *application.RequestBuyEvaluationHandler
}

func newEvaluationRequestFixture(t *testing.T) *evaluationRequestFixture {
	t.Helper()
	fixture := &evaluationRequestFixture{
		registry: newEvaluationRequestRegistry(),
		identity: &evaluationRequestIdentityDouble{},
		handoff:  newEvaluationRequestHandoff(),
	}
	handler, err := application.NewRequestBuyEvaluationHandler(application.RequestBuyEvaluationDeps{
		Registry:   fixture.registry,
		Identity:   fixture.identity,
		Downstream: fixture.handoff,
		Clock:      evaluationRequestClock{at: time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("构造请求评价编排：%v", err)
	}
	fixture.handler = handler
	return fixture
}

func requestBuyEvaluationCommand(t *testing.T, feeItem string) application.RequestBuyEvaluationCommand {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		value(t, domain.NewChargeOccurrenceID, "syn-occ-1"),
		value(t, domain.NewOccurrenceReasonReference, "BOOKING"),
		value(t, domain.NewOccurrenceVersion, "v1"),
		time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("发生项引用：%v", err)
	}
	return application.RequestBuyEvaluationCommand{
		TenantID:    value(t, domain.NewTenantID, "tenant-a"),
		Scope:       value(t, domain.NewPrimaryScopeReference, "syn-scope-1"),
		Occurrence:  occurrence,
		FeeItem:     value(t, domain.NewFeeItemReference, feeItem),
		Agreement:   value(t, domain.NewSupplierAgreementReference, "syn-agreement-1"),
		RequestedBy: value(t, domain.NewRequesterReference, "syn-settlement-job"),
	}
}

// Covers: sa-cc/08 完成判据 2「请求 → 已登记 + 信封一封；同自然键重复 → `已存在`交回原 ID、不重发」。
func TestRequestingABuyEvaluationRegistersOnceAndHandsOffOneEnvelope(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	command := requestBuyEvaluationCommand(t, "syn-fee-1")

	requested, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if requested.Outcome() != application.EvaluationRequested {
		t.Fatalf("outcome = %q, want EVALUATION_REQUESTED", requested.Outcome())
	}
	record, ok := requested.Request()
	if !ok || record.Key.Request.String() != "EVREQ-SYN-1" {
		t.Fatalf("登记下的请求 = %+v, ok = %v", record, ok)
	}
	if record.Request.Purpose() != domain.BuySupplierCost {
		t.Fatalf("计算目的 = %q, want BUY_SUPPLIER_COST", record.Request.Purpose())
	}
	if record.Request.Sources().FeeItem != command.FeeItem || record.Request.Sources().Agreement != command.Agreement ||
		record.Request.Sources().Occurrence != command.Occurrence {
		t.Fatalf("三件引用没照命令登记：%+v", record.Request.Sources())
	}
	if requested.EvaluationRequestHandoffReference() != "" {
		t.Fatalf("交接成功不该留续办引用，实得 %q", requested.EvaluationRequestHandoffReference())
	}
	if got := len(fixture.handoff.intents); got != 1 {
		t.Fatalf("请求成功后意图数 = %d, want 1", got)
	}
	for _, intent := range fixture.handoff.intents {
		if intent.Record.Key.Request.String() != "EVREQ-SYN-1" {
			t.Fatalf("意图携带的请求 ID = %q", intent.Record.Key.Request.String())
		}
	}

	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.EvaluationRequestExisting {
		t.Fatalf("replay outcome = %q, want EXISTING_EVALUATION_REQUEST", replay.Outcome())
	}
	existing, ok := replay.Request()
	if !ok || existing.Key.Request.String() != "EVREQ-SYN-1" {
		t.Fatalf("重放交回的 ID = %q, want 原 ID EVREQ-SYN-1", existing.Key.Request.String())
	}
	if fixture.identity.next != 1 {
		t.Fatalf("重放又铸了一个 ID（共铸 %d 个）——重放不该消耗新身份", fixture.identity.next)
	}
	if got := len(fixture.handoff.intents); got != 1 {
		t.Fatalf("重放后意图数 = %d, want 1——重放交的必须是同一份，由认领键吞掉", got)
	}
}

// Covers: 裁决 2「成分不同就是另一份请求、另一个 ID」。
func TestADifferentSourceReferenceIsAnotherRequestWithItsOwnID(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)

	first, err := fixture.handler.Handle(context.Background(), requestBuyEvaluationCommand(t, "syn-fee-1"))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := fixture.handler.Handle(context.Background(), requestBuyEvaluationCommand(t, "syn-fee-2"))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.Outcome() != application.EvaluationRequested || second.Outcome() != application.EvaluationRequested {
		t.Fatalf("outcomes = %q / %q, want 两次都 EVALUATION_REQUESTED", first.Outcome(), second.Outcome())
	}
	firstRecord, _ := first.Request()
	secondRecord, _ := second.Request()
	if firstRecord.Key.Request == secondRecord.Key.Request {
		t.Fatalf("两份成分不同的请求共用了 ID %q", firstRecord.Key.Request.String())
	}
	if got := len(fixture.handoff.intents); got != 2 {
		t.Fatalf("两份请求后意图数 = %d, want 2", got)
	}
}

// Covers: sa-cc/08 完成判据 2「交接失败 → 技术未形成 + 续办引用」——按 SA 各编排既有形：请求已登记
// 不翻成未决，续办引用非空；重放时同一份再交一次即补上那封。
func TestAFailedHandoffLeavesAContinuationAndReplayResends(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	fixture.handoff.err = errors.New("outbox unavailable")
	command := requestBuyEvaluationCommand(t, "syn-fee-1")

	requested, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if requested.Outcome() != application.EvaluationRequested {
		t.Fatalf("outcome = %q, want EVALUATION_REQUESTED——请求已登记，交接失败不翻它", requested.Outcome())
	}
	if requested.EvaluationRequestHandoffReference() == "" {
		t.Fatal("交接失败必须留续办引用")
	}
	if got := len(fixture.registry.byID); got != 1 {
		t.Fatalf("登记册行数 = %d, want 1——登记已落、意图未入队", got)
	}
	if got := len(fixture.handoff.intents); got != 0 {
		t.Fatalf("交接失败后意图数 = %d, want 0", got)
	}

	fixture.handoff.err = nil
	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.EvaluationRequestExisting {
		t.Fatalf("replay outcome = %q", replay.Outcome())
	}
	if replay.EvaluationRequestHandoffReference() != "" {
		t.Fatalf("补交成功后不该再留续办引用，实得 %q", replay.EvaluationRequestHandoffReference())
	}
	if got := len(fixture.handoff.intents); got != 1 {
		t.Fatalf("补交后意图数 = %d, want 1", got)
	}
}

// Covers: 裁决 2 的并发形——FindByNaturalKey 未见、Save 却答 AlreadyRequested（另一位写入方在两步之间赢了
// 自然键）。输的这一次答`已存在`，交回的必须是**赢家的 ID**，交出去的信封也只有赢家那一封：输家若交
// 自己那份，ID 不同、认领吞不掉，就成两封，而其中一封指向一个没登进册的请求。
func TestALostRaceAnswersWithTheWinnersIDAndHandsOffOnce(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)
	command := requestBuyEvaluationCommand(t, "syn-fee-1")
	fixture.registry.beforeSave = func() {
		won, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || won.Outcome() != application.EvaluationRequested {
			t.Fatalf("赢家请求：outcome = %q, err = %v", won.Outcome(), err)
		}
	}

	lost, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("输家请求：%v", err)
	}
	if lost.Outcome() != application.EvaluationRequestExisting {
		t.Fatalf("输家 outcome = %q, want EXISTING_EVALUATION_REQUEST", lost.Outcome())
	}
	record, ok := lost.Request()
	// 输家先铸了 EVREQ-SYN-1，钩子里的赢家铸了 EVREQ-SYN-2 并先落库；输家交回的必须是赢家那份。
	if !ok || record.Key.Request.String() != "EVREQ-SYN-2" {
		t.Fatalf("输家拿到的 ID = %q, want 赢家的 EVREQ-SYN-2", record.Key.Request.String())
	}
	if got := len(fixture.handoff.intents); got != 1 {
		t.Fatalf("两次请求后意图数 = %d, want 1", got)
	}
	for _, intent := range fixture.handoff.intents {
		if got := intent.Record.Key.Request.String(); got != "EVREQ-SYN-2" {
			t.Fatalf("交出去的请求 ID = %q, want 赢家的 EVREQ-SYN-2", got)
		}
	}
}

func TestAnUnavailableRegistryOrIdentityFactoryLeavesTheRequestUndecided(t *testing.T) {
	registryDown := newEvaluationRequestFixture(t)
	registryDown.registry.findErr = errors.New("registry down")
	result, err := registryDown.handler.Handle(context.Background(), requestBuyEvaluationCommand(t, "syn-fee-1"))
	if err != nil {
		t.Fatalf("registry down: %v", err)
	}
	if result.Outcome() != application.EvaluationRequestUndecided ||
		result.UndecidedReason() != application.EvaluationRequestRegistryUnavailable {
		t.Fatalf("outcome = %q / reason = %q, want 未决 + 登记面不可用", result.Outcome(), result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决必须留续办引用")
	}

	identityDown := newEvaluationRequestFixture(t)
	identityDown.identity.err = errors.New("entropy starved")
	result, err = identityDown.handler.Handle(context.Background(), requestBuyEvaluationCommand(t, "syn-fee-1"))
	if err != nil {
		t.Fatalf("identity down: %v", err)
	}
	if result.Outcome() != application.EvaluationRequestUndecided ||
		result.UndecidedReason() != application.EvaluationRequestIdentityUnavailable {
		t.Fatalf("outcome = %q / reason = %q, want 未决 + 铸造口不可用", result.Outcome(), result.UndecidedReason())
	}
	if got := len(identityDown.handoff.intents); got != 0 {
		t.Fatalf("铸不出 ID 却交了 %d 封", got)
	}

	saveDown := newEvaluationRequestFixture(t)
	saveDown.registry.saveErr = errors.New("registry write down")
	result, err = saveDown.handler.Handle(context.Background(), requestBuyEvaluationCommand(t, "syn-fee-1"))
	if err != nil {
		t.Fatalf("save down: %v", err)
	}
	if result.Outcome() != application.EvaluationRequestUndecided ||
		result.UndecidedReason() != application.EvaluationRequestRegistryUnavailable {
		t.Fatalf("outcome = %q / reason = %q, want 未决 + 登记面不可用", result.Outcome(), result.UndecidedReason())
	}
	if got := len(saveDown.handoff.intents); got != 0 {
		t.Fatalf("没登进册却交了 %d 封", got)
	}
}

func TestABlankTenantOrMissingReferenceIsNotAccepted(t *testing.T) {
	fixture := newEvaluationRequestFixture(t)

	blankTenant := requestBuyEvaluationCommand(t, "syn-fee-1")
	blankTenant.TenantID = domain.TenantID{}
	result, err := fixture.handler.Handle(context.Background(), blankTenant)
	if err != nil || result.Outcome() != application.EvaluationRequestNotAccepted {
		t.Fatalf("空租户：outcome = %q, err = %v", result.Outcome(), err)
	}

	noAgreement := requestBuyEvaluationCommand(t, "syn-fee-1")
	noAgreement.Agreement = domain.SupplierAgreementReference{}
	result, err = fixture.handler.Handle(context.Background(), noAgreement)
	if err != nil || result.Outcome() != application.EvaluationRequestNotAccepted {
		t.Fatalf("缺供应商协议：outcome = %q, err = %v", result.Outcome(), err)
	}

	noRequester := requestBuyEvaluationCommand(t, "syn-fee-1")
	noRequester.RequestedBy = domain.RequesterReference{}
	result, err = fixture.handler.Handle(context.Background(), noRequester)
	if err != nil || result.Outcome() != application.EvaluationRequestNotAccepted {
		t.Fatalf("缺请求方：outcome = %q, err = %v", result.Outcome(), err)
	}

	if fixture.identity.next != 0 || len(fixture.handoff.intents) != 0 || len(fixture.registry.byID) != 0 {
		t.Fatal("未受理的请求不该铸 ID、不该登记、不该交信封")
	}
}

func TestTheHandlerRefusesANilDependencyAtConstruction(t *testing.T) {
	complete := application.RequestBuyEvaluationDeps{
		Registry:   newEvaluationRequestRegistry(),
		Identity:   &evaluationRequestIdentityDouble{},
		Downstream: newEvaluationRequestHandoff(),
		Clock:      evaluationRequestClock{at: time.Now()},
	}
	mutations := map[string]func(*application.RequestBuyEvaluationDeps){
		"缺登记面": func(deps *application.RequestBuyEvaluationDeps) { deps.Registry = nil },
		"缺铸造口": func(deps *application.RequestBuyEvaluationDeps) { deps.Identity = nil },
		"缺交接口": func(deps *application.RequestBuyEvaluationDeps) { deps.Downstream = nil },
		"缺时钟":  func(deps *application.RequestBuyEvaluationDeps) { deps.Clock = nil },
	}
	for name, mutate := range mutations {
		deps := complete
		mutate(&deps)
		if _, err := application.NewRequestBuyEvaluationHandler(deps); err == nil {
			t.Fatalf("%s 却构造成功了", name)
		}
	}
}
