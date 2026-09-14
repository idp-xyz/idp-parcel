package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证渠道择优编排落定后写一条「渠道择优决定」记录（票 `label-channel/14` 裁决：择优编排
// 落库后同事务写入）。三种非错误出口各成一条——选出、并列冲突、无人参选；成本表对不上或
// 币种不齐时不成记录（那一次没有比较发生）。登记口缺席时编排行为与此前一字不变。

var decisionClockNow = time.Date(2026, 9, 4, 18, 45, 0, 0, time.UTC)

type stubDecisionRegistry struct {
	appended []domain.ChannelSelectionDecision
	outcome  ports.ChannelSelectionDecisionAppendOutcome
	err      error
}

func (stub *stubDecisionRegistry) Append(
	_ context.Context,
	decision domain.ChannelSelectionDecision,
) (ports.ChannelSelectionDecisionAppendOutcome, error) {
	if stub.err != nil {
		return ports.ChannelSelectionDecisionAppendOutcomeInvalid, stub.err
	}
	stub.appended = append(stub.appended, decision)
	if stub.outcome == ports.ChannelSelectionDecisionAppendOutcomeInvalid {
		return ports.ChannelSelectionDecisionAppended, nil
	}
	return stub.outcome, nil
}

func (stub *stubDecisionRegistry) ListBySubject(
	_ context.Context,
	_ domain.TenantID,
	_ domain.ChannelSelectionSubject,
) ([]domain.ChannelSelectionDecision, error) {
	return stub.appended, nil
}

type stubDecisionIdentity struct{ next string }

func (stub stubDecisionIdentity) NextChannelSelectionDecisionID(context.Context) (domain.ChannelSelectionDecisionID, error) {
	return domain.NewChannelSelectionDecisionID(stub.next)
}

type stubClock struct{ now time.Time }

func (stub stubClock) Now() time.Time { return stub.now }

func recordingDeps(t testing.TB, costs ...domain.ChannelCandidateCost) (application.SelectChannelCandidateDeps, *stubDecisionRegistry) {
	t.Helper()

	candidates := make([]domain.ChannelCandidateID, 0, len(costs))
	for _, cost := range costs {
		candidates = append(candidates, cost.Candidate())
	}
	registry := &stubDecisionRegistry{}
	return application.SelectChannelCandidateDeps{
		Assembly:    stubAssembly{candidates: candidates},
		Costs:       stubCosts{costs: costs},
		Decisions:   registry,
		DecisionIDs: stubDecisionIdentity{next: "decision-1"},
		Clock:       stubClock{now: decisionClockNow},
	}, registry
}

func selectionUnpriceableCost(t testing.TB, id string, grade domain.ChannelCostUnavailability) domain.ChannelCandidateCost {
	t.Helper()

	cost, err := domain.UnpriceableChannelCandidate(selectionCandidate(t, id), grade)
	if err != nil {
		t.Fatalf("造不可计价候选 %s：%v", id, err)
	}
	return cost
}

// Covers: 选出唯一一条时写一条决定：选中者与编排交回的是同一个，对象引用与装配时点照入参取，
// 决定时刻取时钟，标识取签发口。
func TestASelectionRecordsTheDecisionItReturned(t *testing.T) {
	t.Parallel()

	deps, registry := recordingDeps(t,
		selectionPricedCost(t, "cand-cheap", "10.00"),
		selectionPricedCost(t, "cand-dear", "12.00"),
		selectionUnpriceableCost(t, "cand-out", domain.ChannelCostRatecardExclusion),
	)
	query := selectionQuery(t)

	result, err := application.NewSelectChannelCandidateHandler(deps).Select(context.Background(), query)
	if err != nil {
		t.Fatalf("择优：%v", err)
	}
	selected, present := result.Selected()
	if result.Outcome() != application.ChannelSelectionSelected || !present {
		t.Fatalf("outcome = %q / present = %v，want SELECTED 且带选中候选", result.Outcome(), present)
	}

	if len(registry.appended) != 1 {
		t.Fatalf("决定记录 %d 条，want 1", len(registry.appended))
	}
	decision := registry.appended[0]
	recorded, chosen := decision.Selected()
	if !chosen || recorded != selected.Candidate() {
		t.Fatalf("记录里的选中者 %v/%v 与交回的 %v 不是同一个", recorded, chosen, selected.Candidate())
	}
	if decision.ID().String() != "decision-1" || decision.Tenant() != query.Tenant ||
		decision.Subject().Scope() != query.Scope || decision.Subject().Mapping() != query.Mapping ||
		!decision.AssembledAsOf().Equal(query.At) || !decision.DecidedAt().Equal(decisionClockNow) {
		t.Fatalf("记录头部与入参不符：%+v", decision)
	}
	if len(decision.Results()) != 3 {
		t.Fatalf("逐候选结果 %d 条，want 3", len(decision.Results()))
	}
}

// Covers: 裁决「冲突也是一次决定的结果，同样留痕」与「全部出局」——两种不选中的结局各成一条记录，编排以
// **结果格**交回（票 35 做法二：它们是业务答案，不是错误），选中候选缺席，`error` 为 nil。
func TestTheTwoNonSelectingOutcomesAreRecordedToo(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		costs      []domain.ChannelCandidateCost
		want       application.ChannelSelectionOutcome
		conclusion domain.ChannelSelectionConclusion
	}{
		"最低价并列": {
			costs: []domain.ChannelCandidateCost{
				selectionPricedCost(t, "cand-a", "10.00"),
				selectionPricedCost(t, "cand-b", "10.00"),
			},
			want:       application.ChannelSelectionCostTied,
			conclusion: domain.ChannelSelectionConcludedTied,
		},
		"无人参选": {
			costs: []domain.ChannelCandidateCost{
				selectionUnpriceableCost(t, "cand-a", domain.ChannelCostPendingEvidence),
				selectionUnpriceableCost(t, "cand-b", domain.ChannelCostConflict),
			},
			want:       application.ChannelSelectionNoQualifiedCandidate,
			conclusion: domain.ChannelSelectionConcludedNoneQualified,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			deps, registry := recordingDeps(t, test.costs...)

			result, err := application.NewSelectChannelCandidateHandler(deps).Select(context.Background(), selectionQuery(t))
			if err != nil || result.Outcome() != test.want {
				t.Fatalf("outcome = %q err = %v，want %q 且无 error", result.Outcome(), err, test.want)
			}
			if _, present := result.Selected(); present {
				t.Fatal("没选出却带了选中候选")
			}
			if len(registry.appended) != 1 || registry.appended[0].Conclusion() != test.conclusion {
				t.Fatalf("决定记录 = %d 条 / %v，want 1 条 %s", len(registry.appended), registry.appended, test.conclusion)
			}
		})
	}
}

// Covers: 没有比较发生的两种停下——成本表对不上、币种不齐——不成记录。前者是编排自己的守卫，
// 后者是比较器拒绝比较；硬记会把一次没发生的择优写成一条决定。
func TestNoDecisionIsRecordedWhenNoComparisonHappened(t *testing.T) {
	t.Parallel()

	t.Run("成本表对不上", func(t *testing.T) {
		t.Parallel()
		deps, registry := recordingDeps(t, selectionPricedCost(t, "cand-a", "10.00"))
		deps.Assembly = stubAssembly{candidates: []domain.ChannelCandidateID{
			selectionCandidate(t, "cand-a"), selectionCandidate(t, "cand-b"),
		}}

		_, err := application.NewSelectChannelCandidateHandler(deps).Select(context.Background(), selectionQuery(t))
		if !errors.Is(err, application.ErrChannelCostsIncomplete) || len(registry.appended) != 0 {
			t.Fatalf("err = %v，记录 %d 条；want ErrChannelCostsIncomplete 且零记录", err, len(registry.appended))
		}
	})
	t.Run("币种不齐", func(t *testing.T) {
		t.Parallel()
		other, err := domain.PricedChannelCandidate(
			selectionCandidate(t, "cand-b"),
			selectionValue(t, domain.NewChannelCostAmount, "10.00"),
			selectionValue(t, domain.NewChannelCostCurrency, "SYX"),
		)
		if err != nil {
			t.Fatalf("造异币种候选：%v", err)
		}
		deps, registry := recordingDeps(t, selectionPricedCost(t, "cand-a", "10.00"), other)

		_, err = application.NewSelectChannelCandidateHandler(deps).Select(context.Background(), selectionQuery(t))
		if !errors.Is(err, domain.ErrChannelCostCurrencyMismatch) || len(registry.appended) != 0 {
			t.Fatalf("err = %v，记录 %d 条；want ErrChannelCostCurrencyMismatch 且零记录", err, len(registry.appended))
		}
	})
}

// Covers: 登记口写不进去时择优不算落定——同事务里两者要么一起成立、要么一起消失，编排把登记
// 的错误交回而不是吞掉后照常交出选中者。
func TestASelectionFailsWhenItsDecisionCannotBeRecorded(t *testing.T) {
	t.Parallel()

	deps, registry := recordingDeps(t, selectionPricedCost(t, "cand-a", "10.00"))
	registry.err = errors.New("registry unavailable")

	_, err := application.NewSelectChannelCandidateHandler(deps).Select(context.Background(), selectionQuery(t))
	if err == nil || !errors.Is(err, registry.err) {
		t.Fatalf("err = %v，want 包住 %v", err, registry.err)
	}
}

// Covers: 登记口、签发口、时钟三者只到一半是装配缺件，响亮失败而不是静默不记——静默不记与
// 「没配登记」在结果上同形，而装配缺件是要修的。
func TestAHalfConfiguredDecisionRegistryIsAnAssemblyDefect(t *testing.T) {
	t.Parallel()

	deps, _ := recordingDeps(t, selectionPricedCost(t, "cand-a", "10.00"))
	deps.Clock = nil

	_, err := application.NewSelectChannelCandidateHandler(deps).Select(context.Background(), selectionQuery(t))
	if !errors.Is(err, application.ErrChannelSelectionRecordingMisconfigured) {
		t.Fatalf("err = %v，want %v", err, application.ErrChannelSelectionRecordingMisconfigured)
	}
}

// Covers: 登记口缺席时编排行为与此前一字不变——选中者照常交回，什么都不记。这是 Deps 纯加法
// 的另一半：既有装配点不必知道这一格存在。
func TestSelectionWithoutADecisionRegistryBehavesAsBefore(t *testing.T) {
	t.Parallel()

	deps, _ := recordingDeps(t, selectionPricedCost(t, "cand-a", "10.00"), selectionPricedCost(t, "cand-b", "12.00"))
	deps.Decisions, deps.DecisionIDs, deps.Clock = nil, nil, nil

	result, err := application.NewSelectChannelCandidateHandler(deps).Select(context.Background(), selectionQuery(t))
	selected, present := result.Selected()
	if err != nil || !present || selected.Candidate().String() != "cand-a" {
		t.Fatalf("选中 = %v/%v err = %v，want cand-a", selected, present, err)
	}
}
