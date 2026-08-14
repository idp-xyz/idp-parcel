package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var etaClockAt = time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)

type etaKey struct {
	tenant    domain.TenantID
	parcel    domain.TrackedParcelReference
	milestone domain.MilestoneReference
}

type etaStoreDouble struct {
	current map[etaKey]domain.ETAPrediction
	findErr error
	saveErr error
	saves   int
}

func newETAStore() *etaStoreDouble {
	return &etaStoreDouble{current: map[etaKey]domain.ETAPrediction{}}
}

func (double *etaStoreDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
) (domain.ETAPrediction, bool, error) {
	if double.findErr != nil {
		return domain.ETAPrediction{}, false, double.findErr
	}
	eta, found := double.current[etaKey{tenant: tenant, parcel: parcel, milestone: milestone}]
	return eta, found, nil
}

func (double *etaStoreDouble) Save(_ context.Context, tenant domain.TenantID, eta domain.ETAPrediction) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.current[etaKey{tenant: tenant, parcel: eta.Parcel(), milestone: eta.Milestone()}] = eta
	double.saves++
	return nil
}

type gapKey struct {
	tenant      domain.TenantID
	parcel      domain.TrackedParcelReference
	expectation domain.ExpectedObservationReference
	windowRule  domain.ObservationWindowReference
}

type gapStoreDouble struct {
	current map[gapKey]domain.VisibilityGap
	findErr error
	saveErr error
	saves   int
}

func newGapStore() *gapStoreDouble {
	return &gapStoreDouble{current: map[gapKey]domain.VisibilityGap{}}
}

func (double *gapStoreDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	expectation domain.ExpectedObservationReference,
	windowRule domain.ObservationWindowReference,
) (domain.VisibilityGap, bool, error) {
	if double.findErr != nil {
		return domain.VisibilityGap{}, false, double.findErr
	}
	gap, found := double.current[gapKey{tenant: tenant, parcel: parcel, expectation: expectation, windowRule: windowRule}]
	return gap, found, nil
}

func (double *gapStoreDouble) Save(_ context.Context, tenant domain.TenantID, gap domain.VisibilityGap) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.current[gapKey{tenant: tenant, parcel: gap.Parcel(), expectation: gap.Expectation(), windowRule: gap.WindowRule()}] = gap
	double.saves++
	return nil
}

type etaIdentityDouble struct {
	next int
	err  error
}

func (double *etaIdentityDouble) NextETAVersionID(_ context.Context) (domain.ETAVersionID, error) {
	if double.err != nil {
		return domain.ETAVersionID{}, double.err
	}
	double.next++
	return domain.NewETAVersionID("eta-" + string(rune('0'+double.next)))
}

type etaDownstreamDouble struct {
	intents []ports.ETAHandoffIntent
	err     error
}

func (double *etaDownstreamDouble) HandOffETA(_ context.Context, intent ports.ETAHandoffIntent) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type gapDownstreamDouble struct {
	intents []ports.VisibilityGapHandoffIntent
	err     error
}

func (double *gapDownstreamDouble) HandOffVisibilityGap(
	_ context.Context,
	intent ports.VisibilityGapHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type etaFixture struct {
	handler     *application.FormETAHandler
	predictions *etaStoreDouble
	gaps        *gapStoreDouble
	identities  *etaIdentityDouble
	viewChain   *etaDownstreamDouble
	signalChain *gapDownstreamDouble
}

func newETAFixture(t *testing.T) *etaFixture {
	t.Helper()
	fixture := &etaFixture{
		predictions: newETAStore(),
		gaps:        newGapStore(),
		identities:  &etaIdentityDouble{},
		viewChain:   &etaDownstreamDouble{},
		signalChain: &gapDownstreamDouble{},
	}
	fixture.handler = application.NewFormETAHandler(application.FormETADeps{
		Predictions: fixture.predictions,
		Gaps:        fixture.gaps,
		Identities:  fixture.identities,
		ViewChain:   fixture.viewChain,
		SignalChain: fixture.signalChain,
		Clock:       fixedClock{at: etaClockAt},
	})
	return fixture
}

func etaCommand(t *testing.T, inputs string) application.FormETACommand {
	t.Helper()
	return application.FormETACommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Parcel:     mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:  mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:     domain.OperatorDerivedETA,
		Inputs:     mustValue(t, domain.NewPredictionInputsReference, inputs),
		Model:      mustValue(t, domain.NewPredictionModelReference, "eta-model/v1"),
		RangeFrom:  etaClockAt.Add(24 * time.Hour),
		RangeTo:    etaClockAt.Add(48 * time.Hour),
		Confidence: mustValue(t, domain.NewConfidenceReference, "MEDIUM/history-density"),
	}
}

func gapCommand(t *testing.T, windowRule string, windowEnd time.Time) application.FormVisibilityGapCommand {
	t.Helper()
	return application.FormVisibilityGapCommand{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Parcel:      mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Expectation: mustValue(t, domain.NewExpectedObservationReference, "LINEHAUL_ARRIVAL_SCAN"),
		WindowRule:  mustValue(t, domain.NewObservationWindowReference, windowRule),
		WindowEnd:   windowEnd,
	}
}

// Covers: CONTEXT「预测必须保存来源口径、输入事实、规则或模型版本、时间范围、可信度
// 与判断时间」——足够事实形成版本化内部 ETA，不修改承诺（类型上没有承诺字段）。点名
// `AT-VE-049`「有足够事实形成内部 ETA→保存输入、版本、范围和可信度，不修改承诺」。
func TestSufficientFactsFormAVersionedETA(t *testing.T) {
	fixture := newETAFixture(t)

	result, err := fixture.handler.FormETA(context.Background(), etaCommand(t, "inputs/v1"))
	if err != nil {
		t.Fatalf("form eta: %v", err)
	}

	if result.Outcome() != application.ETAFormed {
		t.Fatalf("outcome = %q, want ETA_FORMED", result.Outcome())
	}
	eta, present := result.ETA()
	if !present {
		t.Fatal("a formed result carries no prediction")
	}
	if eta.Inputs().String() != "inputs/v1" || eta.Model().String() != "eta-model/v1" ||
		eta.Confidence().String() == "" {
		t.Fatal("the prediction lost part of its inputs, model, or confidence")
	}
	from, to := eta.Range()
	if from.IsZero() || to.IsZero() || !to.After(from) {
		t.Fatalf("prediction range = %v..%v; a versioned range was expected", from, to)
	}
	if _, refreshed := eta.PriorVersion(); refreshed {
		t.Fatal("the first prediction version claims to refresh a prior one")
	}
	if len(fixture.viewChain.intents) != 1 ||
		fixture.viewChain.intents[0].Prediction.Version() != eta.Version() {
		t.Fatal("exactly one handoff intent claimed by the prediction version was expected")
	}
}

// Covers: CONTEXT「新的 ETA 形成新版本，不覆盖历史预测」——输入版本变化经 Refresh
// 换版并指回前版；幂等键是（包裹+里程碑+输入版本）。
func TestChangedInputsRefreshTheETAKeepingThePriorVersion(t *testing.T) {
	fixture := newETAFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.FormETA(ctx, etaCommand(t, "inputs/v1"))
	if err != nil {
		t.Fatalf("first form: %v", err)
	}
	firstETA, _ := first.ETA()

	refreshed, err := fixture.handler.FormETA(ctx, etaCommand(t, "inputs/v2"))
	if err != nil {
		t.Fatalf("refresh form: %v", err)
	}

	if refreshed.Outcome() != application.ETARefreshed {
		t.Fatalf("outcome = %q, want ETA_REFRESHED", refreshed.Outcome())
	}
	eta, _ := refreshed.ETA()
	prior, linked := eta.PriorVersion()
	if !linked || prior != firstETA.Version() {
		t.Fatalf("prior = %q, want the superseded version %q", prior, firstETA.Version())
	}
	if eta.Version() == firstETA.Version() {
		t.Fatal("the refreshed prediction reused the prior version identity")
	}
}

// Covers: 幂等半边——同（包裹+里程碑+输入版本）重放不重形成、不签新身份，只把同一份
// 意图再交一次（ADR-0043「重放重发同一份」）。
func TestTheSameInputsVersionDoesNotReformButResendsTheSameIntent(t *testing.T) {
	fixture := newETAFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.FormETA(ctx, etaCommand(t, "inputs/v1"))
	if err != nil {
		t.Fatalf("first form: %v", err)
	}
	firstETA, _ := first.ETA()
	issuedBefore, savedBefore := fixture.identities.next, fixture.predictions.saves

	replay, err := fixture.handler.FormETA(ctx, etaCommand(t, "inputs/v1"))
	if err != nil {
		t.Fatalf("replay form: %v", err)
	}

	if replay.Outcome() != application.ETAExistingResult {
		t.Fatalf("outcome = %q, want ETA_EXISTING_RESULT", replay.Outcome())
	}
	replayed, _ := replay.ETA()
	if replayed.Version() != firstETA.Version() {
		t.Fatal("the replay did not return the prediction that already exists")
	}
	if fixture.identities.next != issuedBefore || fixture.predictions.saves != savedBefore {
		t.Fatal("a replay consumed an identity or wrote the store again")
	}
	if len(fixture.viewChain.intents) != 2 ||
		fixture.viewChain.intents[1].Prediction.Version() != firstETA.Version() {
		t.Fatal("the replay must resend the same intent claimed by the existing prediction")
	}
}

// Covers: 受理半边与 `AT-VE-050` 的七件缺一半边「只有计划时间→ETA 未形成，不用计划
// 填充」——缺输入或模型的请求构不成预测，门口即未受理，不读依赖也不补默认。
func TestACommandMissingAnEssentialIsNotAcceptedAsAnETA(t *testing.T) {
	fixture := newETAFixture(t)

	missingInputs := etaCommand(t, "inputs/v1")
	missingInputs.Inputs = domain.PredictionInputsReference{}
	result, err := fixture.handler.FormETA(context.Background(), missingInputs)
	if err != nil {
		t.Fatalf("form without inputs: %v", err)
	}
	if result.Outcome() != application.FormETANotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}

	missingModel := etaCommand(t, "inputs/v1")
	missingModel.Model = domain.PredictionModelReference{}
	noModel, err := fixture.handler.FormETA(context.Background(), missingModel)
	if err != nil {
		t.Fatalf("form without model: %v", err)
	}
	if noModel.Outcome() != application.FormETANotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", noModel.Outcome())
	}
	if fixture.identities.next != 0 || fixture.predictions.saves != 0 {
		t.Fatal("an unaccepted command still consumed an identity or wrote the store")
	}
}

// Covers: CONTEXT「只有适用产品或履约段明确预期某项观察，且版本化观察窗口已经届满时，
// 才能形成可见性缺口信号」的未届满半边——如实答未成，不造缺口不落库不发意图。点名
// `AT-VE-053`「观察窗口未到→保持观察，不形成缺口或延误」。
func TestAGapIsNotFormedBeforeTheWindowElapses(t *testing.T) {
	fixture := newETAFixture(t)

	result, err := fixture.handler.FormVisibilityGap(
		context.Background(),
		gapCommand(t, "window-rule/v1", etaClockAt.Add(time.Hour)),
	)
	if err != nil {
		t.Fatalf("form gap: %v", err)
	}

	if result.Outcome() != application.GapWindowNotElapsed {
		t.Fatalf("outcome = %q, want GAP_WINDOW_NOT_ELAPSED", result.Outcome())
	}
	if _, formed := result.Gap(); formed {
		t.Fatal("an unelapsed window still produced a gap")
	}
	if fixture.gaps.saves != 0 || len(fixture.signalChain.intents) != 0 {
		t.Fatal("an unelapsed window wrote the store or handed off an intent")
	}
}

// Covers: CONTEXT「无扫描不能直接形成延误、停止移动或遗失结论」与领域「缺口只证明预期
// 数据尚未获得」——届满成缺口、意图交信号链，缺口类型上没有延误/遗失字段。点名
// `AT-VE-054`「观察窗口届满且无新事实→形成缺口，不认定停止或遗失」。
func TestAnElapsedWindowFormsTheGapWithoutConcludingLossOrStall(t *testing.T) {
	fixture := newETAFixture(t)

	result, err := fixture.handler.FormVisibilityGap(
		context.Background(),
		gapCommand(t, "window-rule/v1", etaClockAt.Add(-time.Hour)),
	)
	if err != nil {
		t.Fatalf("form gap: %v", err)
	}

	if result.Outcome() != application.GapFormed {
		t.Fatalf("outcome = %q, want GAP_FORMED", result.Outcome())
	}
	gap, present := result.Gap()
	if !present || gap.WindowRule().String() != "window-rule/v1" {
		t.Fatal("a formed result carries no gap with its window rule")
	}
	if fixture.gaps.saves != 1 {
		t.Fatalf("saves = %d, want 1", fixture.gaps.saves)
	}
	if len(fixture.signalChain.intents) != 1 ||
		fixture.signalChain.intents[0].Gap.Expectation() != gap.Expectation() {
		t.Fatal("exactly one handoff intent claimed by the gap identity was expected")
	}
}

// Covers: 缺口幂等与 `AT-VE-057`「新窗口版本生效→后续采用新版本，原判断保留」——同
// 身份三维重放读回原缺口只重发意图；换窗口规则版本是新判断，原缺口不被覆盖。
func TestAGapIsIdempotentPerWindowRuleVersion(t *testing.T) {
	fixture := newETAFixture(t)
	ctx := context.Background()
	elapsed := etaClockAt.Add(-time.Hour)

	if _, err := fixture.handler.FormVisibilityGap(ctx, gapCommand(t, "window-rule/v1", elapsed)); err != nil {
		t.Fatalf("first gap: %v", err)
	}

	replay, err := fixture.handler.FormVisibilityGap(ctx, gapCommand(t, "window-rule/v1", elapsed))
	if err != nil {
		t.Fatalf("replay gap: %v", err)
	}
	if replay.Outcome() != application.GapExistingResult {
		t.Fatalf("outcome = %q, want GAP_EXISTING_RESULT", replay.Outcome())
	}
	if fixture.gaps.saves != 1 || len(fixture.signalChain.intents) != 2 {
		t.Fatal("the replay must not re-save but must resend the same intent")
	}

	newRule, err := fixture.handler.FormVisibilityGap(ctx, gapCommand(t, "window-rule/v2", elapsed))
	if err != nil {
		t.Fatalf("new rule gap: %v", err)
	}
	if newRule.Outcome() != application.GapFormed {
		t.Fatalf("outcome = %q, want GAP_FORMED under the new window rule version", newRule.Outcome())
	}
	if fixture.gaps.saves != 2 {
		t.Fatal("the new window rule version must form its own judgment, keeping the original")
	}
}

// Covers: 未决语义——预测库与缺口库调不通分别停在各自原因，不虚构幂等或未成。
func TestUnavailableETAStoresAreUndecidedUnderTheirOwnReasons(t *testing.T) {
	predictions := newETAFixture(t)
	predictions.predictions.findErr = errors.New("eta store unavailable")
	result, err := predictions.handler.FormETA(context.Background(), etaCommand(t, "inputs/v1"))
	if err != nil {
		t.Fatalf("form eta: %v", err)
	}
	if result.Outcome() != application.FormETAUndecided ||
		result.UndecidedReason() != application.ETAStoreUnavailable {
		t.Fatalf("result = %q/%q, want UNDECIDED/ETA_STORE_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
	}

	gaps := newETAFixture(t)
	gaps.gaps.findErr = errors.New("gap store unavailable")
	result, err = gaps.handler.FormVisibilityGap(
		context.Background(),
		gapCommand(t, "window-rule/v1", etaClockAt.Add(-time.Hour)),
	)
	if err != nil {
		t.Fatalf("form gap: %v", err)
	}
	if result.Outcome() != application.FormETAUndecided ||
		result.UndecidedReason() != application.VisibilityGapStoreUnavailable {
		t.Fatalf("result = %q/%q, want UNDECIDED/VISIBILITY_GAP_STORE_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
	}
}

// Covers: ADR-0043「首次交付失败不改写业务结果……另留一条发布续办引用」——两条链的
// 意图交不出去时预测/缺口保持已成立，只留可续办引用。
func TestFailedHandoffsKeepTheResultsWithResumableReferences(t *testing.T) {
	fixture := newETAFixture(t)
	fixture.viewChain.err = errors.New("view chain unavailable")
	fixture.signalChain.err = errors.New("signal chain unavailable")

	eta, err := fixture.handler.FormETA(context.Background(), etaCommand(t, "inputs/v1"))
	if err != nil {
		t.Fatalf("form eta: %v", err)
	}
	if eta.Outcome() != application.ETAFormed || eta.HandoffReference() == "" {
		t.Fatalf("eta = %q ref=%q; a failed handoff must keep the prediction with a reference", eta.Outcome(), eta.HandoffReference())
	}

	gap, err := fixture.handler.FormVisibilityGap(
		context.Background(),
		gapCommand(t, "window-rule/v1", etaClockAt.Add(-time.Hour)),
	)
	if err != nil {
		t.Fatalf("form gap: %v", err)
	}
	if gap.Outcome() != application.GapFormed || gap.HandoffReference() == "" {
		t.Fatalf("gap = %q ref=%q; a failed handoff must keep the gap with a reference", gap.Outcome(), gap.HandoffReference())
	}
}
