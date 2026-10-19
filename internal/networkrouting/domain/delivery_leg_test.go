package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// 本文件钉计划带交付段的领域半边（ADR-0217，票 demo-chain-gaps/64）：交付段起于交付节点、止于交付范围；
// 段链里至多一段、只在末尾；计划节点、已执行前缀只由节点间段推导；段引用按整条段链数序位。

func deliveryLeg(t *testing.T, from, scope string, minutes int) domain.PlannedLeg {
	t.Helper()
	window, err := domain.NewPlannedTimeWindow(windowLatest, windowLatest.Add(time.Duration(minutes)*time.Minute),
		mustValue(t, domain.NewWindowBasisReference, "NODE/"+from+"@2"))
	if err != nil {
		t.Fatalf("交付段窗口：%v", err)
	}
	leg, err := domain.NewDeliveryLeg(domain.DeliveryLegSpec{
		From:        mustValue(t, domain.NewPlanNodeReference, from),
		Scope:       mustValue(t, domain.NewServiceAreaVersionReference, scope),
		Responsible: mustValue(t, domain.NewResponsiblePartyReference, scope),
		Window:      window,
	})
	if err != nil {
		t.Fatalf("交付段 %s→%s：%v", from, scope, err)
	}
	return leg
}

// Covers: AT-NR-054——交付段是计划段链的末一段：种类为交付，起点交付节点，终点是交付范围（服务区域版本），
// 不是节点；计划节点序列不含交付范围，节点间段是段链的前缀，段引用按整条段链数，交付段可被引用。
func TestAPlanEndsWithADeliveryLegToTheDeliveryScope(t *testing.T) {
	spec := planSpec(t)
	spec.Legs = append(spec.Legs, deliveryLeg(t, "node-destination", "area-sg@1", 180))
	plan, err := domain.FormInitialRoutePlan(spec)
	if err != nil {
		t.Fatalf("带交付段的计划：%v", err)
	}

	if got := len(plan.Legs()); got != 3 {
		t.Fatalf("段链 %d 段，想要两段节点间段加一段交付段", got)
	}
	last, found := plan.LegAt(3)
	if !found || last.Kind() != domain.DeliveryLeg || last.Kind().String() != "DELIVERY" {
		t.Fatalf("第三段 = %+v found=%v，想要交付段", last, found)
	}
	scope, delivery := last.DeliveryScope()
	if !delivery || scope.String() != "area-sg@1" || last.From().String() != "node-destination" {
		t.Fatalf("交付段起讫 = %s → %s（%v）", last.From(), scope, delivery)
	}
	if last.To().String() != "" {
		t.Fatalf("交付段的终点节点 = %q，交付范围不是节点", last.To())
	}
	if !last.Window().Earliest().Equal(windowLatest) || !last.Window().Latest().Equal(windowLatest.Add(3*time.Hour)) {
		t.Fatalf("交付段窗口 = [%v, %v]", last.Window().Earliest(), last.Window().Latest())
	}
	if _, found := plan.LegAt(4); found {
		t.Fatal("交付段之后还取得到一段")
	}

	nodes := plan.Nodes()
	if len(nodes) != 3 || nodes[2].String() != "node-destination" {
		t.Fatalf("计划节点 = %v，交付范围不该进节点序列", nodes)
	}
	nodeLegs := plan.NodeLegs()
	if len(nodeLegs) != 2 || nodeLegs[1].Kind() != domain.BetweenNodesLeg || nodeLegs[1].To().String() != "node-destination" {
		t.Fatalf("节点间段 = %+v", nodeLegs)
	}
	if _, delivery := nodeLegs[0].DeliveryScope(); delivery {
		t.Fatal("节点间段答出了交付范围")
	}

	// AT-NR-059 的领域半边：已执行前缀只由节点间段推导，剩余段数不含交付段。
	if _, ok := domain.PlanNodes(plan.Legs()); ok {
		t.Fatal("整条段链（含交付段）交给 PlanNodes 应答不出节点序列")
	}
	planNodes, ok := domain.PlanNodes(plan.NodeLegs())
	if !ok {
		t.Fatal("节点间段推不出计划节点")
	}
	prefix := domain.JudgeExecutedPrefix(planNodes, domain.TransportHandoverControl, "node-hub")
	if prefix.Grade() != domain.ExecutedPrefixEstablished || prefix.RemainingSegments() != 1 {
		t.Fatalf("在 node-hub 的剩余段数 = %d（%v），想要 1：交付段不算剩余段", prefix.RemainingSegments(), prefix.Grade())
	}
}

// Covers: AT-NR-056——交付段至多一段、只在末尾、起于末个计划节点；整条段链没有节点间段也不成计划。
func TestAPlanRefusesAMisplacedDeliveryLeg(t *testing.T) {
	cases := map[string][]domain.PlannedLeg{
		"交付段在中间": {
			plannedLeg(t, "node-origin", "node-hub", false),
			deliveryLeg(t, "node-hub", "area-sg@1", 60),
			plannedLeg(t, "node-hub", "node-destination", false),
		},
		"只有交付段": {
			deliveryLeg(t, "node-destination", "area-sg@1", 60),
		},
		"交付段起点不是末个节点": {
			plannedLeg(t, "node-origin", "node-hub", false),
			deliveryLeg(t, "node-elsewhere", "area-sg@1", 60),
		},
		"两段交付段": {
			plannedLeg(t, "node-origin", "node-hub", false),
			deliveryLeg(t, "node-hub", "area-sg@1", 60),
			deliveryLeg(t, "node-hub", "area-sg@1", 60),
		},
	}
	for name, legs := range cases {
		t.Run(name, func(t *testing.T) {
			spec := planSpec(t)
			spec.Legs = legs
			if _, err := domain.FormInitialRoutePlan(spec); !errors.Is(err, domain.ErrInvalidRoutePlan) {
				t.Fatalf("err = %v，想要计划不成立", err)
			}
		})
	}
}

// Covers: 交付段构造门——交付范围、责任与窗口必备；零长窗口（登了 0 分钟派送时长）合法（AT-NR-063 的领域半边）。
// 段种类词封闭，族外与空词都拒。
func TestADeliveryLegNeedsItsScopeAndTheKindVocabularyIsClosed(t *testing.T) {
	window := plannedWindow(t)
	if _, err := domain.NewDeliveryLeg(domain.DeliveryLegSpec{
		From:        mustValue(t, domain.NewPlanNodeReference, "node-destination"),
		Responsible: mustValue(t, domain.NewResponsiblePartyReference, "area-sg@1"),
		Window:      window,
	}); !errors.Is(err, domain.ErrInvalidRoutePlan) {
		t.Fatalf("缺交付范围 err = %v", err)
	}
	if leg := deliveryLeg(t, "node-destination", "area-sg@1", 0); !leg.Window().Earliest().Equal(leg.Window().Latest()) {
		t.Fatal("0 分钟派送时长应是零长窗口")
	}

	for _, kind := range []domain.PlannedLegKind{domain.BetweenNodesLeg, domain.DeliveryLeg} {
		back, err := domain.PlannedLegKindFrom(kind.String())
		if err != nil || back != kind {
			t.Fatalf("种类 %q 往返 = %v, %v", kind, back, err)
		}
	}
	for _, raw := range []string{"", "LAST_MILE", "delivery"} {
		if _, err := domain.PlannedLegKindFrom(raw); !errors.Is(err, domain.ErrUnknownPlannedLegKind) {
			t.Fatalf("族外种类 %q err = %v", raw, err)
		}
	}
}
