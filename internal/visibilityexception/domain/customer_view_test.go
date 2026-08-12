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
