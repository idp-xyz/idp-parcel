package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var publishedAt = time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC)

func shownDimensions(t *testing.T) domain.CustomerViewDimensions {
	t.Helper()
	milestones, err := domain.ShowDimension(mustValue(t, domain.NewViewContentReference, "public-milestones/v3"))
	if err != nil {
		t.Fatalf("show milestones: %v", err)
	}
	return domain.CustomerViewDimensions{
		Milestones: milestones,
		ETA:        domain.PendDimension(),
		Final:      domain.WithholdDimension(),
		Note:       domain.PendDimension(),
	}
}

func publishedView(t *testing.T) domain.CustomerTrackingView {
	t.Helper()
	view, err := domain.PublishCustomerView(
		mustValue(t, domain.NewCustomerViewVersionID, "view-1/v1"),
		mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewProjectionVersionID, "projection-1/v1"),
		shownDimensions(t),
		publishedAt,
	)
	if err != nil {
		t.Fatalf("publish customer view: %v", err)
	}
	return view
}

// Covers: VE CONTEXT「某一维信息待确认或不可披露——只对该维返回待确认、暂不可用或
// 不展示，不虚构里程碑、ETA、终局或异常说明」——单维三态可并存于一个视图；展示无
// 内容与待确认带内容两个方向的虚构都在构造期拦下。
func TestDimensionsStayHonestPerAxis(t *testing.T) {
	view := publishedView(t)
	dimensions := view.Dimensions()

	if content, shown := dimensions.Milestones.Content(); !shown || content.String() != "public-milestones/v3" {
		t.Fatalf("milestones = %v shown = %v", content, shown)
	}
	if dimensions.ETA.State() != domain.DimensionPendingConfirmation {
		t.Fatalf("eta state = %q; 待确认维如实说等", dimensions.ETA.State())
	}
	if _, shown := dimensions.Final.Content(); shown {
		t.Fatal("不展示维交出了内容")
	}

	if _, err := domain.ShowDimension(domain.ViewContentReference{}); !errors.Is(err, domain.ErrInvalidCustomerView) {
		t.Fatalf("err = %v; 无中生有的展示维被收下了", err)
	}
}

// Covers: VE CONTEXT「客户全程追踪视图只基于当前有效的全程追踪投影……形成」与「客户
// 视图只包含当前货主客户账户」——投影锚与账户隔离都是必备字段；查询或展示不产生异常
// 披露或通知（类型上没有那些字段，结构性）。
func TestAViewDemandsItsProjectionAnchorAndAccountIsolation(t *testing.T) {
	if _, err := domain.PublishCustomerView(
		mustValue(t, domain.NewCustomerViewVersionID, "view-1/v1"),
		mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		domain.ProjectionVersionID{},
		shownDimensions(t),
		publishedAt,
	); !errors.Is(err, domain.ErrInvalidCustomerView) {
		t.Fatalf("err = %v; 没有投影锚的视图挂不回任何一次派生", err)
	}
	if _, err := domain.PublishCustomerView(
		mustValue(t, domain.NewCustomerViewVersionID, "view-1/v1"),
		domain.CustomerAccountReference{},
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewProjectionVersionID, "projection-1/v1"),
		shownDimensions(t),
		publishedAt,
	); !errors.Is(err, domain.ErrInvalidCustomerView) {
		t.Fatalf("err = %v; 没有账户维度的视图谈不上隔离", err)
	}
}

// Covers: VE CONTEXT「来源更正、有效性变化、身份谱系变化、ETA 新版本或终局更正——
// 形成新的客户视图版本并标明更正或替代关系；原发布历史保留」——替代换版本换投影锚
// 指回原版；原视图不可变；重号替代拒。
func TestSupersessionKeepsThePublishedHistory(t *testing.T) {
	view := publishedView(t)

	milestones, err := domain.ShowDimension(mustValue(t, domain.NewViewContentReference, "public-milestones/v4"))
	if err != nil {
		t.Fatalf("show milestones: %v", err)
	}
	final, err := domain.ShowDimension(mustValue(t, domain.NewViewContentReference, "final/NETWORK_SERVICE_DELIVERED"))
	if err != nil {
		t.Fatalf("show final: %v", err)
	}
	superseded, err := view.Supersede(
		mustValue(t, domain.NewCustomerViewVersionID, "view-1/v2"),
		mustValue(t, domain.NewProjectionVersionID, "projection-1/v2"),
		domain.CustomerViewDimensions{
			Milestones: milestones,
			ETA:        domain.WithholdDimension(),
			Final:      final,
			Note:       domain.PendDimension(),
		},
		publishedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}

	prior, present := superseded.PriorVersion()
	if !present || prior.String() != "view-1/v1" {
		t.Fatalf("prior = %s present = %v; 替代必须指回原版", prior, present)
	}
	if superseded.BasedOn().String() != "projection-1/v2" {
		t.Fatal("替代没有换投影锚")
	}
	if view.Version().String() != "view-1/v1" || view.BasedOn().String() != "projection-1/v1" {
		t.Fatal("原发布版本被改写了")
	}

	if _, err := view.Supersede(view.Version(),
		mustValue(t, domain.NewProjectionVersionID, "projection-1/v2"),
		shownDimensions(t), publishedAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidCustomerView) {
		t.Fatalf("err = %v; 重号的替代分不出两版", err)
	}
}

// Covers: VE CONTEXT「客户全程追踪视图的版本由披露更正与替代关系驱动，不与内部投影版本
// 一一对应；投影重新派生本身不要求形成新的客户视图版本」与「已经向客户发布的……出现
// 更正时，必须以追加更新或明确替代关系表达，不得静默删除或改写历史披露」——四维不变时
// 重新确认只前移确认依据，版本、四维、发布时间与形成时依据原样；四维变了不许走重新确认，那等于不换代
// 却换内容；投影没换不是重新确认。
func TestReaffirmFollowsANewerProjectionOnlyWhileTheDimensionsStayTheSame(t *testing.T) {
	view := publishedView(t)
	newer := mustValue(t, domain.NewProjectionVersionID, "projection-1/v2")

	reaffirmed, err := view.Reaffirm(newer, shownDimensions(t))
	if err != nil {
		t.Fatalf("reaffirm: %v", err)
	}
	if reaffirmed.ConfirmedOn() != newer {
		t.Fatalf("confirmed on = %s; 重新确认要跟上新投影", reaffirmed.ConfirmedOn())
	}
	if reaffirmed.BasedOn() != view.BasedOn() {
		t.Fatalf("based on = %s; 形成时依据是已发布版本的一部分，重新确认不动它", reaffirmed.BasedOn())
	}
	if reaffirmed.Version() != view.Version() || reaffirmed.Dimensions() != view.Dimensions() ||
		!reaffirmed.PublishedAt().Equal(view.PublishedAt()) {
		t.Fatal("重新确认改写了已发布的版本、四维或发布时间")
	}
	if _, superseding := reaffirmed.PriorVersion(); superseding {
		t.Fatal("重新确认凭空长出了前版")
	}
	if view.BasedOn().String() != "projection-1/v1" {
		t.Fatal("原视图被改写了")
	}

	changed := shownDimensions(t)
	changed.ETA = domain.WithholdDimension()
	if _, err := view.Reaffirm(newer, changed); !errors.Is(err, domain.ErrInvalidCustomerView) {
		t.Fatalf("err = %v; 四维变了必须换代", err)
	}
	if _, err := view.Reaffirm(view.ConfirmedOn(), shownDimensions(t)); !errors.Is(err, domain.ErrInvalidCustomerView) {
		t.Fatalf("err = %v; 投影没换是重放，不是重新确认", err)
	}
	if _, err := reaffirmed.Reaffirm(newer, shownDimensions(t)); !errors.Is(err, domain.ErrInvalidCustomerView) {
		t.Fatalf("err = %v; 已确认过的投影再来是重放，不是重新确认", err)
	}
	if _, err := view.Reaffirm(domain.ProjectionVersionID{}, shownDimensions(t)); !errors.Is(err, domain.ErrInvalidCustomerView) {
		t.Fatalf("err = %v; 没有投影锚的重新确认挂不回任何一次派生", err)
	}
}

// unclassifiedEntry 造一条未归类条目：终局不是里程碑，映射目录登不登它都不改变终局
// 有没有形成，夹具因此刻意不归类。
func unclassifiedEntry(
	t *testing.T,
	source domain.SourceContext,
	fact string,
	kind string,
) domain.MilestoneClassification {
	t.Helper()
	spec := factSpec(t)
	spec.Source = source
	spec.Fact = mustValue(t, domain.NewSourceFactReference, fact)
	spec.Kind = mustValue(t, domain.NewSourceFactKind, kind)
	accepted, err := domain.NewAcceptedSourceFact(spec)
	if err != nil {
		t.Fatalf("new accepted source fact: %v", err)
	}
	entry, err := domain.LeaveUnclassified(
		accepted, mustValue(t, domain.NewMappingVersionReference, "milestone-map/v1"))
	if err != nil {
		t.Fatalf("leave unclassified: %v", err)
	}
	return entry
}

func projectionWith(t *testing.T, entries ...domain.MilestoneClassification) domain.TrackingProjection {
	t.Helper()
	projection, err := domain.DeriveTrackingProjection(
		mustValue(t, domain.NewProjectionVersionID, "projection-1/v1"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		entries,
		publishedAt,
	)
	if err != nil {
		t.Fatalf("derive tracking projection: %v", err)
	}
	return projection
}

// Covers: VE CONTEXT「客户全程追踪视图只基于……`parcel-shipment` 提供的客户服务终局……
// 形成。它不得从内部案件、原始来源消息或单一交付扫描自行推导终局」与「某一维信息待
// 确认……只对该维返回待确认」——策略准许展示终局，终局一维仍要投影里有 PS 已采用的
// 终局才展示；不采用判断、别的上下文借用同一类型字面量都不算。策略不准看的，终局形成
// 了也不给看。点名 `AT-VE-159` 的展示半边「`parcel-shipment` 已形成当前有效客户服务
// 终局→展示终局及其依据版本，不由摘要反推」与 `AT-VE-160` 的终局半边「只有有效交付
// 扫描但终局尚未形成→可展示获准交付里程碑，不显示包裹服务终局已完成」。这里只判终局
// 一维：交付里程碑不在这一维上，终局的依据版本这一维也不带，这两半没有钉。
func TestTheFinalDimensionShowsOnlyWhenTheProjectionCarriesAnAdoptedFinal(t *testing.T) {
	permitted, err := domain.ShowDimension(mustValue(t, domain.NewViewContentReference, "final/v1"))
	if err != nil {
		t.Fatalf("show final: %v", err)
	}
	intake := classified(t, "parcel-1")
	delivered := unclassifiedEntry(t, domain.SourceTransportFulfillment,
		"effective-delivery/parcel-1/attempt-1", "effective-delivery")
	adopted := unclassifiedEntry(t, domain.SourceParcelShipment,
		"final-outcome/parcel-1/EFFECTIVE_DELIVERY", domain.FinalOutcomeAdoptedFactKind)
	notAdopted := unclassifiedEntry(t, domain.SourceParcelShipment,
		"final-outcome/parcel-1/EFFECTIVE_DELIVERY", domain.FinalOutcomeNotAdoptedFactKind)
	borrowed := unclassifiedEntry(t, domain.SourceTransportFulfillment,
		"final-outcome/parcel-1/EFFECTIVE_DELIVERY", domain.FinalOutcomeAdoptedFactKind)

	cases := []struct {
		name       string
		permitted  domain.ViewDimension
		projection domain.TrackingProjection
		want       domain.DimensionState
	}{
		{"在途", permitted, projectionWith(t, intake), domain.DimensionPendingConfirmation},
		{"只有有效交付", permitted, projectionWith(t, intake, delivered), domain.DimensionPendingConfirmation},
		{"终局已采用", permitted, projectionWith(t, intake, delivered, adopted), domain.DimensionShown},
		{"只有不采用判断", permitted, projectionWith(t, intake, notAdopted), domain.DimensionPendingConfirmation},
		{"别的上下文借用已采用字面量", permitted, projectionWith(t, intake, borrowed), domain.DimensionPendingConfirmation},
		{"策略不展示", domain.WithholdDimension(), projectionWith(t, intake, adopted), domain.DimensionNotDisclosed},
		{"策略待确认", domain.PendDimension(), projectionWith(t, intake, adopted), domain.DimensionPendingConfirmation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.FinalDimensionOver(tc.permitted, tc.projection)
			if got.State() != tc.want {
				t.Fatalf("final = %q, want %q", got.State(), tc.want)
			}
			content, shown := got.Content()
			if shown != (tc.want == domain.DimensionShown) {
				t.Fatalf("content shown = %v under state %q", shown, got.State())
			}
			if shown && content.String() != "final/v1" {
				t.Fatalf("content = %q; 展示的内容来处只能是策略批的那一份", content)
			}
		})
	}
}

// Covers: AT-VE-158、AT-VE-216、AT-VE-217 与 VE CONTEXT「客户全程追踪视图只基于当前有效的全程追踪投影、获准对客展示的 ETA……
// 形成」与生命周期「某一维信息待确认或不可披露 → 只对该维返回待确认、暂不可用或不展示，
// 不虚构里程碑、ETA、终局或异常说明」——策略准许展示 ETA 之后还要看 ETA 客户可见性判断：
// 没有判断或判断为待判断 → 待确认（AT-VE-217）；判断为 ETA 不可见 → 不展示（AT-VE-158，不用计划或承诺
// 时间填充）；判断为客户可见 ETA → 展示，内容来处是这一版判断。策略不展示与待确认原样放行，
// 只收窄不放宽——判断说可见也越不过策略。
func TestTheETADimensionFollowsTheCustomerVisibilityJudgementUnderThePolicy(t *testing.T) {
	permitted, err := domain.ShowDimension(mustValue(t, domain.NewViewContentReference, "eta-band/v1"))
	if err != nil {
		t.Fatalf("show eta: %v", err)
	}
	eta := candidateETA(t, "eta-1/v1")
	visible := judgementOver(t, "evj-visible", eta,
		domain.ConfiguredETARule(visibilityRule(t, "DELIVERED", "MEDIUM/history-based")))
	notVisible := judgementOver(t, "evj-hidden", eta,
		domain.ConfiguredETARule(visibilityRule(t, "DELIVERED", "HIGH/carrier-confirmed")))
	pending := judgementOver(t, "evj-pending", eta, domain.UnconfiguredETARule())

	for _, tc := range []struct {
		name      string
		permitted domain.ViewDimension
		judgement domain.ETAVisibilityJudgement
		judged    bool
		want      domain.DimensionState
		content   string
	}{
		{"没有判断", permitted, domain.ETAVisibilityJudgement{}, false, domain.DimensionPendingConfirmation, ""},
		{"判断待判断", permitted, pending, true, domain.DimensionPendingConfirmation, ""},
		{"判断 ETA 不可见", permitted, notVisible, true, domain.DimensionNotDisclosed, ""},
		{"判断客户可见 ETA", permitted, visible, true, domain.DimensionShown, "evj-visible"},
		{"策略不展示时判断可见也不放宽", domain.WithholdDimension(), visible, true, domain.DimensionNotDisclosed, ""},
		{"策略待确认时判断可见也不放宽", domain.PendDimension(), visible, true, domain.DimensionPendingConfirmation, ""},
		{"策略待确认时判断不可见也不改写", domain.PendDimension(), notVisible, true, domain.DimensionPendingConfirmation, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.ETADimensionOver(tc.permitted, tc.judgement, tc.judged)
			if got.State() != tc.want {
				t.Fatalf("eta = %q, want %q", got.State(), tc.want)
			}
			content, shown := got.Content()
			if shown != (tc.want == domain.DimensionShown) || (shown && content.String() != tc.content) {
				t.Fatalf("content = %q shown = %v; 展示的内容来处只能是这一版判断", content, shown)
			}
		})
	}
}

// Covers: VE CONTEXT「客户全程追踪视图只展示适用服务与授权允许的公开里程碑」与生命周期
// 「某一维信息待确认……不虚构里程碑」——策略准许展示里程碑，投影里却没有一条已归类为
// 标准追踪里程碑的条目（例如只有终局或初始路由那类未归类条目）时，这一维答待确认；
// 有一条已归类即照策略展示。策略不展示与待确认原样放行。
func TestTheMilestonesDimensionPendsUntilTheProjectionCarriesAClassifiedMilestone(t *testing.T) {
	permitted, err := domain.ShowDimension(mustValue(t, domain.NewViewContentReference, "public-milestones/v1"))
	if err != nil {
		t.Fatalf("show milestones: %v", err)
	}
	intake := classified(t, "parcel-1")
	route := unclassifiedEntry(t, domain.SourceNetworkRouting, "initial-route/parcel-1", "initial-route-formed")

	for _, tc := range []struct {
		name       string
		permitted  domain.ViewDimension
		projection domain.TrackingProjection
		want       domain.DimensionState
	}{
		{"只有未归类条目", permitted, projectionWith(t, route), domain.DimensionPendingConfirmation},
		{"有一条已归类", permitted, projectionWith(t, route, intake), domain.DimensionShown},
		{"策略不展示", domain.WithholdDimension(), projectionWith(t, intake), domain.DimensionNotDisclosed},
		{"策略待确认", domain.PendDimension(), projectionWith(t, intake), domain.DimensionPendingConfirmation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.MilestonesDimensionOver(tc.permitted, tc.projection)
			if got.State() != tc.want {
				t.Fatalf("milestones = %q, want %q", got.State(), tc.want)
			}
			content, shown := got.Content()
			if shown != (tc.want == domain.DimensionShown) {
				t.Fatalf("content shown = %v under state %q", shown, got.State())
			}
			if shown && content.String() != "public-milestones/v1" {
				t.Fatalf("content = %q; 展示的内容来处只能是策略批的那一份", content)
			}
		})
	}
}
