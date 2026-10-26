package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var cancelRequestedAt = time.Date(2026, 8, 9, 6, 0, 0, 0, time.UTC)

type cancellationAuthorityDouble struct {
	judgment   ports.CancellationAuthorityJudgment
	configured bool
	err        error
}

func (double *cancellationAuthorityDouble) JudgeCancellationAuthority(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.CancellationRequesterReference,
	_ domain.DeclaredParcelID,
) (ports.CancellationAuthorityJudgment, bool, error) {
	if double.err != nil {
		return ports.CancellationAuthorityJudgment{}, false, double.err
	}
	return double.judgment, double.configured, nil
}

type cancellationStoreDouble struct {
	byKey   map[ports.CancellationRequestKey]ports.CancellationRecord
	saved   int
	lookups int
}

func newCancellationStore() *cancellationStoreDouble {
	return &cancellationStoreDouble{byKey: map[ports.CancellationRequestKey]ports.CancellationRecord{}}
}

func (double *cancellationStoreDouble) FindByKey(
	_ context.Context,
	key ports.CancellationRequestKey,
) (ports.CancellationRecord, bool, error) {
	double.lookups++
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *cancellationStoreDouble) Save(
	_ context.Context,
	record ports.CancellationRecord,
) (ports.CancellationSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.CancellationAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	double.saved++
	return ports.CancellationSaved, nil
}

type cancellationDownstreamDouble struct {
	intents []ports.ParcelCancellationHandoffIntent
	err     error
}

func (double *cancellationDownstreamDouble) HandOffParcelCancellation(
	_ context.Context,
	intent ports.ParcelCancellationHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type cancellationIdentityDouble struct{ next int }

func (double *cancellationIdentityDouble) NextParcelCancellationID(
	_ context.Context,
) (domain.ParcelCancellationID, error) {
	double.next++
	return domain.NewParcelCancellationID("cancel-" + string(rune('0'+double.next)))
}

// carrierPickupBoundaryDouble 是面单渠道服务首次有效收寄链尾读口的替身：零值答没有。
type carrierPickupBoundaryDouble struct {
	boundary ports.CarrierPickupBoundary
	err      error
	asked    int
}

func (double *carrierPickupBoundaryDouble) FindCurrentCarrierPickup(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DeclaredParcelID,
) (ports.CarrierPickupBoundary, error) {
	double.asked++
	if double.err != nil {
		return ports.CarrierPickupBoundary{}, double.err
	}
	if double.boundary.State == ports.CarrierPickupBoundaryStateInvalid {
		return ports.CarrierPickupBoundary{State: ports.CarrierPickupBoundaryAbsent}, nil
	}
	return double.boundary, nil
}

type cancelFixture struct {
	handler       *application.CancelParcelHandler
	deps          application.CancelParcelDeps
	lock          *boundaryLockDouble
	authority     *cancellationAuthorityDouble
	adoptions     *adoptionStoreDouble
	finals        *currentFinalViewDouble
	cancellations *cancellationViewDouble
	pickups       *carrierPickupBoundaryDouble
	store         *cancellationStoreDouble
	downstream    *cancellationDownstreamDouble
}

func newCancelFixture(t *testing.T) *cancelFixture {
	t.Helper()
	requests := &shipmentRequestRepositoryDouble{
		records: map[domain.SourceIdentity]domain.ShipmentRequest{},
		record:  func(string) {},
	}
	requests.records[sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")] = acceptedRequest(t)
	fixture := &cancelFixture{
		authority: &cancellationAuthorityDouble{
			judgment:   ports.CancellationAuthorityJudgment{Granted: true, Basis: mustValue(t, domain.NewCheckReason, "CANCEL-RULE/PC-17")},
			configured: true,
		},
		adoptions:     newAdoptionStore(),
		finals:        newCurrentFinalViewDouble(),
		cancellations: &cancellationViewDouble{},
		pickups:       &carrierPickupBoundaryDouble{},
		store:         newCancellationStore(),
		downstream:    &cancellationDownstreamDouble{},
		lock:          &boundaryLockDouble{},
	}
	fixture.deps = application.CancelParcelDeps{
		Requests:       requests,
		Authority:      fixture.authority,
		Adoptions:      fixture.adoptions,
		Finals:         fixture.finals,
		Cancellations:  fixture.cancellations,
		CarrierPickups: fixture.pickups,
		Store:          fixture.store,
		Identities:     &cancellationIdentityDouble{},
		Downstream:     fixture.downstream,
		Clock:          fixedClock{at: cancelRequestedAt.Add(time.Minute)},
		BoundaryLock:   fixture.lock,
	}
	fixture.handler = application.NewCancelParcelHandler(fixture.deps)
	return fixture
}

// formedFinal 真经领域构造一份当前有效终局。
func formedFinal(t *testing.T, parcel, version string) ports.FinalOutcomeRecord {
	t.Helper()
	source, err := domain.NewResponsibilityOutcome(domain.ResponsibilityOutcomeSpec{
		Kind:       domain.LabelServiceOutcome,
		Parcel:     mustValue(t, domain.NewDeclaredParcelID, parcel),
		Decision:   mustValue(t, domain.NewResponsibilityDecisionReference, "LABEL-SERVICE-FINAL/"+parcel),
		Execution:  mustValue(t, domain.NewExecutionEvidenceReference, "TF-PICKUP-7@v1"),
		Version:    mustValue(t, domain.NewResponsibilityOutcomeVersion, "v1"),
		OccurredAt: cancelRequestedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("responsibility outcome: %v", err)
	}
	final, err := domain.FormParcelFinalOutcome(domain.ParcelFinalOutcomeSpec{
		Version:     mustValue(t, domain.NewFinalOutcomeVersionID, version),
		Parcel:      mustValue(t, domain.NewDeclaredParcelID, parcel),
		Kind:        mustValue(t, domain.NewFinalKindReference, "LABEL_SERVICE_FINAL"),
		Source:      source,
		RuleVersion: mustValue(t, domain.NewFinalRuleVersionReference, "FINAL-RULE/v1"),
	})
	if err != nil {
		t.Fatalf("form final: %v", err)
	}
	return ports.FinalOutcomeRecord{Finalized: true, Final: final}
}

func formedCarrierPickupBoundary(t *testing.T) ports.CarrierPickupBoundary {
	t.Helper()
	return ports.CarrierPickupBoundary{
		State: ports.CarrierPickupBoundaryFormed,
		Pickup: domain.CarrierFirstEffectivePickupSpec{
			Fact:        mustValue(t, domain.NewCarrierFirstEffectivePickupFactReference, "CFEP-7"),
			Version:     mustValue(t, domain.NewCarrierFirstEffectivePickupFactVersion, "CFEV-1"),
			EffectiveAt: cancelRequestedAt.Add(time.Hour),
		},
	}
}

// Covers: `AT-PS-086`「包裹已有终局后请求取消——返回既有终局」与 ADR-0254 决定二第 1 格——提交边界
// 上先读当前有效终局：已有非取消终局（面单渠道服务由首次有效收寄形成的那一种，网络服务的交付等同此）
// 或已成立的取消，都交既有终局、不形成取消、不落取消记录、不交释放意图；这一格不比时间。
func TestAStandingFinalIsReturnedInsteadOfACancellation(t *testing.T) {
	t.Run("a current non-cancellation final", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.finals.finals[mustValue(t, domain.NewDeclaredParcelID, "parcel-1")] = formedFinal(t, "parcel-1", "FINAL-7")

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationFinalOutcomeStands {
			t.Fatalf("outcome = %q, want FINAL_OUTCOME_STANDS", result.Outcome())
		}
		if result.Basis().String() != "FINAL_OUTCOME/FINAL-7" {
			t.Fatalf("basis = %q；要指名那份既有终局", result.Basis())
		}
		if fixture.store.saved != 0 || len(fixture.downstream.intents) != 0 {
			t.Fatal("既有终局之上落了取消记录或交了释放意图")
		}
	})

	t.Run("a cancellation already standing under another request", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.cancellations.cancellation = cancelledAt(t, cancelRequestedAt.Add(-time.Hour))
		fixture.cancellations.cancelled = true

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationFinalOutcomeStands ||
			result.Basis().String() != "PARCEL_CANCELLATION/cancel-1" {
			t.Fatalf("outcome = %q basis = %q, want FINAL_OUTCOME_STANDS 指名原取消", result.Outcome(), result.Basis())
		}
		if fixture.store.saved != 0 {
			t.Fatal("同一包裹形成了第二份取消")
		}
	})

	t.Run("a final read that fails stalls instead of letting the cancellation through", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.finals.failWith = errors.New("final store down")

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationUndecided ||
			result.UndecidedReason() != application.CancelFinalStoreUnavailable {
			t.Fatalf("outcome = %q/%q, want CANCELLATION_UNDECIDED/FINAL_OUTCOME_STORE_UNAVAILABLE",
				result.Outcome(), result.UndecidedReason())
		}
		if fixture.store.saved != 0 {
			t.Fatal("读不动终局却落了库")
		}
	})

	t.Run("a cancellation read that fails stalls too", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.cancellations.err = errors.New("cancellation view down")

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.CancelStoreUnavailable {
			t.Fatalf("reason = %q, want CANCELLATION_STORE_UNAVAILABLE", result.UndecidedReason())
		}
	})
}

// Covers: PS CONTEXT「面单渠道服务包裹只允许在实际承运商收寄前……取消」与 ADR-0254 决定二第 1 格的面单
// 渠道一侧——提交边界上读该包裹的首次有效收寄链尾：已形成即不可取消，答取消被拒并指名越过的那一代收寄，
// 记录随之落库（重放答原结果）；待确认是「收寄事实存在但有效性待确认」，取消未决（UC-PS-006 决定矩阵），
// 不先取消后等事实；读不动按依赖未决，不放行；链尾失效或从未有过照常取消。
func TestALabelChannelParcelCannotBeCancelledPastTheCarrierPickup(t *testing.T) {
	t.Run("a formed first effective pickup refuses the cancellation and names it", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.pickups.boundary = formedCarrierPickupBoundary(t)

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationRefused {
			t.Fatalf("outcome = %q, want CANCELLATION_REFUSED", result.Outcome())
		}
		if result.Basis().String() != "CARRIER_FIRST_EFFECTIVE_PICKUP/CFEP-7@CFEV-1" {
			t.Fatalf("basis = %q；要指名越过的那一代收寄", result.Basis())
		}
		record, present := result.Record()
		if !present || record.Kind != ports.RecordCancellationRefused || fixture.store.saved != 1 {
			t.Fatalf("record = %+v present = %v saved = %d", record, present, fixture.store.saved)
		}
		if len(fixture.downstream.intents) != 0 {
			t.Fatal("不可取消却交了释放意图")
		}

		replay, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil || replay.Outcome() != application.CancellationExistingResult {
			t.Fatalf("replay = %q err = %v", replay.Outcome(), err)
		}
	})

	t.Run("a pending pickup leaves the cancellation undecided", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.pickups.boundary = ports.CarrierPickupBoundary{State: ports.CarrierPickupBoundaryPending}

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationUndecided ||
			result.UndecidedReason() != application.CancelCarrierPickupPending {
			t.Fatalf("outcome = %q/%q, want CANCELLATION_UNDECIDED/CARRIER_PICKUP_PENDING",
				result.Outcome(), result.UndecidedReason())
		}
		if fixture.store.saved != 0 {
			t.Fatal("待确认的收寄之上落了取消")
		}
	})

	t.Run("an unreadable pickup register stalls", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.pickups.err = errors.New("tf registry down")

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.CancelCarrierPickupUnavailable || fixture.store.saved != 0 {
			t.Fatalf("reason = %q saved = %d, want CARRIER_PICKUP_UNAVAILABLE 且零落库",
				result.UndecidedReason(), fixture.store.saved)
		}
	})

	t.Run("no pickup or a voided one lets the cancellation stand", func(t *testing.T) {
		fixture := newCancelFixture(t)
		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ParcelCancelled || fixture.pickups.asked != 1 {
			t.Fatalf("outcome = %q asked = %d；收寄读口必须在提交边界上问过", result.Outcome(), fixture.pickups.asked)
		}
	})
}

// Covers: UC-PS-006 结果语义契约——取消编排的结果码与未决原因码逐码对照 String()，封闭集合不悄悄放开。
func TestTheCancellationOutcomeAndReasonSetsAreClosed(t *testing.T) {
	outcomes := map[application.CancelParcelOutcome]string{
		application.ParcelCancelled:                "PARCEL_CANCELLED",
		application.DispositionPending:             "DISPOSITION_PENDING",
		application.CancellationRefused:            "CANCELLATION_REFUSED",
		application.CancellationUndecided:          "CANCELLATION_UNDECIDED",
		application.CancellationExistingResult:     "EXISTING_RESULT",
		application.CancellationConflict:           "REQUEST_CONFLICT",
		application.CancellationNotAccepted:        "REQUEST_NOT_ACCEPTED",
		application.CancellationFinalOutcomeStands: "FINAL_OUTCOME_STANDS",
	}
	for outcome, want := range outcomes {
		if outcome.String() != want {
			t.Fatalf("outcome %d = %q, want %q", outcome, outcome.String(), want)
		}
	}
	if application.CancelParcelOutcome(len(outcomes)+1).String() != "" {
		t.Fatal("集合外的结果带了标签")
	}
	reasons := map[application.CancelUndecidedReason]string{
		application.CancelRequestUnavailable:       "REQUEST_UNAVAILABLE",
		application.CancelAuthorityUnavailable:     "AUTHORITY_UNAVAILABLE",
		application.CancelAuthorityUnconfigured:    "AUTHORITY_UNCONFIGURED",
		application.CancelAdoptionStoreUnavailable: "ADOPTION_STORE_UNAVAILABLE",
		application.CancelStoreUnavailable:         "CANCELLATION_STORE_UNAVAILABLE",
		application.CancelIdentityUnavailable:      "CANCELLATION_IDENTITY_UNAVAILABLE",
		application.CancelFinalStoreUnavailable:    "FINAL_OUTCOME_STORE_UNAVAILABLE",
		application.CancelCarrierPickupUnavailable: "CARRIER_PICKUP_UNAVAILABLE",
		application.CancelCarrierPickupPending:     "CARRIER_PICKUP_PENDING",
	}
	for reason, want := range reasons {
		if reason.String() != want {
			t.Fatalf("reason %d = %q, want %q", reason, reason.String(), want)
		}
	}
	if application.CancelUndecidedReason(len(reasons)+1).String() != "" {
		t.Fatal("集合外的未决原因带了标签")
	}
}

func cancelCommand(t *testing.T, parcel string) application.CancelParcelCommand {
	t.Helper()
	return application.CancelParcelCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		Parcel:            mustValue(t, domain.NewDeclaredParcelID, parcel),
		Requester:         mustValue(t, domain.NewCancellationRequesterReference, "customer-1"),
		Reason:            mustValue(t, domain.NewCancellationReasonReference, "CUSTOMER_CHANGED_MIND"),
		RequestedAt:       cancelRequestedAt,
	}
}

// adoptParcel 往采用库放一份该包裹的责任起点（真经领域构造）。
func adoptParcel(t *testing.T, store *adoptionStoreDouble, parcel string, occurredAt time.Time) {
	t.Helper()
	source, err := domain.NewIntakeSource(domain.IntakeSourceSpec{
		Kind:       domain.NodeIntakeSource,
		Object:     mustValue(t, domain.NewSourceObjectReference, "unit-1"),
		Parcel:     mustValue(t, domain.NewDeclaredParcelID, parcel),
		Place:      mustValue(t, domain.NewIntakePlaceReference, "node-origin"),
		Control:    mustValue(t, domain.NewIntakeControlReference, "NODE-INTAKE/SIGN-7"),
		Version:    mustValue(t, domain.NewSourceResultVersion, "intake-result/v1"),
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("new intake source: %v", err)
	}
	intake, err := domain.AdoptNetworkIntake(source, mustValue(t, domain.NewSubmissionVersionID, "version-1"))
	if err != nil {
		t.Fatalf("adopt network intake: %v", err)
	}
	key := ports.IntakeAdoptionKey{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Parcel:   mustValue(t, domain.NewDeclaredParcelID, parcel),
		Kind:     domain.NodeIntakeSource,
		Version:  mustValue(t, domain.NewSourceResultVersion, "intake-result/v1"),
	}
	store.byKey[key] = ports.IntakeAdoptionRecord{Key: key, Adopted: true, Intake: intake}
}

// Covers: `AT-PS-077`「已接受包裹在有效网络收寄前被合法请求取消——形成包裹取消终局，
// 保留身份与接受基线」——取消决定带授权依据落库、释放意图交付；委托与基线本编排一概
// 不写。
func TestALawfulCancellationBeforeIntakeFormsTheFinalResult(t *testing.T) {
	fixture := newCancelFixture(t)

	result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ParcelCancelled {
		t.Fatalf("outcome = %q, want PARCEL_CANCELLED", result.Outcome())
	}
	record, _ := result.Record()
	if record.Cancellation.Authority().String() != "CANCEL-RULE/PC-17" {
		t.Fatalf("authority = %q; 取消决定必须引用授权规则", record.Cancellation.Authority())
	}
	if !record.Cancellation.RequestedAt().Equal(cancelRequestedAt) {
		t.Fatalf("requested at = %s", record.Cancellation.RequestedAt())
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.downstream.intents))
	}
}

// Covers: `AT-PS-082`「已收寄后客户只发送『取消』消息——明确不能回退取消；没有授权
// 处置决定时保持待处置」与 `AT-PS-079` 的提交边界重读——收寄已先行成立即转待处置，
// 越过的收寄版本随记录保全；待处置不交释放意图（处置请求由后续独立判断发出）。
func TestACrossedBoundaryTurnsIntoDispositionPending(t *testing.T) {
	fixture := newCancelFixture(t)
	adoptParcel(t, fixture.adoptions, "parcel-1", cancelRequestedAt.Add(-time.Hour))

	result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DispositionPending {
		t.Fatalf("outcome = %q, want DISPOSITION_PENDING", result.Outcome())
	}
	record, _ := result.Record()
	if record.IntakeVersion.String() != "intake-result/v1" {
		t.Fatalf("intake version = %q; 越过的收寄版本必须随记录保全", record.IntakeVersion)
	}
	if len(fixture.downstream.intents) != 0 {
		t.Fatal("待处置交了释放意图")
	}
}

// Covers: `AT-PS-078`「三个包裹只有两个满足取消条件——两个取消成功，另一个逐件返回
// 处置路径；不回滚」——批量逐包裹分发，成员结果并存互不相扰。
func TestAPartialBatchDoesNotRollBackItsSuccessfulMembers(t *testing.T) {
	fixture := newCancelFixture(t)
	adoptParcel(t, fixture.adoptions, "parcel-2", cancelRequestedAt.Add(-time.Hour))

	first, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.ParcelCancelled {
		t.Fatalf("first = %q", first.Outcome())
	}

	second, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-2"))
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}
	if second.Outcome() != application.DispositionPending {
		t.Fatalf("second = %q; 已收寄成员逐件返回处置路径", second.Outcome())
	}
	if fixture.store.saved != 2 {
		t.Fatalf("saved = %d; 两件该有两份互不相扰的记录", fixture.store.saved)
	}
	firstRecord, _ := first.Record()
	survivor, found, err := fixture.store.FindByKey(context.Background(), firstRecord.Key)
	if err != nil || !found || survivor.Kind != ports.RecordParcelCancelled {
		t.Fatalf("survivor = %#v; 成功成员被回滚了", survivor)
	}
}

// Covers: UC-PS-006 拒绝结束与实例半边——授权规则明确不允许即拒绝带依据；授权目录
// 未配置即未决不写默认授权（PAR-COM-17）；重复请求返回原结果、同身份异内容冲突不覆盖。
func TestAuthorityRefusalGapAndReplayStayDisciplined(t *testing.T) {
	t.Run("explicit refusal carries its basis", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.authority.judgment = ports.CancellationAuthorityJudgment{
			Granted: false,
			Basis:   mustValue(t, domain.NewCheckReason, "CANCEL_WINDOW_CLOSED/PC-17"),
		}

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationRefused {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		if result.Basis().String() != "CANCEL_WINDOW_CLOSED/PC-17" {
			t.Fatalf("basis = %q", result.Basis())
		}
	})

	t.Run("unconfigured authority stalls", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.authority.configured = false

		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationUndecided ||
			result.UndecidedReason() != application.CancelAuthorityUnconfigured {
			t.Fatalf("outcome = %q/%q", result.Outcome(), result.UndecidedReason())
		}
		if fixture.store.saved != 0 {
			t.Fatal("未决落了库")
		}
	})

	t.Run("replay and conflict", func(t *testing.T) {
		fixture := newCancelFixture(t)
		if _, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1")); err != nil {
			t.Fatalf("first handle: %v", err)
		}

		replay, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("replay handle: %v", err)
		}
		if replay.Outcome() != application.CancellationExistingResult {
			t.Fatalf("replay = %q", replay.Outcome())
		}
		if fixture.store.saved != 1 {
			t.Fatal("重放重复决定了")
		}

		conflicting := cancelCommand(t, "parcel-1")
		conflicting.Reason = mustValue(t, domain.NewCancellationReasonReference, "ANOTHER_REASON")
		conflict, err := fixture.handler.Handle(context.Background(), conflicting)
		if err != nil {
			t.Fatalf("conflict handle: %v", err)
		}
		if conflict.Outcome() != application.CancellationConflict {
			t.Fatalf("conflict = %q", conflict.Outcome())
		}
	})
}

// Covers: `AT-PS-090` 的隔离面——跨客户、查无、编号不符与成员出界答同一个统一不可见错误（ADR-0022：接 HTTP 是 404，
// 不是 `REQUEST_NOT_ACCEPTED`——UC-PS-006 结果语义把那一格限在字段缺），四探的错误逐字相同；取消成立但意图投递失败不翻
// 决定、留续办引用重发同一份。
func TestForeignProbesAndFailedIntentsStayDisciplined(t *testing.T) {
	t.Run("foreign, miss, mismatched and non-member probes are indistinguishable", func(t *testing.T) {
		fixture := newCancelFixture(t)
		foreign := cancelCommand(t, "parcel-1")
		foreign.Identity = sourceIdentity(t, "tenant-1", "customer-2", "source-a", "key-1")
		miss := cancelCommand(t, "parcel-1")
		miss.Identity = sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-unknown")
		mismatched := cancelCommand(t, "parcel-1")
		mismatched.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "request-9")
		outsider := cancelCommand(t, "parcel-9")

		var answers []string
		for name, probe := range map[string]application.CancelParcelCommand{
			"foreign": foreign, "miss": miss, "mismatched": mismatched, "outsider": outsider,
		} {
			result, err := fixture.handler.Handle(context.Background(), probe)
			if !errors.Is(err, application.ErrShipmentRequestNotVisible) {
				t.Fatalf("%s: err = %v, want ErrShipmentRequestNotVisible", name, err)
			}
			if result != (application.CancelParcelResult{}) {
				t.Fatalf("%s: 统一不可见不带结果：%#v", name, result)
			}
			answers = append(answers, err.Error())
		}
		for _, answer := range answers {
			if answer != answers[0] {
				t.Fatalf("answers = %q; 四探必须逐字同答", answers)
			}
		}
		if fixture.store.saved != 0 {
			t.Fatalf("saved = %d; 看不见的委托上不落任何记录", fixture.store.saved)
		}
	})

	t.Run("a missing field is still not accepted", func(t *testing.T) {
		fixture := newCancelFixture(t)
		missing := cancelCommand(t, "parcel-1")
		missing.RequestedAt = time.Time{}
		result, err := fixture.handler.Handle(context.Background(), missing)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationNotAccepted {
			t.Fatalf("outcome = %q, want REQUEST_NOT_ACCEPTED", result.Outcome())
		}
	})

	t.Run("a failed intent is retried without a second decision", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.downstream.err = errors.New("downstream unreachable")

		first, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("first handle: %v", err)
		}
		if first.Outcome() != application.ParcelCancelled {
			t.Fatalf("outcome = %q; 投递失败不得翻取消决定", first.Outcome())
		}
		if first.CancellationHandoffReference().String() == "" {
			t.Fatal("首投失败没有留发布续办引用")
		}

		fixture.downstream.err = nil
		replay, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("replay handle: %v", err)
		}
		if replay.Outcome() != application.CancellationExistingResult {
			t.Fatalf("replay = %q", replay.Outcome())
		}
		if len(fixture.downstream.intents) != 1 || fixture.store.saved != 1 {
			t.Fatalf("intents = %d saved = %d; 重发的必须是原决定那一份", len(fixture.downstream.intents), fixture.store.saved)
		}
	})
}

// Covers: PS CONTEXT「同一包裹的取消提交、收寄采用与面单渠道服务的终局判断依次越过提交边界」在取消这一侧——锁指名命令的
// （租户，包裹），取在取消库的幂等读与提交边界上每一道读之前；取不到锁或没装锁就一道都不读、不落记录、不交意图，停在
// CANCELLATION_STORE_UNAVAILABLE 并带续办引用。
func TestTheCancellationQueuesOnItsParcelBeforeReadingAnything(t *testing.T) {
	reads := func(fixture *cancelFixture) int {
		return fixture.store.lookups + fixture.finals.asked + fixture.cancellations.asked +
			fixture.adoptions.startsAsked + fixture.pickups.asked
	}

	t.Run("the lock comes before every read it guards", func(t *testing.T) {
		fixture := newCancelFixture(t)
		fixture.lock.onLock = func() {
			if n := reads(fixture); n != 0 {
				t.Errorf("取锁之前已读了 %d 道——锁外读到的正是要防的那一份旧状态", n)
			}
		}
		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ParcelCancelled {
			t.Fatalf("outcome = %q, want PARCEL_CANCELLED", result.Outcome())
		}
		if len(fixture.lock.locked) != 1 || fixture.lock.locked[0] != "tenant-1/parcel-1" {
			t.Fatalf("取锁 = %v, want 恰一次、指名 tenant-1/parcel-1", fixture.lock.locked)
		}
	})

	stops := map[string]func(*cancelFixture){
		"an unavailable lock": func(fixture *cancelFixture) { fixture.lock.err = errors.New("lock wait cancelled") },
		"an unwired lock": func(fixture *cancelFixture) {
			fixture.deps.BoundaryLock = nil
			fixture.handler = application.NewCancelParcelHandler(fixture.deps)
		},
	}
	for name, arrange := range stops {
		t.Run(name, func(t *testing.T) {
			fixture := newCancelFixture(t)
			arrange(fixture)
			result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.CancellationUndecided ||
				result.UndecidedReason() != application.CancelStoreUnavailable {
				t.Fatalf("outcome = %q reason = %q, want CANCELLATION_UNDECIDED/CANCELLATION_STORE_UNAVAILABLE",
					result.Outcome(), result.UndecidedReason())
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("未决没带续办引用")
			}
			if n := reads(fixture); n != 0 || fixture.store.saved != 0 || len(fixture.downstream.intents) != 0 {
				t.Fatalf("没排上队却读了 %d 道、落了 %d 份、交了 %d 份意图", n, fixture.store.saved, len(fixture.downstream.intents))
			}
		})
	}
}
