package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var errStoreDown = errors.New("store down")

func unconfiguredHandoff(ports.ProductionHandoffDelivery) ports.ProductionHandoffObservation {
	return ports.ProductionHandoffObservation{Observation: domain.HandoffObservationChannelUnconfigured}
}

// Covers: `AT-PS-111`——来源保全已落、建单那一笔没落成（答 5xx），原样重发要接着把单建起来，答`已提交`，且只建一份；不能因为
// 来源已保全就答一个不指名委托的`已有结果`（ADR-0256 决定一第 2 格「重发必须接着办」）。
func TestAReplayAfterAFailedBuildBuildsTheRequestOnce(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	fixture.requests.failNextInsert = errStoreDown

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); !errors.Is(err, errStoreDown) {
		t.Fatalf("first handle err = %v, want the failed insert surfaced", err)
	}
	if _, found := fixture.requests.stored(t, fixture.identity(t)); found {
		t.Fatal("夹具前提不成立：建单那一笔失败了，委托却在")
	}

	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("replay outcome = %q, want SUBMITTED——这次重发才真把单建起来", replay.Outcome())
	}
	stored, found := fixture.requests.stored(t, fixture.identity(t))
	if !found {
		t.Fatal("重发之后仍没有委托")
	}
	if named, _ := replay.ShipmentRequestID(); named != stored.ShipmentRequestID() {
		t.Fatal("重发的答复没有指名建成的那份委托")
	}
	if len(fixture.requests.records) != 1 || fixture.sources.preserveCount != 1 {
		t.Fatalf("requests = %d, preserves = %d, want 一份委托、一次保全", len(fixture.requests.records), fixture.sources.preserveCount)
	}
	if got := len(fixture.sources.observations); got != 1 {
		t.Fatalf("observations = %d, want 1——重发仍是同一请求的又一次观察", got)
	}

	again, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || again.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("third handle outcome = %q, err = %v, want EXISTING_RESULT——建成之后再发才是已有结果", again.Outcome(), err)
	}
	if fixture.requests.insertCount != 2 {
		t.Fatalf("insert attempts = %d, want 2", fixture.requests.insertCount)
	}
}

// Covers: `AT-PS-111`——判归属的依赖答不出（保全已落、什么判断都没留下），原样重发重新判归属并照常建单。
func TestAReplayAfterTheOwnershipDependencyFailedDecidesOwnershipAgain(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	fixture.ownership.failNext = errStoreDown

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); !errors.Is(err, errStoreDown) {
		t.Fatalf("first handle err = %v, want the ownership failure surfaced", err)
	}
	if got := fixture.answers.recordedFor(fixture.identity(t)); got != 0 {
		t.Fatalf("recorded answers = %d, want 0——依赖答不出时没有答案可记", got)
	}

	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("replay outcome = %q, want SUBMITTED", replay.Outcome())
	}
	if fixture.ownership.decideCount != 2 {
		t.Fatalf("decide count = %d, want 2——重发要重新判归属", fixture.ownership.decideCount)
	}
}

// Covers: `AT-PS-112`——首次答`生产归属未决`，原样重发重判归属（UC-PS-001 未决结束「可安全续办」）：这一次放行就照常建单，
// 两次判断都留下，建单答复带的是建单所据的那一次。
func TestAReplayAfterOwnershipUnresolvedDecidesAgainAndBuilds(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	fixture.ownership.authority = domain.ProductionAuthorityUnresolved

	first, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || first.Outcome() != application.OutcomeOwnershipUnresolved {
		t.Fatalf("first outcome = %q, err = %v, want OWNERSHIP_UNRESOLVED", first.Outcome(), err)
	}

	fixture.ownership.authority = domain.ProductionAuthorityIDPParcel
	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("replay outcome = %q, want SUBMITTED——归属这次判得出，就照常建单", replay.Outcome())
	}
	decision, carried := replay.OwnershipDecision()
	if !carried || decision.DecisionID().String() != "decision-2" {
		t.Fatalf("replay ownership = %v (carried %v), want decision-2", decision.DecisionID(), carried)
	}
	if got := fixture.answers.recordedFor(fixture.identity(t)); got != 2 {
		t.Fatalf("recorded answers = %d, want 2——未决那一次也要留下", got)
	}
	if _, found := fixture.requests.stored(t, fixture.identity(t)); !found {
		t.Fatal("重判放行之后没有建单")
	}
}

// Covers: `AT-PS-112`——重判仍判不出，就仍答`生产归属未决`，带这一次的决定，不建单。
func TestAReplayStillUnresolvedAnswersUnresolvedAgain(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	fixture.ownership.authority = domain.ProductionAuthorityUnresolved

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.OutcomeOwnershipUnresolved {
		t.Fatalf("replay outcome = %q, want OWNERSHIP_UNRESOLVED", replay.Outcome())
	}
	decision, carried := replay.OwnershipDecision()
	if !carried || decision.DecisionID().String() != "decision-2" {
		t.Fatalf("replay ownership = %v (carried %v), want 这一次重判的 decision-2", decision.DecisionID(), carried)
	}
	if fixture.requests.insertCount != 0 {
		t.Fatalf("insert attempts = %d, want 0", fixture.requests.insertCount)
	}
}

// Covers: `AT-PS-113`——首次答`准入暂停`，原样重发答`已有结果`并带原归属结果与门禁原因，不重判：暂停原因解除也不自动恢复
// （UC-PS-001「原因解除……都不自动恢复」），这份请求不因重发就越过当时的暂停建单。
func TestAReplayAfterAdmissionPausedAnswersTheOriginalWithoutDeciding(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	fixture.ownership.control = domain.AdmissionControlPaused

	first, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || first.Outcome() != application.OutcomeAdmissionPaused {
		t.Fatalf("first outcome = %q, err = %v, want ADMISSION_PAUSED", first.Outcome(), err)
	}

	fixture.ownership.control = domain.AdmissionControlOpen
	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("replay outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	decision, carried := replay.OwnershipDecision()
	if !carried || decision.DecisionID().String() != "decision-1" || decision.AdmissionControl() != domain.AdmissionControlPaused {
		t.Fatalf("replay ownership = %v / %v (carried %v), want the original paused decision-1",
			decision.DecisionID(), decision.AdmissionControl(), carried)
	}
	if got := replay.GateBlockReasons(); !slices.Equal(got, first.GateBlockReasons()) {
		t.Fatalf("replay gate reasons = %v, want the original %v", got, first.GateBlockReasons())
	}
	if fixture.ownership.decideCount != 1 {
		t.Fatalf("decide count = %d, want 1——暂停之后的重发不重判", fixture.ownership.decideCount)
	}
	if _, named := replay.ShipmentRequestID(); named || fixture.requests.insertCount != 0 {
		t.Fatal("暂停之后的重发建了单")
	}
}

// Covers: `AT-PS-113`——首次已安全交给其他权威（`非本产品生产归属`），原样重发答`已有结果`并带原归属结果，不重判、不再投递：
// 范围已交出去，再投一次就是同一范围投给两个生产系统的开端（UC-PS-001「不得同时投递多个生产系统」）。
func TestAReplayAfterAConfirmedHandoffAnswersTheOriginalWithoutDeliveringAgain(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	fixture.ownership.authority = domain.ProductionAuthorityOther

	first, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || first.Outcome() != application.OutcomeOtherProductionAuthority {
		t.Fatalf("first outcome = %q, err = %v, want OTHER_PRODUCTION_AUTHORITY", first.Outcome(), err)
	}

	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("replay outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	decision, carried := replay.OwnershipDecision()
	if !carried || decision.DecisionID().String() != "decision-1" {
		t.Fatalf("replay ownership = %v (carried %v), want the original decision-1", decision.DecisionID(), carried)
	}
	if assessment, recorded := decision.SafeHandoff(); !recorded || assessment.Status() != domain.SafeHandoffConfirmed {
		t.Fatal("原归属结果丢了交接确认")
	}
	if len(fixture.handoff.deliveries) != 1 || fixture.ownership.decideCount != 1 {
		t.Fatalf("deliveries = %d, decisions = %d, want 各 1", len(fixture.handoff.deliveries), fixture.ownership.decideCount)
	}
}

// Covers: `AT-PS-114`——交接未确认的`其他权威`答的是`生产归属未决`，重发却不重判归属：按记着的那次尝试重投、重查（ADR-0266
// 决定二）。其间登记册翻成本产品也不在本地建单；确认了答`非本产品生产归属`，此后答`已有结果`。
func TestAReplayAfterAnUnconfirmedHandoffRedeliversTheSameAttempt(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	fixture.ownership.authority = domain.ProductionAuthorityOther
	fixture.handoff.observe = unconfiguredHandoff

	first, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || first.Outcome() != application.OutcomeOwnershipUnresolved {
		t.Fatalf("first outcome = %q, err = %v, want OWNERSHIP_UNRESOLVED", first.Outcome(), err)
	}
	fixture.ownership.authority = domain.ProductionAuthorityIDPParcel
	pending, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || pending.Outcome() != application.OutcomeOwnershipUnresolved {
		t.Fatalf("second outcome = %q, err = %v, want OWNERSHIP_UNRESOLVED", pending.Outcome(), err)
	}
	fixture.handoff.observe = nil
	confirmed, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || confirmed.Outcome() != application.OutcomeOtherProductionAuthority {
		t.Fatalf("third outcome = %q, err = %v, want OTHER_PRODUCTION_AUTHORITY", confirmed.Outcome(), err)
	}
	settled, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || settled.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("fourth outcome = %q, err = %v, want EXISTING_RESULT", settled.Outcome(), err)
	}
	if prior, _ := settled.PriorOutcome(); prior != application.OutcomeOtherProductionAuthority {
		t.Fatalf("prior outcome = %q, want OTHER_PRODUCTION_AUTHORITY", prior)
	}
	if fixture.ownership.decideCount != 1 || fixture.requests.insertCount != 0 {
		t.Fatalf("decisions = %d, inserts = %d, want 1 与 0", fixture.ownership.decideCount, fixture.requests.insertCount)
	}
	deliveries := fixture.handoff.deliveries
	if len(deliveries) != 3 || deliveries[0].AttemptID != deliveries[1].AttemptID || deliveries[1].AttemptID != deliveries[2].AttemptID {
		t.Fatalf("deliveries = %d，尝试身份须同一，want 同一次尝试投三回", len(deliveries))
	}
}

// Covers: `AT-PS-113`——取得放行之前就答完的拒绝（输入未受理、原委托不可见、关联不适用）也留下，原样重发答`已有结果`并带原结果，
// 不再判（ADR-0081 Consequences「重放一份曾被业务拒绝的输入答`已有结果`」；ADR-0266 决定一）。
func TestAReplayOfARefusalAnswersTheOriginalWithoutJudging(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		arrange func(*testing.T, *fixture) application.SubmitShipmentRequestCommand
		want    application.SubmitOutcome
	}{
		"input not accepted": {
			arrange: func(t *testing.T, value *fixture) application.SubmitShipmentRequestCommand {
				command := value.command(t)
				command.DeclaredParcelIDs = nil
				return command
			},
			want: application.OutcomeInputNotAccepted,
		},
		"prior request not found": {
			arrange: func(t *testing.T, value *fixture) application.SubmitShipmentRequestCommand {
				command := value.command(t)
				command.Identity = sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-linked")
				command.Link = application.PriorRequestClaim{
					PriorIdentity:  value.identity(t),
					PriorRequestID: mustValue(t, domain.NewShipmentRequestID, "request-absent"),
					Kind:           domain.LinkRejectedCorrection,
				}
				return command
			},
			want: application.OutcomePriorRequestNotFound,
		},
		"link ineligible": {
			arrange: func(t *testing.T, value *fixture) application.SubmitShipmentRequestCommand {
				if _, err := value.handler.Handle(ctx, value.command(t)); err != nil {
					t.Fatalf("submit the pending prior: %v", err)
				}
				return value.linkedCommand(t, domain.LinkRejectedCorrection)
			},
			want: application.OutcomeLinkIneligible,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			command := test.arrange(t, fixture)
			decisionsBefore := fixture.ownership.decideCount

			first, err := fixture.handler.Handle(ctx, command)
			if err != nil || first.Outcome() != test.want {
				t.Fatalf("first outcome = %q, err = %v, want %q", first.Outcome(), err, test.want)
			}
			replay, err := fixture.handler.Handle(ctx, command)
			if err != nil || replay.Outcome() != application.OutcomeExistingResult {
				t.Fatalf("replay outcome = %q, err = %v, want EXISTING_RESULT", replay.Outcome(), err)
			}
			if prior, present := replay.PriorOutcome(); !present || prior != test.want {
				t.Fatalf("prior outcome = %q（%v），want %q", prior, present, test.want)
			}
			if _, named := replay.ShipmentRequestID(); named {
				t.Fatal("拒绝的重发指名了委托")
			}
			if got := fixture.ownership.decideCount - decisionsBefore; got != 0 {
				t.Fatalf("拒绝的提交与它的重发判了 %d 次归属, want 0", got)
			}
		})
	}
}

// Covers: `AT-PS-112`——重判按这一次的结果作答：上一次未决、这一次判成暂停，就答`准入暂停`，不建单。
func TestAReplayAfterOwnershipUnresolvedAnswersWhatThisJudgmentSays(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	fixture.ownership.authority = domain.ProductionAuthorityUnresolved
	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	fixture.ownership.authority = domain.ProductionAuthorityIDPParcel
	fixture.ownership.control = domain.AdmissionControlPaused
	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil || replay.Outcome() != application.OutcomeAdmissionPaused {
		t.Fatalf("replay outcome = %q, err = %v, want ADMISSION_PAUSED", replay.Outcome(), err)
	}
	if fixture.requests.insertCount != 0 {
		t.Fatal("暂停的重判建了单")
	}
}

// Covers: 判得的归属答案没记下就没有形成答案（ADR-0256 决定一：提交编排保全之后出错答 5xx）。挡下的答案不记就答，重发会把一次
// 暂停重判成放行。
func TestAnOwnershipAnswerThatCannotBeRecordedFormsNoAnswer(t *testing.T) {
	fixture := newFixture(t)
	fixture.ownership.control = domain.AdmissionControlPaused
	fixture.answers.err = errStoreDown

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); !errors.Is(err, errStoreDown) {
		t.Fatalf("err = %v, want the record failure surfaced", err)
	}
	if fixture.requests.insertCount != 0 {
		t.Fatal("答案没记下却建了单")
	}
}

// Covers: `AT-PS-103` 一路带上原归属结果——并发下建单撞「已存在」，答复带的是先建成那一份所据的归属答案，不是自己这一次的。
func TestAConcurrentInsertConflictCarriesTheWinnersOwnership(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	// 接单入口每次现签一组新标识，后到的那一次带的是另一个委托号。
	fixture.sources.missPreservedOnce = true
	fixture.requests.missesOnFind = 2
	late := fixture.command(t)
	late.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "request-late")
	concurrent, err := fixture.handler.Handle(ctx, late)
	if err != nil || concurrent.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("outcome = %q, err = %v, want EXISTING_RESULT", concurrent.Outcome(), err)
	}
	decision, carried := concurrent.OwnershipDecision()
	if !carried || decision.DecisionID().String() != "decision-1" {
		t.Fatalf("ownership = %v (carried %v), want 先建成那一份的 decision-1", decision.DecisionID(), carried)
	}
}
