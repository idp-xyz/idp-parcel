package application_test

import (
	"reflect"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证出发门禁读数展开集运单元（ADR-0225）：段里在控的一袋不问关务，按出发那一刻的直接成员逐件问，袋那一格取成员里最重的
// 一格。成员关系与身份由替身答，真库与真适配器在 cmd/parcel-api 的用例里证。

func parcelMember(t *testing.T, handling, parcel string) ports.ConsolidationMember {
	t.Helper()
	return ports.ConsolidationMember{Kind: ports.ConsolidationMemberParcel, Handling: handling,
		Object: mustRefValue(t, domain.NewCarriedObjectReference, parcel)}
}

func nestedMember(t *testing.T, unit string) ports.ConsolidationMember {
	t.Helper()
	return ports.ConsolidationMember{Kind: ports.ConsolidationMemberUnit, Handling: unit,
		Object: mustRefValue(t, domain.NewCarriedObjectReference, unit)}
}

func cellsOf(reading domain.DepartureGateReading) map[string]domain.GateFinding {
	cells := map[string]domain.GateFinding{}
	for _, finding := range reading.Findings() {
		cells[finding.Object().String()] = finding
	}
	return cells
}

func askedObjects(customs *departureCustomsDouble) []string {
	var objects []string
	for _, query := range customs.queries {
		objects = append(objects, query.Object.String())
	}
	slices.Sort(objects)
	return objects
}

func memberNames(finding domain.GateFinding) ([]string, bool) {
	members, unit := finding.ConsolidationUnit()
	var names []string
	for _, member := range members {
		names = append(names, member.String())
	}
	return names, unit
}

// Covers: AT-TF-276 编排半边（ADR-0225 决定一、三）——段里在控的是一袋：成员关系按出发那一刻问，袋本身不问关务，袋里各件按包裹
// 逐件问；袋那一格取成员里最重的阻断，整条出发阻断，不是不受门禁。同段另有一件散件照常按包裹问。
func TestABagIsAskedThroughItsMembersAndTakesTheHeaviest(t *testing.T) {
	fixture := newDepartureGateFixture(t, "bag-1", "parcel-loose")
	fixture.memberships.units["bag-1"] = []ports.ConsolidationMember{
		parcelMember(t, "hu-1", "parcel-held"), parcelMember(t, "hu-2", "parcel-clean"),
	}
	fixture.customs.answers["parcel-held"] = domain.GateBlocked

	reading, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if reading.Answer() != domain.GateBlocked {
		t.Fatalf("整格 = %s，袋里一件被扣的境内出发要读成阻断", reading.Answer())
	}
	if asked := askedObjects(fixture.customs); !slices.Equal(asked, []string{"parcel-clean", "parcel-held", "parcel-loose"}) {
		t.Fatalf("问关务的对象 = %v，袋本身不问、袋里与散件各问一次", asked)
	}
	for _, query := range fixture.memberships.queries {
		if !query.At.Equal(movedAtFixture) || query.TenantID.String() != "tenant-1" {
			t.Fatalf("成员关系的问法 = %+v，时点要取出发的业务时间", query)
		}
	}
	cells := cellsOf(reading)
	if names, unit := memberNames(cells["bag-1"]); !unit || cells["bag-1"].Answer() != domain.GateBlocked ||
		!slices.Equal(names, []string{"parcel-clean", "parcel-held"}) {
		t.Fatalf("袋那一格 = %s %v unit=%v", cells["bag-1"].Answer(), names, unit)
	}
	if cells["parcel-clean"].Answer() != domain.GateNotGuarded || len(cells) != 4 {
		t.Fatalf("成员各自那一格要进逐对象明细：%v", cells)
	}
}

// Covers: AT-TF-277 编排半边（ADR-0225 决定四）——说不出一件是不是袋时那一件记读不动、依据指名节点作业那一口，不按包裹问关务；调用
// 失败与译不回来分记依据种类，其余照问。袋里身份不成立的成员让袋那一格答未决、身份读不动让它答读不动，依据指名那几件作业实物——
// 都不读成不受门禁。
func TestMembersThatCannotBeReadKeepTheBagFromReadingClean(t *testing.T) {
	fixture := newDepartureGateFixture(t, "obj-down", "obj-garbled", "bag-unresolved", "bag-unreadable", "parcel-1")
	fixture.memberships.errors["obj-down"] = true
	fixture.memberships.untranslatable["obj-garbled"] = true
	fixture.memberships.units["bag-unresolved"] = []ports.ConsolidationMember{
		parcelMember(t, "hu-1", "parcel-2"),
		{Kind: ports.ConsolidationMemberIdentityUnresolved, Handling: "hu-pending"},
	}
	fixture.memberships.units["bag-unreadable"] = []ports.ConsolidationMember{
		{Kind: ports.ConsolidationMemberIdentityUnreadable, Handling: "hu-unreadable"},
	}

	reading, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if asked := askedObjects(fixture.customs); !slices.Equal(asked, []string{"parcel-1", "parcel-2"}) {
		t.Fatalf("问关务的对象 = %v，说不出种类的那两件不该按包裹问", asked)
	}
	cells := cellsOf(reading)
	for object, cause := range map[string]string{
		"obj-down":    "CONSOLIDATION_MEMBERSHIP_UNAVAILABLE",
		"obj-garbled": "CONSOLIDATION_MEMBERSHIP_UNTRANSLATABLE",
	} {
		if cells[object].Answer() != domain.GateUnreadable || cells[object].Bases()[0].Cause != cause {
			t.Fatalf("%s = %s %+v，要读不动、依据 %s", object, cells[object].Answer(), cells[object].Bases(), cause)
		}
	}
	unresolved := cells["bag-unresolved"]
	if unresolved.Answer() != domain.GateUndecided || unresolved.Bases()[0].Cause != domain.ConsolidationMemberIdentityUnresolved.String() ||
		!slices.Equal(unresolved.Bases()[0].Sources, []string{"hu-pending"}) {
		t.Fatalf("身份不成立的袋 = %s %+v", unresolved.Answer(), unresolved.Bases())
	}
	if unreadable := cells["bag-unreadable"]; unreadable.Answer() != domain.GateUnreadable ||
		unreadable.Bases()[0].Cause != domain.ConsolidationMemberIdentityUnreadable.String() {
		t.Fatalf("身份读不动的袋 = %s %+v", unreadable.Answer(), unreadable.Bases())
	}
	if reading.Answer() != domain.GateUnreadable {
		t.Fatalf("整格 = %s", reading.Answer())
	}
}

// Covers: AT-TF-278 编排半边（ADR-0225 决定四）——袋套袋一层层展开到包裹；同一件包裹既直接在段里、又在两层袋里，只问一次、只记一格；
// 成员关系成环时不沿环回头、照样收尾，那一袋答读不动并指名环上那一件；出发那一刻什么也不装的袋答未决。
func TestNestedBagsAreExpandedToParcelsOnceAndACycleEnds(t *testing.T) {
	fixture := newDepartureGateFixture(t, "cage-1", "parcel-a", "bag-empty", "loop-1")
	fixture.memberships.units["cage-1"] = []ports.ConsolidationMember{nestedMember(t, "bag-1"), parcelMember(t, "hu-a", "parcel-a")}
	fixture.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-b", "parcel-b"), parcelMember(t, "hu-a2", "parcel-a")}
	fixture.memberships.units["bag-empty"] = []ports.ConsolidationMember{}
	fixture.memberships.units["loop-1"] = []ports.ConsolidationMember{nestedMember(t, "loop-2")}
	fixture.memberships.units["loop-2"] = []ports.ConsolidationMember{nestedMember(t, "loop-1"), parcelMember(t, "hu-c", "parcel-c")}
	fixture.customs.answers["parcel-b"] = domain.GateBlocked

	reading, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if asked := askedObjects(fixture.customs); !slices.Equal(asked, []string{"parcel-a", "parcel-b", "parcel-c"}) {
		t.Fatalf("问关务的对象 = %v，每件包裹只问一次", asked)
	}
	cells := cellsOf(reading)
	if cells["cage-1"].Answer() != domain.GateBlocked || cells["bag-1"].Answer() != domain.GateBlocked {
		t.Fatalf("笼 %s、袋 %s，要沿内层袋取到阻断", cells["cage-1"].Answer(), cells["bag-1"].Answer())
	}
	if names, _ := memberNames(cells["cage-1"]); !slices.Equal(names, []string{"bag-1", "parcel-a"}) {
		t.Fatalf("笼的成员 = %v", names)
	}
	if empty := cells["bag-empty"]; empty.Answer() != domain.GateUndecided ||
		empty.Bases()[0].Cause != domain.ConsolidationUnitWithoutMembers.String() {
		t.Fatalf("空袋 = %s %+v", empty.Answer(), empty.Bases())
	}
	loop := cells["loop-2"]
	if loop.Answer() != domain.GateUnreadable || loop.Bases()[0].Cause != domain.ConsolidationMembershipCycle.String() ||
		!slices.Equal(loop.Bases()[0].Sources, []string{"loop-1"}) {
		t.Fatalf("环上那一袋 = %s %+v", loop.Answer(), loop.Bases())
	}
	if names, _ := memberNames(cells["loop-1"]); !slices.Equal(names, []string{"loop-2"}) || cells["loop-1"].Answer() != domain.GateUnreadable {
		t.Fatalf("环的起点 = %s %v", cells["loop-1"].Answer(), names)
	}
}

// Covers: AT-TF-278 编排半边（ADR-0225 决定一、四，ADR-0214 决定二）——袋里一件成员认出的包裹号恰与段里另一件单元同串，且袋先被展开：
// 那个号照样先问是不是单元、按单元展开，不按包裹问关务；后到的段里那件取到的是单元那一格，不是一格先记下的不受门禁。
func TestAMemberParcelNamedLikeAUnitIsStillReadAsTheUnit(t *testing.T) {
	fixture := newDepartureGateFixture(t, "bag-1", "unit-x")
	fixture.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "unit-x")}
	fixture.memberships.units["unit-x"] = []ports.ConsolidationMember{parcelMember(t, "hu-2", "parcel-held")}
	fixture.customs.answers["parcel-held"] = domain.GateBlocked

	reading, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if asked := askedObjects(fixture.customs); !slices.Equal(asked, []string{"parcel-held"}) {
		t.Fatalf("问关务的对象 = %v，同串的那个号要按单元展开、不按包裹问", asked)
	}
	cells := cellsOf(reading)
	if names, unit := memberNames(cells["unit-x"]); !unit || cells["unit-x"].Answer() != domain.GateBlocked ||
		!slices.Equal(names, []string{"parcel-held"}) {
		t.Fatalf("unit-x = %s %v unit=%v，要按单元展开到被扣的那一件", cells["unit-x"].Answer(), names, unit)
	}
	if cells["bag-1"].Answer() != domain.GateBlocked || reading.Answer() != domain.GateBlocked {
		t.Fatalf("袋 %s、整格 %s，要沿 unit-x 取到阻断", cells["bag-1"].Answer(), reading.Answer())
	}
}

// 成员落在封闭集外是适配器坏了，上抛而不记成一格。
func TestAMemberOfNoKnownKindIsAnError(t *testing.T) {
	fixture := newDepartureGateFixture(t, "bag-1")
	fixture.memberships.units["bag-1"] = []ports.ConsolidationMember{{Handling: "hu-1"}}
	if _, err := fixture.reader(t).Read(t.Context(), departureCommand(t)); err == nil {
		t.Fatal("封闭集外的成员被记成了一格")
	}
}

// Covers: AT-TF-280 编排半边（ADR-0225 决定六，ADR-0219 决定二）——出发前问询走同一条读数路径：段里有袋时，问询交回的读数与随后
// 同参出发逐格相同，袋那一格与它的成员也相同，问成员关系与问关务的问法逐问相同。
func TestAPreviewExpandsBagsExactlyAsTheDepartureDoes(t *testing.T) {
	arrange := func() *departureGateFixture {
		fixture := newDepartureGateFixture(t, "bag-1", "parcel-loose")
		fixture.memberships.units["bag-1"] = []ports.ConsolidationMember{
			parcelMember(t, "hu-1", "parcel-held"),
			{Kind: ports.ConsolidationMemberIdentityUnresolved, Handling: "hu-pending"},
		}
		fixture.customs.answers["parcel-held"] = domain.GateBlocked
		return fixture
	}
	command := departureCommand(t)

	departed := arrange()
	reading, err := departed.reader(t).Read(t.Context(), command)
	if err != nil {
		t.Fatalf("出发读数：%v", err)
	}
	previewed := arrange()
	result, err := previewHandler(t, previewed).Preview(t.Context(), previewOf(command))
	if err != nil || result.Outcome() != application.DepartureGatePreviewed {
		t.Fatalf("问询：%s / %v", result.Outcome(), err)
	}
	preview, _ := result.Reading()
	if !reflect.DeepEqual(preview, reading) {
		t.Fatalf("问询读数 %+v 与出发读数 %+v 不是同一套", preview, reading)
	}
	if !reflect.DeepEqual(previewed.memberships.queries, departed.memberships.queries) ||
		!reflect.DeepEqual(previewed.customs.queries, departed.customs.queries) {
		t.Fatal("问询与出发的问法不一致")
	}
}
