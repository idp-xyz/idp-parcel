package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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
	pshandoff "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/productionhandoff"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证只因歧义信号落下的身份冲突，在声明它的委托被拒绝或撤回之后，经生产派发装配自动解开（ADR-0211，
// 票 no-reception-identity/07）：PS 委托仓储在状态变化那一笔交声明撤除门铃，生产派发器一拍投给 NO 的重核消费者，重核追加关联版。
// 夹具全为合成（S 级）。

const (
	releaseRetiredType = "parcel-shipment.customer-parcel-number-declaration.retired"
	releaseMark        = "SYN-CUSTOMER-PARCEL-NRI07"
)

// Covers: 路由表认得出 PS 声明撤除这一类，投给 NO 的重核消费门。手法同关联已形成那一条：毒丸载荷让消费门显式拒收入账并交回 nil，
// 于是这一条定稿；漏挂或挂错撞的是无订阅者。
func TestARetiredDeclarationReachesTheRecheckConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "declaration-retired-1", releaseRetiredType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把声明撤除门铃投给重核消费门",
			published, recordedFailureCode(t, db, "declaration-retired-1"))
	}
}

// Covers: NO 消费者自写的声明撤除门铃类型串、PS outbox 适配器写出的那一个与本文件用的字面三者相等（消费者不 import 提供方，两串
// 各写一份，由这条对照用例钉住，同关联已形成那一条）。
func TestTheRecheckConsumerAndThePSAgreeOnTheRetiredDeclarationType(t *testing.T) {
	if string(noinbox.CustomerParcelNumberDeclarationRetiredEventType) != pspostgres.CustomerParcelNumberDeclarationRetiredEventType ||
		pspostgres.CustomerParcelNumberDeclarationRetiredEventType != releaseRetiredType {
		t.Fatalf("NO 消费者认 %q，PS 写出 %q，用例按 %q", noinbox.CustomerParcelNumberDeclarationRetiredEventType,
			pspostgres.CustomerParcelNumberDeclarationRetiredEventType, releaseRetiredType)
	}
}

// Covers: `AT-NO-062` 的真库面，经生产装配：号码在册里只标识客户一那件包裹（委托已接受），客户二的委托也报了它、只提交未接受 →
// 贴着它的实物送站，落成身份冲突、来由只因歧义信号 → 客户二的委托被拒绝（或撤回），委托仓储那一笔交一封声明撤除门铃 → 生产
// 派发器把门铃投给 NO 的重核消费者 → 重核追加一版`节点收寄已形成`：关联那件包裹、前版是冲突那一版、业务时间与控制照原收寄，
// 收寄意图恰一封、指回追加版；方向依据的身份一格判成立、依追加版。冲突那一版原样留着。
//
// 送站带着一张客户一与客户二都已确认的送站交接单：ADR-0212 决定四之后客户包裹件号的唯一识别要有可验证交付范围，有范围时信号只认
// 范围内客户账户的声明（决定八），客户二在范围里，它的声明才使这一件只因歧义信号冲突。没有范围的那一格见
// TestTheWiredDispatchReleasesAnUnscopedAmbiguityIntoAnUnverifiedPendingUnit。
func TestTheWiredDispatchReleasesAnAmbiguityOnlyConflictOnceTheDeclarationIsGone(t *testing.T) {
	for name, end := range map[string]func(*testing.T, *synVerticalFixture, psdomain.SourceIdentity){
		"被拒绝": rejectTheDeclaringRequest,
		"被撤回": withdrawTheDeclaringRequest,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newSYNVerticalFixture(t)
			ctx := t.Context()
			tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
			receivedAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
			acceptTheHolder(t, fixture)
			declaring := declareTheMarkInAnotherRequest(t, fixture, "B")
			settleTheBeat(t, fixture, declaring)

			reception := wiredReceptionForRelease(t, fixture.db)
			scope := confirmedHandoverNoteForDispatch(t, fixture.db, tenant, receivedAt.Add(-time.Hour),
				fixture.identity.CustomerAccountID().String(), declaring.CustomerAccountID().String())
			contested := receiveMarkedForRelease(t, fixture.db, reception, tenant, "1", receivedAt, scope)
			assertContestedForRelease(t, fixture, contested, "AMBIGUITY_ONLY")
			if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != 0 {
				t.Fatalf("冲突落下时已有 %d 封收寄意图, want 0", n)
			}

			end(t, fixture, declaring)
			retired := fixture.outboxIDsOfType(t, releaseRetiredType)
			if len(retired) != 1 {
				t.Fatalf("委托%s之后声明撤除门铃 %d 封, want 1——状态变化那一笔没入队", name, len(retired))
			}
			dispatchUntilRechecked(t, fixture, retired[0])

			receptions, err := nopostgres.NewReceptions(fixture.db)
			if err != nil {
				t.Fatalf("构造收寄库：%v", err)
			}
			derived := noports.ReceptionKey{TenantID: tenant, SourceID: "IDENTITY-RECHECK/" + contested.JudgmentVersion.String()}
			formed, found, err := receptions.FindByKey(ctx, derived)
			if err != nil || !found {
				t.Fatalf("追加版没落册：found = %v err = %v", found, err)
			}
			association, associated := formed.Intake.Association()
			switch {
			case formed.Kind != noports.RecordIntakeFormed || !associated || association.String() != "SYN-PARCEL-01":
				t.Fatalf("追加版 = %+v, want 节点收寄已形成、关联 SYN-PARCEL-01", formed)
			case formed.Predecessor != contested.JudgmentVersion || formed.IdentityConflict:
				t.Fatalf("追加版前版 %s 冲突 %v, want 接续冲突那一版、不记冲突", formed.Predecessor, formed.IdentityConflict)
			case !formed.Intake.ReceivedAt().Equal(receivedAt) || formed.Control.Basis() != contested.Control.Basis() ||
				!formed.Control.EstablishedAt().Equal(contested.Control.EstablishedAt()):
				t.Fatalf("追加版接收 %s、控制 %s @ %s, want 照原收寄", formed.Intake.ReceivedAt(), formed.Control.Basis(), formed.Control.EstablishedAt())
			}
			if cause := conflictCauseOf(t, fixture, contested.Key.SourceID); cause != "AMBIGUITY_ONLY" {
				t.Fatalf("冲突那一版来由变成了 %q", cause)
			}

			if intents := intakeIntentSources(t, fixture.db); len(intents) != 1 || intents[0] != derived.SourceID {
				t.Fatalf("收寄意图 = %v, want 恰一封、指回追加版 %s", intents, derived.SourceID)
			}
			identities, err := nopostgres.NewIntakeIdentities(fixture.db)
			if err != nil {
				t.Fatalf("构造身份读口：%v", err)
			}
			observations, err := identities.FindIntakeIdentities(ctx, tenant, mustNO(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRI07-1"))
			if err != nil {
				t.Fatalf("读收寄身份：%v", err)
			}
			basis, err := nodomain.JudgeIdentityBasis(observations)
			if err != nil {
				t.Fatalf("判身份：%v", err)
			}
			if basis.Cell().Answer() != nodomain.BasisEstablished || basis.IntakeSourceID() != derived.SourceID {
				t.Fatalf("身份一格 = %v（%v）依 %q, want 成立、依追加版", basis.Cell().Answer(), basis.Cell().Reasons(), basis.IntakeSourceID())
			}
		})
	}
}

// Covers: `AT-NO-062` 末句与 ADR-0212 决定六新补那一格的真库面，经生产装配：送站不带依据，号码只经客户包裹件号命中持号那一件、
// 客户二另有一份`已提交`委托也报了它，落只因歧义信号的冲突（没有范围时信号在租户内认）→ 客户二的委托被拒绝，声明撤除门铃投给重核
// 消费者 → 信号没了，却仍没有可验证交付范围：追加一版不记冲突的待识别，候选照记那一件，交付范围核对态未证·没有依据，业务时间与控制
// 照原收寄，不交收寄意图；方向依据的身份一格从此是缺少·待识别，不再显示一个已不存在的冲突。冲突那一版原样留着。
func TestTheWiredDispatchReleasesAnUnscopedAmbiguityIntoAnUnverifiedPendingUnit(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
	receivedAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	acceptTheHolder(t, fixture)
	declaring := declareTheMarkInAnotherRequest(t, fixture, "B")
	settleTheBeat(t, fixture, declaring)

	reception := wiredReceptionForRelease(t, fixture.db)
	contested := receiveMarkedForRelease(t, fixture.db, reception, tenant, "1", receivedAt, nodomain.DeliveryScopeBasisReference{})
	assertContestedForRelease(t, fixture, contested, "AMBIGUITY_ONLY")

	rejectTheDeclaringRequest(t, fixture, declaring)
	retired := fixture.outboxIDsOfType(t, releaseRetiredType)
	if len(retired) != 1 {
		t.Fatalf("拒绝之后声明撤除门铃 %d 封, want 1", len(retired))
	}
	dispatchUntilRechecked(t, fixture, retired[0])

	receptions, err := nopostgres.NewReceptions(fixture.db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	derived := noports.ReceptionKey{TenantID: tenant, SourceID: "IDENTITY-RECHECK/" + contested.JudgmentVersion.String()}
	released, found, err := receptions.FindByKey(ctx, derived)
	if err != nil || !found {
		t.Fatalf("追加版没落册：found = %v err = %v", found, err)
	}
	_, associated := released.Intake.Association()
	check := released.DeliveryScopeCheck
	switch {
	case released.Kind != noports.RecordPendingIdentification || associated || released.IdentityConflict:
		t.Fatalf("追加版 = %+v, want 不记冲突、不关联的待识别", released)
	case len(released.Candidates) != 1 || released.Candidates[0].String() != "SYN-PARCEL-01":
		t.Fatalf("追加版候选 = %v, want 照记 SYN-PARCEL-01", released.Candidates)
	case check.State() != nodomain.DeliveryScopeUnverified || check.Reason() != nodomain.ScopeNoBasis:
		t.Fatalf("追加版交付范围核对态 = %s/%s, want UNVERIFIED/NO_BASIS", check.State(), check.Reason())
	case released.Predecessor != contested.JudgmentVersion || !released.Intake.ReceivedAt().Equal(receivedAt) ||
		released.Control.Basis() != contested.Control.Basis():
		t.Fatalf("追加版前版 %s、接收 %s、控制 %s, want 接续冲突那一版、照原收寄", released.Predecessor, released.Intake.ReceivedAt(), released.Control.Basis())
	}
	if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != 0 {
		t.Fatalf("收寄意图 %d 封, want 0——范围未证不交意图", n)
	}
	if cause := conflictCauseOf(t, fixture, contested.Key.SourceID); cause != "AMBIGUITY_ONLY" {
		t.Fatalf("冲突那一版来由变成了 %q", cause)
	}
	identities, err := nopostgres.NewIntakeIdentities(fixture.db)
	if err != nil {
		t.Fatalf("构造身份读口：%v", err)
	}
	observations, err := identities.FindIntakeIdentities(ctx, tenant, mustNO(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRI07-1"))
	if err != nil {
		t.Fatalf("读收寄身份：%v", err)
	}
	basis, err := nodomain.JudgeIdentityBasis(observations)
	if err != nil {
		t.Fatalf("判身份：%v", err)
	}
	if reasons := basis.Cell().Reasons(); basis.Cell().Answer() != nodomain.BasisMissing ||
		len(reasons) != 1 || reasons[0] != nodomain.ReasonPendingIdentification {
		t.Fatalf("身份一格 = %v %v, want 缺少·待识别", basis.Cell().Answer(), reasons)
	}
}

// Covers: `AT-NO-062` 反向两格，经生产装配：
//   - 拒绝之后仍有第三份`已提交`委托声明同号：声明撤除门铃照样投到、处理完，重核仍答恰一个候选带信号，不追加、不交意图；第三份
//     也撤回之后，它的门铃一到这件就解开——反证前一拍不是没找到这一行；
//   - 一码两物的冲突（来由一码两物）：号码另一份声明随后被拒绝，门铃按号重核找不到它们，两件都不追加，意图不多一封。
//
// 两格的送站都带着可验证交付范围（ADR-0212 决定四）：「第三份声明仍在」带一张持号与两个声明方的客户账户都已确认的交接单，声明都在
// 范围内；「一码两物」带持号那个客户账户的送站预报，号码才唯一识别、走得到一码两物那一支。
func TestTheWiredDispatchKeepsConflictsTheRetiredDeclarationDidNotCause(t *testing.T) {
	t.Run("第三份声明仍在", func(t *testing.T) {
		fixture := newSYNVerticalFixture(t)
		tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
		acceptTheHolder(t, fixture)
		declaring := declareTheMarkInAnotherRequest(t, fixture, "B")
		third := declareTheMarkInAnotherRequest(t, fixture, "C")
		settleTheBeat(t, fixture, declaring, third)

		reception := wiredReceptionForRelease(t, fixture.db)
		at := time.Now().UTC().Add(-10 * time.Minute)
		scope := confirmedHandoverNoteForDispatch(t, fixture.db, tenant, at.Add(-time.Hour), fixture.identity.CustomerAccountID().String(),
			declaring.CustomerAccountID().String(), third.CustomerAccountID().String())
		contested := receiveMarkedForRelease(t, fixture.db, reception, tenant, "1", at, scope)
		assertContestedForRelease(t, fixture, contested, "AMBIGUITY_ONLY")
		rows, intents := fixture.countSQL(t, `SELECT count(*) FROM node_operations.reception`), fixture.countOutboxOfType(t, nodeIntakeFormedType)

		rejectTheDeclaringRequest(t, fixture, declaring)
		retired := fixture.outboxIDsOfType(t, releaseRetiredType)
		if len(retired) != 1 {
			t.Fatalf("拒绝之后声明撤除门铃 %d 封, want 1", len(retired))
		}
		dispatchUntilRechecked(t, fixture, retired[0])
		if after := fixture.countSQL(t, `SELECT count(*) FROM node_operations.reception`); after != rows {
			t.Fatalf("第三份声明仍在，册上 %d → %d 行, want 不追加", rows, after)
		}
		if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != intents {
			t.Fatalf("收寄意图 %d → %d 封, want 不多", intents, n)
		}
		if state := requestState(t, fixture, third); state != psdomain.ShipmentRequestSubmitted {
			t.Fatalf("第三份委托此刻 %q, want 仍是已提交——这一格的前提", state)
		}

		// 反证上面那一拍确实重核过、只是信号还在：第三份也被撤回，它的门铃一到，这件就解开。
		withdrawTheDeclaringRequest(t, fixture, third)
		retired = fixture.outboxIDsOfType(t, releaseRetiredType)
		if len(retired) != 2 {
			t.Fatalf("第三份撤回之后声明撤除门铃 %d 封, want 2", len(retired))
		}
		for _, id := range retired {
			dispatchUntilRechecked(t, fixture, id)
		}
		receptions, err := nopostgres.NewReceptions(fixture.db)
		if err != nil {
			t.Fatalf("构造收寄库：%v", err)
		}
		derived := noports.ReceptionKey{TenantID: tenant, SourceID: "IDENTITY-RECHECK/" + contested.JudgmentVersion.String()}
		if formed, found, err := receptions.FindByKey(t.Context(), derived); err != nil || !found || formed.Kind != noports.RecordIntakeFormed {
			t.Fatalf("最后一份声明也撤下之后追加版 = %+v（found %v err %v）, want 节点收寄已形成", formed, found, err)
		}
	})

	t.Run("一码两物", func(t *testing.T) {
		fixture := newSYNVerticalFixture(t)
		tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
		acceptTheHolder(t, fixture)
		settleTheBeat(t, fixture)

		reception := wiredReceptionForRelease(t, fixture.db)
		at := time.Now().UTC().Add(-10 * time.Minute)
		scope := preAdviseForDispatch(t, fixture.db, tenant, fixture.identity.CustomerAccountID().String(), releaseMark, at)
		early := receiveMarkedForRelease(t, fixture.db, reception, tenant, "EARLY", at, scope)
		if early.Kind != noports.RecordIntakeFormed {
			t.Fatalf("先到那件 = %v, want 节点收寄已形成", early.Kind)
		}
		late := receiveMarkedForRelease(t, fixture.db, reception, tenant, "LATE", at.Add(time.Minute), scope)
		assertContestedForRelease(t, fixture, late, "ONE_MARK_ON_TWO_UNITS")

		declaring := declareTheMarkInAnotherRequest(t, fixture, "B")
		settleTheBeat(t, fixture, declaring)
		rows, intents := fixture.countSQL(t, `SELECT count(*) FROM node_operations.reception`), fixture.countOutboxOfType(t, nodeIntakeFormedType)
		rejectTheDeclaringRequest(t, fixture, declaring)
		retired := fixture.outboxIDsOfType(t, releaseRetiredType)
		if len(retired) != 1 {
			t.Fatalf("拒绝之后声明撤除门铃 %d 封, want 1", len(retired))
		}
		dispatchUntilRechecked(t, fixture, retired[0])
		if after := fixture.countSQL(t, `SELECT count(*) FROM node_operations.reception`); after != rows {
			t.Fatalf("一码两物的冲突被声明撤除门铃动了：册上 %d → %d 行", rows, after)
		}
		if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != intents {
			t.Fatalf("收寄意图 %d → %d 封, want 不多", intents, n)
		}
	})
}

// acceptTheHolder 照 SYN 夹具建下客户一的委托并接受：那件声明包裹配上客户包裹件号 releaseMark，接受那一笔形成关联。
func acceptTheHolder(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	submitWithCustomerParcelNumber(t, fixture, releaseMark)
	fixture.recordPassingJudgments(t, t.Context())
	if result := fixture.formDecision(t, t.Context()); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("持号那一份 state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
}

// declaringRequestIdentities 给另起的委托现签各自的版本与任务标识：夹具的签发器每次签同一个号，第二份委托会与第一份同号。
type declaringRequestIdentities struct{ suffix string }

func (identities declaringRequestIdentities) NextSubmissionVersionID(context.Context) (psdomain.SubmissionVersionID, error) {
	return psdomain.NewSubmissionVersionID("SYN-VER-NRI07-" + identities.suffix)
}

func (identities declaringRequestIdentities) NextAcceptanceDecisionTaskID(context.Context) (psdomain.AcceptanceDecisionTaskID, error) {
	return psdomain.NewAcceptanceDecisionTaskID("SYN-TASK-NRI07-" + identities.suffix)
}

var _ psports.SubmissionIdentityFactory = declaringRequestIdentities{}

// declareTheMarkInAnotherRequest 经提交编排（生产仓储）另建一份委托，来自另一个客户账户，一件声明包裹报了同一个号：只提交，不接受。
func declareTheMarkInAnotherRequest(t *testing.T, fixture *synVerticalFixture, suffix string) psdomain.SourceIdentity {
	t.Helper()
	sources, err := pspostgres.NewSourceSubmissions(fixture.db)
	if err != nil {
		t.Fatalf("构造来源仓储：%v", err)
	}
	handler := psapplication.NewSubmitShipmentRequestHandler(sources, fixture.requests, &synSProductionOwnership{t: t},
		pshandoff.UnconfiguredOtherProductionAuthorityChannel{}, declaringRequestIdentities{suffix: suffix}, systemClock{})
	identity, err := psdomain.NewSourceIdentity(
		fixture.identity.TenantID(),
		mustPS(t, psdomain.NewCustomerAccountID, "SYN-CUSTOMER-NRI07-"+suffix),
		mustPS(t, psdomain.NewSource, "SYN-SOURCE-01"),
		mustPS(t, psdomain.NewSourceRequestKey, "SYN-KEY-NRI07-"+suffix),
	)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	parcel := mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-NRI07-"+suffix)
	declaration, err := psdomain.DeclareCustomerParcelNumber(parcel, mustPS(t, psdomain.NewExternalIdentifierNumber, releaseMark))
	if err != nil {
		t.Fatalf("配对：%v", err)
	}
	command := fixture.submitCommand(t)
	command.Identity = identity
	command.PayloadDigest = mustPS(t, psdomain.NewPayloadDigest, "SYN-DIGEST-NRI07-"+suffix)
	command.BatchID = mustPS(t, psdomain.NewSubmissionBatchID, "SYN-BATCH-NRI07-"+suffix)
	command.ShipmentRequestID = mustPS(t, psdomain.NewShipmentRequestID, "SYN-REQ-NRI07-"+suffix)
	command.DeclaredParcelIDs = []psdomain.DeclaredParcelID{parcel}
	command.CustomerParcelNumbers = []psdomain.CustomerParcelNumberDeclaration{declaration}
	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		result, err := handler.Handle(txCtx, command)
		outcome = result.Outcome()
		return err
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("另一份委托 submit outcome = %q, want SUBMITTED", outcome)
	}
	return identity
}

// settleTheBeat 先把此前入队的信封各拍一遍（委托已提交、接受决定、关联已形成门铃），再断言声明着号的那几份委托仍是`已提交`：
// 本文件的前提是它们没被派发链推到终局，推到了就该另写一格。
func settleTheBeat(t *testing.T, fixture *synVerticalFixture, declaring ...psdomain.SourceIdentity) {
	t.Helper()
	for range 3 {
		if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
	}
	for _, identity := range declaring {
		if state := requestState(t, fixture, identity); state != psdomain.ShipmentRequestSubmitted {
			t.Fatalf("%s 被派发链推到了 %q, want 仍是已提交", identity.RequestKey(), state)
		}
	}
}

func requestState(t *testing.T, fixture *synVerticalFixture, identity psdomain.SourceIdentity) psdomain.ShipmentRequestState {
	t.Helper()
	request, found, err := fixture.requests.FindBySourceIdentity(t.Context(), identity)
	if err != nil || !found {
		t.Fatalf("读回 %s：found = %v err = %v", identity.RequestKey(), found, err)
	}
	return request.State()
}

// rejectTheDeclaringRequest 读回那份委托，按一项失败的合同校验形成拒绝，经真委托仓储落库；事务闭包里不 Fatalf。
func rejectTheDeclaringRequest(t *testing.T, fixture *synVerticalFixture, identity psdomain.SourceIdentity) {
	t.Helper()
	failed, err := psdomain.NewAcceptanceCheck(psdomain.LegalEntityAndContractCheck, psdomain.DeclaredParcelID{},
		psdomain.CheckFailed, mustPS(t, psdomain.NewCheckReason, "SYN-NO-APPLICABLE-BASIS"))
	if err != nil {
		t.Fatalf("失败校验项：%v", err)
	}
	endTheDeclaringRequest(t, fixture, identity, psdomain.ShipmentRequestRejected, func(request psdomain.ShipmentRequest) (psdomain.ShipmentRequest, error) {
		return request.Decide(psdomain.AcceptanceDecisionSpec{
			DecisionID: mustPS(t, psdomain.NewAcceptanceDecisionID, "SYN-DECISION-NRI07-REJECT-"+identity.RequestKey().String()),
			Checks:     []psdomain.AcceptanceCheck{failed},
			DecidedAt:  time.Now().UTC().Add(-time.Second),
		})
	})
}

// withdrawTheDeclaringRequest 读回那份委托，由客户撤回，经真委托仓储落库。
func withdrawTheDeclaringRequest(t *testing.T, fixture *synVerticalFixture, identity psdomain.SourceIdentity) {
	t.Helper()
	endTheDeclaringRequest(t, fixture, identity, psdomain.ShipmentRequestWithdrawn, func(request psdomain.ShipmentRequest) (psdomain.ShipmentRequest, error) {
		return request.WithdrawByCustomer(psdomain.WithdrawalSpec{
			DecisionID: mustPS(t, psdomain.NewAcceptanceDecisionID, "SYN-DECISION-NRI07-WITHDRAW-"+identity.RequestKey().String()),
			Authority:  mustPS(t, psdomain.NewWithdrawalAuthorityReference, "SYN-WITHDRAW-ROLE-NRI07"),
			Requester:  mustPS(t, psdomain.NewWithdrawalRequesterReference, "SYN-CUSTOMER-CONTACT-NRI07"),
			Reason:     mustPS(t, psdomain.NewWithdrawalReasonReference, "SYN-ORDER-CANCELLED"),
			DecidedAt:  time.Now().UTC().Add(-time.Second),
		})
	})
}

func endTheDeclaringRequest(
	t *testing.T,
	fixture *synVerticalFixture,
	identity psdomain.SourceIdentity,
	want psdomain.ShipmentRequestState,
	decide func(psdomain.ShipmentRequest) (psdomain.ShipmentRequest, error),
) {
	t.Helper()
	var state psdomain.ShipmentRequestState
	err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		request, found, err := fixture.requests.FindBySourceIdentity(txCtx, identity)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("委托读不回来")
		}
		ended, err := decide(request)
		if err != nil {
			return err
		}
		state = ended.State()
		saved, err := fixture.requests.Save(txCtx, identity, ended)
		if err != nil {
			return err
		}
		if saved != psports.ShipmentRequestSaved {
			return fmt.Errorf("save outcome = %v", saved)
		}
		return nil
	})
	if err != nil || state != want {
		t.Fatalf("终结委托：state = %q err = %v, want %q", state, err, want)
	}
}

// dispatchUntilRechecked 一拍一拍地派发，直到重核消费者给这封门铃记了账；别的信封（委托已提交那几封在派发链上未决、按退避重投）
// 可能先占了一拍的名额。门铃自己的失败码必须是空的：处理完了，不是记了失败。
func dispatchUntilRechecked(t *testing.T, fixture *synVerticalFixture, eventID string) {
	t.Helper()
	for range 8 {
		if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
		if fixture.countInbox(t, recheckConsumerName, eventID) == 1 {
			if got := recordedFailureCode(t, fixture.db, eventID); got != "" {
				t.Fatalf("门铃失败码 = %q, want 已处理", got)
			}
			return
		}
	}
	t.Fatalf("门铃 %s 八拍之后仍没被重核消费者处理；失败码 = %q", eventID, recordedFailureCode(t, fixture.db, eventID))
}

// wiredReceptionForRelease 用生产适配器装一只收寄编排（与 parcel-api 的收寄口同一组口）。
func wiredReceptionForRelease(t *testing.T, db *bentopg.DB) *noapplication.ReceiveDeliveredUnitHandler {
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
	doorbell, err := nopostgres.NewOutboxIdentityRecheckDoorbell(db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造重核门铃：%v", err)
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
		Downstream: downstream, Doorbell: doorbell, Scope: deliveryScopeEvidenceFor(t, db), Clock: systemClock{},
	})
	if err != nil {
		t.Fatalf("构造收寄编排：%v", err)
	}
	return handler
}

// receiveMarkedForRelease 让一件贴着 releaseMark、带着依据 basis 的实物明确接收，每次一个新来源、一件新实物。
func receiveMarkedForRelease(
	t *testing.T,
	db *bentopg.DB,
	reception *noapplication.ReceiveDeliveredUnitHandler,
	tenant nodomain.TenantID,
	suffix string,
	at time.Time,
	basis nodomain.DeliveryScopeBasisReference,
) noports.ReceptionRecord {
	t.Helper()
	var result noapplication.ReceiveDeliveredUnitResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = reception.Handle(txCtx, noapplication.ReceiveDeliveredUnitCommand{
			TenantID:    tenant,
			SourceID:    "SYN-DELIVERY-NRI07-" + suffix,
			Node:        mustNO(t, nodomain.NewNodeReference, "SYN-NODE-01"),
			DeliveredBy: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-CUSTOMER-01"),
			Unit:        mustNO(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRI07-"+suffix),
			Mark:        noports.ExternalMarkObservation{Mark: releaseMark},
			Claim:       noapplication.ExplicitReception,
			Evidence:    mustNO(t, nodomain.NewReceptionEvidenceReference, "SYN-SIGN-NRI07-"+suffix),
			OccurredAt:  at.Truncate(time.Microsecond),
			ScopeBasis:  basis,
		})
		return err
	})
	record, recorded := result.Record()
	if !recorded {
		t.Fatalf("收寄 %s 没有落册：outcome = %v", suffix, result.Outcome())
	}
	return record
}

// assertContestedForRelease 断言这次送站落成待识别、记身份冲突、候选只记持号那一件，库里记下的来由是 cause。
func assertContestedForRelease(t *testing.T, fixture *synVerticalFixture, record noports.ReceptionRecord, cause string) {
	t.Helper()
	if record.Kind != noports.RecordPendingIdentification || !record.IdentityConflict ||
		len(record.Candidates) != 1 || record.Candidates[0].String() != "SYN-PARCEL-01" {
		t.Fatalf("收寄 %s = %v 冲突 %v 候选 %v, want 待识别、身份冲突、候选 SYN-PARCEL-01",
			record.Key.SourceID, record.Kind, record.IdentityConflict, record.Candidates)
	}
	if got := conflictCauseOf(t, fixture, record.Key.SourceID); got != cause {
		t.Fatalf("收寄 %s 的冲突来由 = %q, want %q", record.Key.SourceID, got, cause)
	}
}

// conflictCauseOf 读库里那一行记下的冲突来由，缺席读成空串。
func conflictCauseOf(t *testing.T, fixture *synVerticalFixture, sourceID string) string {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var cause string
	if err := querier.QueryRow(t.Context(),
		`SELECT COALESCE(identity_conflict_cause, '') FROM node_operations.reception WHERE source_id = $1`, sourceID,
	).Scan(&cause); err != nil {
		t.Fatalf("读冲突来由：%v", err)
	}
	return cause
}
