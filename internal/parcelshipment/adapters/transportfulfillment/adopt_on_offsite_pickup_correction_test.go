package transportfulfillment_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证票 label-channel/24 的两向（ADR-0117）：TF 真登记库落首登与更正
// 两代 → 真 Inbox 消费门 → 按信封所指版本读回 → 真采用编排 → 真采用账。同来源更正接续链尾形成
// 新采用判断版本（回指前版、承诺在前版上重述、生效随更正后的发生时刻），根行一字不动；另一来源
// 种类照旧不采用，依据指名链尾（AT-PS-049 一字不动）。委托与资格用替身：已接受委托与资格声明
// 属别的缝，这里证的是采用链本身。

// correctionFixture 把两侧的真库件与 PS 的替身件装在一起。
type correctionFixture struct {
	db        *bentopg.DB
	pickups   *tfpostgres.OffsitePickupRegistrations
	adoptions *pspostgres.IntakeAdoptions
	handler   *psapplication.AdoptNetworkIntakeHandler
	consumer  *psinbox.OffsitePickupConsumer
	tenant    tfdomain.TenantID
	key       tfports.OffsitePickupKey
}

func newCorrectionFixture(t *testing.T) *correctionFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	pickups, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		t.Fatalf("构造揽收登记库：%v", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		t.Fatalf("构造采用账：%v", err)
	}
	handler := psapplication.NewAdoptNetworkIntakeHandler(psapplication.AdoptNetworkIntakeDeps{
		Requests: &requestStoreDouble{records: map[psdomain.SourceIdentity]psdomain.ShipmentRequest{
			identity(t): acceptedRequest(t),
		}},
		Eligibility: eligibilityDouble{},
		Adoptions:   adoptions,
		Identities:  &commitmentIdentityDouble{},
		Downstream:  downstreamDouble{},
		Clock:       fixedClock{at: pickedUpAt.Add(time.Minute)},
	})
	processing, err := adapter.NewAdoptOnOffsitePickupAdapter(
		pickups,
		&pickupTargetViewDouble{target: pickupTarget(t), found: true},
		adapter.NewOffsitePickupAdapter(handler),
	)
	if err != nil {
		t.Fatalf("构造处理适配器：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := psinbox.NewOffsitePickupConsumer(db.Transactor(), store, processing)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	tenant := value(t, tfdomain.NewTenantID, "tenant-1")
	return &correctionFixture{
		db:        db,
		pickups:   pickups,
		adoptions: adoptions,
		handler:   handler,
		consumer:  consumer,
		tenant:    tenant,
		key: tfports.OffsitePickupKey{
			TenantID: tenant,
			Object:   value(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
			Attempt:  value(t, tfdomain.NewAttemptReference, "attempt-1"),
		},
	}
}

// register 把一代揽收落进 TF 真登记库（首登与更正走同一个 Save）。
func (fixture *correctionFixture) register(t *testing.T, record tfports.OffsitePickupRecord) {
	t.Helper()
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := fixture.pickups.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != tfports.OffsitePickupSaved {
			t.Fatalf("save outcome = %d, want 已写入", outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("登记揽收：%v", err)
	}
}

// deliver 按 TF 交接的形状造一封对象级揽收登记信封（更正版本 ID 带版本段、载荷带 pickupVersion）
// 并投给消费门。
func (fixture *correctionFixture) deliver(t *testing.T, version string, corrected bool) error {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"tenantId":      "tenant-1",
		"object":        "parcel-1",
		"attempt":       "attempt-1",
		"pickupVersion": version,
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	eventID := "tenant-1/parcel-1/attempt-1"
	if corrected {
		eventID += "/" + version
	}
	now := pickedUpAt.Add(time.Minute)
	return fixture.consumer.Consume(t.Context(), eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID + "/offsite-pickup-registration"),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         psinbox.OffsitePickupRegisteredEventType,
		Version:      1,
		Scope:        "tenant-1",
		Subject:      "parcel-1/attempt-1",
		PartitionKey: "tenant-1/parcel-1/offsite-pickup-registration",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	})
}

func (fixture *correctionFixture) adoption(t *testing.T, kind psdomain.IntakeSourceKind, version string) psports.IntakeAdoptionRecord {
	t.Helper()
	record, found, err := fixture.adoptions.FindByKey(t.Context(), psports.IntakeAdoptionKey{
		TenantID: value(t, psdomain.NewTenantID, "tenant-1"),
		Parcel:   value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Kind:     kind,
		Version:  value(t, psdomain.NewSourceResultVersion, version),
	})
	if err != nil || !found {
		t.Fatalf("采用记录 %s/%s：found=%v err=%v", kind, version, found, err)
	}
	return record
}

// Covers: `AT-PS-050` 正向 + `AT-PS-049` 反向，真库两向。
func TestASameSourceCorrectionSupersedesAndACompetingSourceIsStillRefusedInTheDatabase(t *testing.T) {
	fixture := newCorrectionFixture(t)
	ctx := t.Context()

	original := registeredPickup(t, "parcel-1")
	fixture.register(t, original)
	if err := fixture.deliver(t, "pickup-result/v1", false); err != nil {
		t.Fatalf("首登那一封：%v", err)
	}
	root := fixture.adoption(t, psdomain.OffsitePickupSource, "pickup-result/v1")
	if !root.Adopted || !root.Commitment.EffectiveAt().Equal(pickedUpAt) {
		t.Fatalf("首登没有采用为根：%+v", root)
	}
	if _, chained := root.Supersedes(); chained {
		t.Fatal("根行凭空长出了回指")
	}

	// TF 更正：发生时刻提前十五分钟、地点换了；PS 那一封按版本读回更正那一代。
	fixture.register(t, correctedRegisteredPickup(t, original))
	if err := fixture.deliver(t, "pickup-result/v2", true); err != nil {
		t.Fatalf("更正那一封：%v", err)
	}
	corrected := fixture.adoption(t, psdomain.OffsitePickupSource, "pickup-result/v2")
	if !corrected.Adopted {
		t.Fatalf("更正版被拒了：%q", corrected.RefusalBasis)
	}
	if supersedes, chained := corrected.Supersedes(); !chained || supersedes.String() != "pickup-result/v1" {
		t.Fatalf("更正版没回指首登：%v %v", supersedes, chained)
	}
	if prior, restated := corrected.Commitment.PriorVersion(); !restated || prior != root.Commitment.Version() {
		t.Fatalf("承诺没在根承诺上重述：%v %v", prior, restated)
	}
	if reason, _ := corrected.Commitment.AdjustmentReason(); reason.String() != "SOURCE_CORRECTED/OFFSITE_PICKUP/pickup-result/v1" {
		t.Fatalf("原因 = %q", reason)
	}
	correctedAt := pickedUpAt.Add(-15 * time.Minute)
	if !corrected.Commitment.EffectiveAt().Equal(correctedAt) || corrected.Intake.Source().Place().String() != "customer-gate-2" {
		t.Fatalf("更正版没换上更正后的收寄：effective=%s place=%s", corrected.Commitment.EffectiveAt(), corrected.Intake.Source().Place())
	}
	again := fixture.adoption(t, psdomain.OffsitePickupSource, "pickup-result/v1")
	if !again.Commitment.EffectiveAt().Equal(pickedUpAt) || again.Commitment.Version() != root.Commitment.Version() {
		t.Fatal("根行被改写了——历史只插不改")
	}
	tail, started, err := fixture.adoptions.FindResponsibilityStart(ctx, value(t, psdomain.NewTenantID, "tenant-1"), value(t, psdomain.NewDeclaredParcelID, "parcel-1"))
	if err != nil || !started || tail.Key.Version.String() != "pickup-result/v2" {
		t.Fatalf("链尾 = %s started=%v err=%v，想要 v2", tail.Key.Version, started, err)
	}

	// 反向：另一来源种类（节点收寄）想开第二个责任起点，照旧不采用，依据指名链尾。
	node := psapplication.AdoptNetworkIntakeCommand{
		Identity:          identity(t),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		Source: psdomain.IntakeSourceSpec{
			Kind:       psdomain.NodeIntakeSource,
			Object:     value(t, psdomain.NewSourceObjectReference, "handling-unit-1"),
			Parcel:     value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
			Place:      value(t, psdomain.NewIntakePlaceReference, "node-origin"),
			Control:    value(t, psdomain.NewIntakeControlReference, "NODE-CONTROL/NO-7"),
			Version:    value(t, psdomain.NewSourceResultVersion, "intake-result/v1"),
			OccurredAt: pickedUpAt.Add(time.Hour),
		},
	}
	var result psapplication.AdoptNetworkIntakeResult
	if err := fixture.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		result, err = fixture.handler.Handle(txCtx, node)
		return err
	}); err != nil {
		t.Fatalf("节点收寄：%v", err)
	}
	if result.Outcome() != psapplication.IntakeSourceNotAdopted || result.Basis().String() != "RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/pickup-result/v2" {
		t.Fatalf("outcome = %q basis = %q; 另一来源必须照旧不采用且指名链尾", result.Outcome(), result.Basis())
	}
	refusal := fixture.adoption(t, psdomain.NodeIntakeSource, "intake-result/v1")
	if refusal.Adopted {
		t.Fatal("竞争来源被采用了")
	}
	tail, _, err = fixture.adoptions.FindResponsibilityStart(ctx, value(t, psdomain.NewTenantID, "tenant-1"), value(t, psdomain.NewDeclaredParcelID, "parcel-1"))
	if err != nil || tail.Key.Version.String() != "pickup-result/v2" {
		t.Fatalf("不采用行动了链尾：%s err=%v", tail.Key.Version, err)
	}
}
