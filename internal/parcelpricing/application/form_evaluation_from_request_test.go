package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件钉票 sa-cc/11 裁决 1 / 4 的应用入口：三步「解析在用价卡 → 造输入快照 → 交既有 EvaluatePricingHandler」
// 与它的封闭结果格——已存在（回指命中）/ 未配置 / 适用冲突 / 输入不可得 / 已形成（带回指）/ 未受理 / 未决。
// 计算一步都不在这里：形成那一格走的是真实的 EvaluatePricingHandler 与 domain.EvaluatePricing。

type evaluationByRequestDouble struct {
	byReference map[string]domain.PricingEvaluation
	err         error
	calls       int
}

func (double *evaluationByRequestDouble) FindByRequestReference(
	_ context.Context,
	tenant domain.TenantID,
	reference domain.EvaluationRequestReference,
) (domain.PricingEvaluation, bool, error) {
	double.calls++
	if double.err != nil {
		return domain.PricingEvaluation{}, false, double.err
	}
	evaluation, found := double.byReference[tenant.String()+"/"+reference.String()]
	return evaluation, found, nil
}

type priceCardResolverDouble struct {
	resolution ports.PriceCardInForceResolution
	err        error
	asked      []time.Time
}

func (double *priceCardResolverDouble) ResolveInForce(
	_ context.Context,
	_ domain.TenantID,
	_ domain.PricingScopeID,
	_ domain.PricingDirection,
	_ domain.PricingPurpose,
	at time.Time,
) (ports.PriceCardInForceResolution, error) {
	double.asked = append(double.asked, at)
	if double.err != nil {
		return ports.PriceCardInForceResolution{}, double.err
	}
	return double.resolution, nil
}

type identityDouble struct {
	next   string
	minted int
	err    error
}

func (double *identityDouble) MintEvaluationID(_ context.Context) (domain.EvaluationID, error) {
	if double.err != nil {
		return domain.EvaluationID{}, double.err
	}
	double.minted++
	return domain.NewEvaluationID(double.next)
}

type inputResolverDouble struct {
	resolution ports.PricingInputResolution
	err        error
	queries    []ports.PricingInputQuery
}

func (double *inputResolverDouble) ResolvePricingInput(_ context.Context, query ports.PricingInputQuery) (ports.PricingInputResolution, error) {
	double.queries = append(double.queries, query)
	if double.err != nil {
		return ports.PricingInputResolution{}, double.err
	}
	return double.resolution, nil
}

var requestBasisAt = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

// buyPlan 造一张 BUY·SUPPLIER_COST 的合成价卡（S 级）：单分区 Z1 重量段、实重策略、不声明金额取整。
func buyPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-BUY-PLAN", "v1"),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, "SYN-BUY-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, "SYN-BUY-WEIGHT", "v1"),
		Scope:                 "scope-1",
		Direction:             domain.PricingDirectionBuy,
		Purpose:               domain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "USD",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "SYN-BUY-ENTRY",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "12",
		WeightRounding:      domain.RoundingCeiling,
		WeightStepKilograms: "0.5",
	})
}

func buyInput(t *testing.T) domain.PricingInputSnapshot {
	t.Helper()
	return pptest.Input(t, pptest.InputSpec{
		Tenant:     "tenant-1",
		Scope:      "scope-1",
		PackageID:  "SYN-PKG-1",
		Zone:       "Z1",
		Kilograms:  "5",
		BusinessAt: requestBasisAt,
	})
}

func requestCommand(t *testing.T) application.FormEvaluationFromRequestCommand {
	t.Helper()
	return application.FormEvaluationFromRequestCommand{
		Tenant:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:   mustValue(t, domain.NewEvaluationRequestReference, "EVREQ-SYN-1"),
		Scope:     mustValue(t, domain.NewPricingScopeID, "scope-1"),
		Direction: domain.PricingDirectionBuy,
		Purpose:   domain.PricingPurposeSupplierCost,
		BasisAt:   requestBasisAt,
		Sources: ports.EligibleSourceReferences{
			Occurrence:        "SYN-OCC-1",
			OccurrenceVersion: "v1",
			FeeItem:           "SYN-FEE-1",
			SupplierAgreement: "SYN-AGR-1@v1",
		},
		Evidence: domain.EvidenceSynthetic,
	}
}

type formFixture struct {
	handler    *application.FormEvaluationFromRequestHandler
	byRequest  *evaluationByRequestDouble
	priceCards *priceCardResolverDouble
	identity   *identityDouble
	inputs     *inputResolverDouble
	store      *evaluationStoreDouble
	downstream *evaluationDownstreamDouble
}

// newFormFixture 装一套入口：默认价卡唯一命中、输入可得、铸造可用；inputs 为 nil 即「今天没有任何输入读口」。
func newFormFixture(t *testing.T, inputs *inputResolverDouble) *formFixture {
	t.Helper()
	fixture := &formFixture{
		byRequest:  &evaluationByRequestDouble{byReference: map[string]domain.PricingEvaluation{}},
		priceCards: &priceCardResolverDouble{resolution: ports.PriceCardInForceResolution{Outcome: ports.PriceCardVersionInForce, Plan: buyPlan(t)}},
		identity:   &identityDouble{next: "EVAL-SYN-1"},
		inputs:     inputs,
		store:      &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
		downstream: &evaluationDownstreamDouble{},
	}
	evaluate := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:      fixture.store,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)},
	})
	deps := application.FormEvaluationFromRequestDeps{
		Evaluations: fixture.byRequest,
		PriceCards:  fixture.priceCards,
		Identity:    fixture.identity,
		Evaluate:    evaluate,
	}
	if inputs != nil {
		deps.Inputs = inputs
	}
	handler, err := application.NewFormEvaluationFromRequestHandler(deps)
	if err != nil {
		t.Fatalf("construct handler: %v", err)
	}
	fixture.handler = handler
	return fixture
}

func resolvedInputs(t *testing.T) *inputResolverDouble {
	t.Helper()
	return &inputResolverDouble{resolution: ports.PricingInputResolution{Outcome: ports.PricingInputResolved, Input: buyInput(t)}}
}

// Covers: 裁决 1 三步走通——解析到的那一版价卡 + 造出的快照 + 回指交给既有 EvaluatePricingHandler，评价入册、
// 意图交下游、回指在评价上；铸造只发生一次；解析价卡用的时点是计价基准时点，不是形成时刻。
func TestFormEvaluationFromRequestFormsAndBackReferencesTheRequest(t *testing.T) {
	fixture := newFormFixture(t, resolvedInputs(t))

	result, err := fixture.handler.Handle(context.Background(), requestCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed {
		t.Fatalf("outcome = %s, want FORMED", result.Outcome)
	}
	if !result.HasEvaluation {
		t.Fatal("formed without an evaluation")
	}
	reference, has := result.Evaluation.RequestReference()
	if !has || reference.String() != "EVREQ-SYN-1" {
		t.Fatalf("evaluation back-reference = %q present=%v", reference, has)
	}
	if result.Evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s issues=%v", result.Evaluation.Status(), result.Evaluation.Issues())
	}
	if result.Evaluation.ID().String() != "EVAL-SYN-1" || fixture.identity.minted != 1 {
		t.Fatalf("evaluation id = %s minted=%d", result.Evaluation.ID(), fixture.identity.minted)
	}
	if fixture.store.saved != 1 || len(fixture.downstream.intents) != 1 {
		t.Fatalf("saved=%d intents=%d", fixture.store.saved, len(fixture.downstream.intents))
	}
	if len(fixture.priceCards.asked) != 1 || !fixture.priceCards.asked[0].Equal(requestBasisAt) {
		t.Fatalf("price card resolved at %v, want the pricing basis time %v", fixture.priceCards.asked, requestBasisAt)
	}
	if len(fixture.inputs.queries) != 1 || fixture.inputs.queries[0].Sources.Occurrence != "SYN-OCC-1" || !fixture.inputs.queries[0].BasisAt.Equal(requestBasisAt) {
		t.Fatalf("input query = %+v", fixture.inputs.queries)
	}
}

// Covers: 做法 3「同请求重放 → 已存在」——回指命中时不解析价卡、不造快照、不铸标识、不入册第二份。
func TestFormEvaluationFromRequestAnswersExistingByBackReference(t *testing.T) {
	fixture := newFormFixture(t, resolvedInputs(t))
	first, err := fixture.handler.Handle(context.Background(), requestCommand(t))
	if err != nil || first.Outcome != application.RequestEvaluationFormed {
		t.Fatalf("first: %v outcome=%s", err, first.Outcome)
	}
	fixture.byRequest.byReference["tenant-1/EVREQ-SYN-1"] = first.Evaluation
	fixture.identity.next = "EVAL-SYN-2"

	second, err := fixture.handler.Handle(context.Background(), requestCommand(t))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Outcome != application.RequestEvaluationExisting {
		t.Fatalf("outcome = %s, want EXISTING", second.Outcome)
	}
	if !second.HasEvaluation || second.Evaluation.ID() != first.Evaluation.ID() {
		t.Fatalf("existing did not return the first evaluation: %v", second.Evaluation.ID())
	}
	if fixture.identity.minted != 1 || fixture.store.saved != 1 || len(fixture.priceCards.asked) != 1 || len(fixture.inputs.queries) != 1 {
		t.Fatalf("replay did work again: minted=%d saved=%d resolved=%d inputs=%d",
			fixture.identity.minted, fixture.store.saved, len(fixture.priceCards.asked), len(fixture.inputs.queries))
	}
}

// Covers: 裁决 3 的两格在入口上的落法——未配置点名缺价卡、不造快照不铸标识；适用冲突把候选带出交人裁，同样
// 零写入。两格都不是「未决」：恢复动作是登记 / 裁，不是等依赖。
func TestFormEvaluationFromRequestStopsWhenNoOrManyPriceCardsApply(t *testing.T) {
	fixture := newFormFixture(t, resolvedInputs(t))
	fixture.priceCards.resolution = ports.PriceCardInForceResolution{Outcome: ports.PriceCardNotConfigured}

	result, err := fixture.handler.Handle(context.Background(), requestCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestPriceCardNotConfigured || result.HasEvaluation {
		t.Fatalf("outcome = %s hasEvaluation=%v, want PRICE_CARD_NOT_CONFIGURED", result.Outcome, result.HasEvaluation)
	}

	first := pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-BUY-PLAN", "v1")
	second := pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-BUY-PLAN-B", "v1")
	fixture.priceCards.resolution = ports.PriceCardInForceResolution{
		Outcome:    ports.PriceCardApplicabilityConflict,
		Candidates: []domain.VersionReference{first, second},
	}
	result, err = fixture.handler.Handle(context.Background(), requestCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestPriceCardApplicabilityConflict {
		t.Fatalf("outcome = %s, want PRICE_CARD_APPLICABILITY_CONFLICT", result.Outcome)
	}
	if len(result.Candidates) != 2 || !result.Candidates[1].SameIdentity(second) {
		t.Fatalf("candidates = %v", result.Candidates)
	}
	if fixture.identity.minted != 0 || fixture.store.saved != 0 || len(fixture.inputs.queries) != 0 {
		t.Fatalf("stopped outcomes did work: minted=%d saved=%d inputs=%d", fixture.identity.minted, fixture.store.saved, len(fixture.inputs.queries))
	}
}

// Covers: 裁决 4「输入不可得」——今天生产装配没有任何输入读口（Inputs 为 nil）时，入口在解析价卡之后停下，
// 点名缺的三只读口（TF 发生项成员对象、NO / PS 实重尺寸、PS 邮编路线），不猜不填不拿默认重量顶；零写入、零铸造。
func TestFormEvaluationFromRequestNamesTheMissingReadPortsWhenNoInputResolverIsWired(t *testing.T) {
	fixture := newFormFixture(t, nil)

	result, err := fixture.handler.Handle(context.Background(), requestCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestPricingInputUnavailable || result.HasEvaluation {
		t.Fatalf("outcome = %s hasEvaluation=%v, want PRICING_INPUT_UNAVAILABLE", result.Outcome, result.HasEvaluation)
	}
	joined := strings.Join(result.Missing, "\n")
	for _, expected := range []string{"transport-fulfillment", "node-operations", "parcel-shipment", "postal route"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing read ports do not name %q:\n%s", expected, joined)
		}
	}
	if len(fixture.priceCards.asked) != 1 {
		t.Fatalf("price card must be resolved before the input stop; resolved %d times", len(fixture.priceCards.asked))
	}
	if fixture.identity.minted != 0 || fixture.store.saved != 0 {
		t.Fatalf("input-unavailable did work: minted=%d saved=%d", fixture.identity.minted, fixture.store.saved)
	}
}

// Covers: 接了输入读口而它答「不可得」——同一格，缺项照读口点名的转述，不替它补。
func TestFormEvaluationFromRequestRelaysTheResolverMissingList(t *testing.T) {
	inputs := &inputResolverDouble{resolution: ports.PricingInputResolution{
		Outcome: ports.PricingInputUnavailable,
		Missing: []string{"node-operations: measured weight for SYN-PKG-1 not yet registered"},
	}}
	fixture := newFormFixture(t, inputs)

	result, err := fixture.handler.Handle(context.Background(), requestCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestPricingInputUnavailable {
		t.Fatalf("outcome = %s, want PRICING_INPUT_UNAVAILABLE", result.Outcome)
	}
	if len(result.Missing) != 1 || result.Missing[0] != inputs.resolution.Missing[0] {
		t.Fatalf("missing = %v", result.Missing)
	}
	if fixture.identity.minted != 0 || fixture.store.saved != 0 {
		t.Fatalf("input-unavailable did work: minted=%d saved=%d", fixture.identity.minted, fixture.store.saved)
	}
}

// Covers: 受理门——租户 / 回指 / 范围 / 时点 / 证据层级缺一即未受理，方向与目的不配对同样未受理；发生项引用
// （身份或版本）缺席也未受理——它是步 ② 唯一能拿去键事实的来源引用，业务时点也从它上面来，一份没有发生项的
// 请求形成不了任何评价；零依赖调用。
func TestFormEvaluationFromRequestRejectsAnUnformedCommand(t *testing.T) {
	fixture := newFormFixture(t, resolvedInputs(t))

	mutations := map[string]func(*application.FormEvaluationFromRequestCommand){
		"tenant": func(command *application.FormEvaluationFromRequestCommand) { command.Tenant = domain.TenantID{} },
		"request": func(command *application.FormEvaluationFromRequestCommand) {
			command.Request = domain.EvaluationRequestReference{}
		},
		"scope":    func(command *application.FormEvaluationFromRequestCommand) { command.Scope = domain.PricingScopeID{} },
		"basis":    func(command *application.FormEvaluationFromRequestCommand) { command.BasisAt = time.Time{} },
		"evidence": func(command *application.FormEvaluationFromRequestCommand) { command.Evidence = "" },
		"direction": func(command *application.FormEvaluationFromRequestCommand) {
			command.Direction = domain.PricingDirectionSell
		},
		"purpose": func(command *application.FormEvaluationFromRequestCommand) { command.Purpose = "" },
		"occurrence": func(command *application.FormEvaluationFromRequestCommand) {
			command.Sources.Occurrence = ""
		},
		"occurrence version": func(command *application.FormEvaluationFromRequestCommand) {
			command.Sources.OccurrenceVersion = " "
		},
	}
	for name, mutate := range mutations {
		command := requestCommand(t)
		mutate(&command)
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.Outcome != application.RequestEvaluationNotAccepted {
			t.Fatalf("%s: outcome = %s, want NOT_ACCEPTED", name, result.Outcome)
		}
	}
	if len(fixture.priceCards.asked) != 0 || fixture.byRequest.calls != 0 || fixture.identity.minted != 0 || len(fixture.inputs.queries) != 0 {
		t.Fatal("not-accepted commands reached a dependency")
	}
}

// Covers: 依赖故障一律未决并指名停在哪一口——回指读口 / 解析读口 / 输入读口 / 铸造口；形成那一步的未决照
// EvaluatePricingHandler 的答案转述。
func TestFormEvaluationFromRequestReportsWhichDependencyLeftItUndecided(t *testing.T) {
	boom := errors.New("boom")

	fixture := newFormFixture(t, resolvedInputs(t))
	fixture.byRequest.err = boom
	result, _ := fixture.handler.Handle(context.Background(), requestCommand(t))
	if result.Outcome != application.RequestEvaluationUndecided || result.Reason != application.RequestEvaluationRegistryUnavailable {
		t.Fatalf("registry: outcome=%s reason=%s", result.Outcome, result.Reason)
	}

	fixture = newFormFixture(t, resolvedInputs(t))
	fixture.priceCards.err = boom
	result, _ = fixture.handler.Handle(context.Background(), requestCommand(t))
	if result.Outcome != application.RequestEvaluationUndecided || result.Reason != application.RequestPriceCardResolutionUnavailable {
		t.Fatalf("price cards: outcome=%s reason=%s", result.Outcome, result.Reason)
	}

	inputs := resolvedInputs(t)
	inputs.err = boom
	fixture = newFormFixture(t, inputs)
	result, _ = fixture.handler.Handle(context.Background(), requestCommand(t))
	if result.Outcome != application.RequestEvaluationUndecided || result.Reason != application.RequestPricingInputResolutionUnavailable {
		t.Fatalf("inputs: outcome=%s reason=%s", result.Outcome, result.Reason)
	}

	fixture = newFormFixture(t, resolvedInputs(t))
	fixture.identity.err = boom
	result, _ = fixture.handler.Handle(context.Background(), requestCommand(t))
	if result.Outcome != application.RequestEvaluationUndecided || result.Reason != application.RequestEvaluationIdentityUnavailable {
		t.Fatalf("identity: outcome=%s reason=%s", result.Outcome, result.Reason)
	}

	fixture = newFormFixture(t, resolvedInputs(t))
	fixture.store.err = boom
	result, _ = fixture.handler.Handle(context.Background(), requestCommand(t))
	if result.Outcome != application.RequestEvaluationUndecided || result.Reason != application.RequestEvaluationFormationUndecided {
		t.Fatalf("formation: outcome=%s reason=%s", result.Outcome, result.Reason)
	}
	if fixture.store.saved != 0 {
		t.Fatal("undecided formation saved an evaluation")
	}
}

// Covers: 构造期拒 nil——四口必备缺一即报 ErrNilDependency 并点名；Inputs 可缺席不是漏装。
func TestFormEvaluationFromRequestHandlerRejectsMissingDependencies(t *testing.T) {
	evaluate := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:      &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
		Downstream: &evaluationDownstreamDouble{},
		Clock:      fixedClock{at: requestBasisAt},
	})
	complete := application.FormEvaluationFromRequestDeps{
		Evaluations: &evaluationByRequestDouble{},
		PriceCards:  &priceCardResolverDouble{},
		Identity:    &identityDouble{},
		Evaluate:    evaluate,
	}
	if _, err := application.NewFormEvaluationFromRequestHandler(complete); err != nil {
		t.Fatalf("complete deps rejected: %v", err)
	}
	for name, strip := range map[string]func(*application.FormEvaluationFromRequestDeps){
		"evaluations": func(deps *application.FormEvaluationFromRequestDeps) { deps.Evaluations = nil },
		"price cards": func(deps *application.FormEvaluationFromRequestDeps) { deps.PriceCards = nil },
		"identity":    func(deps *application.FormEvaluationFromRequestDeps) { deps.Identity = nil },
		"evaluate":    func(deps *application.FormEvaluationFromRequestDeps) { deps.Evaluate = nil },
	} {
		deps := complete
		strip(&deps)
		_, err := application.NewFormEvaluationFromRequestHandler(deps)
		if !errors.Is(err, application.ErrNilDependency) {
			t.Fatalf("%s: err = %v, want ErrNilDependency", name, err)
		}
	}
}
