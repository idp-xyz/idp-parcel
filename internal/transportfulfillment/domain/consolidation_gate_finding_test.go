package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func unitFinding(
	t *testing.T,
	unit string,
	members []domain.GateFinding,
	gaps ...domain.ConsolidationMemberGap,
) domain.GateFinding {
	t.Helper()
	finding, err := domain.FormConsolidationUnitGateFinding(mustValue(t, domain.NewCarriedObjectReference, unit), members, gaps)
	if err != nil {
		t.Fatalf("单元 %s 那一格：%v", unit, err)
	}
	return finding
}

func departureReadingOf(findings ...domain.GateFinding) (domain.DepartureGateReading, error) {
	from, _ := domain.NewNetworkNodeReference("NODE-A")
	to, _ := domain.NewNetworkNodeReference("NODE-B")
	return domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{From: from, To: to, Findings: findings})
}

// Covers: AT-TF-276 领域半边（ADR-0225 决定三）——集运单元那一格取成员里最重的一格：袋里一件阻断、一件不受门禁，袋那一格是阻断，
// 整条出发也是阻断；成员各自那一格与袋同在逐对象那几格里，袋那一格指名它们、不带关务判断标识。
func TestAUnitTakesTheHeaviestAnswerOfItsMembers(t *testing.T) {
	blocked := gateFinding(t, "parcel-held", domain.GateBlocked)
	clean := gateFinding(t, "parcel-clean", domain.GateNotGuarded)
	bag := unitFinding(t, "bag-1", []domain.GateFinding{clean, blocked})

	if bag.Answer() != domain.GateBlocked {
		t.Fatalf("袋那一格 = %s，要取成员里最重的阻断", bag.Answer())
	}
	if _, judged := bag.Judgement(); judged {
		t.Fatal("袋那一格不是问关务问来的，不该带关务判断标识")
	}
	members, unit := bag.ConsolidationUnit()
	if !unit || len(members) != 2 || members[0].String() != "parcel-clean" || members[1].String() != "parcel-held" {
		t.Fatalf("袋的成员 = %v / %v，要定序的两件", members, unit)
	}

	reading, err := departureReadingOf(bag, blocked, clean)
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if reading.Answer() != domain.GateBlocked || len(reading.Findings()) != 3 {
		t.Fatalf("读数 = %s，%d 格，要阻断、袋与两件成员各一格", reading.Answer(), len(reading.Findings()))
	}
}

// Covers: AT-TF-277、AT-TF-278 领域半边（ADR-0225 决定三、四）——本侧问不到的成员记成单元那一格的依据，每种缘由只属一格：身份不成立
// 答未决、身份读不动与成环答读不动，与成员那几格一起取最重；被更重的成员压下去的缘由照样留在依据里。出发那一刻什么也不装的单元
// 答未决，依据是「单元无成员」，不是不受门禁。
func TestMembersThatCannotBeAskedWeighOnTheUnitCell(t *testing.T) {
	unresolved := domain.ConsolidationMemberGap{Stop: domain.ConsolidationMemberIdentityUnresolved, Sources: []string{"hu-9"}}
	unreadable := domain.ConsolidationMemberGap{Stop: domain.ConsolidationMemberIdentityUnreadable, Sources: []string{"hu-8"}}

	cases := []struct {
		name    string
		members []domain.GateFinding
		gaps    []domain.ConsolidationMemberGap
		want    domain.GateAnswer
	}{
		{"只有身份不成立的成员", nil, []domain.ConsolidationMemberGap{unresolved}, domain.GateUndecided},
		{"不受门禁的成员加身份不成立的成员", []domain.GateFinding{gateFinding(t, "parcel-1", domain.GateNotGuarded)},
			[]domain.ConsolidationMemberGap{unresolved}, domain.GateUndecided},
		{"身份读不动压过未决", nil, []domain.ConsolidationMemberGap{unresolved, unreadable}, domain.GateUnreadable},
		{"阻断压过读不动", []domain.GateFinding{gateFinding(t, "parcel-1", domain.GateBlocked)},
			[]domain.ConsolidationMemberGap{unreadable}, domain.GateBlocked},
		{"成环", nil, []domain.ConsolidationMemberGap{{Stop: domain.ConsolidationMembershipCycle, Sources: []string{"bag-0"}}},
			domain.GateUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bag := unitFinding(t, "bag-1", tc.members, tc.gaps...)
			if bag.Answer() != tc.want {
				t.Fatalf("袋那一格 = %s，要 %s", bag.Answer(), tc.want)
			}
			if len(bag.Bases()) != len(tc.gaps) {
				t.Fatalf("依据 = %+v，每种缘由都要留下", bag.Bases())
			}
		})
	}

	empty := unitFinding(t, "bag-empty", nil)
	if empty.Answer() != domain.GateUndecided || len(empty.Bases()) != 1 ||
		empty.Bases()[0].Cause != domain.ConsolidationUnitWithoutMembers.String() {
		t.Fatalf("空单元 = %s %+v，要未决、依据「单元无成员」", empty.Answer(), empty.Bases())
	}
}

// Covers: AT-TF-278 领域半边（ADR-0225 决定四）——袋套袋：外层那一格取内层那一格与外层其余成员里最重的一格，内层那一格再取它自己的
// 成员；三层各一格同在一份读数里。
func TestANestedUnitWeighsThroughEveryLevel(t *testing.T) {
	held := gateFinding(t, "parcel-held", domain.GateBlocked)
	inner := unitFinding(t, "bag-inner", []domain.GateFinding{held})
	loose := gateFinding(t, "parcel-loose", domain.GateAdmitted)
	cage := unitFinding(t, "cage-1", []domain.GateFinding{inner, loose})
	if cage.Answer() != domain.GateBlocked {
		t.Fatalf("笼那一格 = %s，要沿内层袋取到阻断", cage.Answer())
	}
	if _, err := departureReadingOf(cage, inner, held, loose); err != nil {
		t.Fatalf("三层读数：%v", err)
	}
}

// Covers: AT-TF-279 领域半边（ADR-0225 决定五）——重建门在整份读数上核单元那一格：存下的那一格与成员推出的不一致、成员在读数里
// 没有一格、单元互相装着，都不装回。形状不对的单元那一格立不起来：成员是它自己、成员重复、缘由不在封闭集里或带了动作、「单元无
// 成员」与成员并存、什么都没有。
func TestTheReadingGateChecksEveryUnitAgainstItsMembers(t *testing.T) {
	held := gateFinding(t, "parcel-held", domain.GateBlocked)
	object := func(raw string) domain.CarriedObjectReference {
		return mustValue(t, domain.NewCarriedObjectReference, raw)
	}
	rehydrated := func(unit string, recorded domain.GateAnswer, members ...string) domain.GateFinding {
		t.Helper()
		references := make([]domain.CarriedObjectReference, 0, len(members))
		for _, member := range members {
			references = append(references, object(member))
		}
		finding, err := domain.RehydrateConsolidationUnitGateFinding(object(unit), recorded, references, nil)
		if err != nil {
			t.Fatalf("重建单元 %s：%v", unit, err)
		}
		return finding
	}

	if _, err := departureReadingOf(rehydrated("bag-1", domain.GateBlocked, "parcel-held"), held); err != nil {
		t.Fatalf("对得上的一份要装得回：%v", err)
	}
	for name, findings := range map[string][]domain.GateFinding{
		"存下的是不受门禁，成员说阻断": {rehydrated("bag-1", domain.GateNotGuarded, "parcel-held"), held},
		"成员在读数里没有一格":     {rehydrated("bag-1", domain.GateBlocked, "parcel-held")},
		"两件单元互相装着": {rehydrated("bag-1", domain.GateBlocked, "bag-2", "parcel-held"),
			rehydrated("bag-2", domain.GateBlocked, "bag-1"), held},
	} {
		if _, err := departureReadingOf(findings...); !errors.Is(err, domain.ErrInvalidDepartureGateReading) {
			t.Fatalf("%s：err = %v，要读数立不起来", name, err)
		}
	}

	for name, attempt := range map[string]func() error{
		"成员是它自己": func() error {
			_, err := domain.RehydrateConsolidationUnitGateFinding(object("bag-1"), domain.GateBlocked,
				[]domain.CarriedObjectReference{object("bag-1")}, nil)
			return err
		},
		"成员重复": func() error {
			_, err := domain.RehydrateConsolidationUnitGateFinding(object("bag-1"), domain.GateBlocked,
				[]domain.CarriedObjectReference{object("p-1"), object("p-1")}, nil)
			return err
		},
		"缘由不在封闭集里": func() error {
			_, err := domain.RehydrateConsolidationUnitGateFinding(object("bag-1"), domain.GateUndecided, nil,
				[]domain.GateBasis{{Cause: "RESTRICTION_IN_FORCE", Sources: []string{"r-1"}}})
			return err
		},
		"缘由带了关务的动作": func() error {
			_, err := domain.RehydrateConsolidationUnitGateFinding(object("bag-1"), domain.GateUndecided, nil,
				[]domain.GateBasis{{Action: "LOADING_DEPARTURE", Cause: domain.ConsolidationMemberIdentityUnresolved.String(), Sources: []string{"hu-1"}}})
			return err
		},
		"「单元无成员」与成员并存": func() error {
			_, err := domain.RehydrateConsolidationUnitGateFinding(object("bag-1"), domain.GateUndecided,
				[]domain.CarriedObjectReference{object("p-1")}, []domain.GateBasis{{Cause: domain.ConsolidationUnitWithoutMembers.String()}})
			return err
		},
		"身份不成立却不指名哪一件": func() error {
			_, err := domain.FormConsolidationUnitGateFinding(object("bag-1"), nil,
				[]domain.ConsolidationMemberGap{{Stop: domain.ConsolidationMemberIdentityUnresolved}})
			return err
		},
		"什么都没有": func() error {
			_, err := domain.RehydrateConsolidationUnitGateFinding(object("bag-1"), domain.GateUndecided, nil, nil)
			return err
		},
	} {
		if err := attempt(); !errors.Is(err, domain.ErrInvalidConsolidationUnitFinding) {
			t.Fatalf("%s：err = %v，要单元那一格立不起来", name, err)
		}
	}
}

// Covers: AT-TF-279 领域半边（ADR-0225 决定五）——交出与交付读数只装得下一格，单元那一格指名的成员无处安放，两种读数都不收它。
func TestOnlyTheDepartureReadingHoldsAUnitCell(t *testing.T) {
	bag := unitFinding(t, "bag-1", nil)
	node := mustValue(t, domain.NewNetworkNodeReference, "NODE-A")
	if _, err := domain.FormHandoverGateReading(domain.HandoverGateReadingSpec{Node: node, Finding: bag}); !errors.Is(err, domain.ErrInvalidHandoverGateReading) {
		t.Fatalf("交出读数收了单元那一格：%v", err)
	}
	if _, err := domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{Node: node, Finding: bag}); !errors.Is(err, domain.ErrInvalidDeliveryGateReading) {
		t.Fatalf("交付读数收了单元那一格：%v", err)
	}
}
