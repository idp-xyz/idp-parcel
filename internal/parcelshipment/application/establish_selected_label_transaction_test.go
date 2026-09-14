package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证渠道择优作面单交易建立前置步的编排（票 label-channel/28 裁乙、判据 2）：择优 → 29 翻译 →
// Establish 三段串起来，择优交回冲突 / 无人参选 / 未配置、翻译停下时**不建立交易**，停点各自可分辨。
//
// 三段各自的规则不在这里重证——择优归 SelectChannelCandidateHandler、翻译归 adapters/partycommercial、
// 建立归 LabelTransactionHandler，各有各的测试。编排自己的风险是**在段与段之间掉东西**或**停下了还往下走**。

type selectorDouble struct {
	result application.ChannelSelectionResult
	err    error
	calls  int
}

func (double *selectorDouble) Select(context.Context, ports.ChannelSelectionQuery) (application.ChannelSelectionResult, error) {
	double.calls++
	return double.result, double.err
}

// lookupDouble 是建立前那一问的替身：交易在不在、读口通不通。
type lookupDouble struct {
	existing domain.LabelTransaction
	found    bool
	err      error
	calls    int
}

func (double *lookupDouble) FindByID(context.Context, domain.TenantID, domain.LabelTransactionID) (domain.LabelTransaction, bool, error) {
	double.calls++
	return double.existing, double.found, double.err
}

// selectionResultOf 借真的择优编排造出三格结果：结果类型的字段不导出，只能从真编排的答复里取——这也保证
// 替身交出的对象与择优编排真交出的一字不差（形照 appliedEstablishment）。
func selectionResultOf(t *testing.T, costs ...domain.ChannelCandidateCost) application.ChannelSelectionResult {
	t.Helper()
	candidates := make([]domain.ChannelCandidateID, 0, len(costs))
	for _, cost := range costs {
		candidates = append(candidates, cost.Candidate())
	}
	handler := application.NewSelectChannelCandidateHandler(application.SelectChannelCandidateDeps{
		Assembly: stubAssembly{candidates: candidates},
		Costs:    stubCosts{costs: costs},
	})
	result, err := handler.Select(context.Background(), selectionQuery(t))
	if err != nil {
		t.Fatalf("造择优结果：%v", err)
	}
	return result
}

type translatorDouble struct {
	basis    domain.SelectedChannelBasis
	err      error
	received domain.SelectedChannelCandidate
	calls    int
}

func (double *translatorDouble) TranslateSelectedCandidate(
	_ context.Context,
	_ ports.ChannelSelectionQuery,
	selected domain.SelectedChannelCandidate,
) (domain.SelectedChannelBasis, error) {
	double.calls++
	double.received = selected
	return double.basis, double.err
}

type establisherDouble struct {
	result   application.LabelTransactionResult
	err      error
	received application.EstablishLabelTransactionCommand
	calls    int
}

func (double *establisherDouble) Establish(
	_ context.Context,
	command application.EstablishLabelTransactionCommand,
) (application.LabelTransactionResult, error) {
	double.calls++
	double.received = command
	return double.result, double.err
}

type selectedFlowFixture struct {
	handler     *application.EstablishSelectedLabelTransactionHandler
	lookup      *lookupDouble
	selector    *selectorDouble
	translator  *translatorDouble
	establisher *establisherDouble
}

func newSelectedFlowFixture(t *testing.T) *selectedFlowFixture {
	t.Helper()
	fixture := &selectedFlowFixture{
		lookup:      &lookupDouble{},
		selector:    &selectorDouble{result: selectionResultOf(t, selectionPricedCost(t, "CAND-1", "10.00"))},
		translator:  &translatorDouble{basis: selectedBasisFixture(t)},
		establisher: &establisherDouble{},
	}
	fixture.handler = application.NewEstablishSelectedLabelTransactionHandler(application.EstablishSelectedLabelTransactionDeps{
		Lookup:       fixture.lookup,
		Selector:     fixture.selector,
		Translator:   fixture.translator,
		Transactions: fixture.establisher,
	})
	return fixture
}

func (fixture *selectedFlowFixture) selectedCandidate(t *testing.T) domain.SelectedChannelCandidate {
	t.Helper()
	selected, present := fixture.selector.result.Selected()
	if !present {
		t.Fatal("夹具的择优结果没有选中候选")
	}
	return selected
}

func selectedFlowCommand(t *testing.T) application.EstablishSelectedLabelTransactionCommand {
	t.Helper()
	return application.EstablishSelectedLabelTransactionCommand{
		Tenant:        mustValue(t, domain.NewTenantID, "tenant-1"),
		TransactionID: mustValue(t, domain.NewLabelTransactionID, "LT-1"),
		CoveredParcels: []domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "PARCEL-1"),
		},
		Selection: selectionQuery(t),
	}
}

// Covers: 判据 2 正路——择优交回的候选原样交给翻译，翻译交回的择优结果原样嵌进建立命令（覆盖范围、标识、
// 关系照命令），建立的结果原样交回；三段各恰调一次。
func TestASelectedCandidateIsTranslatedAndHandedToEstablish(t *testing.T) {
	fixture := newSelectedFlowFixture(t)
	fixture.establisher.result = appliedEstablishment(t)
	command := selectedFlowCommand(t)
	command.PriorTransactionID = mustValue(t, domain.NewLabelTransactionID, "LT-0")
	command.PriorLinkKind = domain.LabelTransactionRetry

	result, err := fixture.handler.Establish(context.Background(), command)
	if err != nil {
		t.Fatalf("establish selected: %v", err)
	}
	if result.Outcome() != application.SelectedLabelTransactionEstablished {
		t.Fatalf("outcome = %q, want ESTABLISHED", result.Outcome())
	}
	if fixture.lookup.calls != 1 || fixture.selector.calls != 1 || fixture.translator.calls != 1 || fixture.establisher.calls != 1 {
		t.Fatalf("calls = %d/%d/%d/%d, want 1/1/1/1", fixture.lookup.calls, fixture.selector.calls, fixture.translator.calls, fixture.establisher.calls)
	}
	if fixture.translator.received.Candidate() != fixture.selectedCandidate(t).Candidate() {
		t.Fatal("翻译收到的不是择优交回的那个候选")
	}
	received := fixture.establisher.received
	if received.Basis != fixture.translator.basis || received.TransactionID != command.TransactionID ||
		received.Tenant != command.Tenant || len(received.CoveredParcels) != 1 ||
		received.PriorTransactionID != command.PriorTransactionID || received.PriorLinkKind != domain.LabelTransactionRetry {
		t.Fatalf("建立命令变形：%#v", received)
	}
	establishment, present := result.Establishment()
	if !present || establishment.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("建立结果没原样交回：%v/%v", establishment.Outcome(), present)
	}
	if basis, present := result.Basis(); !present || basis != fixture.translator.basis {
		t.Fatal("择优结果没随结果交回")
	}
}

// Covers: 判据 2「择优交回冲突……不建立交易，停点各自可分辨」——并列冲突与无人参选是择优步已记决定的两个
// 业务答案，以结果格交回（票 35 做法二），本层各映一格；翻译与建立一步都不走。
func TestATiedOrEmptySelectionStopsBeforeTranslationAndEstablishment(t *testing.T) {
	cases := map[string]struct {
		selection application.ChannelSelectionResult
		want      application.SelectedLabelTransactionOutcome
	}{
		"tied": {
			selectionResultOf(t, selectionPricedCost(t, "CAND-A", "10.00"), selectionPricedCost(t, "CAND-B", "10.00")),
			application.ChannelSelectionTied,
		},
		"no qualified": {
			selectionResultOf(t, selectionUnpriceableCost(t, "CAND-A", domain.ChannelCostPendingEvidence)),
			application.NoQualifiedChannelCandidate,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newSelectedFlowFixture(t)
			fixture.selector.result = testCase.selection

			result, err := fixture.handler.Establish(context.Background(), selectedFlowCommand(t))
			if err != nil {
				t.Fatalf("establish selected: %v", err)
			}
			if result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), testCase.want)
			}
			if fixture.translator.calls != 0 || fixture.establisher.calls != 0 {
				t.Fatal("择优没选出还往下走了")
			}
			if _, present := result.Establishment(); present {
				t.Fatal("没建立却交回了建立结果")
			}
		})
	}
}

// Covers: 判据 2「未配置……停点各自可分辨」——择优步没比出来（取数口未配置、成本表不全、留痕半配、依赖故障）
// 由它自己具名，本编排只标明停在**择优那一段**并原样带出成因，不建立交易；翻译停下同理标明停在**翻译那一段**。
// 两段的成因错误来自适配器层、本层认不出具体哪一格，所以只加段标不改写——调用方对段标与成因各自 Is。
func TestSelectionAndTranslationStopsAreNamedByStageAndCarryTheirCause(t *testing.T) {
	notConfigured := errors.New("synthetic: channel constraint not configured")

	t.Run("selection stopped", func(t *testing.T) {
		fixture := newSelectedFlowFixture(t)
		fixture.selector.err = notConfigured

		_, err := fixture.handler.Establish(context.Background(), selectedFlowCommand(t))
		if !errors.Is(err, application.ErrChannelSelectionStopped) || !errors.Is(err, notConfigured) {
			t.Fatalf("error = %v, want ErrChannelSelectionStopped wrapping the cause", err)
		}
		if fixture.translator.calls != 0 || fixture.establisher.calls != 0 {
			t.Fatal("择优停下还往下走了")
		}
	})

	t.Run("translation stopped", func(t *testing.T) {
		fixture := newSelectedFlowFixture(t)
		fixture.translator.err = notConfigured

		_, err := fixture.handler.Establish(context.Background(), selectedFlowCommand(t))
		if !errors.Is(err, application.ErrChannelBasisTranslationStopped) || !errors.Is(err, notConfigured) {
			t.Fatalf("error = %v, want ErrChannelBasisTranslationStopped wrapping the cause", err)
		}
		if errors.Is(err, application.ErrChannelSelectionStopped) {
			t.Fatal("翻译段的停点被标成了择优段")
		}
		if fixture.establisher.calls != 0 {
			t.Fatal("翻译停下还建立了交易")
		}
	})
}

// Covers: 建立一步的业务拒绝（输入未受理 / 原交易未定案 / 重放……）原样交回，不折进本层的格——恢复动作在 06
// 的代数里已经分好；建立步的依赖故障照样是错误。
func TestEstablishmentAnswersPassThroughUnchanged(t *testing.T) {
	fixture := newSelectedFlowFixture(t)
	fixture.establisher.result = priorNotFinalizedEstablishment(t)

	result, err := fixture.handler.Establish(context.Background(), selectedFlowCommand(t))
	if err != nil {
		t.Fatalf("establish selected: %v", err)
	}
	if result.Outcome() != application.SelectedLabelTransactionEstablished {
		t.Fatalf("outcome = %q; 建立步答过了，本层只交回它的答案", result.Outcome())
	}
	if establishment, present := result.Establishment(); !present || establishment.Outcome() != application.LabelTransactionPriorNotFinalized {
		t.Fatalf("建立结果没原样交回：%#v", establishment)
	}

	down := errors.New("synthetic: repository down")
	fixture.establisher.err = down
	if _, err := fixture.handler.Establish(context.Background(), selectedFlowCommand(t)); !errors.Is(err, down) {
		t.Fatalf("error = %v, want the establishment failure surfaced", err)
	}
}

// Covers: 装配缺件响亮失败——任一口为 nil 不得静默走到别的段。
func TestAHalfWiredFlowRefusesLoudly(t *testing.T) {
	handler := application.NewEstablishSelectedLabelTransactionHandler(application.EstablishSelectedLabelTransactionDeps{})
	if _, err := handler.Establish(context.Background(), selectedFlowCommand(t)); !errors.Is(err, application.ErrSelectedLabelTransactionFlowMisconfigured) {
		t.Fatalf("error = %v, want ErrSelectedLabelTransactionFlowMisconfigured", err)
	}
	full := newSelectedFlowFixture(t)
	withoutLookup := application.NewEstablishSelectedLabelTransactionHandler(application.EstablishSelectedLabelTransactionDeps{
		Selector: full.selector, Translator: full.translator, Transactions: full.establisher,
	})
	if _, err := withoutLookup.Establish(context.Background(), selectedFlowCommand(t)); !errors.Is(err, application.ErrSelectedLabelTransactionFlowMisconfigured) {
		t.Fatalf("缺建立前那一问的口：error = %v, want ErrSelectedLabelTransactionFlowMisconfigured", err)
	}
	if full.selector.calls != 0 {
		t.Fatal("缺口时不得择优")
	}
}

// Covers: 票 35 完成判据 2（裁决 1 取甲）——同 TransactionID 再调一次：交易已在即直接答重放（建立结果为
// `ALREADY_APPLIED`、既有交易原样带回），**不择优、不翻译、不记决定**；选中候选与择优结果缺席——这一次没有择优。
// 读口故障是依赖故障，原样上抛，同样不择优。
func TestAnExistingTransactionIsReplayedWithoutSelectingAgain(t *testing.T) {
	fixture := newSelectedFlowFixture(t)
	established := appliedEstablishment(t)
	existing, _ := established.Transaction()
	fixture.lookup.existing, fixture.lookup.found = existing, true

	result, err := fixture.handler.Establish(context.Background(), selectedFlowCommand(t))
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if result.Outcome() != application.SelectedLabelTransactionEstablished {
		t.Fatalf("outcome = %q, want ESTABLISHED（建立步的现名，不新造格）", result.Outcome())
	}
	establishment, present := result.Establishment()
	if !present || establishment.Outcome() != application.LabelTransactionAlreadyApplied {
		t.Fatalf("建立结果 = %v/%v, want ALREADY_APPLIED", establishment.Outcome(), present)
	}
	if replayed, ok := establishment.Transaction(); !ok || replayed.ID() != existing.ID() {
		t.Fatal("重放没带回既有那一笔")
	}
	if fixture.selector.calls != 0 || fixture.translator.calls != 0 || fixture.establisher.calls != 0 {
		t.Fatalf("重放还择优 / 翻译 / 建立了：%d/%d/%d", fixture.selector.calls, fixture.translator.calls, fixture.establisher.calls)
	}
	if _, present := result.Selected(); present {
		t.Fatal("重放时没有这一次的择优，选中候选该缺席")
	}
	if _, present := result.Basis(); present {
		t.Fatal("重放时没有这一次的翻译，择优结果该缺席")
	}

	down := errors.New("synthetic: repository down")
	fixture.lookup.err = down
	if _, err := fixture.handler.Establish(context.Background(), selectedFlowCommand(t)); !errors.Is(err, down) {
		t.Fatalf("读口故障该原样上抛：%v", err)
	}
	if fixture.selector.calls != 0 {
		t.Fatal("读口故障还择优了")
	}
}

// appliedEstablishment / priorNotFinalizedEstablishment 借真的 LabelTransactionHandler 造出本层要透传的两种建立
// 结果：结果类型的字段不导出，只能从真编排的答复里取——这也保证透传的对象与 06 真交出的一字不差。
func appliedEstablishment(t *testing.T) application.LabelTransactionResult {
	t.Helper()
	fixture := newLabelTransactionFixture(t)
	result, err := fixture.handler.Establish(context.Background(), fixture.establishCommand(t, "LT-1"))
	if err != nil || result.Outcome() != application.LabelTransactionApplied {
		t.Fatalf("造已建立结果：%v / %v", result.Outcome(), err)
	}
	return result
}

func priorNotFinalizedEstablishment(t *testing.T) application.LabelTransactionResult {
	t.Helper()
	fixture := newLabelTransactionFixture(t)
	fixture.mustEstablish(t, "LT-1")
	fixture.mustSubmit(t, "LT-1")
	fixture.mustMarkUncertain(t, "LT-1")
	command := fixture.establishCommand(t, "LT-2")
	command.PriorTransactionID = mustValue(t, domain.NewLabelTransactionID, "LT-1")
	command.PriorLinkKind = domain.LabelTransactionRetry
	result, err := fixture.handler.Establish(context.Background(), command)
	if err != nil || result.Outcome() != application.LabelTransactionPriorNotFinalized {
		t.Fatalf("造原交易未定案结果：%v / %v", result.Outcome(), err)
	}
	return result
}
