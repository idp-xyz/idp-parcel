package networkrouting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/networkrouting"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 本文件钉初始路由进运输准备那一缝（票 demo-chain-gaps/18，ADR-0201 决定二、七）：计划段按 NR 拼写读出连接代码与起讫、
// 段引用照「版本#序位」拼；无路线判断照样交编排；读不回答未决、记录对不上键与说不清连接的段响亮失败；编排各格落成消费
// 两格。NR 与编排两侧都用替身，夹具全部合成。

var routeJudgedAt = time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC)

func nr[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

var formedRoute = tfinbox.FormedInitialRoute{
	TenantID: "SYN-TENANT-01", CustomerAccountID: "SYN-CUSTOMER-01", Shipment: "SYN-REQ-01",
	Parcel: "SYN-PARCEL-01", Baseline: "SYN-VER-01", Purpose: "NETWORK_SERVICE",
}

func routeKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           nr(t, nrdomain.NewTenantID, formedRoute.TenantID),
		CustomerAccountID:  nr(t, nrdomain.NewCustomerAccountID, formedRoute.CustomerAccountID),
		ShipmentRequestID:  nr(t, nrdomain.NewShipmentRequestID, formedRoute.Shipment),
		AcceptanceBaseline: nr(t, nrdomain.NewAcceptanceBaselineReference, formedRoute.Baseline),
		DeclaredParcelID:   nr(t, nrdomain.NewDeclaredParcelID, formedRoute.Parcel),
		ServicePurpose:     nr(t, nrdomain.NewServicePurpose, formedRoute.Purpose),
	}
}

func legOn(t *testing.T, basis, from, to string, offset time.Duration) nrdomain.PlannedLeg {
	t.Helper()
	window, err := nrdomain.NewPlannedTimeWindow(routeJudgedAt.Add(offset), routeJudgedAt.Add(offset+4*time.Hour),
		nr(t, nrdomain.NewWindowBasisReference, basis))
	if err != nil {
		t.Fatalf("窗口：%v", err)
	}
	leg, err := nrdomain.NewPlannedLeg(nrdomain.PlannedLegSpec{
		From: nr(t, nrdomain.NewPlanNodeReference, from), To: nr(t, nrdomain.NewPlanNodeReference, to),
		Responsible: nr(t, nrdomain.NewResponsiblePartyReference, "SYN-PARTY-01"), Window: window,
	})
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	return leg
}

func formedRecord(t *testing.T, legs ...nrdomain.PlannedLeg) nrports.InitialRouteRecord {
	t.Helper()
	qualified, err := nrdomain.NewRouteCandidate(nr(t, nrdomain.NewCandidateID, "SYN-CAND-01"), nrdomain.CandidateQualified, nrdomain.CandidateReason{})
	if err != nil {
		t.Fatalf("候选：%v", err)
	}
	plan, err := nrdomain.FormInitialRoutePlan(nrdomain.InitialRoutePlanSpec{
		Key: routeKey(t), Version: nr(t, nrdomain.NewRoutePlanVersionID, "RPV-7"),
		Selected: qualified.ID(), Candidates: []nrdomain.RouteCandidate{qualified}, Legs: legs,
		Strategy:     nr(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY@1"),
		ViewRevision: nr(t, nrdomain.NewNetworkViewRevision, "SYN-NETVIEW-01"),
		JudgedAt:     routeJudgedAt, EffectiveFrom: routeJudgedAt,
	})
	if err != nil {
		t.Fatalf("计划：%v", err)
	}
	return nrports.InitialRouteRecord{Key: routeKey(t), Plan: plan, HasPlan: true, RecordedAt: routeJudgedAt}
}

type routeFinderStub struct {
	record nrports.InitialRouteRecord
	found  bool
	err    error
}

func (stub routeFinderStub) FindByKey(context.Context, nrdomain.InitialRouteJudgmentKey) (nrports.InitialRouteRecord, bool, error) {
	return stub.record, stub.found, stub.err
}

type preparationTriggerStub struct {
	calls   int
	command tfapplication.PrepareOnPlannedLegsCommand
	result  tfapplication.PrepareOnPlannedLegsResult
}

func (stub *preparationTriggerStub) Prepare(
	_ context.Context, command tfapplication.PrepareOnPlannedLegsCommand,
) (tfapplication.PrepareOnPlannedLegsResult, error) {
	stub.calls++
	stub.command = command
	return stub.result, nil
}

func handle(t *testing.T, finder routeFinderStub, trigger *preparationTriggerStub) error {
	t.Helper()
	processing, err := adapter.NewPrepareOnInitialRoute(finder, trigger)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return processing.HandleFormedInitialRoute(t.Context(), formedRoute)
}

// Covers: AT-TF-126——三段计划逐段译成本上下文的读法：段引用 `RPV-7#序位`，连接代码取窗口依据里唯一那一格 `CONNECTION/`、
// 去掉版本（与别的依据以 `+` 连着也认得），起讫照 NR 拼写，窗口照抄；对象取判断键上的声明包裹。
func TestPlannedLegsAreTranslatedFromTheConnectionBasis(t *testing.T) {
	record := formedRecord(t,
		legOn(t, "CONNECTION/SYN-CONN-SHA-SZX@1", "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", 0),
		legOn(t, "NODE/SYN-NODE-SZX-GATE@1+CONNECTION/SYN-CONN-SZX-SIN@3", "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB", 4*time.Hour),
		legOn(t, "CONNECTION/SYN@CONN@2", "SYN-NODE-SIN-HUB", "SYN-NODE-SIN-LM", 8*time.Hour),
	)
	trigger := &preparationTriggerStub{}
	if err := handle(t, routeFinderStub{record: record, found: true}, trigger); err == nil ||
		!errors.Is(err, adapter.ErrUnexpectedPreparationOutcome) {
		t.Fatalf("零值结果应答封闭集合以外：%v", err)
	}
	command := trigger.command
	if command.TenantID.String() != "SYN-TENANT-01" || command.Object != "SYN-PARCEL-01" ||
		command.Route != tfapplication.PlannedRouteFormed || len(command.Legs) != 3 {
		t.Fatalf("命令 = %+v", command)
	}
	want := []struct{ reference, connection, from, to string }{
		{"RPV-7#1", "SYN-CONN-SHA-SZX", "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"},
		{"RPV-7#2", "SYN-CONN-SZX-SIN", "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB"},
		{"RPV-7#3", "SYN@CONN", "SYN-NODE-SIN-HUB", "SYN-NODE-SIN-LM"},
	}
	for index, leg := range command.Legs {
		if leg.Reference().String() != want[index].reference || leg.Direction().Connection().String() != want[index].connection ||
			leg.Direction().From().String() != want[index].from || leg.Direction().To().String() != want[index].to {
			t.Fatalf("段 %d = %s %s %s→%s", index+1, leg.Reference(), leg.Direction().Connection(),
				leg.Direction().From(), leg.Direction().To())
		}
	}
	if !command.Legs[1].WindowStart().Equal(routeJudgedAt.Add(4*time.Hour)) || !command.Legs[1].WindowEnd().Equal(routeJudgedAt.Add(8*time.Hour)) {
		t.Fatalf("窗口没照抄：%s – %s", command.Legs[1].WindowStart(), command.Legs[1].WindowEnd())
	}
}

// Covers: AT-TF-127——无路线判断也交编排，由编排答零写入；翻译层不悄悄跳过。
func TestANoRouteJudgmentReachesThePreparationAsNotFormed(t *testing.T) {
	eliminated, err := nrdomain.NewRouteCandidate(nr(t, nrdomain.NewCandidateID, "SYN-CAND-01"), nrdomain.CandidateEliminated,
		nr(t, nrdomain.NewCandidateReason, "SYN-PATH-CLOSED"))
	if err != nil {
		t.Fatalf("候选：%v", err)
	}
	judgment, err := nrdomain.FormNoCurrentRouteJudgment(nrdomain.NoCurrentRouteJudgmentSpec{
		Key: routeKey(t), Candidates: []nrdomain.RouteCandidate{eliminated},
		Strategy:     nr(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY@1"),
		ViewRevision: nr(t, nrdomain.NewNetworkViewRevision, "SYN-NETVIEW-01"), JudgedAt: routeJudgedAt,
	})
	if err != nil {
		t.Fatalf("无路线判断：%v", err)
	}
	trigger := &preparationTriggerStub{}
	_ = handle(t, routeFinderStub{record: nrports.InitialRouteRecord{Key: routeKey(t), NoRoute: judgment, HasNoRoute: true}, found: true}, trigger)
	if trigger.calls != 1 || trigger.command.Route != tfapplication.PlannedRouteNotFormed || len(trigger.command.Legs) != 0 {
		t.Fatalf("无路线交编排 = %d 次 %+v", trigger.calls, trigger.command)
	}
}

// Covers: AT-TF-135、计划 D01-1 的哨兵分格——读不回与读失败答未决哨兵；记录对不上键答不变量已破；窗口依据一格连接都不指或
// 指两格，答译不过去、不交编排——不折成`未配置`。
func TestUnreadableOrUntranslatableRoutesFailWithTheirOwnSentinels(t *testing.T) {
	trigger := &preparationTriggerStub{}
	if err := handle(t, routeFinderStub{}, trigger); !errors.Is(err, adapter.ErrInitialRouteNotVisible) {
		t.Fatalf("读不回：%v", err)
	}
	if err := handle(t, routeFinderStub{err: errors.New("down")}, trigger); !errors.Is(err, adapter.ErrInitialRouteNotVisible) {
		t.Fatalf("读失败：%v", err)
	}
	inconsistent := formedRecord(t, legOn(t, "CONNECTION/C@1", "A", "B", 0))
	inconsistent.HasNoRoute = true
	if err := handle(t, routeFinderStub{record: inconsistent, found: true}, trigger); !errors.Is(err, adapter.ErrInitialRouteRecordInconsistent) {
		t.Fatalf("两支都带：%v", err)
	}
	for name, basis := range map[string]string{
		"不指连接":   "SYN-CALENDAR/v1",
		"指两条连接":  "CONNECTION/C1@1+CONNECTION/C2@1",
		"连接没有版本": "CONNECTION/C1",
	} {
		record := formedRecord(t, legOn(t, basis, "A", "B", 0))
		if err := handle(t, routeFinderStub{record: record, found: true}, trigger); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("%s：%v", name, err)
		}
	}
	if trigger.calls != 0 {
		t.Fatalf("读不回或译不过去时调了编排 %d 次", trigger.calls)
	}
}
