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
	selected domain.SelectedChannelCandidate
	err      error
	calls    int
}

func (double *selectorDouble) Select(context.Context, ports.ChannelSelectionQuery) (domain.SelectedChannelCandidate, error) {
	double.calls++
	return double.selected, double.err
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
	selector    *selectorDouble
	translator  *translatorDouble
	establisher *establisherDouble
}

func newSelectedFlowFixture(t *testing.T) *selectedFlowFixture {
	t.Helper()
	selected, err := domain.NewSelectedChannelCandidate(mustValue(t, domain.NewChannelCandidateID, "CAND-1"))
	if err != nil {
		t.Fatalf("造选中候选：%v", err)
	}
	fixture := &selectedFlowFixture{
		selector:    &selectorDouble{selected: selected},
		translator:  &translatorDouble{basis: selectedBasisFixture(t)},
		establisher: &establisherDouble{},
	}
	fixture.handler = application.NewEstablishSelectedLabelTransactionHandler(application.EstablishSelectedLabelTransactionDeps{
		Selector:     fixture.selector,
		Translator:   fixture.translator,
		Transactions: fixture.establisher,
	})
	return fixture
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
	if fixture.selector.calls != 1 || fixture.translator.calls != 1 || fixture.establisher.calls != 1 {
		t.Fatalf("calls = %d/%d/%d, want 1/1/1", fixture.selector.calls, fixture.translator.calls, fixture.establisher.calls)
	}
	if fixture.translator.received.Candidate() != fixture.selector.selected.Candidate() {
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
// 业务答案，各成一格；翻译与建立一步都不走。
func TestATiedOrEmptySelectionStopsBeforeTranslationAndEstablishment(t *testing.T) {
	cases := map[string]struct {
		err  error
		want application.SelectedLabelTransactionOutcome
	}{
		"tied":         {domain.ErrChannelCandidateCostTied, application.ChannelSelectionTied},
		"no qualified": {domain.ErrNoQualifiedChannelCandidate, application.NoQualifiedChannelCandidate},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newSelectedFlowFixture(t)
			fixture.selector.err = testCase.err

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

// Covers: 装配缺件响亮失败——三口任一为 nil 不得静默走到别的段。
func TestAHalfWiredFlowRefusesLoudly(t *testing.T) {
	handler := application.NewEstablishSelectedLabelTransactionHandler(application.EstablishSelectedLabelTransactionDeps{})
	if _, err := handler.Establish(context.Background(), selectedFlowCommand(t)); !errors.Is(err, application.ErrSelectedLabelTransactionFlowMisconfigured) {
		t.Fatalf("error = %v, want ErrSelectedLabelTransactionFlowMisconfigured", err)
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
