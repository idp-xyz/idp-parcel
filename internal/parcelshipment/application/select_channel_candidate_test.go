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

// 本文件证渠道择优编排（票 `label-channel/12` 接线那一步）。它把三段串起来：装配出候选、
// 逐候选取成本、按成本单维择优。
//
// 三段各自的规则不在这里重证——收窄归 party-commercial、评价归 parcel-pricing、出局与并列
// 归本上下文的比较器，各有各的测试。**编排自己的风险是段与段之间掉东西**：装配出来的候选
// 在成本表里不见了、或者多出一个从没装配过的候选。两者在下游都看不出来。

func selectionQuery(t testing.TB) ports.ChannelSelectionQuery {
	t.Helper()

	return ports.ChannelSelectionQuery{
		Tenant:  selectionValue(t, domain.NewTenantID, "tenant-1"),
		Scope:   selectionValue(t, domain.NewCommercialScopeReference, "scope-a"),
		Mapping: selectionValue(t, domain.NewProductChannelMappingReference, "mapping-1"),
		At:      time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	}
}

func selectionValue[T any](t testing.TB, constructor func(string) (T, error), value string) T {
	t.Helper()
	result, err := constructor(value)
	if err != nil {
		t.Fatalf("构造 %q：%v", value, err)
	}
	return result
}

func selectionCandidate(t testing.TB, id string) domain.ChannelCandidateID {
	t.Helper()
	return selectionValue(t, domain.NewChannelCandidateID, id)
}

func selectionPricedCost(t testing.TB, id, amount string) domain.ChannelCandidateCost {
	t.Helper()

	cost, err := domain.PricedChannelCandidate(
		selectionCandidate(t, id),
		selectionValue(t, domain.NewChannelCostAmount, amount),
		selectionValue(t, domain.NewChannelCostCurrency, "SYN"),
	)
	if err != nil {
		t.Fatalf("造已定价候选 %s：%v", id, err)
	}
	return cost
}

type stubAssembly struct {
	candidates []domain.ChannelCandidateID
	err        error
}

func (stub stubAssembly) AssembleChannelCandidates(
	_ context.Context,
	_ ports.ChannelSelectionQuery,
) ([]domain.ChannelCandidateID, error) {
	return stub.candidates, stub.err
}

type stubCosts struct {
	costs []domain.ChannelCandidateCost
	err   error
}

func (stub stubCosts) ChannelCandidateCosts(
	_ context.Context,
	_ ports.ChannelSelectionQuery,
	_ []domain.ChannelCandidateID,
) ([]domain.ChannelCandidateCost, error) {
	return stub.costs, stub.err
}

// Covers: 票 29 判据 2——择优步交出的不只是「选中了谁」，还带赢家按之出价的评价痕迹与费率引用，且与
// Costs 口交回的那一份同源（不是另取的）；Handle 的旧签名照旧只交候选标识（expand，不改旧调用点）。
func TestASelectionHandsOverTheWinnersEvaluationAndRate(t *testing.T) {
	t.Parallel()

	evaluation := selectionValue(t, domain.NewChannelCostEvaluationReference, "SYN-EVAL-A")
	rate := selectionValue(t, domain.NewChannelRateReference, "SYN-BUY-PLAN-A/v1")
	winner, err := selectionPricedCost(t, "cand-a", "12.00").WithEvaluation(evaluation)
	if err != nil {
		t.Fatalf("赢家带评价：%v", err)
	}
	winner, err = winner.WithRate(rate)
	if err != nil {
		t.Fatalf("赢家带费率：%v", err)
	}
	handler := application.NewSelectChannelCandidateHandler(application.SelectChannelCandidateDeps{
		Assembly: stubAssembly{candidates: []domain.ChannelCandidateID{
			selectionCandidate(t, "cand-a"),
			selectionCandidate(t, "cand-b"),
		}},
		Costs: stubCosts{costs: []domain.ChannelCandidateCost{
			winner,
			selectionPricedCost(t, "cand-b", "15.00"),
		}},
	})

	selected, err := handler.Select(context.Background(), selectionQuery(t))
	if err != nil {
		t.Fatalf("择优：%v", err)
	}
	if selected.Candidate() != selectionCandidate(t, "cand-a") {
		t.Fatalf("选中 = %s，want cand-a", selected.Candidate().String())
	}
	if got, present := selected.Evaluation(); !present || got != evaluation {
		t.Fatalf("评价痕迹 = %v/%v，want %v", got, present, evaluation)
	}
	if got, present := selected.Rate(); !present || got != rate {
		t.Fatalf("费率 = %v/%v，want %v", got, present, rate)
	}

	plain, err := handler.Handle(context.Background(), selectionQuery(t))
	if err != nil || plain != selected.Candidate() {
		t.Fatalf("Handle = %s/%v，want 与 Select 同一个候选", plain.String(), err)
	}
}

// Covers: 成本表与装配出的候选对不上时编排停下，不把差额当成「那个候选不参选」。
//
// 这一格是编排**独有**的风险：三段各自都对，掉东西只发生在段与段之间。取成本口少答一条时，
// 比较器照样能从剩下的里选出一个「最便宜的」并且一路绿——而它是从一个比客户合同允许的更小
// 的集合里选出来的，少掉的那个可能恰好最便宜。下游没有任何东西看得出集合被削过。
//
// 摆法取「装配出两个、成本只回一个」，且回的那个是**装配过的**：若回一个没装配过的，错法
// 就变成另一种（多出候选），两种要分开钉。
func TestASelectionStopsWhenTheCostsDoNotCoverEveryAssembledCandidate(t *testing.T) {
	t.Parallel()

	handler := application.NewSelectChannelCandidateHandler(application.SelectChannelCandidateDeps{
		Assembly: stubAssembly{candidates: []domain.ChannelCandidateID{
			selectionCandidate(t, "cand-a"),
			selectionCandidate(t, "cand-b"),
		}},
		Costs: stubCosts{costs: []domain.ChannelCandidateCost{
			selectionPricedCost(t, "cand-a", "12.00"),
		}},
	})

	_, err := handler.Handle(context.Background(), selectionQuery(t))
	if !errors.Is(err, application.ErrChannelCostsIncomplete) {
		t.Fatalf("择优 err = %v，want %v", err, application.ErrChannelCostsIncomplete)
	}
}
