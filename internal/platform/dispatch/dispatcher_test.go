package dispatch_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证派发一拍的三条：发布成功即定稿不再重派、发布失败
// 记时刻后到点重派、租约与批量参数经框架合同生效。发布通道用替身——通道语义
//（broker 送达）不是这一拍要证的，定稿与重派的存储语义才是。

type publisherDouble struct {
	published []eventing.Envelope
	err       error
}

func (double *publisherDouble) Publish(_ context.Context, envelope eventing.Envelope) error {
	if double.err != nil {
		return double.err
	}
	double.published = append(double.published, envelope)
	return nil
}

type movableClock struct{ at time.Time }

func (clock *movableClock) Now() time.Time { return clock.at }

func newDispatchFixture(t *testing.T) (*dispatch.Dispatcher, *outbox.Store, *publisherDouble, *movableClock, *bentopg.DB) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	publisher := &publisherDouble{}
	clock := &movableClock{at: time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC)}
	dispatcher, err := dispatch.NewDispatcher(store, store, publisher, clock, dispatch.Config{
		Limit:       10,
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
		RetryAfter:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("构造派发器：%v", err)
	}
	return dispatcher, store, publisher, clock, db
}

func enqueueEnvelope(t *testing.T, db *bentopg.DB, store *outbox.Store, id string, at time.Time) {
	t.Helper()

	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(id),
		Source:       "idp-parcel/dispatch-test",
		Type:         "dispatch.test.event",
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "subject-1",
		PartitionKey: "tenant-a/" + id,
		OccurredAt:   at,
		RecordedAt:   at,
		ContentType:  eventing.JSONContentType,
		Payload:      []byte(`{"ok":true}`),
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return store.Enqueue(txCtx, envelope)
	}); err != nil {
		t.Fatalf("入队 %s：%v", id, err)
	}
}

// TestAPublishedDeliveryIsFinalizedAndNotRedelivered 证发布成功即定稿：第二拍不再
// 认领同一条。
func TestAPublishedDeliveryIsFinalizedAndNotRedelivered(t *testing.T) {
	dispatcher, store, publisher, clock, db := newDispatchFixture(t)
	enqueueEnvelope(t, db, store, "event-1", clock.at)

	published, err := dispatcher.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("首拍：%v", err)
	}
	if published != 1 || len(publisher.published) != 1 {
		t.Fatalf("published = %d, delivered = %d", published, len(publisher.published))
	}

	clock.at = clock.at.Add(2 * time.Minute)
	again, err := dispatcher.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("第二拍：%v", err)
	}
	if again != 0 || len(publisher.published) != 1 {
		t.Fatalf("已定稿的条目被重派：published = %d, delivered = %d", again, len(publisher.published))
	}
}

// TestAFailedPublishRetriesAfterItsRetryInstant 证发布失败记录重试时刻：到点前不
// 重派、到点后重派并成功。
func TestAFailedPublishRetriesAfterItsRetryInstant(t *testing.T) {
	dispatcher, store, publisher, clock, db := newDispatchFixture(t)
	enqueueEnvelope(t, db, store, "event-1", clock.at)

	publisher.err = errors.New("broker unreachable")
	published, err := dispatcher.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("失败拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("published = %d, want 0", published)
	}

	publisher.err = nil
	clock.at = clock.at.Add(10 * time.Second)
	early, err := dispatcher.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("重试时刻前一拍：%v", err)
	}
	if early != 0 {
		t.Fatalf("重试时刻未到就重派了：published = %d", early)
	}

	clock.at = clock.at.Add(time.Minute)
	retried, err := dispatcher.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("重试拍：%v", err)
	}
	if retried != 1 || len(publisher.published) != 1 {
		t.Fatalf("重试没有派出：published = %d, delivered = %d", retried, len(publisher.published))
	}
}

// TestOnePartitionFailureDoesNotBlockAnother 证分区独立：一个分区的失败不拖住另一
// 分区的发布（逐条定稿而非攒批的行为面）。
func TestOnePartitionFailureDoesNotBlockAnother(t *testing.T) {
	_, store, _, clock, db := newDispatchFixture(t)
	enqueueEnvelope(t, db, store, "event-a", clock.at)
	enqueueEnvelope(t, db, store, "event-b", clock.at)

	failFirst := true
	selective := &selectivePublisher{fail: func(envelope eventing.Envelope) bool {
		if failFirst && envelope.ID == "event-a" {
			return true
		}
		return false
	}}
	retryDispatcher, err := dispatch.NewDispatcher(store, store, selective, clock, dispatch.Config{
		Limit:       10,
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
		RetryAfter:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("构造派发器：%v", err)
	}

	published, err := retryDispatcher.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("混合拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1——b 分区不被 a 的失败拖住", published)
	}

	failFirst = false
	clock.at = clock.at.Add(time.Minute)
	if _, err := retryDispatcher.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("补拍：%v", err)
	}
	if len(selective.published) != 2 {
		t.Fatalf("delivered = %d, want 2", len(selective.published))
	}
}

// TestAFailureIsRecordedUnderTheCodeThatTellsOpsWhatToDo 证发布失败按处置动作分格记码。
// 合成一个码时，运维读不出该改装配、该救下游，还是该去下游核对重复投递。
func TestAFailureIsRecordedUnderTheCodeThatTellsOpsWhatToDo(t *testing.T) {
	tests := []struct {
		name       string
		publishErr error
		wantCode   string
	}{
		{"没有订阅者", dispatch.ErrNoSubscriber, "dispatch.no_subscriber"},
		// 真实发布通道会带上下文包一层，分格必须穿过包装认出来。
		{"包装后的没有订阅者", fmt.Errorf("route %q: %w", "dispatch.test.event", dispatch.ErrNoSubscriber), "dispatch.no_subscriber"},
		{"结果不确定", fmt.Errorf("ack lost: %w", eventing.ErrPublishUncertain), "dispatch.publish_uncertain"},
		{"下游失败", errors.New("broker unreachable"), "dispatch.publish_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dispatcher, store, publisher, clock, db := newDispatchFixture(t)
			enqueueEnvelope(t, db, store, "event-1", clock.at)
			publisher.err = test.publishErr

			published, err := dispatcher.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("失败拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("published = %d，want 0", published)
			}
			if got := recordedFailureCode(t, db, "event-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q，want %q", got, test.wantCode)
			}
		})
	}
}

func recordedFailureCode(t *testing.T, db *bentopg.DB, eventID string) string {
	t.Helper()

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var code string
	query := `SELECT failure_code FROM ` + migrate.SchemaBento + `.outbox WHERE event_id = $1`
	if err := querier.QueryRow(t.Context(), query, eventID).Scan(&code); err != nil {
		t.Fatalf("读回失败码：%v", err)
	}
	return code
}

type selectivePublisher struct {
	fail      func(eventing.Envelope) bool
	published []eventing.Envelope
}

func (double *selectivePublisher) Publish(_ context.Context, envelope eventing.Envelope) error {
	if double.fail != nil && double.fail(envelope) {
		return errors.New("selective failure")
	}
	double.published = append(double.published, envelope)
	return nil
}
