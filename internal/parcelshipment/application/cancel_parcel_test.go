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
	byKey map[ports.CancellationRequestKey]ports.CancellationRecord
	saved int
}

func newCancellationStore() *cancellationStoreDouble {
	return &cancellationStoreDouble{byKey: map[ports.CancellationRequestKey]ports.CancellationRecord{}}
}

func (double *cancellationStoreDouble) FindByKey(
	_ context.Context,
	key ports.CancellationRequestKey,
) (ports.CancellationRecord, bool, error) {
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

type cancelFixture struct {
	handler    *application.CancelParcelHandler
	authority  *cancellationAuthorityDouble
	adoptions  *adoptionStoreDouble
	store      *cancellationStoreDouble
	downstream *cancellationDownstreamDouble
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
		adoptions:  newAdoptionStore(),
		store:      newCancellationStore(),
		downstream: &cancellationDownstreamDouble{},
	}
	fixture.handler = application.NewCancelParcelHandler(application.CancelParcelDeps{
		Requests:   requests,
		Authority:  fixture.authority,
		Adoptions:  fixture.adoptions,
		Store:      fixture.store,
		Identities: &cancellationIdentityDouble{},
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: cancelRequestedAt.Add(time.Minute)},
	})
	return fixture
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

// Covers: `AT-PS-090` 的隔离面——跨客户/查无/成员出界统一不可见（两探同形整结构比对，
// 同采用侧的纪律）；取消成立但意图投递失败不翻决定、留续办引用重发同一份。
func TestForeignProbesAndFailedIntentsStayDisciplined(t *testing.T) {
	t.Run("foreign and miss probes are indistinguishable", func(t *testing.T) {
		fixture := newCancelFixture(t)
		foreign := cancelCommand(t, "parcel-1")
		foreign.Identity = sourceIdentity(t, "tenant-1", "customer-2", "source-a", "key-1")
		foreignResult, err := fixture.handler.Handle(context.Background(), foreign)
		if err != nil {
			t.Fatalf("foreign probe: %v", err)
		}

		miss := cancelCommand(t, "parcel-1")
		miss.Identity = sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-unknown")
		missResult, err := fixture.handler.Handle(context.Background(), miss)
		if err != nil {
			t.Fatalf("miss probe: %v", err)
		}

		if foreignResult != missResult {
			t.Fatalf("foreign = %#v miss = %#v; 两探必须同形", foreignResult, missResult)
		}
		if foreignResult.Outcome() != application.CancellationNotAccepted {
			t.Fatalf("outcome = %q", foreignResult.Outcome())
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
