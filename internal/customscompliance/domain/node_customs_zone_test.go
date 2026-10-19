package domain_test

import (
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func node(t *testing.T, code string) domain.NetworkNodeReference {
	t.Helper()
	return mustValue(t, domain.NewNetworkNodeReference, code)
}

func zoned(t *testing.T, code, zone string) domain.NodeZoneReading {
	t.Helper()
	return domain.NodeZoneReading{
		Node:       node(t, code),
		Zone:       mustValue(t, domain.NewCustomsZoneReference, zone),
		Registered: true,
	}
}

func unzoned(t *testing.T, code string) domain.NodeZoneReading {
	t.Helper()
	return domain.NodeZoneReading{Node: node(t, code)}
}

func departure(t *testing.T, from, to string) domain.ActionIntent {
	t.Helper()
	intent, err := domain.NewDepartureIntent(node(t, from), node(t, to))
	if err != nil {
		t.Fatalf("构造出发意图：%v", err)
	}
	return intent
}

// 演示动线两段（证据 S）：同区段只适用装载出发，跨境段另加跨关务区域移动——不是每次出发都问两种。
// Covers: AT-CC-411、AT-CC-412 的领域半边——适用动作与跨区判断按区域读数定；区域读数从哪来归真库那一半。
func TestDepartureAppliesCrossZoneMovementOnlyWhenZonesDiffer(t *testing.T) {
	tests := map[string]struct {
		from, to string
		readings []domain.NodeZoneReading
		want     []domain.GuardedAction
		cross    domain.CrossZoneState
	}{
		"SHA→SZX 同在 CN": {
			from: "SYN-NODE-SHA-HUB", to: "SYN-NODE-SZX-GATE",
			readings: []domain.NodeZoneReading{zoned(t, "SYN-NODE-SHA-HUB", "SYN-ZONE-CN"), zoned(t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN")},
			want:     []domain.GuardedAction{domain.LoadingDeparture},
			cross:    domain.CrossZoneSame,
		},
		"SZX→SIN 由 CN 入 SG": {
			from: "SYN-NODE-SZX-GATE", to: "SYN-NODE-SIN-HUB",
			readings: []domain.NodeZoneReading{zoned(t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN"), zoned(t, "SYN-NODE-SIN-HUB", "SYN-ZONE-SG")},
			want:     []domain.GuardedAction{domain.LoadingDeparture, domain.CrossCustomsMovement},
			cross:    domain.CrossZoneCrossing,
		},
		"终点未登记：跨区判不出，不预先列跨区": {
			from: "SYN-NODE-SZX-GATE", to: "SYN-NODE-SIN-HUB",
			readings: []domain.NodeZoneReading{zoned(t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN"), unzoned(t, "SYN-NODE-SIN-HUB")},
			want:     []domain.GuardedAction{domain.LoadingDeparture},
			cross:    domain.CrossZoneUndetermined,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			applicability, err := domain.DetermineGuardedActions(departure(t, test.from, test.to), test.readings)
			if err != nil {
				t.Fatalf("定适用动作：%v", err)
			}
			if !slices.Equal(applicability.Guarded(), test.want) {
				t.Fatalf("适用动作 = %v，要 %v", applicability.Guarded(), test.want)
			}
			if applicability.CrossZone() != test.cross {
				t.Fatalf("跨区 = %s，要 %s", applicability.CrossZone(), test.cross)
			}
		})
	}
}

func TestHandoverAndDeliveryMapToTheirOwnGuardedActions(t *testing.T) {
	handover, err := domain.NewHandoverIntent(node(t, "SYN-NODE-SZX-GATE"), node(t, "SYN-NODE-SIN-HUB"))
	if err != nil {
		t.Fatalf("构造交出意图：%v", err)
	}
	applicability, err := domain.DetermineGuardedActions(handover, []domain.NodeZoneReading{
		zoned(t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN"), zoned(t, "SYN-NODE-SIN-HUB", "SYN-ZONE-SG"),
	})
	if err != nil {
		t.Fatalf("定交出适用动作：%v", err)
	}
	if want := []domain.GuardedAction{domain.OutboundRelease, domain.CrossCustomsMovement}; !slices.Equal(applicability.Guarded(), want) {
		t.Fatalf("交出适用 = %v，要 %v", applicability.Guarded(), want)
	}

	// 交出没给去向：不问跨区，也不读区域——拿所在节点顶替去向会把跨境交出判成同区。
	bare, err := domain.NewHandoverIntent(node(t, "SYN-NODE-SZX-GATE"), domain.NetworkNodeReference{})
	if err != nil {
		t.Fatalf("构造无去向交出：%v", err)
	}
	if len(bare.ZoneNodes()) != 0 {
		t.Fatalf("无去向交出不该读区域：%v", bare.ZoneNodes())
	}
	applicability, err = domain.DetermineGuardedActions(bare, nil)
	if err != nil || applicability.CrossZone() != domain.CrossZoneNotAsked {
		t.Fatalf("无去向交出：%v / %s", err, applicability.CrossZone())
	}

	delivery, err := domain.NewDeliveryIntent(node(t, "SYN-NODE-SIN-LM"))
	if err != nil {
		t.Fatalf("构造交付意图：%v", err)
	}
	applicability, err = domain.DetermineGuardedActions(delivery, nil)
	if err != nil {
		t.Fatalf("定交付适用动作：%v", err)
	}
	if want := []domain.GuardedAction{domain.FinalDelivery}; !slices.Equal(applicability.Guarded(), want) {
		t.Fatalf("交付适用 = %v，要 %v", applicability.Guarded(), want)
	}
}

// 缺读数或多读数是调用方没去读，不折成「未登记」——未登记是读回来的答案。
func TestDetermineGuardedActionsRefusesReadingsThatDoNotMatchTheIntent(t *testing.T) {
	intent := departure(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	if _, err := domain.DetermineGuardedActions(intent, []domain.NodeZoneReading{zoned(t, "SYN-NODE-SHA-HUB", "SYN-ZONE-CN")}); !errors.Is(err, domain.ErrNodeZoneReadingMissing) {
		t.Fatalf("缺终点读数：err = %v", err)
	}
	if _, err := domain.DetermineGuardedActions(intent, []domain.NodeZoneReading{
		zoned(t, "SYN-NODE-SHA-HUB", "SYN-ZONE-CN"), zoned(t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN"), zoned(t, "SYN-NODE-SIN-HUB", "SYN-ZONE-SG"),
	}); !errors.Is(err, domain.ErrNodeZoneReadingMismatch) {
		t.Fatalf("多出意图以外的节点：err = %v", err)
	}
	if _, err := domain.DetermineGuardedActions(intent, []domain.NodeZoneReading{
		{Node: node(t, "SYN-NODE-SHA-HUB"), Zone: mustValue(t, domain.NewCustomsZoneReference, "SYN-ZONE-CN")},
		zoned(t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN"),
	}); !errors.Is(err, domain.ErrInvalidNodeZoneReading) {
		t.Fatalf("未登记却带区域：err = %v", err)
	}
}

func TestActionIntentConstructionRefusesDegenerateShapes(t *testing.T) {
	if _, err := domain.NewDepartureIntent(node(t, "SYN-NODE-A"), node(t, "SYN-NODE-A")); !errors.Is(err, domain.ErrInvalidActionIntent) {
		t.Fatalf("起讫同一节点：err = %v", err)
	}
	if _, err := domain.NewDepartureIntent(node(t, "SYN-NODE-A"), domain.NetworkNodeReference{}); !errors.Is(err, domain.ErrInvalidActionIntent) {
		t.Fatalf("出发缺终点：err = %v", err)
	}
	if _, err := domain.NewHandoverIntent(node(t, "SYN-NODE-A"), node(t, "SYN-NODE-A")); !errors.Is(err, domain.ErrInvalidActionIntent) {
		t.Fatalf("交出去向即所在：err = %v", err)
	}
	if _, err := domain.NewDeliveryIntent(domain.NetworkNodeReference{}); !errors.Is(err, domain.ErrInvalidActionIntent) {
		t.Fatalf("交付缺节点：err = %v", err)
	}
}
