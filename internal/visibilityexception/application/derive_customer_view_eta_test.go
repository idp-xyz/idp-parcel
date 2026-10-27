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

// judgementReadDouble 只替派生编排答「这个账户这件包裹的当前判断是哪一版」，不是库的行为凭据。
type judgementReadDouble struct {
	current domain.ETAVisibilityJudgement
	has     bool
	err     error
	calls   int
}

func (double *judgementReadDouble) FindCurrent(
	_ context.Context,
	_ domain.TenantID,
	_ domain.CustomerAccountReference,
	_ domain.TrackedParcelReference,
) (domain.ETAVisibilityJudgement, bool, error) {
	double.calls++
	if double.err != nil {
		return domain.ETAVisibilityJudgement{}, false, double.err
	}
	return double.current, double.has, nil
}

var _ ports.ETAVisibilityJudgementRead = (*judgementReadDouble)(nil)

func (double *judgementReadDouble) set(judgement domain.ETAVisibilityJudgement) {
	double.current, double.has = judgement, true
}

func viewJudgementRule(t *testing.T, qualifying string) domain.ETAVisibilityRuleBasis {
	t.Helper()
	rule, err := domain.NewETAVisibilityRule(domain.ETAVisibilityRuleSpec{
		GateVersion:     mustValue(t, domain.NewETAQualityGateVersion, "gate/v1"),
		ServiceRule:     mustValue(t, domain.NewCustomerServiceRuleVersion, "contract-c1/v3"),
		TargetMilestone: mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Qualifying:      []domain.ConfidenceReference{mustValue(t, domain.NewConfidenceReference, qualifying)},
		Expression:      domain.ETAShownAsTimeRange,
		StopBoundary:    mustValue(t, domain.NewETAStopShowingBoundary, "stop/after-delivered"),
	})
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}
	return domain.ConfiguredETARule(rule)
}

// viewJudgement 造一份判断：候选 ETA 的可信度依据是 MEDIUM/history-based，规则把 qualifying
// 登为达标——登 MEDIUM/history-based 则可见，登别的则不可见；basis 给 nil 时规则缺失。
func viewJudgement(t *testing.T, version string, etaVersion string, qualifying string) domain.ETAVisibilityJudgement {
	t.Helper()
	eta, err := domain.FormETAPrediction(domain.ETAPredictionSpec{
		Version:     mustValue(t, domain.NewETAVersionID, etaVersion),
		Parcel:      mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:   mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:      domain.OperatorDerivedETA,
		Inputs:      mustValue(t, domain.NewPredictionInputsReference, "inputs/"+etaVersion),
		Model:       mustValue(t, domain.NewPredictionModelReference, "eta-model/v4"),
		RangeFrom:   factOccurredAt.Add(30 * time.Hour),
		RangeTo:     factOccurredAt.Add(42 * time.Hour),
		Confidence:  mustValue(t, domain.NewConfidenceReference, "MEDIUM/history-based"),
		PredictedAt: factOccurredAt,
	})
	if err != nil {
		t.Fatalf("form eta: %v", err)
	}
	basis := domain.UnconfiguredETARule()
	if qualifying != "" {
		basis = viewJudgementRule(t, qualifying)
	}
	finding, err := domain.AssessETAVisibility(eta, basis)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	judgement, err := domain.FormETAVisibilityJudgement(
		mustValue(t, domain.NewETAVisibilityJudgementVersionID, version), viewCustomer(t), finding,
		factOccurredAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("form judgement: %v", err)
	}
	return judgement
}

// Covers: AT-VE-158、AT-VE-217——ETA 客户可见性判断答「ETA 不可见」（质量未达对客门槛）：客户视图 ETA 一维
// 不展示，不带内容，也不拿计划或承诺时间填；判断答「客户可见 ETA」：这一维展示，内容来处是这一版判断；
// 判断答「待判断」：待确认。三格各一用例，策略都准许展示。
func TestTheETADimensionFollowsTheCurrentCustomerVisibilityJudgement(t *testing.T) {
	for _, tc := range []struct {
		name       string
		judgement  domain.ETAVisibilityJudgement
		want       domain.DimensionState
		wantSource string
	}{
		{"客户可见 ETA", viewJudgement(t, "evj-1", "eta-1", "MEDIUM/history-based"), domain.DimensionShown, "evj-1"},
		{"ETA 不可见（AT-VE-158）", viewJudgement(t, "evj-1", "eta-1", "HIGH/carrier-confirmed"), domain.DimensionNotDisclosed, ""},
		{"ETA 可见性待判断", viewJudgement(t, "evj-1", "eta-1", ""), domain.DimensionPendingConfirmation, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newViewFixture(t)
			fixture.judgements.set(tc.judgement)

			result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			view, _ := result.View()
			eta := view.Dimensions().ETA
			if eta.State() != tc.want {
				t.Fatalf("eta = %q, want %q", eta.State(), tc.want)
			}
			content, shown := eta.Content()
			if shown != (tc.wantSource != "") || content.String() != tc.wantSource {
				t.Fatalf("eta content = %q shown = %v, want source %q", content, shown, tc.wantSource)
			}
			if fixture.judgements.calls != 1 {
				t.Fatalf("judgement reads = %d, want 1", fixture.judgements.calls)
			}
		})
	}
}

// Covers: AT-VE-216——策略仍是上限——策略不展示 ETA 时不去读判断（读不动不该把一个本来就不展示的维度拖成
// 未决），判断说可见也不放宽。
func TestAPolicyThatDoesNotShowETANeverConsultsTheJudgement(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.policy.answer.ETA = ports.DimensionDisclosure{State: domain.DimensionNotDisclosed}
	fixture.judgements.set(viewJudgement(t, "evj-1", "eta-1", "MEDIUM/history-based"))
	fixture.judgements.err = errors.New("judgement store down")

	result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	view, _ := result.View()
	if view.Dimensions().ETA.State() != domain.DimensionNotDisclosed || fixture.judgements.calls != 0 {
		t.Fatalf("eta = %q, judgement reads = %d", view.Dimensions().ETA.State(), fixture.judgements.calls)
	}
}

// Covers: AT-VE-215 的未形成半边与 ADR-0029——判断库读不动是未决（重试依赖），不是「没有判断」：把读不动说成待确认，会让
// 一次故障把本该展示的 ETA 悄悄压成待确认并发布出去。不签身份、不写视图。
func TestAnUnreadableJudgementStoreIsUndecidedRatherThanPending(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.judgements.err = errors.New("judgement store down")

	result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.CustomerViewUndecided ||
		result.UndecidedReason() != application.ETAJudgementUnavailable {
		t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
	}
	if fixture.identities.next != 0 || fixture.views.saved != 0 || len(fixture.downstream.intents) != 0 {
		t.Fatal("读不动判断库的派生仍然签了身份、写了视图或发了意图")
	}
}

// Covers: AT-VE-215 与 CONTEXT「ETA 新版本……必须重新派生客户视图并形成追加更正或明确替代关系。原已发布版本保留」
// ——投影版本没变，只有判断换了版：由判断变化触发的重派生不被「同一投影版本重放」短路，ETA 一维的
// 内容来处随判断换版，四维变了，新视图指回前版，前一版按版本读得回。
func TestAJudgementChangeOnTheSameProjectionSupersedesTheViewKeepingHistory(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()
	fixture.judgements.set(viewJudgement(t, "evj-1", "eta-1", "MEDIUM/history-based"))

	first, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstView, _ := first.View()

	fixture.judgements.set(viewJudgement(t, "evj-2", "eta-2", "HIGH/carrier-confirmed"))
	command := viewCommand(t, "projection-1")
	command.ETAJudgementChanged = true
	second, err := fixture.handler.Handle(ctx, command)
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}

	if second.Outcome() != application.CustomerViewPublished {
		t.Fatalf("outcome = %q, want PUBLISHED; 判断换版被当成同一投影版本重放吞掉了", second.Outcome())
	}
	secondView, _ := second.View()
	if secondView.Dimensions().ETA.State() != domain.DimensionNotDisclosed {
		t.Fatalf("eta = %q, want NOT_DISCLOSED", secondView.Dimensions().ETA.State())
	}
	if prior, superseding := secondView.PriorVersion(); !superseding || prior != firstView.Version() {
		t.Fatalf("prior = %q, want %q", prior, firstView.Version())
	}
	kept, found, err := fixture.views.FindVersion(ctx, command.TenantID, command.Customer,
		command.Projection.Parcel(), firstView.Version())
	if err != nil || !found {
		t.Fatalf("前一版读不回：found = %v err = %v", found, err)
	}
	if content, shown := kept.Dimensions().ETA.Content(); !shown || content.String() != "evj-1" {
		t.Fatalf("前一版的 ETA 来处被改写：%q shown = %v", content, shown)
	}
}

// Covers: AT-VE-215——判断换了版而客户看到的四维没变（例如新 ETA 仍待判断）：不换代、不签身份、不写版本，当前
// 版原样答已有结果——客户读到的仍是同一版。判断变化触发的重派生也不会让没变的视图空转出新版本。
func TestAJudgementChangeThatLeavesTheDimensionsUnchangedKeepsTheViewVersion(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()
	fixture.judgements.set(viewJudgement(t, "evj-1", "eta-1", ""))

	first, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstView, _ := first.View()
	issued, saved, reaffirmed := fixture.identities.next, fixture.views.saved, fixture.views.reaffirmed

	fixture.judgements.set(viewJudgement(t, "evj-2", "eta-2", ""))
	command := viewCommand(t, "projection-1")
	command.ETAJudgementChanged = true
	second, err := fixture.handler.Handle(ctx, command)
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}

	if second.Outcome() != application.CustomerViewExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", second.Outcome())
	}
	if view, _ := second.View(); view.Version() != firstView.Version() {
		t.Fatalf("view version = %q, want the unchanged %q", view.Version(), firstView.Version())
	}
	if fixture.identities.next != issued || fixture.views.saved != saved || fixture.views.reaffirmed != reaffirmed {
		t.Fatal("四维没变的判断重派生仍然签了身份、写了版本或前移了确认依据")
	}
}
