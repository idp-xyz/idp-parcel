package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件钉交接、场外揽收、有效交付与外部承运轨迹四路认集运单元：对象那一刻是单元就入账不派生，不当一件追踪包裹投，也不投到
// 成员上；是包裹照旧派生。

// unitQuestionDouble 只答「是不是单元」。这几路不投到成员上，问成员即用例失败：成员身份那一侧的故障不该挡住它们的答复。
type unitQuestionDouble struct {
	t     *testing.T
	units map[string]bool
	err   error
	asked []veports.ConsolidationMembershipQuery
}

func (double *unitQuestionDouble) IsUnitAt(_ context.Context, query veports.ConsolidationMembershipQuery) (bool, error) {
	double.asked = append(double.asked, query)
	if double.err != nil {
		return false, double.err
	}
	return double.units[query.Object], nil
}

func (double *unitQuestionDouble) MembersAt(
	_ context.Context, query veports.ConsolidationMembershipQuery,
) (veports.ConsolidationMembership, error) {
	double.t.Fatalf("这几路此刻不投到成员上，不该问 %q 的成员", query.Object)
	return veports.ConsolidationMembership{}, nil
}

// controlFactLane 是一路：handle 把对象为 parcel-1 的一份登记经真派生编排交给适配器，交回落下的事实份数；at 是那份事实的业务
// 发生时间。
type controlFactLane struct {
	name   string
	at     time.Time
	handle func(t *testing.T, members veports.ConsolidationMembershipView) (int, error)
}

func controlFactLanes() []controlFactLane {
	handover := func(verdict tfdomain.HandoverVerdict) func(*testing.T, veports.ConsolidationMembershipView) (int, error) {
		return func(t *testing.T, members veports.ConsolidationMembershipView) (int, error) {
			derive, facts, _ := handoverDeriveHandler(t, handoverMappingViewDouble{}, handoverProjectionDownstreamDouble{})
			subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
				&handoverFinderDouble{record: registeredHandover(t, verdict), found: true}, members, derive)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			err = subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef())
			return len(facts.byKey), err
		}
	}
	return []controlFactLane{
		{name: "交接已交接", at: handoverJudgedAt, handle: handover(tfdomain.ObjectHandedOver)},
		{name: "交接已拒收", at: handoverJudgedAt, handle: handover(tfdomain.HandoverRefused)},
		{name: "交接待确认", at: handoverJudgedAt, handle: handover(tfdomain.HandoverPendingConfirmation)},
		{name: "场外揽收", at: pickedUpAt, handle: func(t *testing.T, members veports.ConsolidationMembershipView) (int, error) {
			derive, facts, _ := pickupDeriveHandler(t, pickupMappingViewDouble{}, pickupProjectionDownstreamDouble{})
			subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
				&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true}, members, derive)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			err = subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef())
			return len(facts.byKey), err
		}},
		{name: "有效交付", at: deliveredAt, handle: func(t *testing.T, members veports.ConsolidationMembershipView) (int, error) {
			derive, facts, _ := deliveryDeriveHandler(t, deliveryMappingViewDouble{}, deliveryProjectionDownstreamDouble{})
			subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(
				&deliveryFinderDouble{record: registeredDelivery(t, "parcel-1"), found: true}, members, derive)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			err = subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef())
			return len(facts.byKey), err
		}},
		// 外部承运轨迹有三个时间：问单元取源给的发生时间，与进投影的业务发生时间同一格，不取判断过的有效时间。
		{name: "外部承运轨迹", at: trackingOccurredAtVE, handle: func(t *testing.T, members veports.ConsolidationMembershipView) (int, error) {
			judgment, err := tfdomain.JudgeEffectiveTimeExplicitly(trackingEffectiveVE)
			if err != nil {
				t.Fatalf("有效时间判断：%v", err)
			}
			derive, facts, _ := deliveryDeriveHandler(t, deliveryMappingViewDouble{}, deliveryProjectionDownstreamDouble{})
			subject, err := adapter.NewDeriveOnExternalCarrierTrackingAdapter(
				&externalTrackingFinderDouble{record: judgedTrackingRecord(t, "EXTV-1", "", judgment), found: true}, members, derive)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			err = subject.HandleJudgedExternalTracking(t.Context(), judgedTrackingRef("EXTV-1"))
			return len(facts.byKey), err
		}},
	}
}

func assertAskedOnceAt(t *testing.T, members *unitQuestionDouble, at time.Time) {
	t.Helper()
	if len(members.asked) != 1 {
		t.Fatalf("问了 %d 次是不是单元, want 1", len(members.asked))
	}
	query := members.asked[0]
	if query.Object != "parcel-1" || query.TenantID.String() != "tenant-1" || !query.At.Equal(at) {
		t.Fatalf("问的是 %s / %s / %s，要按事实的对象、本租户、业务发生时间问", query.Object, query.TenantID, query.At)
	}
}

// Covers: AT-VE-199——交接（三种判断）、场外揽收、有效交付与外部承运轨迹的对象在事实发生那一刻是集运单元：入账不派生，单元不立
// 事实也不立投影；不认成员身份，也不投到成员上。按事实的业务发生时间问一次。
func TestAControlFactOnAConsolidationUnitIsAccountedForWithoutDeriving(t *testing.T) {
	for _, lane := range controlFactLanes() {
		t.Run(lane.name, func(t *testing.T) {
			members := &unitQuestionDouble{t: t, units: map[string]bool{"parcel-1": true}}
			facts, err := lane.handle(t, members)
			if err != nil {
				t.Fatalf("单元那一份应入账，got %v", err)
			}
			if facts != 0 {
				t.Fatalf("单元被当成一件追踪包裹立了 %d 份事实", facts)
			}
			assertAskedOnceAt(t, members, lane.at)
		})
	}
}

// Covers: AT-VE-199——对象那一刻不是集运单元时照旧：同一份事实派生到这件包裹上。
func TestAControlFactOnAParcelStillDerives(t *testing.T) {
	for _, lane := range controlFactLanes() {
		t.Run(lane.name, func(t *testing.T) {
			members := &unitQuestionDouble{t: t}
			facts, err := lane.handle(t, members)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if facts != 1 {
				t.Fatalf("包裹上的事实 %d 份, want 1", facts)
			}
			assertAskedOnceAt(t, members, lane.at)
		})
	}
}

// Covers: AT-VE-200——问节点作业是不是单元读不动是续办，一份也不派生；节点作业答了却译不回来是确定性的，记答复不一致，不当未决。
// 分格与移动事实展开成员同一处。
func TestControlFactUnitQuestionFailuresKeepTheirSentinels(t *testing.T) {
	for _, lane := range controlFactLanes() {
		for name, test := range map[string]struct {
			err  error
			want error
		}{
			"读不动":   {err: errors.New("node operations down"), want: adapter.ErrConsolidationMembershipNotVisible},
			"答复译不回": {err: veports.ErrConsolidationMembershipUntranslatable, want: adapter.ErrConsolidationMembershipInconsistent},
		} {
			t.Run(lane.name+"/"+name, func(t *testing.T) {
				facts, err := lane.handle(t, &unitQuestionDouble{t: t, err: test.err})
				if !errors.Is(err, test.want) {
					t.Fatalf("err = %v, want %v", err, test.want)
				}
				if facts != 0 {
					t.Fatalf("说不出是不是单元就派生了 %d 份", facts)
				}
			})
		}
	}
}

func TestTheControlFactAdaptersRefuseAMissingMembershipView(t *testing.T) {
	derive := &countingProjectionHandler{}
	if _, err := adapter.NewDeriveOnTransportHandoverAdapter(&handoverFinderDouble{}, nil, derive); err == nil {
		t.Fatal("交接：缺成员关系那一口必须构造即拒")
	}
	if _, err := adapter.NewDeriveOnOffsitePickupAdapter(&pickupRegistryDouble{}, nil, derive); err == nil {
		t.Fatal("揽收：缺成员关系那一口必须构造即拒")
	}
	if _, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(&deliveryFinderDouble{}, nil, derive); err == nil {
		t.Fatal("交付：缺成员关系那一口必须构造即拒")
	}
	if _, err := adapter.NewDeriveOnExternalCarrierTrackingAdapter(&externalTrackingFinderDouble{}, nil, derive); err == nil {
		t.Fatal("外部承运轨迹：缺成员关系那一口必须构造即拒")
	}
}
