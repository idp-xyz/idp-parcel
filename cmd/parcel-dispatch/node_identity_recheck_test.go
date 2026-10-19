package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件对真实 PostgreSQL 16 证 node-operations 第一个 inbox 消费者经生产装配接得住 PS 的门铃（ADR-0199 决定三，票
// no-reception-source/03）。夹具全为合成（S 级）。

const (
	recheckConsumerName  = "node-operations/recheck-pending-identities"
	recheckDoorbellType  = "parcel-shipment.external-identifier-relation.formed"
	recheckMark          = "SYN-CUSTOMER-PARCEL-NRS03"
	recheckPendingSource = "SYN-DELIVERY-NRS03"
	recheckUnit          = "SYN-UNIT-NRS03"
)

// Covers: 路由表认得出 PS 外部标识关联已形成这一类，投给 NO 的重核消费门。手法同别的路由表用例：毒丸载荷让消费门显式拒收
// 入账并交回 nil，于是这一条定稿；漏挂或挂错撞的是无订阅者。
func TestAFormedExternalIdentifierRelationReachesTheRecheckConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "relation-formed-1", noinbox.ExternalIdentifierRelationFormedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把门铃投给重核消费门",
			published, recordedFailureCode(t, db, "relation-formed-1"))
	}
}

// Covers: NO 消费者自写的门铃类型串与 PS outbox 适配器写出的那一个相等（消费者不 import 提供方，两串各写一份，由这条对照
// 用例钉住；对照落在组合根，理由同承运商首次有效收寄那一条）。
func TestTheRecheckConsumerAndThePSDoorbellAgreeOnTheEventType(t *testing.T) {
	if string(noinbox.ExternalIdentifierRelationFormedEventType) != pspostgres.ExternalIdentifierRelationFormedEventType {
		t.Fatalf("NO 消费者认 %q，PS 写出 %q", noinbox.ExternalIdentifierRelationFormedEventType, pspostgres.ExternalIdentifierRelationFormedEventType)
	}
}

// Covers: `AT-NO-026`「身份后来被 `parcel-shipment` 确认——建立版本化关联，原待识别记录和既有事实保留；业务时间取原接收时刻，
// 不另起控制起点，发布意图一封」的真库面，经生产装配：号码不在册时收寄落成零候选的待识别 → PS 委托接受、客户包裹件号形成
// 这个号的关联，同笔入队门铃 → 生产派发器一拍把门铃投给 NO 的重核消费者 → NO 追加一版`节点收寄已形成`（前版是原待识别、
// 来源标识由原判断版本派生）并交收寄意图一封，意图载荷指回的那一版业务时间是原收寄时刻 → 方向依据的身份一格对这件实物判
// 成立：身份读口读回的追加版带着前版，身份一格只读链头（ADR-0210 决定五），原待识别已被接续、不再参与。同一个号再响一次
// 门铃不再追加，也不再交意图。
func TestTheWiredDispatchRechecksAPendingUnitOnceItsMarkIsRelated(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
	receivedAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)

	pending := receivePendingForRecheck(t, fixture.db, tenant, receivedAt)
	submitWithCustomerParcelNumber(t, fixture, recheckMark)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
	doorbells := fixture.outboxIDsOfType(t, recheckDoorbellType)
	if len(doorbells) != 1 {
		t.Fatalf("接受之后门铃 %d 封, want 1——关联形成的那一笔没入队", len(doorbells))
	}
	if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != 0 {
		t.Fatalf("门铃投递之前已有 %d 封收寄意图", n)
	}

	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if got := recordedFailureCode(t, fixture.db, doorbells[0]); got != "" {
		t.Fatalf("门铃失败码 = %q, want 已处理", got)
	}
	if n := fixture.countInbox(t, recheckConsumerName, doorbells[0]); n != 1 {
		t.Fatalf("重核消费门 inbox %d 行, want 1", n)
	}

	receptions, err := nopostgres.NewReceptions(fixture.db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	derived := noports.ReceptionKey{TenantID: tenant, SourceID: "IDENTITY-RECHECK/" + pending.JudgmentVersion.String()}
	formed, found, err := receptions.FindByKey(ctx, derived)
	if err != nil || !found {
		t.Fatalf("追加版没落册：found = %v err = %v", found, err)
	}
	association, associated := formed.Intake.Association()
	switch {
	case formed.Kind != noports.RecordIntakeFormed || !associated || association.String() != "SYN-PARCEL-01":
		t.Fatalf("追加版 = %+v, want 节点收寄已形成、关联 SYN-PARCEL-01", formed)
	case formed.Predecessor != pending.JudgmentVersion:
		t.Fatalf("追加版前版 = %s, want 原待识别 %s", formed.Predecessor, pending.JudgmentVersion)
	case !formed.Intake.ReceivedAt().Equal(receivedAt) || formed.Control.Basis() != pending.Control.Basis() ||
		!formed.Control.EstablishedAt().Equal(pending.Control.EstablishedAt()):
		t.Fatalf("追加版接收 %s、控制 %s @ %s, want 照原收寄 %s、%s @ %s", formed.Intake.ReceivedAt(), formed.Control.Basis(),
			formed.Control.EstablishedAt(), receivedAt, pending.Control.Basis(), pending.Control.EstablishedAt())
	case formed.ServiceResultCheck != noports.ServiceResultsChecked:
		t.Fatalf("追加版核对态 = %q, want CHECKED", formed.ServiceResultCheck)
	}
	original, found, err := receptions.FindByKey(ctx, pending.Key)
	if err != nil || !found || original.Kind != noports.RecordPendingIdentification || original.JudgmentVersion != pending.JudgmentVersion {
		t.Fatalf("原待识别没有原样留着：%+v（found %v err %v）", original, found, err)
	}

	intents := intakeIntentSources(t, fixture.db)
	if len(intents) != 1 || intents[0] != derived.SourceID {
		t.Fatalf("收寄意图 = %v, want 恰一封、指回追加版 %s", intents, derived.SourceID)
	}
	pointed, found, err := receptions.FindByKey(ctx, noports.ReceptionKey{TenantID: tenant, SourceID: intents[0]})
	if err != nil || !found || !pointed.Intake.ReceivedAt().Equal(receivedAt) {
		t.Fatalf("意图指回的收寄业务时间 = %s, want 原收寄时刻 %s", pointed.Intake.ReceivedAt(), receivedAt)
	}

	identities, err := nopostgres.NewIntakeIdentities(fixture.db)
	if err != nil {
		t.Fatalf("构造身份读口：%v", err)
	}
	observations, err := identities.FindIntakeIdentities(ctx, tenant, mustNO(t, nodomain.NewHandlingUnitID, recheckUnit))
	if err != nil {
		t.Fatalf("读收寄身份：%v", err)
	}
	var readBack *nodomain.IntakeIdentityObservation
	for index := range observations {
		if observations[index].SourceID == derived.SourceID {
			readBack = &observations[index]
		}
	}
	if len(observations) != 2 || readBack == nil || readBack.Predecessor != pending.JudgmentVersion {
		t.Fatalf("身份读口读回 %d 行、追加版 %+v, want 原待识别与追加版两行、追加版带前版 %s", len(observations), readBack, pending.JudgmentVersion)
	}
	basis, err := nodomain.JudgeIdentityBasis(observations)
	if err != nil {
		t.Fatalf("判身份：%v", err)
	}
	if basis.Cell().Answer() != nodomain.BasisEstablished || basis.IntakeSourceID() != derived.SourceID {
		t.Fatalf("身份一格 = %v（%v）依 %q, want 成立、依追加版", basis.Cell().Answer(), basis.Cell().Reasons(), basis.IntakeSourceID())
	}

	rows := fixture.countSQL(t, `SELECT count(*) FROM node_operations.reception`)
	ringAgain(t, fixture.db, tenant.String())
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("再一拍：%v", err)
	}
	if got := recordedFailureCode(t, fixture.db, "SYN-DOORBELL-NRS03-AGAIN"); got != "" {
		t.Fatalf("再响的门铃失败码 = %q", got)
	}
	if after := fixture.countSQL(t, `SELECT count(*) FROM node_operations.reception`); after != rows {
		t.Fatalf("同一个号再响一次门铃，册上 %d → %d 行, want 不再追加", rows, after)
	}
	if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != 1 {
		t.Fatalf("收寄意图变成 %d 封, want 仍是 1", n)
	}
}

// receivePendingForRecheck 经收寄编排（生产适配器）收一件号码还不在关联册里的实物：查无此号，落成零候选的待识别。
func receivePendingForRecheck(t *testing.T, db *bentopg.DB, tenant nodomain.TenantID, receivedAt time.Time) noports.ReceptionRecord {
	t.Helper()
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		t.Fatalf("构造版本签发：%v", err)
	}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造收寄交接：%v", err)
	}
	identifiers, err := pspostgres.NewExternalIdentifiers(db)
	if err != nil {
		t.Fatalf("构造关联册读面：%v", err)
	}
	identity, err := noparcel.NewParcelIdentityView(identifiers)
	if err != nil {
		t.Fatalf("构造身份核对读口：%v", err)
	}
	services, err := nodeIdentityRecheckServiceResults(db)
	if err != nil {
		t.Fatalf("构造服务结果读口：%v", err)
	}
	handler, err := noapplication.NewReceiveDeliveredUnitHandler(noapplication.ReceiveDeliveredUnitDeps{
		Identity: identity, ServiceResults: services, Receptions: receptions, Versions: versions,
		Downstream: downstream, Clock: systemClock{},
	})
	if err != nil {
		t.Fatalf("构造收寄编排：%v", err)
	}
	var result noapplication.ReceiveDeliveredUnitResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = handler.Handle(txCtx, noapplication.ReceiveDeliveredUnitCommand{
			TenantID:    tenant,
			SourceID:    recheckPendingSource,
			Node:        mustNO(t, nodomain.NewNodeReference, "SYN-NODE-01"),
			DeliveredBy: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-CUSTOMER-01"),
			Unit:        mustNO(t, nodomain.NewHandlingUnitID, recheckUnit),
			Mark:        noports.ExternalMarkObservation{Mark: recheckMark},
			Claim:       noapplication.ExplicitReception,
			Evidence:    mustNO(t, nodomain.NewReceptionEvidenceReference, "SYN-SIGN-NRS03"),
			OccurredAt:  receivedAt,
		})
		return handleErr
	})
	record, recorded := result.Record()
	if result.Outcome() != noapplication.UnitPendingIdentification || !recorded || record.IdentityConflict {
		t.Fatalf("号码入册之前收寄 outcome = %v, want 零候选的待识别", result.Outcome())
	}
	return record
}

// submitWithCustomerParcelNumber 照 SYN 夹具的提交命令建单，那一件声明包裹配上客户包裹件号 number。
func submitWithCustomerParcelNumber(t *testing.T, fixture *synVerticalFixture, number string) {
	t.Helper()
	command := fixture.submitCommand(t)
	declaration, err := psdomain.DeclareCustomerParcelNumber(command.DeclaredParcelIDs[0],
		mustPS(t, psdomain.NewExternalIdentifierNumber, number))
	if err != nil {
		t.Fatalf("配对：%v", err)
	}
	command.CustomerParcelNumbers = []psdomain.CustomerParcelNumberDeclaration{declaration}
	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		result, err := fixture.submitHandler.Handle(txCtx, command)
		outcome = result.Outcome()
		return err
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("submit outcome = %q, want SUBMITTED", outcome)
	}
}

// intakeIntentSources 交回各封收寄意图载荷里的来源标识。
func intakeIntentSources(t *testing.T, db *bentopg.DB) []string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 ORDER BY event_id`, nodeIntakeFormedType)
	if err != nil {
		t.Fatalf("读收寄意图：%v", err)
	}
	defer rows.Close()
	var sources []string
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("扫收寄意图：%v", err)
		}
		var payload struct {
			SourceID string `json:"sourceId"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("收寄意图载荷：%v", err)
		}
		sources = append(sources, payload.SourceID)
	}
	return sources
}

// ringAgain 照 PS 门铃的形状再入队一封同一个号的门铃（另一条关联形成时就是这样）。
func ringAgain(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	payload, err := json.Marshal(map[string]string{"tenantId": tenant, "number": recheckMark})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	at := beatInstant()
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		return store.Enqueue(txCtx, eventing.Envelope{
			SpecVersion: eventing.SpecVersion, ID: "SYN-DOORBELL-NRS03-AGAIN", Source: "idp-parcel/parcel-shipment",
			Type: recheckDoorbellType, Version: 1, Scope: tenant, Subject: recheckMark, PartitionKey: "SYN-DOORBELL-NRS03-AGAIN",
			OccurredAt: at, RecordedAt: at, ContentType: eventing.JSONContentType, Payload: payload,
		})
	})
}
