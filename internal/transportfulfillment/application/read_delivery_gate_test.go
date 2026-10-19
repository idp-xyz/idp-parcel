package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证交付门禁读数的编排半边（ADR-0213 决定五）：写事务之前按交付意图问关务，at 取对象进入这次交付所在那一段时那次
// 交接的交出节点；读数随交付同笔落册、不拒收。段册、交接册与关务都是替身，节点与对象都是合成串，证据只记 `S`。

// deliveryGateSegmentsDouble 答「对象此刻在哪段在场」与「那一段长什么样」；各口可单独坏。
type deliveryGateSegmentsDouble struct {
	active    []ports.FulfillmentSegmentKey
	activeErr error
	records   map[string]ports.FulfillmentSegmentRecord
	findErr   error
}

func (double *deliveryGateSegmentsDouble) FindActiveSegments(
	context.Context, domain.TenantID, domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	return double.active, double.activeErr
}

func (double *deliveryGateSegmentsDouble) FindByKey(
	_ context.Context, key ports.FulfillmentSegmentKey,
) (ports.FulfillmentSegmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.FulfillmentSegmentRecord{}, false, double.findErr
	}
	record, found := double.records[key.Segment.String()]
	return record, found, nil
}

// entryHandoversDouble 按版本交回`已交接`的各版；记下问过几次。
type entryHandoversDouble struct {
	handovers []domain.TransportHandover
	err       error
	asked     int
}

func (double *entryHandoversDouble) FindHandedOverByVersion(
	_ context.Context, _ domain.TenantID, object domain.CarriedObjectReference, version domain.HandoverResultVersion,
) ([]domain.TransportHandover, error) {
	double.asked++
	if double.err != nil {
		return nil, double.err
	}
	var matched []domain.TransportHandover
	for _, handover := range double.handovers {
		if handover.Object() == object && handover.Version() == version {
			matched = append(matched, handover)
		}
	}
	return matched, nil
}

// customsDeliveryGateDouble 是关务动作准入判断的消费侧替身：记下每一问，按 answer 答；err 非空时这一问没形成答复。
type customsDeliveryGateDouble struct {
	answer    domain.GateAnswer
	err       error
	answerFor string
	queries   []ports.DeliveryGateQuery
}

func (double *customsDeliveryGateDouble) ReadDeliveryGate(
	_ context.Context, query ports.DeliveryGateQuery,
) (domain.GateFinding, error) {
	double.queries = append(double.queries, query)
	if double.err != nil {
		return domain.GateFinding{}, double.err
	}
	object := query.Object
	if double.answerFor != "" {
		other, err := domain.NewCarriedObjectReference(double.answerFor)
		if err != nil {
			return domain.GateFinding{}, err
		}
		object = other
	}
	judgement, basis := "AADJ-"+object.String(), domain.GateBasis{Action: "FINAL_DELIVERY", Cause: "OUTSIDE_CUSTOMS_BOOKS"}
	switch double.answer {
	case domain.GateUnreadable:
		judgement, basis = "", domain.GateBasis{Cause: "RELEASE_OUTCOME_VIEW_UNAVAILABLE"}
	case domain.GateUndecided:
		basis = domain.GateBasis{Action: "FINAL_DELIVERY", Cause: "CUSTOMS_SCOPE_UNRESOLVED"}
	case domain.GateBlocked:
		basis = domain.GateBasis{Action: "FINAL_DELIVERY", Cause: "RESTRICTION_IN_FORCE", Sources: []string{"SYN-RESTRICTION-01"}}
	}
	return domain.NewGateFinding(object, double.answer, judgement, []domain.GateBasis{basis})
}

// deliveryGateFixture 是一份登记编排连同写事务之前那一侧：段册、交接册、关务与派送尝试读面都是替身。
type deliveryGateFixture struct {
	*deliveryFixture
	segments  *deliveryGateSegmentsDouble
	handovers *entryHandoversDouble
	customs   *customsDeliveryGateDouble
	reader    *application.DeliveryGateReader
}

func newDeliveryGateFixture(t *testing.T) *deliveryGateFixture {
	t.Helper()
	fixture := &deliveryGateFixture{
		deliveryFixture: newDeliveryFixture(t),
		segments:        &deliveryGateSegmentsDouble{records: map[string]ports.FulfillmentSegmentRecord{}},
		handovers:       &entryHandoversDouble{},
		customs:         &customsDeliveryGateDouble{answer: domain.GateNotGuarded},
	}
	reader, err := application.NewDeliveryGateReader(application.DeliveryGateReadDeps{
		Attempts:  fixture.view,
		Segments:  fixture.segments,
		Handovers: fixture.handovers,
		Customs:   fixture.customs,
	})
	if err != nil {
		t.Fatalf("交付门禁读数：%v", err)
	}
	fixture.reader = reader
	return fixture
}

// enteredByHandover 让 parcel-1 凭一版`已交接`进 segment-1；releasing 为空即那次交接没声明交出节点。
func (fixture *deliveryGateFixture) enteredByHandover(t *testing.T, releasing string) domain.TransportHandover {
	t.Helper()
	handover := deliveryEntryHandover(t, "handover-scope-1", releasing, deliveryArrivedAt.Add(-2*time.Hour))
	segment, err := domain.EstablishSegmentWithHandover(deliverySegmentReference(t), handover, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	fixture.placeSegment(t, segment)
	fixture.handovers.handovers = append(fixture.handovers.handovers, handover)
	return handover
}

// enteredByLegacyHandover 摆一条 ADR-0221 之前的存量：参与凭`已交接`进段，入场依据是只带版本的旧拼法
// `TRANSPORT-HANDOVER/<版本>`，起点是那次交接的业务时间。
func (fixture *deliveryGateFixture) enteredByLegacyHandover(t *testing.T, releasing string) domain.TransportHandover {
	t.Helper()
	handover := deliveryEntryHandover(t, "handover-scope-1", releasing, deliveryArrivedAt.Add(-2*time.Hour))
	basis, err := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/" + handover.Version().String())
	if err != nil {
		t.Fatalf("旧拼法依据：%v", err)
	}
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: handover.TenantID(),
		Segment:  deliverySegmentReference(t),
		Participations: []domain.RehydrateParticipationSpec{{
			Object:     handover.Object(),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: basis,
			EnteredAt:  handover.JudgedAt(),
		}},
	})
	if err != nil {
		t.Fatalf("重建存量段：%v", err)
	}
	fixture.placeSegment(t, segment)
	fixture.handovers.handovers = append(fixture.handovers.handovers, handover)
	return handover
}

func (fixture *deliveryGateFixture) placeSegment(t *testing.T, segment domain.ActualFulfillmentSegment) {
	t.Helper()
	key := ports.FulfillmentSegmentKey{TenantID: segment.TenantID(), Segment: deliverySegmentReference(t)}
	fixture.segments.active = []ports.FulfillmentSegmentKey{key}
	fixture.segments.records[key.Segment.String()] = ports.FulfillmentSegmentRecord{Key: key, Segment: segment}
}

func deliverySegmentReference(t *testing.T) domain.FulfillmentSegmentReference {
	t.Helper()
	reference, err := domain.NewFulfillmentSegmentReference("segment-1")
	if err != nil {
		t.Fatalf("segment: %v", err)
	}
	return reference
}

func deliveryEntryHandover(t *testing.T, scope, releasing string, judgedAt time.Time) domain.TransportHandover {
	t.Helper()
	spec := domain.TransportHandoverSpec{JudgedAt: judgedAt, Verdict: domain.ObjectHandedOver}
	var err error
	for _, step := range []func() error{
		func() (err error) { spec.TenantID, err = domain.NewTenantID("tenant-1"); return },
		func() (err error) { spec.Object, err = domain.NewCarriedObjectReference("parcel-1"); return },
		func() (err error) { spec.Scope, err = domain.NewHandoverScopeReference(scope); return },
		func() (err error) {
			spec.ReleasedBy, err = domain.NewHandoverPartyReference("SYN-CARRIER-TRUNK")
			return
		},
		func() (err error) {
			spec.ReceivedBy, err = domain.NewHandoverPartyReference("SYN-NODE-SIN-HUB")
			return
		},
		func() (err error) {
			spec.ReleasingEvidence, err = domain.NewHandoverEvidenceReference("evidence-release-1")
			return
		},
		func() (err error) {
			spec.ReceivingEvidence, err = domain.NewHandoverEvidenceReference("evidence-receive-1")
			return
		},
		func() (err error) { spec.Rule, err = domain.NewHandoverRuleReference("handover-rule/v1"); return },
		func() (err error) {
			spec.Version, err = domain.NewHandoverResultVersion("handover-result/parcel-1/v1")
			return
		},
	} {
		if err = step(); err != nil {
			t.Fatalf("交接夹具：%v", err)
		}
	}
	if releasing != "" {
		if spec.Nodes.Releasing, err = domain.NewNetworkNodeReference(releasing); err != nil {
			t.Fatalf("交出节点：%v", err)
		}
	}
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("交接：%v", err)
	}
	return handover
}

// gatedDeliveryRegister 照组合根（cmd/parcel-api 的 transactionalDelivery）的次序登一份首登：先认重放、取在册读数，认不出
// 再在写事务之前问一份，然后带着它登。
func (fixture *deliveryGateFixture) gatedDeliveryRegister(
	t *testing.T,
	command application.RegisterEffectiveDeliveryCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	t.Helper()
	handler := fixture.handler.RegisterEffectiveDeliveryHandler
	reading, replayed := handler.RecordedDeliveryGateReading(context.Background(), command)
	if !replayed {
		var err error
		if reading, err = fixture.reader.ReadRegistration(context.Background(), command); err != nil {
			return application.RegisterEffectiveDeliveryResult{}, err
		}
	}
	return handler.RegisterWithGateReading(context.Background(), command, reading)
}

// registerAndRead 照组合根的次序登一份 registerCommand，交回落册那一版的读数。
func (fixture *deliveryGateFixture) registerAndRead(t *testing.T) domain.DeliveryGateReading {
	t.Helper()
	result, err := fixture.gatedDeliveryRegister(t, registerCommand(t))
	return registeredGateReading(t, result, err)
}

func registeredGateReading(t *testing.T, result application.RegisterEffectiveDeliveryResult, err error) domain.DeliveryGateReading {
	t.Helper()
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.DeliveryRegistered && result.Outcome() != application.DeliveryExistingVersion {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	record, _ := result.Record()
	reading, read := record.Delivery.GateReading()
	if !read {
		t.Fatal("登下的交付没带读数")
	}
	return reading
}

func readingStop(t *testing.T, reading domain.DeliveryGateReading) domain.DeliveryGateStop {
	t.Helper()
	stop, stopped := reading.Stop()
	if !stopped {
		t.Fatalf("读数 %s 没停在本上下文这一侧", reading.Answer())
	}
	return stop
}

// Covers: `AT-TF-256`——有效交付按交付意图问关务：at 取对象进入这次交付所在那一段时那次交接声明的交出节点，时点取交付的
// 业务时间（对象结果的发生时刻，不是登记时刻）；读数随交付同笔落册。
func TestADeliveryIsJudgedAtTheReleasingNodeOfTheHandoverThatBroughtItIn(t *testing.T) {
	fixture := newDeliveryGateFixture(t)
	fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")

	reading := fixture.registerAndRead(t)

	if len(fixture.customs.queries) != 1 {
		t.Fatalf("问关务 %d 次，want 1", len(fixture.customs.queries))
	}
	query := fixture.customs.queries[0]
	if query.Node.String() != "SYN-NODE-SIN-HUB" || query.Object.String() != "parcel-1" || query.TenantID.String() != "tenant-1" {
		t.Fatalf("问询 = %+v", query)
	}
	if want := deliveryArrivedAt.Add(10 * time.Minute); !query.At.Equal(want) {
		t.Fatalf("时点 = %s，want 交付业务时间 %s", query.At, want)
	}
	if reading.Answer() != domain.GateNotGuarded {
		t.Fatalf("读数 = %s", reading.Answer())
	}
	if node, asked := reading.Node(); !asked || node.String() != "SYN-NODE-SIN-HUB" {
		t.Fatalf("at = %q", node)
	}
	stored, _ := fixture.store.records[deliveryKey(ports.EffectiveDeliveryKey{
		TenantID: query.TenantID, Object: query.Object, Attempt: mustAttempt(t, "attempt-1"),
	})].Delivery.GateReading()
	if !reflect.DeepEqual(stored, reading) {
		t.Fatalf("落册的读数 = %+v", stored)
	}
}

// Covers: `AT-TF-257`、`AT-TF-258`、`AT-TF-259`——关务答什么都照登：放行、不受门禁、阻断、未决各成一格；关务自己答读不动、
// 调用失败、答复译不回来都记读不动，后两种依据种类分开。读数不拒收交付。
func TestEveryCustomsAnswerIsRecordedAndTheDeliveryStillRegisters(t *testing.T) {
	for _, answer := range []domain.GateAnswer{
		domain.GateNotGuarded, domain.GateAdmitted, domain.GateBlocked, domain.GateUndecided, domain.GateUnreadable,
	} {
		t.Run(answer.String(), func(t *testing.T) {
			fixture := newDeliveryGateFixture(t)
			fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
			fixture.customs.answer = answer
			if reading := fixture.registerAndRead(t); reading.Answer() != answer {
				t.Fatalf("读数 = %s", reading.Answer())
			}
		})
	}

	for cause, failure := range map[string]error{
		"CUSTOMS_ADMISSION_UNAVAILABLE":    errors.New("customs down"),
		"CUSTOMS_ADMISSION_UNTRANSLATABLE": fmt.Errorf("drifted: %w", ports.ErrDeliveryGateAnswerUntranslatable),
	} {
		t.Run(cause, func(t *testing.T) {
			fixture := newDeliveryGateFixture(t)
			fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
			fixture.customs.err = failure
			reading := fixture.registerAndRead(t)
			finding, asked := reading.Finding()
			if reading.Answer() != domain.GateUnreadable || !asked || finding.Bases()[0].Cause != cause {
				t.Fatalf("读数 = %s，依据 %+v", reading.Answer(), finding.Bases())
			}
			if _, judged := finding.Judgement(); judged {
				t.Fatal("没形成答复却带了关务判断标识")
			}
		})
	}
}

// Covers: `AT-TF-260`、`AT-TF-261`——交付节点不可得：对象凭收寄进的这一段、或进段那次交接没声明交出节点，读数记未决、
// 不问关务，交付照登。
func TestAPickupEntryOrAnUndeclaredReleasingNodeLeavesTheDeliveryNodeUnavailable(t *testing.T) {
	t.Run("entered by pickup", func(t *testing.T) {
		fixture := newDeliveryGateFixture(t)
		segment, err := domain.EstablishSegmentWithPickup(deliverySegmentReference(t), entryPickup(t), domain.PlannedSegmentReference{})
		if err != nil {
			t.Fatalf("立段：%v", err)
		}
		fixture.placeSegment(t, segment)
		reading := fixture.registerAndRead(t)
		if stop := readingStop(t, reading); stop != domain.DeliveryEnteredByPickup || reading.Answer() != domain.GateUndecided {
			t.Fatalf("读数 = %s / %s", reading.Answer(), stop)
		}
		if len(fixture.customs.queries) != 0 || fixture.handovers.asked != 0 {
			t.Fatal("凭收寄进段却去问了交接册或关务")
		}
	})

	t.Run("the entry handover declared no releasing node", func(t *testing.T) {
		fixture := newDeliveryGateFixture(t)
		fixture.enteredByHandover(t, "")
		reading := fixture.registerAndRead(t)
		if stop := readingStop(t, reading); stop != domain.DeliveryReleasingNodeNotDeclared || reading.Answer() != domain.GateUndecided {
			t.Fatalf("读数 = %s / %s", reading.Answer(), stop)
		}
		if len(fixture.customs.queries) != 0 {
			t.Fatal("没声明交出节点却问了关务")
		}
	})
}

func entryPickup(t *testing.T) domain.OffsitePickup {
	t.Helper()
	spec := domain.OffsitePickupSpec{OccurredAt: deliveryArrivedAt.Add(-24 * time.Hour)}
	var err error
	for _, step := range []func() error{
		func() (err error) { spec.TenantID, err = domain.NewTenantID("tenant-1"); return },
		func() (err error) { spec.Object, err = domain.NewCarriedObjectReference("parcel-1"); return },
		func() (err error) { spec.Task, err = domain.NewPickupTaskReference("pickup-task-1"); return },
		func() (err error) { spec.Attempt, err = domain.NewAttemptReference("pickup-attempt-1"); return },
		func() (err error) { spec.Place, err = domain.NewPickupPlaceReference("sender-door-1"); return },
		func() (err error) {
			spec.Control, err = domain.NewTransportControlReference("TRANSPORT-CONTROL/1")
			return
		},
		func() (err error) { spec.ExecutedBy, err = domain.NewExecutingPartyReference("courier-1"); return },
		func() (err error) {
			spec.Version, err = domain.NewPickupResultVersion("pickup-result/parcel-1/v1")
			return
		},
	} {
		if err = step(); err != nil {
			t.Fatalf("收寄夹具：%v", err)
		}
	}
	pickup, err := domain.FormOffsitePickup(spec)
	if err != nil {
		t.Fatalf("收寄：%v", err)
	}
	return pickup
}

// Covers: `AT-TF-262`——其余取不到节点或读不动的几格各自具名、照登、不问关务：对象此刻不在任何段里；进段依据指名的交接
// 不在册（库面不一致），或在册上对上不止一版（ADR-0221 之前只带版本的存量依据，同版本号在两个范围、业务时间也相同），两种
// 分开具名；派送尝试读面、段册、交接册在写事务之前读不动。登不出
// 交付的命令不带读数；对象同时在两段里在场是库面不一致，响亮报错。
func TestTheOtherDeliveryNodeStopsAreNamedAndRegister(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *deliveryGateFixture)
		want    domain.DeliveryGateStop
	}{
		"no active segment": {
			arrange: func(*testing.T, *deliveryGateFixture) {},
			want:    domain.DeliveryNoActiveSegment,
		},
		"the entry handover is not on the register": {
			arrange: func(t *testing.T, fixture *deliveryGateFixture) {
				fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
				fixture.handovers.handovers = nil
			},
			want: domain.DeliveryEntryHandoverNotOnRegister,
		},
		"a legacy entry basis meets two scopes reusing the version at the same instant": {
			arrange: func(t *testing.T, fixture *deliveryGateFixture) {
				entry := fixture.enteredByLegacyHandover(t, "SYN-NODE-SIN-HUB")
				fixture.handovers.handovers = append(fixture.handovers.handovers,
					deliveryEntryHandover(t, "handover-scope-2", "SYN-NODE-SZX-GATE", entry.JudgedAt()))
			},
			want: domain.DeliveryEntryHandoverAmbiguous,
		},
		"the attempt view is down before the write": {
			arrange: func(t *testing.T, fixture *deliveryGateFixture) {
				fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
				fixture.reader = readerWithAttemptView(t, fixture, &deliveryViewDouble{err: errors.New("view down")})
			},
			want: domain.DeliveryAttemptViewUnavailable,
		},
		"the segment registry is down": {
			arrange: func(t *testing.T, fixture *deliveryGateFixture) {
				fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
				fixture.segments.findErr = errors.New("segments down")
			},
			want: domain.DeliverySegmentRegistryUnavailable,
		},
		"the active segment lookup is down": {
			arrange: func(t *testing.T, fixture *deliveryGateFixture) {
				fixture.segments.activeErr = errors.New("segments down")
			},
			want: domain.DeliverySegmentRegistryUnavailable,
		},
		"the handover registry is down": {
			arrange: func(t *testing.T, fixture *deliveryGateFixture) {
				fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
				fixture.handovers.err = errors.New("handovers down")
			},
			want: domain.DeliveryHandoverRegistryUnavailable,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryGateFixture(t)
			test.arrange(t, fixture)
			reading := fixture.registerAndRead(t)
			if stop := readingStop(t, reading); stop != test.want || reading.Answer() != test.want.Answer() {
				t.Fatalf("读数 = %s / %s，want %s", reading.Answer(), stop, test.want)
			}
			if len(fixture.customs.queries) != 0 {
				t.Fatal("取不到节点却问了关务")
			}
		})
	}

	t.Run("a failed result gets no reading", func(t *testing.T) {
		fixture := newDeliveryGateFixture(t)
		fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
		fixture.view.outcome = domain.NoOneToReceive
		reading, err := fixture.reader.ReadRegistration(context.Background(), registerCommand(t))
		if err != nil || reading.Answer() != domain.GateAnswerInvalid || len(fixture.customs.queries) != 0 {
			t.Fatalf("未妥投也读了：%s %v", reading.Answer(), err)
		}
	})

	t.Run("two active segments are a loud error", func(t *testing.T) {
		fixture := newDeliveryGateFixture(t)
		fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
		fixture.segments.active = append(fixture.segments.active, fixture.segments.active[0])
		if _, err := fixture.reader.ReadRegistration(context.Background(), registerCommand(t)); !errors.Is(err, application.ErrObjectActiveInSeveralSegments) {
			t.Fatalf("err = %v, want ErrObjectActiveInSeveralSegments", err)
		}
	})
}

func readerWithAttemptView(t *testing.T, fixture *deliveryGateFixture, view ports.DeliveryAttemptView) *application.DeliveryGateReader {
	t.Helper()
	reader, err := application.NewDeliveryGateReader(application.DeliveryGateReadDeps{
		Attempts: view, Segments: fixture.segments, Handovers: fixture.handovers, Customs: fixture.customs,
	})
	if err != nil {
		t.Fatalf("交付门禁读数：%v", err)
	}
	return reader
}

// Covers: `AT-TF-271`（ADR-0221）——入场依据由交接键铸之后，同一对象在两个范围同用一个版本号、业务时间也相同，交付门禁照样
// 认得出进段的那一次：取它声明的交出节点问关务，不停在 ENTRY_HANDOVER_AMBIGUOUS。
func TestAKeyedEntryBasisTellsTwoScopesReusingTheVersionApart(t *testing.T) {
	fixture := newDeliveryGateFixture(t)
	entry := fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
	fixture.handovers.handovers = append([]domain.TransportHandover{
		deliveryEntryHandover(t, "handover-scope-0", "SYN-NODE-SZX-GATE", entry.JudgedAt()),
	}, fixture.handovers.handovers...)

	reading := fixture.registerAndRead(t)

	if node, asked := reading.Node(); !asked || node.String() != "SYN-NODE-SIN-HUB" {
		t.Fatalf("at = %q（读数 %s），want 进段那一次声明的交出节点", node, reading.Answer())
	}
	if len(fixture.customs.queries) != 1 {
		t.Fatalf("问关务 %d 次，want 1", len(fixture.customs.queries))
	}
}

// Covers: `AT-TF-263`——同内容重放在写事务之前就认出来，回显首次读数，不重问关务；交付之后参与已结束，按此刻重读会读成
// 「不在任何段里」，所以这一步不能省。POD 更正不问关务，照前版承继同一份。
func TestAReplayEchoesTheRecordedReadingAndACorrectionInheritsIt(t *testing.T) {
	fixture := newDeliveryGateFixture(t)
	fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
	first := fixture.registerAndRead(t)

	fixture.segments.active = nil
	fixture.customs.answer = domain.GateBlocked
	replay, err := fixture.gatedDeliveryRegister(t, registerCommand(t))
	if err != nil || replay.Outcome() != application.DeliveryExistingVersion {
		t.Fatalf("重放 = %s %v", replay.Outcome(), err)
	}
	if echoed := registeredGateReading(t, replay, nil); !reflect.DeepEqual(echoed, first) {
		t.Fatalf("重放回显 %s，want 首次的 %s", echoed.Answer(), first.Answer())
	}
	if len(fixture.customs.queries) != 1 {
		t.Fatalf("问关务 %d 次，want 只有首登那一次", len(fixture.customs.queries))
	}

	record, _ := replay.Record()
	corrected, err := fixture.handler.Correct(context.Background(), application.CorrectDeliveryProofCommand{
		TenantID:           record.Key.TenantID,
		Attempt:            "attempt-1",
		Object:             "parcel-1",
		PredecessorVersion: record.Delivery.Version().String(),
		NewProof:           "pod-2",
		CorrectedAt:        deliveryRecordedAt.Add(time.Hour),
	})
	if err != nil || corrected.Outcome() != application.DeliveryCorrected {
		t.Fatalf("更正 = %s %v", corrected.Outcome(), err)
	}
	correctedRecord, _ := corrected.Record()
	if inherited, _ := correctedRecord.Delivery.GateReading(); !reflect.DeepEqual(inherited, first) {
		t.Fatalf("更正版读数 = %s，want 承继首登那一份", inherited.Answer())
	}
	if len(fixture.customs.queries) != 1 {
		t.Fatal("更正去问了关务")
	}
}

// Covers: `AT-TF-264`——要新落一份交付却没带读数是接线错误：上抛 ErrDeliveryGateReadingMisplaced，不落册；读数问的是另一件
// 对象同样上抛。关务端口答了另一件对象是实现坏了，读数那一侧报错，不记成一格读不动。登不出交付的各条路不需要读数。
func TestANewDeliveryWithoutAGateReadingIsAWiringError(t *testing.T) {
	fixture := newDeliveryFixture(t)
	raw := fixture.handler.RegisterEffectiveDeliveryHandler
	if _, err := raw.Register(context.Background(), registerCommand(t)); !errors.Is(err, domain.ErrDeliveryGateReadingMisplaced) {
		t.Fatalf("err = %v, want ErrDeliveryGateReadingMisplaced", err)
	}
	if fixture.store.saves != 0 {
		t.Fatal("没带读数的交付落册了")
	}

	misplaced, err := domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{
		Node:    mustNode(t, "SYN-NODE-SIN-HUB"),
		Finding: mustFinding(t, "parcel-2"),
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if _, err := raw.RegisterWithGateReading(context.Background(), registerCommand(t), misplaced); !errors.Is(err, domain.ErrDeliveryGateReadingMisplaced) {
		t.Fatalf("err = %v, want ErrDeliveryGateReadingMisplaced", err)
	}

	fixture.view.outcome = domain.NoOneToReceive
	if result, err := raw.Register(context.Background(), registerCommand(t)); err != nil || result.Outcome() != application.DeliveryNotEffective {
		t.Fatalf("未生效那一路要了读数：%s %v", result.Outcome(), err)
	}

	gate := newDeliveryGateFixture(t)
	gate.enteredByHandover(t, "SYN-NODE-SIN-HUB")
	gate.customs.answerFor = "parcel-2"
	if _, err := gate.reader.ReadRegistration(context.Background(), registerCommand(t)); err == nil {
		t.Fatal("关务答了另一件对象，读数那一侧却没报错")
	}
}

// Covers: `AT-TF-264`——登不出交付的命令不问关务：缺 POD（交付证明必备）、方式或接收方受理不了的，读数那一侧与登记编排按
// 同一处受理认出来，交回零值读数，登记照常答未受理；关务一笔判断都不记，不留一份从没落册的交付的判断。
func TestACommandThatCannotFormADeliveryDoesNotAskCustoms(t *testing.T) {
	for name, spoil := range map[string]func(*application.RegisterEffectiveDeliveryCommand){
		"missing proof":     func(command *application.RegisterEffectiveDeliveryCommand) { command.Proof = "" },
		"missing method":    func(command *application.RegisterEffectiveDeliveryCommand) { command.Method = "" },
		"missing recipient": func(command *application.RegisterEffectiveDeliveryCommand) { command.Recipient = "" },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryGateFixture(t)
			fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
			command := registerCommand(t)
			spoil(&command)

			result, err := fixture.gatedDeliveryRegister(t, command)
			if err != nil || result.Outcome() != application.DeliveryNotAccepted {
				t.Fatalf("outcome = %s %v, want NOT_ACCEPTED", result.Outcome(), err)
			}
			if len(fixture.customs.queries) != 0 {
				t.Fatalf("问关务 %d 次，want 0：这次交付登不出来", len(fixture.customs.queries))
			}
			if fixture.store.saves != 0 {
				t.Fatal("受理不了的命令落册了")
			}
		})
	}
}

func mustNode(t *testing.T, raw string) domain.NetworkNodeReference {
	t.Helper()
	node, err := domain.NewNetworkNodeReference(raw)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	return node
}

func mustFinding(t *testing.T, object string) domain.GateFinding {
	t.Helper()
	reference, err := domain.NewCarriedObjectReference(object)
	if err != nil {
		t.Fatalf("object: %v", err)
	}
	finding, err := domain.NewGateFinding(reference, domain.GateNotGuarded, "AADJ-"+object,
		[]domain.GateBasis{{Action: "FINAL_DELIVERY", Cause: "OUTSIDE_CUSTOMS_BOOKS"}})
	if err != nil {
		t.Fatalf("finding: %v", err)
	}
	return finding
}
