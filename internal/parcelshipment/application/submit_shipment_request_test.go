package application_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-001 步骤 1、2、3A、3B、3C — 先保全来源再判定归属，且只有本产品对整个准入
// 范围持有生产权威之后才建立委托。
func TestSubmitEstablishesSubmittedRequestAfterPreservingSource(t *testing.T) {
	fixture := newFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("outcome = %q, want SUBMITTED", result.Outcome())
	}
	if got, want := fixture.calls, []string{"preserve-source", "decide-ownership", "insert-request"}; !slices.Equal(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}

	stored, found := fixture.requests.stored(t, fixture.identity(t))
	if !found {
		t.Fatal("no shipment request was stored for the submitted source")
	}
	if stored.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("stored state = %q, want SUBMITTED", stored.State())
	}
	if stored.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("submission completed the acceptance decision task")
	}
	requestID, present := result.ShipmentRequestID()
	if !present || requestID != stored.ShipmentRequestID() {
		t.Fatal("result does not reference the stored request")
	}
}

// Covers: UC-PS-001 结果语义 — 同一逻辑请求的重放返回原结果，而不是造出第二份委托。
//
// Covers: `AT-PS-003`「同一逻辑请求以相同内容重试 → 返回原结果……不创建第二份来源、
// 归属记录或委托」的编排半边。数据库级唯一性（并发重放挤过内存判重时的最后一道）属
// 持久化半边，仍阻断于 Bento 闸门（ADR-0017/0026），此处钉不了也不冒领。
func TestSubmitReplayReturnsExistingResultWithoutASecondRequest(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	fixture.calls = nil

	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}

	if replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("replay outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	replayed, present := replay.ShipmentRequestID()
	original, _ := first.ShipmentRequestID()
	if !present || replayed != original {
		t.Fatal("replay returned a different shipment request")
	}
	if fixture.requests.insertCount != 1 {
		t.Fatalf("insert count = %d, want 1", fixture.requests.insertCount)
	}
	if fixture.ownership.decideCount != 1 {
		t.Fatalf("replay decided ownership again: %d decisions", fixture.ownership.decideCount)
	}
}

// Covers: UC-PS-001 一致性幂等与并发 — occurredAt/receivedAt 不同仍算重放，只追加这次观察；
// 它不重新保全、不重新判定，也不覆盖原始来源事实。
func TestSubmitReplayAppendsTheObservationWithoutOverwriting(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	later := fixture.command(t)
	later.OccurredAt = later.OccurredAt.Add(time.Hour)
	later.ReceivedAt = later.ReceivedAt.Add(time.Hour)

	replay, err := fixture.handler.Handle(ctx, later)
	if err != nil {
		t.Fatalf("later handle: %v", err)
	}

	if replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT; timestamps must not reclassify a replay", replay.Outcome())
	}
	if got := len(fixture.sources.observations); got != 1 {
		t.Fatalf("appended observations = %d, want 1", got)
	}
	observed := fixture.sources.observations[0]
	if !observed.OccurredAt().Equal(later.OccurredAt) || !observed.ReceivedAt().Equal(later.ReceivedAt) {
		t.Fatalf("appended observation carries %v/%v, want the later timestamps", observed.OccurredAt(), observed.ReceivedAt())
	}

	preserved, _ := fixture.sources.stored(t, fixture.identity(t))
	if !preserved.OccurredAt().Equal(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("the observation overwrote the preserved source: %v", preserved.OccurredAt())
	}
	if fixture.sources.preserveCount != 1 {
		t.Fatalf("preserve count = %d, want 1", fixture.sources.preserveCount)
	}
}

// Covers: UC-PS-001 结果语义 — 同一逻辑身份携带不同的规范化载荷是冲突，绝不是第二份委托。
//
// Covers: `AT-PS-004`「同一逻辑请求身份携带不同内容 → 返回冲突或进入受控纠正；原始提交
// 和原决定不被覆盖」——本用例钉「冲突且原件不被覆盖」这一支；受控纠正是或语义的另一支，
// 其接入契约属实例半边（`PAR-INT-01`），不在此冒领。
func TestSubmitConflictPreservesTheOriginalWithoutBuilding(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	changed := fixture.command(t)
	changed.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "digest-2")
	conflict, err := fixture.handler.Handle(ctx, changed)
	if err != nil {
		t.Fatalf("conflicting handle: %v", err)
	}

	if conflict.Outcome() != application.OutcomeIngressConflict {
		t.Fatalf("outcome = %q, want INGRESS_CONFLICT", conflict.Outcome())
	}
	if fixture.requests.insertCount != 1 {
		t.Fatalf("a conflict created a second request: %d inserts", fixture.requests.insertCount)
	}
	preserved, _ := fixture.sources.stored(t, fixture.identity(t))
	if preserved.Digest().String() != "digest-1" {
		t.Fatalf("conflict overwrote the preserved payload: %q", preserved.Digest())
	}
}

// Covers: PBC-04 / ADR-0031 — Insert 答「已存在」不是技术错误。同内容按重放规则答已有结果，
// 且不追加观察：走到这一格时本次已经 Preserve 过自己那一份。
func TestSubmitConcurrentInsertAlreadyExistsReturnsExistingResultWithoutAppendingObservation(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstID, present := first.ShipmentRequestID()
	if !present {
		t.Fatal("first submit did not name a shipment request")
	}
	fixture.calls = nil
	fixture.sources.missPreservedOnce = true

	concurrent, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("concurrent handle: %v", err)
	}
	if concurrent.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", concurrent.Outcome())
	}
	replayed, present := concurrent.ShipmentRequestID()
	if !present || replayed != firstID {
		t.Fatal("concurrent insert conflict did not name the existing request")
	}
	if got := len(fixture.sources.observations); got != 0 {
		t.Fatalf("appended observations = %d, want 0", got)
	}
	if fixture.requests.insertCount != 2 {
		t.Fatalf("insert attempts = %d, want 2", fixture.requests.insertCount)
	}
	if len(fixture.requests.records) != 1 {
		t.Fatalf("stored requests = %d, want 1", len(fixture.requests.records))
	}
}

// Covers: ADR-0031 入口条件 — 「已存在」不是终局的已有结果；内容不同仍答接入冲突。
func TestSubmitConcurrentInsertAlreadyExistsWithDifferentContentIsIngressConflict(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	fixture.sources.missPreservedOnce = true

	changed := fixture.command(t)
	changed.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "digest-2")
	result, err := fixture.handler.Handle(ctx, changed)
	if err != nil {
		t.Fatalf("concurrent handle: %v", err)
	}
	if result.Outcome() != application.OutcomeIngressConflict {
		t.Fatalf("outcome = %q, want INGRESS_CONFLICT", result.Outcome())
	}
	if got := len(fixture.sources.observations); got != 0 {
		t.Fatalf("appended observations = %d, want 0", got)
	}
	preserved, _ := fixture.sources.stored(t, fixture.identity(t))
	if preserved.Digest().String() != "digest-1" {
		t.Fatalf("concurrent preserve overwrote the original: %q", preserved.Digest())
	}
}

// Covers: UC-PS-001 结果语义与首发试点叠加条件 — 三种拒绝彼此分明。暂停回答的是本产品此刻
// 是否接新活，而不是范围归谁，所以它不得被报成归属未决。
//
// Covers: `AT-PS-010`「试点准入规则排除整份委托 → 不建立委托接受或拒绝决定……否则保持
// 生产归属未决」——三分支各断不建单且回报归属决定与闸门理由。真实权威方与交接证据属
// 实例半边（`PAR-GOV-03..07` 待登记），不构成此处的洞。
func TestSubmitFormsNoRequestWhenThisProductLacksAuthority(t *testing.T) {
	cases := map[string]struct {
		authority domain.ProductionAuthorityKind
		control   domain.AdmissionControl
		want      application.SubmitOutcome
	}{
		"other authority":  {domain.ProductionAuthorityOther, domain.AdmissionControlOpen, application.OutcomeOtherProductionAuthority},
		"unresolved":       {domain.ProductionAuthorityUnresolved, domain.AdmissionControlOpen, application.OutcomeOwnershipUnresolved},
		"admission paused": {domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, application.OutcomeAdmissionPaused},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			fixture.ownership.authority = testCase.authority
			fixture.ownership.control = testCase.control

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), testCase.want)
			}
			if _, present := result.ShipmentRequestID(); present {
				t.Fatal("a refusal named a shipment request")
			}
			if decision, present := result.OwnershipDecision(); !present || decision.Authority() != testCase.authority {
				t.Fatal("a refusal did not report the ownership decision behind it")
			}
			if len(result.GateBlockReasons()) == 0 {
				t.Fatal("a refusal reported no gate block reason")
			}
			if fixture.requests.insertCount != 0 {
				t.Fatalf("a request was built without production authority: %d inserts", fixture.requests.insertCount)
			}
			if _, found := fixture.sources.stored(t, fixture.identity(t)); !found {
				t.Fatal("the source was not preserved even though ownership was evaluated")
			}
		})
	}
}

// Covers: UC-PS-001 步骤 3A 先于 3B — 立不起最小委托身份的输入不产生占位委托，也没有值得
// 拿去问权威的准入范围。
func TestSubmitWithoutDeclaredParcelsIsNotAccepted(t *testing.T) {
	fixture := newFixture(t)
	command := fixture.command(t)
	command.DeclaredParcelIDs = nil

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.OutcomeInputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
	if fixture.ownership.decideCount != 0 {
		t.Fatal("ownership was decided for input that established no request identity")
	}
	if fixture.requests.insertCount != 0 {
		t.Fatalf("a placeholder request was created: %d inserts", fixture.requests.insertCount)
	}
	if _, found := fixture.sources.stored(t, fixture.identity(t)); !found {
		t.Fatal("unusable input discarded the preserved source")
	}
}

// Covers: UC-PS-001 — 一次查询不得泄露某个对象存在于另一个租户或客户范围里。
func TestSubmitIsolatesScopesSharingASourceRequestKey(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	neighbour := fixture.command(t)
	neighbour.Identity = sourceIdentity(t, "tenant-2", "customer-1", "source-a", "key-1")
	neighbour.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "request-2")

	result, err := fixture.handler.Handle(ctx, neighbour)
	if err != nil {
		t.Fatalf("neighbour handle: %v", err)
	}

	if result.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("neighbour outcome = %q, want SUBMITTED", result.Outcome())
	}
	if fixture.requests.insertCount != 2 {
		t.Fatalf("insert count = %d, want 2 independent requests", fixture.requests.insertCount)
	}
}

// Covers: 本切片不得越过的边界 — 仓储失败不得被悄悄吞成一个业务结果。
func TestSubmitSurfacesPreservationFailureRatherThanDeciding(t *testing.T) {
	fixture := newFixture(t)
	failure := errors.New("preservation unavailable")
	fixture.sources.preserveErr = failure

	_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the preservation failure", err)
	}
	if fixture.ownership.decideCount != 0 {
		t.Fatal("ownership was decided after the source failed to be preserved")
	}
}

type fixture struct {
	handler   *application.SubmitShipmentRequestHandler
	sources   *sourceRepositoryDouble
	requests  *shipmentRequestRepositoryDouble
	ownership *ownershipAuthorityDouble
	calls     []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	value := &fixture{}
	record := func(name string) { value.calls = append(value.calls, name) }

	value.sources = &sourceRepositoryDouble{records: map[domain.SourceIdentity]domain.SourceSubmissionFingerprint{}, record: record}
	value.requests = &shipmentRequestRepositoryDouble{records: map[domain.SourceIdentity]domain.ShipmentRequest{}, record: record}
	value.ownership = &ownershipAuthorityDouble{
		t:         t,
		authority: domain.ProductionAuthorityIDPParcel,
		control:   domain.AdmissionControlOpen,
		record:    record,
	}
	value.handler = application.NewSubmitShipmentRequestHandler(
		value.sources,
		value.requests,
		value.ownership,
		&identityFactoryDouble{},
		fixedClock{at: time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)},
	)
	return value
}

func (value *fixture) identity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	return sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")
}

func (value *fixture) command(t *testing.T) application.SubmitShipmentRequestCommand {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("new admission scope: %v", err)
	}
	return application.SubmitShipmentRequestCommand{
		Identity:          value.identity(t),
		PayloadDigest:     mustValue(t, domain.NewPayloadDigest, "digest-1"),
		OccurredAt:        time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
		ReceivedAt:        time.Date(2026, 8, 7, 10, 0, 1, 0, time.UTC),
		BatchID:           mustValue(t, domain.NewSubmissionBatchID, "batch-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-1")},
		AdmissionScope:    scope,
		ExpectedRevision:  mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
	}
}

// rejectPrior 让夹具里已建的委托真经领域越过拒绝边界（先例：supplementableRequestStore）：
// 假状态挡不住 EstablishPriorRequestLink 的互证，也证明不了编排读回的是那份终态。
func (value *fixture) rejectPrior(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	prior, found := value.requests.stored(t, value.identity(t))
	if !found {
		t.Fatal("no prior request to reject")
	}
	rejected, err := prior.RejectByAuthority(domain.ActiveRejectionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-0"),
		Authority:  mustValue(t, domain.NewRejectionAuthorityReference, "PC-REJECT-ROLE-0"),
		Decider:    mustValue(t, domain.NewDeciderReference, "OPERATOR-0"),
		Reason:     mustValue(t, domain.NewRejectionReasonReference, "EARLIER_DECISION"),
		Evidence:   mustValue(t, domain.NewRejectionEvidenceReference, "EVID-0"),
		DecidedAt:  time.Date(2026, 8, 7, 12, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("reject prior: %v", err)
	}
	value.requests.records[value.identity(t)] = rejected
	return rejected
}

// linkedCommand 造第二份提交：新来源身份、新委托编号，指名夹具里那份原委托。
func (value *fixture) linkedCommand(t *testing.T, kind domain.RequestLinkKind) application.SubmitShipmentRequestCommand {
	t.Helper()
	command := value.command(t)
	command.Identity = sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-2")
	command.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "request-2")
	command.Link = application.PriorRequestClaim{
		PriorIdentity:  value.identity(t),
		PriorRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		Kind:           kind,
	}
	return command
}

// Covers: `AT-PS-036`②「已拒绝委托修正资料 → 关联新委托，保留原决定」与 `AT-PS-076`
// 的建立半边——关联新委托走完整提交管线出生，出处指回原委托；原委托的状态与决定原样
// 留在库里，没有被这次关联改写。
func TestALinkedSubmissionIsBornCarryingItsProvenance(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("submit prior: %v", err)
	}
	fixture.rejectPrior(t)

	result, err := fixture.handler.Handle(ctx, fixture.linkedCommand(t, domain.LinkRejectedCorrection))
	if err != nil {
		t.Fatalf("submit linked: %v", err)
	}

	if result.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("outcome = %q, want SUBMITTED", result.Outcome())
	}
	linked, found := fixture.requests.stored(t, sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-2"))
	if !found {
		t.Fatal("no linked request was stored")
	}
	link, present := linked.PriorRequestLink()
	if !present || link.PriorRequestID().String() != "request-1" || link.Kind() != domain.LinkRejectedCorrection {
		t.Fatalf("link = %#v present = %v; 出处必须指回原委托并声明方向", link, present)
	}
	prior, _ := fixture.requests.stored(t, fixture.identity(t))
	if prior.State() != domain.ShipmentRequestRejected {
		t.Fatalf("prior state = %q; 建立关联改写了原委托", prior.State())
	}
}

// Covers: 关联指名的统一不可见——查无此委托、编号不符、跨客户指名同一个答案`查无原委托`
// （`AT-PS-075` 的纪律在关联指名上一字不差）；话没说全另归输入未受理。四种情况都不建委托。
func TestAClaimOnAnInvisiblePriorIsUniformlyNotFound(t *testing.T) {
	ctx := context.Background()

	t.Run("never submitted", func(t *testing.T) {
		fixture := newFixture(t)
		result, err := fixture.handler.Handle(ctx, fixture.linkedCommand(t, domain.LinkRejectedCorrection))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.OutcomePriorRequestNotFound {
			t.Fatalf("outcome = %q, want PRIOR_REQUEST_NOT_FOUND", result.Outcome())
		}
		if fixture.requests.insertCount != 0 {
			t.Fatal("指名立不住还建了委托")
		}
	})

	t.Run("request id mismatch", func(t *testing.T) {
		fixture := newFixture(t)
		if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
			t.Fatalf("submit prior: %v", err)
		}
		fixture.rejectPrior(t)
		command := fixture.linkedCommand(t, domain.LinkRejectedCorrection)
		command.Link.PriorRequestID = mustValue(t, domain.NewShipmentRequestID, "request-9")

		result, err := fixture.handler.Handle(ctx, command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.OutcomePriorRequestNotFound {
			t.Fatalf("outcome = %q, want PRIOR_REQUEST_NOT_FOUND", result.Outcome())
		}
	})

	t.Run("another customer's prior", func(t *testing.T) {
		fixture := newFixture(t)
		if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
			t.Fatalf("submit prior: %v", err)
		}
		fixture.rejectPrior(t)
		command := fixture.linkedCommand(t, domain.LinkRejectedCorrection)
		command.Identity = sourceIdentity(t, "tenant-1", "customer-2", "source-a", "key-2")

		result, err := fixture.handler.Handle(ctx, command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.OutcomePriorRequestNotFound {
			t.Fatalf("outcome = %q, want PRIOR_REQUEST_NOT_FOUND；跨客户指名与查无必须同答", result.Outcome())
		}
	})

	t.Run("half a claim", func(t *testing.T) {
		fixture := newFixture(t)
		command := fixture.linkedCommand(t, domain.LinkRejectedCorrection)
		command.Link.PriorIdentity = domain.SourceIdentity{}

		result, err := fixture.handler.Handle(ctx, command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.OutcomeInputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
		}
	})
}

// Covers: 方向与原委托终态不符时关联不适用——待决委托没有方向可用（普通纠错走同一委托
// 的新提交版本，BD-PS-005），答复与「查无」分格，客户才知道该走哪条路。
func TestAClaimAgainstAPendingPriorIsIneligible(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("submit prior: %v", err)
	}

	result, err := fixture.handler.Handle(ctx, fixture.linkedCommand(t, domain.LinkRejectedCorrection))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.OutcomeLinkIneligible {
		t.Fatalf("outcome = %q, want LINK_INELIGIBLE", result.Outcome())
	}
	if fixture.requests.insertCount != 1 {
		t.Fatalf("insert count = %d, want 1（只有原委托那一次）", fixture.requests.insertCount)
	}
	if fixture.ownership.decideCount != 1 {
		t.Fatalf("decide count = %d; 指名立不住不该消耗准入决定", fixture.ownership.decideCount)
	}
}

type sourceRepositoryDouble struct {
	records           map[domain.SourceIdentity]domain.SourceSubmissionFingerprint
	observations      []domain.SourceSubmissionFingerprint
	record            func(string)
	preserveErr       error
	preserveCount     int
	missPreservedOnce bool
}

func (double *sourceRepositoryDouble) FindPreserved(
	_ context.Context,
	identity domain.SourceIdentity,
) (domain.SourceSubmissionFingerprint, bool, error) {
	// 模拟并发下「读保全」尚未看见另一方刚写上的指纹，好让编排走到 Insert 的已存在分支。
	if double.missPreservedOnce {
		double.missPreservedOnce = false
		return domain.SourceSubmissionFingerprint{}, false, nil
	}
	existing, found := double.records[identity]
	return existing, found, nil
}

func (double *sourceRepositoryDouble) Preserve(
	_ context.Context,
	submission domain.SourceSubmissionFingerprint,
) error {
	double.record("preserve-source")
	if double.preserveErr != nil {
		return double.preserveErr
	}
	double.preserveCount++
	// 先到先得：已保全的来源事实不可被并发后到者覆盖。
	if _, found := double.records[submission.Identity()]; found {
		return nil
	}
	double.records[submission.Identity()] = submission
	return nil
}

func (double *sourceRepositoryDouble) AppendObservation(
	_ context.Context,
	observed domain.SourceSubmissionFingerprint,
) error {
	double.record("append-observation")
	double.observations = append(double.observations, observed)
	return nil
}

func (double *sourceRepositoryDouble) stored(
	t *testing.T,
	identity domain.SourceIdentity,
) (domain.SourceSubmissionFingerprint, bool) {
	t.Helper()
	existing, found := double.records[identity]
	return existing, found
}

type shipmentRequestRepositoryDouble struct {
	records     map[domain.SourceIdentity]domain.ShipmentRequest
	record      func(string)
	insertCount int
}

func (double *shipmentRequestRepositoryDouble) FindBySourceIdentity(
	_ context.Context,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	existing, found := double.records[identity]
	return existing, found, nil
}

func (double *shipmentRequestRepositoryDouble) Insert(
	_ context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	double.record("insert-request")
	double.insertCount++
	if _, found := double.records[identity]; found {
		return ports.ShipmentRequestAlreadyExists, nil
	}
	double.records[identity] = request
	return ports.ShipmentRequestInserted, nil
}

func (double *shipmentRequestRepositoryDouble) Save(
	_ context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	double.records[identity] = request
	return ports.ShipmentRequestSaved, nil
}

func (double *shipmentRequestRepositoryDouble) stored(
	t *testing.T,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool) {
	t.Helper()
	existing, found := double.records[identity]
	return existing, found
}

type ownershipAuthorityDouble struct {
	t           *testing.T
	authority   domain.ProductionAuthorityKind
	control     domain.AdmissionControl
	record      func(string)
	decideCount int
}

func (double *ownershipAuthorityDouble) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	double.t.Helper()
	double.record("decide-ownership")
	double.decideCount++

	spec := domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustValue(double.t, domain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        double.authority,
		AdmissionControl: double.control,
		RuleVersion:      mustValue(double.t, domain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
		Validity:         validity(double.t),
		Revision:         mustValue(double.t, domain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
	}
	switch double.authority {
	case domain.ProductionAuthorityOther:
		spec.OtherAuthorityRef = mustValue(double.t, domain.NewProductionAuthorityReference, "other-authority-1")
		spec.HandoffRef = mustValue(double.t, domain.NewHandoffConfirmationReference, "handoff-1")
	case domain.ProductionAuthorityUnresolved:
		spec.UnresolvedReason = domain.OwnershipUnresolvedAuthorityNotUnique
		spec.ContinuationRef = mustValue(double.t, domain.NewOwnershipContinuationReference, "continue-1")
	}
	if double.control == domain.AdmissionControlPaused {
		spec.SuspensionRef = mustValue(double.t, domain.NewOwnershipSuspensionReference, "suspend-1")
	}

	decision, err := domain.NewProductionOwnershipDecision(spec)
	if err != nil {
		double.t.Fatalf("new ownership decision: %v", err)
	}
	return decision, nil
}

type identityFactoryDouble struct {
	versions int
	tasks    int
}

func (double *identityFactoryDouble) NextSubmissionVersionID(_ context.Context) (domain.SubmissionVersionID, error) {
	double.versions++
	return domain.NewSubmissionVersionID("version-" + strconv.Itoa(double.versions))
}

func (double *identityFactoryDouble) NextAcceptanceDecisionTaskID(_ context.Context) (domain.AcceptanceDecisionTaskID, error) {
	double.tasks++
	return domain.NewAcceptanceDecisionTaskID("task-" + strconv.Itoa(double.tasks))
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

var (
	_ ports.SourceSubmissionRepository   = (*sourceRepositoryDouble)(nil)
	_ ports.ShipmentRequestRepository    = (*shipmentRequestRepositoryDouble)(nil)
	_ ports.ProductionOwnershipAuthority = (*ownershipAuthorityDouble)(nil)
	_ ports.SubmissionIdentityFactory    = (*identityFactoryDouble)(nil)
	_ ports.Clock                        = fixedClock{}
)
