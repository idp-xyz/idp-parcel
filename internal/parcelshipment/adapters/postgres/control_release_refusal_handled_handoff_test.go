package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「释放拒收已处理」意图（ADR-0293 决定四）：信封与处理留痕同一事务（一起成立或一起消失）、信封签委托此刻的当前
// 版本（被拒收的那一版多半已被取代）、一笔占用至多一封（重放不翻倍，同委托另一笔占用另一封）、无事务拒跑（PBC-08）、对不上的留痕响亮报错。
// 事件类型字面在这里再写一遍而不导入提供方常量，理由同复核完成那份用例。

const refusalHandledEventType = "parcel-shipment.shipment-request.control-release-refusal-handled"

var refusalHandledNow = submittedAtFixture.Add(5 * time.Hour)

type refusalHandledClock struct{ at time.Time }

func (clock refusalHandledClock) Now() time.Time { return clock.at }

type refusalHandledFixture struct {
	requests   *adapter.ShipmentRequests
	handlings  *adapter.ControlReleaseRefusalHandlings
	register   *adapter.ControlReleaseRefusals
	handoff    *adapter.OutboxControlReleaseRefusalHandledHandoff
	store      *outbox.Store
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newRefusalHandledFixture(t *testing.T) *refusalHandledFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	requests, err := adapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	handlings, err := adapter.NewControlReleaseRefusalHandlings(db)
	if err != nil {
		t.Fatalf("构造处理留痕：%v", err)
	}
	register, err := adapter.NewControlReleaseRefusals(db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxControlReleaseRefusalHandledHandoff(db, store, refusalHandledClock{at: refusalHandledNow})
	if err != nil {
		t.Fatalf("构造处理续办交接：%v", err)
	}
	return &refusalHandledFixture{
		requests: requests, handlings: handlings, register: register, handoff: handoff,
		store: store, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *refusalHandledFixture) within(t *testing.T, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(t.Context(), fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

// refusalOnSeededRequest 在库里摆一份已提交的委托与册上被拒收的一笔（被拒收的是首版），交回来源身份、委托与那一笔。
func (fixture *refusalHandledFixture) refusalOnSeededRequest(
	t *testing.T,
	key, requestID, version string,
) (domain.SourceIdentity, domain.ShipmentRequest, ports.ControlReleaseRefusal) {
	t.Helper()
	identity := requestIdentity(t, key)
	mustInsert(t, fixture.transactor, t.Context(), fixture.requests, submittedShipmentRequest(t, key, requestID))
	loaded, found, err := fixture.requests.FindBySourceIdentity(t.Context(), identity)
	if err != nil || !found {
		t.Fatalf("读回 %s：found=%v err=%v", requestID, found, err)
	}
	refusal := ports.ControlReleaseRefusal{
		TenantID:          identity.TenantID(),
		CustomerAccountID: identity.CustomerAccountID(),
		ShipmentRequestID: loaded.ShipmentRequestID(),
		SubmissionVersion: mustBuild(t, domain.NewSubmissionVersionID, version),
		ControlResultID:   mustBuild(t, domain.NewFinancialControlResultID, requestID+"/"+version),
		Reason:            mustBuild(t, domain.NewCheckReason, "SA-RELEASE_REQUEST_NOT_ACCEPTED"),
		RecordedAt:        refusalRecordedAt,
	}
	fixture.within(t, func(ctx context.Context) error {
		_, err := fixture.register.Record(ctx, refusal)
		return err
	})
	return identity, loaded, refusal
}

func (fixture *refusalHandledFixture) handling(t *testing.T, refusal ports.ControlReleaseRefusal) ports.ControlReleaseRefusalHandling {
	t.Helper()
	return ports.ControlReleaseRefusalHandling{
		Key:       ports.KeyOfRefusal(refusal),
		Kind:      ports.ControlReleaseRefusalMarkedHandled,
		Handler:   mustBuild(t, domain.NewControlReleaseRefusalHandlerReference, "https://id.syn.example#alice"),
		Basis:     mustBuild(t, domain.NewControlReleaseRefusalHandlingBasis, "SYN-CHECKED"),
		HandledAt: refusalHandledNow.Add(-time.Minute),
	}
}

func refusalHandledEventID(identity domain.SourceIdentity, refusal ports.ControlReleaseRefusal) string {
	return string(outboxintent.FingerprintEventID("control-release-refusal-handled",
		identity.TenantID().String(), identity.CustomerAccountID().String(), identity.Source().String(), identity.RequestKey().String(),
		refusal.ShipmentRequestID.String(), refusal.SubmissionVersion.String(), refusal.ControlResultID.String()))
}

func (fixture *refusalHandledFixture) countIntents(t *testing.T, eventID string) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1 AND event_type = $2`,
		eventID, refusalHandledEventType,
	).Scan(&count); err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

// Covers: AT-PS-165；ADR-0094 决定四、ADR-0293 决定四——处理留痕与续办信封同一事务成立；信封签委托此刻的当前版本与成员清单，不是被拒收的那一版。
func TestTheResumeEnvelopeCommitsWithTheHandlingAndCarriesTheCurrentVersion(t *testing.T) {
	fixture := newRefusalHandledFixture(t)
	identity, loaded, refusal := fixture.refusalOnSeededRequest(t, "req-key-1", "request-1", "version-0")
	handling := fixture.handling(t, refusal)

	fixture.within(t, func(ctx context.Context) error {
		if outcome, _, err := fixture.handlings.HandleControlReleaseRefusal(ctx, handling); err != nil ||
			outcome != ports.ControlReleaseRefusalHandlingRecorded {
			t.Fatalf("留痕 = %d err=%v", outcome, err)
		}
		return fixture.handoff.HandOffControlReleaseRefusalHandled(ctx,
			ports.ControlReleaseRefusalHandledHandoffIntent{Identity: identity, Request: loaded, Handling: handling})
	})

	if count := fixture.countIntents(t, refusalHandledEventID(identity, refusal)); count != 1 {
		t.Fatalf("outbox 行数 = %d, want 1", count)
	}
	if _, found, err := fixture.handlings.FindControlReleaseRefusalHandling(t.Context(), handling.Key); err != nil || !found {
		t.Fatalf("留痕没落库 found=%v err=%v", found, err)
	}
	envelope := claimEnvelopeOfType(t, fixture.store, refusalHandledNow.Add(time.Hour), refusalHandledEventType)
	var payload struct {
		TenantID                   string   `json:"tenantId"`
		ShipmentRequestID          string   `json:"shipmentRequestId"`
		SubmissionVersionID        string   `json:"submissionVersionId"`
		DeclaredParcelIDs          []string `json:"declaredParcelIds"`
		HandledSubmissionVersionID string   `json:"handledSubmissionVersionId"`
		HandledControlResultID     string   `json:"handledControlResultId"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("解载荷：%v", err)
	}
	if payload.SubmissionVersionID != loaded.CurrentSubmissionVersion().VersionID().String() || payload.SubmissionVersionID == "version-0" {
		t.Fatalf("载荷提交版本 = %q, want 委托此刻的当前版本 %q，不是被拒收的那一版",
			payload.SubmissionVersionID, loaded.CurrentSubmissionVersion().VersionID())
	}
	if payload.HandledSubmissionVersionID != "version-0" || payload.HandledControlResultID != "request-1/version-0" ||
		payload.TenantID != identity.TenantID().String() || payload.ShipmentRequestID != "request-1" ||
		len(payload.DeclaredParcelIDs) != len(loaded.CurrentSubmissionVersion().DeclaredParcelIDs()) {
		t.Fatalf("载荷 = %+v", payload)
	}
	if !envelope.OccurredAt.Equal(handling.HandledAt) || envelope.Subject != "request-1" {
		t.Fatalf("occurredAt = %s subject = %q, want 处理成立的时刻与委托号", envelope.OccurredAt, envelope.Subject)
	}
}

// Covers: AT-PS-165；ADR-0094 决定四的另一半——一起消失：回滚之后处理留痕与信封都不在，那一笔仍是未处理，接受链那一路不会被一封空信封惊动。
func TestARolledBackHandlingLeavesNeitherTheRowNorTheEnvelope(t *testing.T) {
	fixture := newRefusalHandledFixture(t)
	identity, loaded, refusal := fixture.refusalOnSeededRequest(t, "req-key-1", "request-1", "version-0")
	handling := fixture.handling(t, refusal)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, _, err := fixture.handlings.HandleControlReleaseRefusal(ctx, handling); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffControlReleaseRefusalHandled(ctx,
			ports.ControlReleaseRefusalHandledHandoffIntent{Identity: identity, Request: loaded, Handling: handling}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, found, err := fixture.handlings.FindControlReleaseRefusalHandling(t.Context(), handling.Key); err != nil || found {
		t.Fatalf("回滚后处理留痕仍在 found=%v err=%v", found, err)
	}
	if count := fixture.countIntents(t, refusalHandledEventID(identity, refusal)); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d, want 0", count)
	}
}

// Covers: AT-PS-165；ADR-0043——重放重发同一份；同一委托上另一笔占用的处理是另一封，不被当成重放吞掉。
func TestTheResumeEnvelopeIsOnePerRefusedOccupation(t *testing.T) {
	fixture := newRefusalHandledFixture(t)
	identity, loaded, first := fixture.refusalOnSeededRequest(t, "req-key-1", "request-1", "version-0")
	second := first
	second.SubmissionVersion = mustBuild(t, domain.NewSubmissionVersionID, "version-00")
	second.ControlResultID = mustBuild(t, domain.NewFinancialControlResultID, "request-1/version-00")
	fixture.within(t, func(ctx context.Context) error {
		_, err := fixture.register.Record(ctx, second)
		return err
	})

	send := func(refusal ports.ControlReleaseRefusal) {
		fixture.within(t, func(ctx context.Context) error {
			return fixture.handoff.HandOffControlReleaseRefusalHandled(ctx, ports.ControlReleaseRefusalHandledHandoffIntent{
				Identity: identity, Request: loaded, Handling: fixture.handling(t, refusal)})
		})
	}
	send(first)
	send(first)
	send(second)
	if count := fixture.countIntents(t, refusalHandledEventID(identity, first)); count != 1 {
		t.Fatalf("同一笔占用的信封 = %d 封, want 1——重发的必须是同一份", count)
	}
	if count := fixture.countIntents(t, refusalHandledEventID(identity, second)); count != 1 {
		t.Fatalf("另一笔占用的信封 = %d 封, want 1——不该被当成第一笔的重放吞掉", count)
	}
}

// Covers: AT-PS-164；PBC-08 行为面——写口在无事务上下文必须被 RequireExecutor 拒绝，落库行数为零。
func TestTheResumeEnvelopeRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newRefusalHandledFixture(t)
	identity, loaded, refusal := fixture.refusalOnSeededRequest(t, "req-key-1", "request-1", "version-0")

	err := fixture.handoff.HandOffControlReleaseRefusalHandled(t.Context(),
		ports.ControlReleaseRefusalHandledHandoffIntent{Identity: identity, Request: loaded, Handling: fixture.handling(t, refusal)})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := fixture.countIntents(t, refusalHandledEventID(identity, refusal)); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

// Covers: 装配缺陷响亮：缺来源身份、缺委托、缺留痕，或留痕属于别的租户、别的委托，都不入一封指向别处的信封。
func TestAResumeIntentThatDoesNotMatchItsRequestIsLoud(t *testing.T) {
	fixture := newRefusalHandledFixture(t)
	identity, loaded, refusal := fixture.refusalOnSeededRequest(t, "req-key-1", "request-1", "version-0")
	good := fixture.handling(t, refusal)

	foreignTenant := good
	foreignTenant.Key.TenantID = psTenant(t, "tenant-other")
	foreignRequest := good
	foreignRequest.Key.ShipmentRequestID = mustBuild(t, domain.NewShipmentRequestID, "request-other")
	for name, intent := range map[string]ports.ControlReleaseRefusalHandledHandoffIntent{
		"缺来源身份": {Request: loaded, Handling: good},
		"缺委托":   {Identity: identity, Handling: good},
		"缺留痕":   {Identity: identity, Request: loaded},
		"别的租户":  {Identity: identity, Request: loaded, Handling: foreignTenant},
		"别的委托":  {Identity: identity, Request: loaded, Handling: foreignRequest},
	} {
		if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			return fixture.handoff.HandOffControlReleaseRefusalHandled(ctx, intent)
		}); err == nil {
			t.Errorf("%s：必须响亮报错", name)
		}
	}
	if count := fixture.countIntents(t, refusalHandledEventID(identity, refusal)); count != 0 {
		t.Fatalf("被拒绝的意图仍入了 %d 封", count)
	}
}

// claimEnvelopeOfType 按派发一拍的同一条认领路径取回指定类型的信封：认领时刻取在入队时刻之后。
func claimEnvelopeOfType(t *testing.T, store *outbox.Store, now time.Time, eventType string) eventing.Envelope {
	t.Helper()
	deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{Now: now, Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5})
	if err != nil {
		t.Fatalf("认领待发信封：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.Type) == eventType {
			return delivery.Envelope
		}
	}
	t.Fatalf("认领到 %d 封，其中没有 %s", len(deliveries), eventType)
	return eventing.Envelope{}
}
