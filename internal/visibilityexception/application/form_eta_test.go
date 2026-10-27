package application_test

import (
	"context"
	"errors"
	"fmt"
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
	source    domain.ETASourceKind
}

type etaVersionKey struct {
	line    etaKey
	version domain.ETAVersionID
}

// etaStoreDouble 按预测线（租户+包裹+里程碑+来源）存当前版，并像真库那样把每个版本都留着：Save 只增版本、
// 移当前标记，不改写旧版。
type etaStoreDouble struct {
	current   map[etaKey]domain.ETAPrediction
	versions  map[etaVersionKey]domain.ETAPrediction
	findErr   error
	listErr   error
	lockErr   error
	inputsErr error
	saveErr   error
	saves     int
	locks     int
	finds     int
}

func newETAStore() *etaStoreDouble {
	return &etaStoreDouble{
		current:  map[etaKey]domain.ETAPrediction{},
		versions: map[etaVersionKey]domain.ETAPrediction{},
	}
}

func (double *etaStoreDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
	source domain.ETASourceKind,
) (domain.ETAPrediction, bool, error) {
	double.finds++
	if double.findErr != nil {
		return domain.ETAPrediction{}, false, double.findErr
	}
	eta, found := double.current[etaKey{tenant: tenant, parcel: parcel, milestone: milestone, source: source}]
	return eta, found, nil
}

func (double *etaStoreDouble) ListCurrent(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
) ([]domain.ETAPrediction, error) {
	if double.listErr != nil {
		return nil, double.listErr
	}
	var currents []domain.ETAPrediction
	for _, source := range []domain.ETASourceKind{domain.CarrierProvidedETA, domain.OperatorDerivedETA} {
		if eta, found := double.current[etaKey{tenant: tenant, parcel: parcel, milestone: milestone, source: source}]; found {
			currents = append(currents, eta)
		}
	}
	return currents, nil
}

func (double *etaStoreDouble) LockCurrent(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
	source domain.ETASourceKind,
) (domain.ETAPrediction, bool, error) {
	double.locks++
	if double.lockErr != nil {
		return domain.ETAPrediction{}, false, double.lockErr
	}
	eta, found := double.current[etaKey{tenant: tenant, parcel: parcel, milestone: milestone, source: source}]
	return eta, found, nil
}

func (double *etaStoreDouble) FindVersion(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
	source domain.ETASourceKind,
	version domain.ETAVersionID,
) (domain.ETAPrediction, bool, error) {
	eta, found := double.versions[etaVersionKey{
		line:    etaKey{tenant: tenant, parcel: parcel, milestone: milestone, source: source},
		version: version,
	}]
	return eta, found, nil
}

func (double *etaStoreDouble) FindByInputs(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
	milestone domain.MilestoneReference,
	source domain.ETASourceKind,
	inputs domain.PredictionInputsReference,
) (domain.ETAPrediction, bool, error) {
	if double.inputsErr != nil {
		return domain.ETAPrediction{}, false, double.inputsErr
	}
	line := etaKey{tenant: tenant, parcel: parcel, milestone: milestone, source: source}
	for key, eta := range double.versions {
		if key.line == line && eta.Inputs() == inputs {
			return eta, true, nil
		}
	}
	return domain.ETAPrediction{}, false, nil
}

func (double *etaStoreDouble) Save(_ context.Context, tenant domain.TenantID, eta domain.ETAPrediction) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	line := etaKey{tenant: tenant, parcel: eta.Parcel(), milestone: eta.Milestone(), source: eta.Source()}
	double.current[line] = eta
	double.versions[etaVersionKey{line: line, version: eta.Version()}] = eta
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
	return etaCommandFrom(t, domain.OperatorDerivedETA, inputs)
}

func etaCommandFrom(t *testing.T, source domain.ETASourceKind, inputs string) application.FormETACommand {
	t.Helper()
	return application.FormETACommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Parcel:     mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:  mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:     source,
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

// Covers: CONTEXT「ETA 与可见性缺口」一节「每次 ETA 必须明确预测对象、目标里程碑、预测时点、输入事实、规则或模型版本、
// 预计时间范围和可信程度」——足够事实形成版本化内部 ETA，不修改承诺（类型上没有承诺字段）。点名
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

// Covers: AT-VE-222 与 CONTEXT「新的 ETA 形成新版本，不覆盖历史预测」——输入版本变化经 Refresh
// 换版并指回前版；幂等键是（包裹+里程碑+来源+输入版本）。
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

// Covers: AT-VE-222 的重放半边——同（包裹+里程碑+来源+输入版本）重放不重形成、不签新身份，只把同一份
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

// Covers: AT-VE-051 与 CONTEXT「承运商提供的预测与运营企业派生的预测分别保留来源和口径。新的 ETA 形成新版本，不覆盖历史预测」
// ——两路来源各是一条预测线：另一路来源带着同名输入版本来也是它自己线上的首版（不是重放），运营企业一路刷新不触碰承运商那一路，
// 承运商那一路重放只认它自己线上的输入版本。
func TestTheTwoSourcesKeepSeparateLinesAndNeverOverwriteEachOther(t *testing.T) {
	fixture := newETAFixture(t)
	ctx := context.Background()

	operatorFirst, err := fixture.handler.FormETA(ctx, etaCommandFrom(t, domain.OperatorDerivedETA, "inputs/shared"))
	if err != nil {
		t.Fatalf("operator first: %v", err)
	}
	operatorV1, _ := operatorFirst.ETA()

	carrierFirst, err := fixture.handler.FormETA(ctx, etaCommandFrom(t, domain.CarrierProvidedETA, "inputs/shared"))
	if err != nil {
		t.Fatalf("carrier first: %v", err)
	}
	if carrierFirst.Outcome() != application.ETAFormed {
		t.Fatalf("另一路来源的首份 outcome = %q, want ETA_FORMED：同名输入版本在另一条线上不是重放", carrierFirst.Outcome())
	}
	carrierV1, _ := carrierFirst.ETA()
	if _, linked := carrierV1.PriorVersion(); linked || carrierV1.Source() != domain.CarrierProvidedETA {
		t.Fatalf("承运商首份 = %#v，不该指回运营企业那一路", carrierV1)
	}

	operatorRefresh, err := fixture.handler.FormETA(ctx, etaCommandFrom(t, domain.OperatorDerivedETA, "inputs/v2"))
	if err != nil {
		t.Fatalf("operator refresh: %v", err)
	}
	operatorV2, _ := operatorRefresh.ETA()
	if prior, linked := operatorV2.PriorVersion(); operatorRefresh.Outcome() != application.ETARefreshed || !linked || prior != operatorV1.Version() {
		t.Fatalf("运营企业刷新 = %q 指回 %q, want REFRESHED 且指回 %q", operatorRefresh.Outcome(), prior, operatorV1.Version())
	}

	command := etaCommandFrom(t, domain.CarrierProvidedETA, "inputs/shared")
	tenant, parcel, milestone := command.TenantID, command.Parcel, command.Milestone
	if current, _, _ := fixture.predictions.LockCurrent(ctx, tenant, parcel, milestone, domain.CarrierProvidedETA); current.Version() != carrierV1.Version() {
		t.Fatalf("承运商线的当前版 = %q，被运营企业那一路的刷新动了；want %q", current.Version(), carrierV1.Version())
	}
	replay, err := fixture.handler.FormETA(ctx, command)
	if err != nil {
		t.Fatalf("carrier replay: %v", err)
	}
	if again, _ := replay.ETA(); replay.Outcome() != application.ETAExistingResult || again.Version() != carrierV1.Version() {
		t.Fatalf("承运商重放 = %q 版本 %q, want EXISTING_RESULT 且仍是 %q", replay.Outcome(), again.Version(), carrierV1.Version())
	}
	if fixture.predictions.saves != 3 {
		t.Fatalf("saves = %d, want 3（两路首份 + 运营企业一次刷新），重放不写", fixture.predictions.saves)
	}
}

// Covers: AT-VE-222 与 CONTEXT「新的 ETA 形成新版本，不覆盖历史预测」与 GLOSSARY「新的 ETA 不覆盖历史」——同一条线上连续三版：
// 每个被接替的版本都还在库里、按版本读得回，输入与区间原样；指回环环相扣；别的来源线名下查不到它们。
func TestEveryRefreshedVersionStaysReadableByVersion(t *testing.T) {
	fixture := newETAFixture(t)
	ctx := context.Background()

	var versions []domain.ETAPrediction
	for _, inputs := range []string{"inputs/v1", "inputs/v2", "inputs/v3"} {
		result, err := fixture.handler.FormETA(ctx, etaCommand(t, inputs))
		if err != nil {
			t.Fatalf("form %s: %v", inputs, err)
		}
		eta, _ := result.ETA()
		versions = append(versions, eta)
	}

	command := etaCommand(t, "inputs/v1")
	for index, formed := range versions {
		kept, found, err := fixture.predictions.FindVersion(ctx, command.TenantID, command.Parcel, command.Milestone,
			domain.OperatorDerivedETA, formed.Version())
		if err != nil || !found {
			t.Fatalf("第 %d 版读不回：found=%v err=%v", index+1, found, err)
		}
		if kept.Inputs() != formed.Inputs() {
			t.Fatalf("第 %d 版的输入被改写：%q vs %q", index+1, kept.Inputs(), formed.Inputs())
		}
		prior, linked := kept.PriorVersion()
		if index == 0 && linked {
			t.Fatal("首版带了指回")
		}
		if index > 0 && (!linked || prior != versions[index-1].Version()) {
			t.Fatalf("第 %d 版指回 %q, want %q", index+1, prior, versions[index-1].Version())
		}
		if _, found, _ := fixture.predictions.FindVersion(ctx, command.TenantID, command.Parcel, command.Milestone,
			domain.CarrierProvidedETA, formed.Version()); found {
			t.Fatalf("第 %d 版在承运商线名下也读得到：版本不属于别的来源线", index+1)
		}
	}
}

// Covers: AT-VE-222 与 CONTEXT「同一条线上相同输入版本的重复到达，不论先后，不形成新版本」——输入版本依次到 A、B、A：第三个是
// 这条线上曾经出现过的输入版本，答当初形成的那一版，不形成新版本、不签身份、不移当前版、不交意图；线上的当前版仍是 B。
// 重复到达当前版自己的输入版本时重发同一份意图。
func TestAnInputVersionThatAppearedBeforeOnTheLineIsAReplayNoMatterWhenItArrives(t *testing.T) {
	fixture := newETAFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.FormETA(ctx, etaCommand(t, "inputs/A"))
	if err != nil {
		t.Fatalf("form A: %v", err)
	}
	second, err := fixture.handler.FormETA(ctx, etaCommand(t, "inputs/B"))
	if err != nil {
		t.Fatalf("form B: %v", err)
	}
	firstETA, _ := first.ETA()
	secondETA, _ := second.ETA()
	issued, saved, intents := fixture.identities.next, fixture.predictions.saves, len(fixture.viewChain.intents)

	late, err := fixture.handler.FormETA(ctx, etaCommand(t, "inputs/A"))
	if err != nil {
		t.Fatalf("late A: %v", err)
	}
	if lateETA, _ := late.ETA(); late.Outcome() != application.ETAExistingResult || lateETA.Version() != firstETA.Version() {
		t.Fatalf("晚到的旧输入版本 outcome = %q 版本 %q, want EXISTING_RESULT 且是当初形成的 %q",
			late.Outcome(), lateETA.Version(), firstETA.Version())
	}
	if late.HandoffReference() != "" {
		t.Fatalf("续办引用 = %q，晚到的旧输入版本不该交意图", late.HandoffReference())
	}
	if fixture.identities.next != issued || fixture.predictions.saves != saved || len(fixture.viewChain.intents) != intents {
		t.Fatal("晚到的旧输入版本签了身份、写了库或交了意图")
	}
	command := etaCommand(t, "inputs/A")
	if current, _, _ := fixture.predictions.LockCurrent(ctx, command.TenantID, command.Parcel, command.Milestone,
		domain.OperatorDerivedETA); current.Version() != secondETA.Version() {
		t.Fatalf("线上当前版 = %q，晚到的旧输入把线推回去了；want %q", current.Version(), secondETA.Version())
	}

	again, err := fixture.handler.FormETA(ctx, etaCommand(t, "inputs/B"))
	if err != nil {
		t.Fatalf("repeat B: %v", err)
	}
	if againETA, _ := again.ETA(); again.Outcome() != application.ETAExistingResult || againETA.Version() != secondETA.Version() ||
		len(fixture.viewChain.intents) != intents+1 {
		t.Fatal("当前版自己的输入版本重复到达应答已有预测并重发同一份意图")
	}
}

// Covers: AT-VE-223 同一预测线的写者串行（ADR-0065 追加版本之上的并发纪律）——写者先 LockCurrent 取线锁再决定，不走不带锁的读：
// 不带锁的读之后并发的两个首版会双双看到「没有当前版」。
func TestTheWriterTakesTheLineLockBeforeDecidingAndNeverReadsTheCurrentUnlocked(t *testing.T) {
	fixture := newETAFixture(t)
	ctx := context.Background()

	for _, inputs := range []string{"inputs/v1", "inputs/v1", "inputs/v2"} {
		if _, err := fixture.handler.FormETA(ctx, etaCommand(t, inputs)); err != nil {
			t.Fatalf("form %s: %v", inputs, err)
		}
	}
	if fixture.predictions.locks != 3 || fixture.predictions.finds != 0 {
		t.Fatalf("locks = %d finds = %d, want 3 与 0：写路径只经线锁读当前版", fixture.predictions.locks, fixture.predictions.finds)
	}
}

// Covers: AT-VE-223 与 ADR-0029 按恢复动作分格——线上并发输家（首版被占、前版已变）与预测库读不动是两种恢复：前者重读再走，不是等库；
// 输家不留意图，也不当成已形成。
func TestALostRaceOnTheLineIsUndecidedUnderItsOwnReason(t *testing.T) {
	fixture := newETAFixture(t)
	fixture.predictions.saveErr = fmt.Errorf("save eta: %w", ports.ErrETACurrentChanged)

	result, err := fixture.handler.FormETA(context.Background(), etaCommand(t, "inputs/v1"))
	if err != nil {
		t.Fatalf("form eta: %v", err)
	}
	if result.Outcome() != application.FormETAUndecided || result.UndecidedReason() != application.ETACurrentChanged {
		t.Fatalf("result = %q/%q, want UNDECIDED/ETA_CURRENT_CHANGED", result.Outcome(), result.UndecidedReason())
	}
	if _, present := result.ETA(); present || len(fixture.viewChain.intents) != 0 {
		t.Fatal("并发输家不该带着预测或交出意图")
	}

	fixture.predictions.saveErr = errors.New("eta store unavailable")
	unavailable, err := fixture.handler.FormETA(context.Background(), etaCommand(t, "inputs/v1"))
	if err != nil {
		t.Fatalf("form eta: %v", err)
	}
	if unavailable.UndecidedReason() != application.ETAStoreUnavailable {
		t.Fatalf("reason = %q, want ETA_STORE_UNAVAILABLE：库写不动与输了并发不是一格", unavailable.UndecidedReason())
	}
}

// Covers: 受理半边与 `AT-VE-050` 的缺件半边「只有计划时间→ETA 未形成，不用计划
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
	predictions.predictions.lockErr = errors.New("eta store unavailable")
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
