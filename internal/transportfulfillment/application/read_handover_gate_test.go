package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证交出门禁读数的编排半边（ADR-0213 决定四）：写事务之前按交出意图问关务、读数随`已交接`那一版落册、不拒收。
// 关务一侧是替身，节点与对象都是合成串，证据只记 `S`。

// customsHandoverGateDouble 是关务动作准入判断的消费侧替身：记下每一问，按 answer 答；err 非空时这一问没形成答复。
type customsHandoverGateDouble struct {
	answer  domain.GateAnswer
	err     error
	queries []ports.HandoverGateQuery
	// answerFor 非空时按它答另一件对象，钉「端口答错了对象」那一格。
	answerFor string
}

func (double *customsHandoverGateDouble) ReadHandoverGate(
	_ context.Context,
	query ports.HandoverGateQuery,
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
	judgement := "AADJ-" + object.String()
	basis := domain.GateBasis{Action: "OUTBOUND_RELEASE", Cause: "OUTSIDE_CUSTOMS_BOOKS"}
	switch double.answer {
	case domain.GateUnreadable:
		judgement, basis = "", domain.GateBasis{Cause: "RELEASE_OUTCOME_VIEW_UNAVAILABLE"}
	case domain.GateUndecided:
		basis = domain.GateBasis{Action: "CROSS_CUSTOMS_MOVEMENT", Cause: "CUSTOMS_SCOPE_UNRESOLVED"}
	case domain.GateBlocked:
		basis = domain.GateBasis{Action: "OUTBOUND_RELEASE", Cause: "RESTRICTION_IN_FORCE", Sources: []string{"SYN-RESTRICTION-01"}}
	}
	return domain.NewGateFinding(object, double.answer, judgement, []domain.GateBasis{basis})
}

func notGuardedCustoms() *customsHandoverGateDouble {
	return &customsHandoverGateDouble{answer: domain.GateNotGuarded}
}

func handoverGateReader(t *testing.T, registry *handoverRegistryDouble, customs ports.HandoverGateView) *application.HandoverGateReader {
	t.Helper()
	reader, err := application.NewHandoverGateReader(application.HandoverGateReadDeps{Handovers: registry, Customs: customs})
	if err != nil {
		t.Fatalf("交出门禁读数：%v", err)
	}
	return reader
}

// gatedRegister 照组合根（cmd/parcel-api 的 transactionalHandover）的次序登一份首登：先认重放、取在册读数，认不出再在写
// 事务之前问一份，然后带着它登。
func gatedRegister(
	t *testing.T,
	fixture *handoverFixture,
	customs ports.HandoverGateView,
	command application.RegisterTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	t.Helper()
	reading, replayed := fixture.handler.RecordedHandoverGateReading(context.Background(), command)
	if !replayed {
		var err error
		if reading, err = handoverGateReader(t, fixture.registry, customs).ReadRegistration(context.Background(), command); err != nil {
			return application.RegisterTransportHandoverResult{}, err
		}
	}
	return fixture.handler.RegisterWithGateReading(context.Background(), command, reading)
}

// gatedCorrect 是 gatedRegister 的更正口。
func gatedCorrect(
	t *testing.T,
	fixture *handoverFixture,
	customs ports.HandoverGateView,
	command application.CorrectTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	t.Helper()
	reading, replayed := fixture.handler.RecordedCorrectionGateReading(context.Background(), command)
	if !replayed {
		var err error
		if reading, err = handoverGateReader(t, fixture.registry, customs).ReadCorrection(context.Background(), command); err != nil {
			return application.RegisterTransportHandoverResult{}, err
		}
	}
	return fixture.handler.CorrectWithGateReading(context.Background(), command, reading)
}

func releasedAtGate(t *testing.T, onward string) application.RegisterTransportHandoverCommand {
	t.Helper()
	command := registerHandoverCommand(t)
	command.ReleasingNode = "SYN-NODE-SZX-GATE"
	command.ReceivedBy = "SYN-PARTY-CARRIER-TRUNK-01"
	command.OnwardNode = onward
	return command
}

func recordedReading(t *testing.T, result application.RegisterTransportHandoverResult) domain.HandoverGateReading {
	t.Helper()
	record, present := result.Record()
	if !present {
		t.Fatalf("答复没带记录：%s", result.Outcome())
	}
	reading, read := record.Handover.GateReading()
	if !read {
		t.Fatalf("`已交接`那一版没带交出门禁读数：%+v", record.Handover)
	}
	return reading
}

// Covers: AT-TF-236——`已交接`声明了交出与去向节点：写事务之前按交出意图问关务，at 取交出节点、onward 取去向节点、时点取
// 交接业务时间；关务答什么，读数就随那一版照记什么，阻断也照样落册、答已登记——读数不拒收交接。
func TestAHandedOverHandoverAsksCustomsForTheOutboundReleaseAndRecordsTheAnswer(t *testing.T) {
	for _, answer := range []domain.GateAnswer{
		domain.GateNotGuarded, domain.GateAdmitted, domain.GateUndecided, domain.GateBlocked,
	} {
		t.Run(answer.String(), func(t *testing.T) {
			fixture := newHandoverFixture(t)
			customs := &customsHandoverGateDouble{answer: answer}
			command := releasedAtGate(t, "SYN-NODE-SIN-HUB")

			result, err := gatedRegister(t, fixture, customs, command)
			wantOutcome(t, result.Outcome(), err, application.HandoverRegistered)
			if len(customs.queries) != 1 {
				t.Fatalf("问了关务 %d 次，want 1", len(customs.queries))
			}
			query := customs.queries[0]
			if query.Object.String() != "parcel-1" || query.Node.String() != "SYN-NODE-SZX-GATE" ||
				query.Onward.String() != "SYN-NODE-SIN-HUB" || !query.At.Equal(handoverJudgedTime) || query.TenantID.String() != "tenant-1" {
				t.Fatalf("交出意图 = %+v", query)
			}

			reading := recordedReading(t, result)
			finding, asked := reading.Finding()
			node, _ := reading.Node()
			onward, _ := reading.Onward()
			if reading.Answer() != answer || !asked || finding.Answer() != answer || node.String() != "SYN-NODE-SZX-GATE" ||
				onward.String() != "SYN-NODE-SIN-HUB" || reading.OnwardNotDeclared() {
				t.Fatalf("读数 = %+v", reading)
			}
			if judgement, judged := finding.Judgement(); !judged || judgement != "AADJ-parcel-1" {
				t.Fatalf("关务判断标识没随读数落下：%q", judgement)
			}
			if stored, _ := onlyHandoverRecord(t, fixture.registry).Handover.GateReading(); stored.Answer() != answer {
				t.Fatalf("在册那一版的读数 = %s", stored.Answer())
			}
		})
	}
}

// Covers: AT-TF-237——没声明交出节点：不问关务，读数未决「交出节点未声明」，声明了去向也一样；不从交出方或范围补节点。
func TestAHandoverWithoutAReleasingNodeIsUndecidedWithoutAskingCustoms(t *testing.T) {
	fixture := newHandoverFixture(t)
	customs := notGuardedCustoms()
	command := registerHandoverCommand(t)
	command.OnwardNode = "SYN-NODE-SIN-HUB"

	result, err := gatedRegister(t, fixture, customs, command)
	wantOutcome(t, result.Outcome(), err, application.HandoverRegistered)
	if len(customs.queries) != 0 {
		t.Fatalf("没声明交出节点却问了关务：%+v", customs.queries)
	}
	reading := recordedReading(t, result)
	stop, stopped := reading.Stop()
	if reading.Answer() != domain.GateUndecided || !stopped || stop != domain.HandoverReleasingNodeNotDeclared {
		t.Fatalf("读数 = %s %s", reading.Answer(), stop)
	}
	if _, asked := reading.Node(); asked {
		t.Fatalf("没问关务的读数声称问过一份意图")
	}

	// 不经组合根的那一口给出同一份读数：不必问关务时它定得出，与问读数那一侧逐字相同。
	plain := newHandoverFixture(t)
	direct, err := plain.handler.Register(context.Background(), command)
	wantOutcome(t, direct.Outcome(), err, application.HandoverRegistered)
	if !reflect.DeepEqual(recordedReading(t, direct), reading) {
		t.Fatalf("两口给出的读数不同：%+v vs %+v", recordedReading(t, direct), reading)
	}
}

// Covers: AT-TF-238——声明了交出节点、没声明去向：照问，不带去向；读数写明去向未声明，跨关务区域那一道由关务不列。
func TestAHandoverWithoutAnOnwardNodeIsAskedAndSaysSo(t *testing.T) {
	fixture := newHandoverFixture(t)
	customs := notGuardedCustoms()

	result, err := gatedRegister(t, fixture, customs, releasedAtGate(t, ""))
	wantOutcome(t, result.Outcome(), err, application.HandoverRegistered)
	if len(customs.queries) != 1 || customs.queries[0].Onward.String() != "" {
		t.Fatalf("没声明去向时的问询 = %+v", customs.queries)
	}
	reading := recordedReading(t, result)
	if reading.Answer() != domain.GateNotGuarded || !reading.OnwardNotDeclared() {
		t.Fatalf("读数没写明去向未声明：%+v", reading)
	}
}

// Covers: AT-TF-239——拒收与待确认不转出控制，没有交出可读：不问关务，那一版不带读数。
func TestRefusedAndPendingHandoversCarryNoGateReading(t *testing.T) {
	for _, verdict := range []domain.HandoverVerdict{domain.HandoverRefused, domain.HandoverPendingConfirmation} {
		t.Run(verdict.String(), func(t *testing.T) {
			fixture := newHandoverFixture(t)
			customs := notGuardedCustoms()
			command := releasedAtGate(t, "SYN-NODE-SIN-HUB")
			command.Verdict, command.Rule, command.Basis = verdict, "", "SYN-BASIS-SEAL-BROKEN"

			result, err := gatedRegister(t, fixture, customs, command)
			wantOutcome(t, result.Outcome(), err, application.HandoverRegistered)
			if len(customs.queries) != 0 {
				t.Fatalf("%s 问了关务", verdict)
			}
			if record, _ := result.Record(); func() bool { _, read := record.Handover.GateReading(); return read }() {
				t.Fatalf("%s 那一版带了读数", verdict)
			}
		})
	}
}

// Covers: AT-TF-240——关务那一问没形成答复：交接照登，那一件记读不动，调用失败与答复译不回来各记各的依据种类。
func TestAnUnreachableCustomsAnswerIsRecordedAsUnreadable(t *testing.T) {
	for name, tc := range map[string]struct {
		err   error
		cause string
	}{
		"call failed":    {err: errors.New("customs down"), cause: "CUSTOMS_ADMISSION_UNAVAILABLE"},
		"untranslatable": {err: ports.ErrHandoverGateAnswerUntranslatable, cause: "CUSTOMS_ADMISSION_UNTRANSLATABLE"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newHandoverFixture(t)
			result, err := gatedRegister(t, fixture, &customsHandoverGateDouble{err: tc.err}, releasedAtGate(t, "SYN-NODE-SIN-HUB"))
			wantOutcome(t, result.Outcome(), err, application.HandoverRegistered)
			reading := recordedReading(t, result)
			finding, _ := reading.Finding()
			bases := finding.Bases()
			if reading.Answer() != domain.GateUnreadable || len(bases) != 1 || bases[0].Cause != tc.cause {
				t.Fatalf("读数 = %s %+v", reading.Answer(), bases)
			}
			if _, judged := finding.Judgement(); judged {
				t.Fatalf("读不动的那一格带了关务判断标识")
			}
		})
	}
}

// Covers: AT-TF-241——同一交接同内容重投：答已有版本，回显首次读数，不重问关务——此刻关务答得不同也一样；换了内容的那一份
// 照常问、答冲突，在册那一版的读数不动。
func TestAReplayedHandoverEchoesTheFirstReadingWithoutAskingAgain(t *testing.T) {
	fixture := newHandoverFixture(t)
	first := &customsHandoverGateDouble{answer: domain.GateUndecided}
	command := releasedAtGate(t, "SYN-NODE-SIN-HUB")
	registered, err := gatedRegister(t, fixture, first, command)
	wantOutcome(t, registered.Outcome(), err, application.HandoverRegistered)

	later := &customsHandoverGateDouble{answer: domain.GateAdmitted}
	replay, err := gatedRegister(t, fixture, later, command)
	wantOutcome(t, replay.Outcome(), err, application.HandoverExistingVersion)
	if len(later.queries) != 0 {
		t.Fatalf("重投重问了关务：%+v", later.queries)
	}
	if !reflect.DeepEqual(recordedReading(t, replay), recordedReading(t, registered)) {
		t.Fatalf("重投回显的不是首次读数：%+v", recordedReading(t, replay))
	}

	changed := command
	changed.OnwardNode = "SYN-NODE-SIN-GW"
	conflict, err := gatedRegister(t, fixture, later, changed)
	wantOutcome(t, conflict.Outcome(), err, application.HandoverRegistrationConflict)
	if stored, _ := onlyHandoverRecord(t, fixture.registry).Handover.GateReading(); stored.Answer() != domain.GateUndecided {
		t.Fatalf("冲突改了在册那一版的读数：%s", stored.Answer())
	}
}

// Covers: AT-TF-242——更正成`已交接`的那一版带自己的读数：照前版承继的节点与业务时间问关务（前版待确认、本没有读数）；
// 原样重投更正不重问；更正成拒收的那一版不带读数，前版的读数原样留在前版上。
func TestAHandoverCorrectedIntoHandedOverReadsTheGateOnTheInheritedIntent(t *testing.T) {
	fixture := newHandoverFixture(t)
	customs := &customsHandoverGateDouble{answer: domain.GateUndecided}
	pending := releasedAtGate(t, "SYN-NODE-SIN-HUB")
	pending.Verdict, pending.Rule, pending.Basis = domain.HandoverPendingConfirmation, "", "SYN-BASIS-COUNT-PENDING"
	registered, err := gatedRegister(t, fixture, customs, pending)
	wantOutcome(t, registered.Outcome(), err, application.HandoverRegistered)

	confirm := correctHandoverCommand(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")
	confirm.Verdict, confirm.Basis = domain.ObjectHandedOver, ""
	confirm.ReleasingEvidence, confirm.ReceivingEvidence, confirm.Rule = "evidence-release-2", "evidence-receive-2", "handover-rule/v1"
	corrected, err := gatedCorrect(t, fixture, customs, confirm)
	wantOutcome(t, corrected.Outcome(), err, application.HandoverCorrected)
	if len(customs.queries) != 1 {
		t.Fatalf("更正问了关务 %d 次，want 1", len(customs.queries))
	}
	query := customs.queries[0]
	if query.Node.String() != "SYN-NODE-SZX-GATE" || query.Onward.String() != "SYN-NODE-SIN-HUB" || !query.At.Equal(handoverJudgedTime) {
		t.Fatalf("更正问的不是承继来的意图：%+v", query)
	}
	if reading := recordedReading(t, corrected); reading.Answer() != domain.GateUndecided {
		t.Fatalf("更正版读数 = %s", reading.Answer())
	}

	replay, err := gatedCorrect(t, fixture, customs, confirm)
	wantOutcome(t, replay.Outcome(), err, application.HandoverExistingVersion)
	if len(customs.queries) != 1 {
		t.Fatalf("原样重投更正重问了关务")
	}

	refuse := correctHandoverCommand(t, "handover-result/parcel-1/v2", "handover-result/parcel-1/v3")
	refused, err := gatedCorrect(t, fixture, customs, refuse)
	wantOutcome(t, refused.Outcome(), err, application.HandoverCorrected)
	if record, _ := refused.Record(); func() bool { _, read := record.Handover.GateReading(); return read }() {
		t.Fatalf("更正成拒收的那一版带了读数")
	}
	if len(customs.queries) != 1 {
		t.Fatalf("更正成拒收问了关务")
	}
}

// Covers: AT-TF-243——更正在写事务之前读不动前版：不知道该问哪一份意图，读数停在「交接登记册读不动」，写事务里读得动就照登，
// 不因读不动挡下更正。
func TestACorrectionWhosePredecessorCannotBeReadRecordsAnUnreadableGate(t *testing.T) {
	fixture := newHandoverFixture(t)
	customs := notGuardedCustoms()
	registered, err := gatedRegister(t, fixture, customs, releasedAtGate(t, "SYN-NODE-SIN-HUB"))
	wantOutcome(t, registered.Outcome(), err, application.HandoverRegistered)

	confirm := correctHandoverCommand(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")
	confirm.Verdict, confirm.Basis = domain.ObjectHandedOver, ""
	confirm.ReleasingEvidence, confirm.ReceivingEvidence, confirm.Rule = "evidence-release-2", "evidence-receive-2", "handover-rule/v1"

	unreachable := &handoverRegistryDouble{records: fixture.registry.records, findErr: errors.New("pool down")}
	reading, err := handoverGateReader(t, unreachable, customs).ReadCorrection(context.Background(), confirm)
	if err != nil {
		t.Fatalf("读不动前版不该报错：%v", err)
	}
	result, err := fixture.handler.CorrectWithGateReading(context.Background(), confirm, reading)
	wantOutcome(t, result.Outcome(), err, application.HandoverCorrected)
	stored := recordedReading(t, result)
	if stop, _ := stored.Stop(); stored.Answer() != domain.GateUnreadable || stop != domain.HandoverGateRegistryUnavailable {
		t.Fatalf("读数 = %s %s", stored.Answer(), stop)
	}
	if len(customs.queries) != 1 {
		t.Fatalf("读不动前版却问了关务")
	}
}

// Covers: AT-TF-244——读数放错地方是接线错误，上抛而不答一格：要问关务却没带读数、拒收带了读数、读数问的是另一件对象；
// 端口答了另一件对象也上抛，不记成读不动。这几种都不落册。
func TestAMisplacedHandoverGateReadingIsAWiringError(t *testing.T) {
	ctx := context.Background()
	t.Run("not asked", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		if _, err := fixture.handler.Register(ctx, releasedAtGate(t, "")); !errors.Is(err, domain.ErrHandoverGateReadingMisplaced) {
			t.Fatalf("err = %v", err)
		}
		if len(fixture.registry.records) != 0 {
			t.Fatalf("接线错误落了册")
		}
	})
	t.Run("reading on a refusal", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		asked, err := handoverGateReader(t, fixture.registry, notGuardedCustoms()).ReadRegistration(ctx, releasedAtGate(t, ""))
		if err != nil {
			t.Fatalf("读数：%v", err)
		}
		refused := releasedAtGate(t, "")
		refused.Verdict, refused.Rule, refused.Basis = domain.HandoverRefused, "", "SYN-BASIS-SEAL-BROKEN"
		if _, err := fixture.handler.RegisterWithGateReading(ctx, refused, asked); !errors.Is(err, domain.ErrHandoverGateReadingMisplaced) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("reading for another object", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		other := releasedAtGate(t, "")
		other.Object = "parcel-2"
		asked, err := handoverGateReader(t, fixture.registry, notGuardedCustoms()).ReadRegistration(ctx, other)
		if err != nil {
			t.Fatalf("读数：%v", err)
		}
		if _, err := fixture.handler.RegisterWithGateReading(ctx, releasedAtGate(t, ""), asked); !errors.Is(err, domain.ErrHandoverGateReadingMisplaced) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("port answered another object", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		customs := &customsHandoverGateDouble{answer: domain.GateNotGuarded, answerFor: "parcel-9"}
		if _, err := handoverGateReader(t, fixture.registry, customs).ReadRegistration(ctx, releasedAtGate(t, "")); err == nil {
			t.Fatalf("端口答错对象却没报错")
		}
	})
}

// Covers: 构造期拒缺件——漏装的一口不等到第一次交出才演成一串读不动。
func TestTheHandoverGateReaderRefusesMissingPorts(t *testing.T) {
	if _, err := application.NewHandoverGateReader(application.HandoverGateReadDeps{Customs: notGuardedCustoms()}); err == nil {
		t.Fatalf("缺交接登记册也构造成了")
	}
	if _, err := application.NewHandoverGateReader(application.HandoverGateReadDeps{Handovers: newHandoverRegistry()}); err == nil {
		t.Fatalf("缺关务那一口也构造成了")
	}
}
