package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件经生产装配 buildReceptionOrchestration 在真库上钉身份核对落到节点上的答法：ADR-0184 决定六「核对不按范围收窄」，
// 与 ADR-0195 的歧义信号。委托经真提交编排带号建单、经真仓储接受或终结落库，节点按现场号码核对。夹具全为合成（S 级）。

// Covers: ADR-0184 决定六与其复核记录「决定六与 UC-NO-002「权限、隔离与审计」」——候选在租户内按号码取，不按交付方
// 挑：两个客户账户的已接受委托用了同一个客户包裹件号，交付方写客户一、客户二还是一个代理交付方，都答待识别、身份冲突、
// 两个候选，控制照常建立。交付方这一格今天只验非空；按它的自报收窄，自报一错，身份冲突就变成唯一关联。
func TestTheWiredReceptionAnswersAConflictWhenTwoAcceptedRequestsShareANumber(t *testing.T) {
	db, reception := wiredReceptionOnRealDatabase(t)
	for _, command := range []shipmentapp.SubmitShipmentRequestCommand{
		numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-SHARED-1", "SYN-PARCEL-SHARED-1", "SYN-SHARED-NUMBER-0001"),
		numberedSubmission(t, "SYN-CUSTOMER-2", "SYN-KEY-SHARED-2", "SYN-PARCEL-SHARED-2", "SYN-SHARED-NUMBER-0001"),
	} {
		submitOnRealAssembly(t, db, command)
		acceptNumbered(t, db, command)
	}

	for _, deliveredBy := range []string{"SYN-CUSTOMER-1", "SYN-CUSTOMER-2", "SYN-AGENT-1"} {
		result := deliverMarked(t, reception, "SHARED-BY-"+deliveredBy, deliveredBy, "SYN-SHARED-NUMBER-0001")
		if got := result.Outcome(); got != nodeopsapp.UnitPendingIdentification {
			t.Fatalf("交付方 %s：outcome = %v, want PENDING_IDENTIFICATION", deliveredBy, got)
		}
		record, has := result.Record()
		if !has || !record.IdentityConflict || !record.Control.Active() {
			t.Fatalf("交付方 %s：收寄记录 = %+v, want 身份冲突且控制建立", deliveredBy, record)
		}
		if got := candidatesOf(record); !slices.Equal(got, []string{"SYN-PARCEL-SHARED-1", "SYN-PARCEL-SHARED-2"}) {
			t.Fatalf("交付方 %s：候选 = %v, want 两个客户账户的两件——核对按交付方收窄了", deliveredBy, got)
		}
	}
}

// Covers: ADR-0195（票 ps-external-mark-relations/05 裁定）——号码在册里只标识一件包裹，同时又被本租户另一份`已提交`
// 委托声明为客户包裹件号：这次唯一命中不算唯一，答待识别、记身份冲突。候选只记在册的那一件，因为声明不进候选；控制
// 照常建立，不关联。本用例由票 04 的刻画用例翻转而来。被翻转的是 ADR-0184 复核记录「决定六与 UC-NO-002
// 「权限、隔离与审计」」记下的代价「其二」里对方已提交的那一格；对方从未报过这个号的那一格不在此列。
//
// 跨客户账户：客户账户二的委托用同号只提交、还没接受。客户账户二的实物送站，不再关联到客户账户一的包裹。交付方写客户二、
// 写客户一还是写一个代理交付方，答案都一样，因为核对不按交付方的自报收窄。客户一自己的那件也一并答身份冲突，这是
// ADR-0195 记下的代价。
// 同一客户账户：两份委托重复用号，只有第一份已接受。第二份的实物送站答身份冲突，同号的另一件再到站也一样；第二份也
// 接受之后照旧是身份冲突，候选变成两件。
func TestTheWiredReceptionAnswersAConflictWhenAnUnacceptedRequestAlsoDeclaresTheNumber(t *testing.T) {
	t.Run("跨客户账户", func(t *testing.T) {
		db, reception := wiredReceptionOnRealDatabase(t)
		accepted := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-HOLDER-1", "SYN-PARCEL-HOLDER-1", "SYN-HOLDER-NUMBER-0001")
		submitOnRealAssembly(t, db, accepted)
		acceptNumbered(t, db, accepted)
		submitOnRealAssembly(t, db,
			numberedSubmission(t, "SYN-CUSTOMER-2", "SYN-KEY-HOLDER-2", "SYN-PARCEL-HOLDER-2", "SYN-HOLDER-NUMBER-0001"))

		for _, deliveredBy := range []string{"SYN-CUSTOMER-2", "SYN-CUSTOMER-1", "SYN-AGENT-1"} {
			result := deliverMarked(t, reception, "HOLDER-BY-"+deliveredBy, deliveredBy, "SYN-HOLDER-NUMBER-0001")
			assertContestedBy(t, result, "SYN-PARCEL-HOLDER-1")
		}
	})

	t.Run("同一客户账户", func(t *testing.T) {
		db, reception := wiredReceptionOnRealDatabase(t)
		first := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-REUSE-1", "SYN-PARCEL-REUSE-1", "SYN-REUSE-NUMBER-0001")
		second := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-REUSE-2", "SYN-PARCEL-REUSE-2", "SYN-REUSE-NUMBER-0001")
		submitOnRealAssembly(t, db, first)
		acceptNumbered(t, db, first)
		submitOnRealAssembly(t, db, second)

		early := deliverMarked(t, reception, "REUSE-SECOND-EARLY", "SYN-CUSTOMER-1", "SYN-REUSE-NUMBER-0001")
		assertContestedBy(t, early, "SYN-PARCEL-REUSE-1")
		another := deliverMarked(t, reception, "REUSE-SECOND-ANOTHER", "SYN-CUSTOMER-1", "SYN-REUSE-NUMBER-0001")
		assertContestedBy(t, another, "SYN-PARCEL-REUSE-1")

		acceptNumbered(t, db, second)
		late := deliverMarked(t, reception, "REUSE-SECOND-LATE", "SYN-CUSTOMER-1", "SYN-REUSE-NUMBER-0001")
		assertContestedBy(t, late, "SYN-PARCEL-REUSE-1", "SYN-PARCEL-REUSE-2")
	})

	t.Run("册里没有、只有声明", func(t *testing.T) {
		// 号码只被`已提交`委托报过、册里一件都不标识：零候选照旧是待识别，不记身份冲突。记了冲突，那一份委托接受之后的
		// 重核就会把它跳过（ADR-0199 决定三「记身份冲突的不重核」）。
		db, reception := wiredReceptionOnRealDatabase(t)
		submitOnRealAssembly(t, db,
			numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-DECLARED-1", "SYN-PARCEL-DECLARED-1", "SYN-DECLARED-NUMBER-0001"))
		submitOnRealAssembly(t, db,
			numberedSubmission(t, "SYN-CUSTOMER-2", "SYN-KEY-DECLARED-2", "SYN-PARCEL-DECLARED-2", "SYN-DECLARED-NUMBER-0001"))

		result := deliverMarked(t, reception, "DECLARED-ONLY", "SYN-CUSTOMER-1", "SYN-DECLARED-NUMBER-0001")
		if got := result.Outcome(); got != nodeopsapp.UnitPendingIdentification {
			t.Fatalf("outcome = %v, want PENDING_IDENTIFICATION", got)
		}
		record, has := result.Record()
		if !has || record.IdentityConflict || len(record.Candidates) != 0 || !record.Control.Active() {
			t.Fatalf("收寄记录 = %+v, want 零候选、不记身份冲突、控制建立——声明不进候选，零候选不因信号变成冲突", record)
		}
	})
}

// Covers: ADR-0195 歧义信号只认`已提交`——声明它的委托被拒绝或撤回之后，那份声明不再算歧义；号码在册里唯一标识的那一件
// 照常关联、建立控制。已接受的那一份不经信号：它的号已在册里，是候选。
func TestTheWiredReceptionAssociatesOnceTheOtherDeclarationIsRejectedOrWithdrawn(t *testing.T) {
	for name, end := range map[string]func(*testing.T, *bentopg.DB, shipmentapp.SubmitShipmentRequestCommand){
		"被拒绝": rejectNumbered,
		"被撤回": withdrawNumbered,
	} {
		t.Run(name, func(t *testing.T) {
			db, reception := wiredReceptionOnRealDatabase(t)
			accepted := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-ENDED-1", "SYN-PARCEL-ENDED-1", "SYN-ENDED-NUMBER-0001")
			submitOnRealAssembly(t, db, accepted)
			acceptNumbered(t, db, accepted)
			other := numberedSubmission(t, "SYN-CUSTOMER-2", "SYN-KEY-ENDED-2", "SYN-PARCEL-ENDED-2", "SYN-ENDED-NUMBER-0001")
			submitOnRealAssembly(t, db, other)

			contested := deliverMarked(t, reception, "ENDED-BEFORE", "SYN-CUSTOMER-1", "SYN-ENDED-NUMBER-0001")
			assertContestedBy(t, contested, "SYN-PARCEL-ENDED-1")

			end(t, db, other)
			result := deliverMarked(t, reception, "ENDED-AFTER", "SYN-CUSTOMER-1", "SYN-ENDED-NUMBER-0001")
			assertAssociatedWith(t, result, "SYN-PARCEL-ENDED-1")
		})
	}
}

func wiredReceptionOnRealDatabase(t *testing.T) (*bentopg.DB, nodeopshttp.ReceptionHandler) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	return db, reception
}

// numberedSubmission 在 submissionCommand 上换掉来源身份与全部现签标识，只给一件成员、配一个客户包裹件号：
// 同一库里的几份委托各是各的，只在号码上相撞。
func numberedSubmission(t *testing.T, customer, key, member, number string) shipmentapp.SubmitShipmentRequestCommand {
	t.Helper()
	command := submissionCommand(t)
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, customer),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	parcel := mustValue(t, domain.NewDeclaredParcelID, member)
	declaration, err := domain.DeclareCustomerParcelNumber(parcel, mustValue(t, domain.NewExternalIdentifierNumber, number))
	if err != nil {
		t.Fatalf("配对：%v", err)
	}
	command.Identity = identity
	command.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "SYN-DIGEST-"+key)
	command.BatchID = mustValue(t, domain.NewSubmissionBatchID, "SYN-BATCH-"+key)
	command.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "SYN-REQUEST-"+key)
	command.DeclaredParcelIDs = []domain.DeclaredParcelID{parcel}
	command.CustomerParcelNumbers = []domain.CustomerParcelNumberDeclaration{declaration}
	return command
}

// acceptNumbered 读回经真提交建下的委托，照 acceptAndSave 的配方接受落库。几份委托共用同一个接受决定标识：册键里
// 被标识对象各不相同，形成依据相同不会把哪一条并掉。
func acceptNumbered(t *testing.T, db *bentopg.DB, command shipmentapp.SubmitShipmentRequestCommand) {
	t.Helper()
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), command.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	acceptAndSave(t, db, requests, command, request)
}

// rejectNumbered 读回经真提交建下的委托，按一项失败的合同校验形成拒绝并经真仓储落库（配方同
// TestASubmissionReadsBackARuleRejectedRequest）。
func rejectNumbered(t *testing.T, db *bentopg.DB, command shipmentapp.SubmitShipmentRequestCommand) {
	t.Helper()
	failed, err := domain.NewAcceptanceCheck(domain.LegalEntityAndContractCheck, domain.DeclaredParcelID{},
		domain.CheckFailed, mustValue(t, domain.NewCheckReason, "SYN-NO-APPLICABLE-BASIS"))
	if err != nil {
		t.Fatalf("失败校验项：%v", err)
	}
	endNumbered(t, db, command, domain.ShipmentRequestRejected, func(request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
		return request.Decide(domain.AcceptanceDecisionSpec{
			DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-REJECT-1"),
			Checks:     []domain.AcceptanceCheck{failed},
			DecidedAt:  time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC),
		})
	})
}

// withdrawNumbered 读回经真提交建下的委托，由客户撤回并经真仓储落库。
func withdrawNumbered(t *testing.T, db *bentopg.DB, command shipmentapp.SubmitShipmentRequestCommand) {
	t.Helper()
	endNumbered(t, db, command, domain.ShipmentRequestWithdrawn, func(request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
		return request.WithdrawByCustomer(domain.WithdrawalSpec{
			DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-WITHDRAW-1"),
			Authority:  mustValue(t, domain.NewWithdrawalAuthorityReference, "SYN-WITHDRAW-ROLE-1"),
			Requester:  mustValue(t, domain.NewWithdrawalRequesterReference, "SYN-CUSTOMER-CONTACT-1"),
			Reason:     mustValue(t, domain.NewWithdrawalReasonReference, "SYN-ORDER-CANCELLED"),
			DecidedAt:  time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC),
		})
	})
}

// endNumbered 在一笔事务里读回委托、按 decide 推到终局并保存；事务闭包里不 Fatalf，断言留到提交之后。
func endNumbered(
	t *testing.T,
	db *bentopg.DB,
	command shipmentapp.SubmitShipmentRequestCommand,
	want domain.ShipmentRequestState,
	decide func(domain.ShipmentRequest) (domain.ShipmentRequest, error),
) {
	t.Helper()
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	var state domain.ShipmentRequestState
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		request, found, err := requests.FindBySourceIdentity(txCtx, command.Identity)
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
		saved, err := requests.Save(txCtx, command.Identity, ended)
		if err != nil {
			return err
		}
		if saved != ports.ShipmentRequestSaved {
			return fmt.Errorf("save outcome = %v", saved)
		}
		return nil
	})
	if err != nil || state != want {
		t.Fatalf("终结委托：state = %q err = %v, want %q", state, err, want)
	}
}

// deliverMarked 让一件贴着 mark 的实物由 deliveredBy 送站并明确接收；每次一个新来源、一件新实物。
func deliverMarked(
	t *testing.T,
	reception nodeopshttp.ReceptionHandler,
	suffix, deliveredBy, mark string,
) nodeopsapp.ReceiveDeliveredUnitResult {
	t.Helper()
	command := receptionCommand(t, "SYN-DELIVERY-"+suffix, nodeopsapp.ExplicitReception)
	command.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-"+suffix)
	command.DeliveredBy = mustValue(t, nodomain.NewDeliveringPartyReference, deliveredBy)
	command.Mark = noports.ExternalMarkObservation{Mark: mark}
	result, err := reception.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("收寄 %s：%v", suffix, err)
	}
	return result
}

func assertAssociatedWith(t *testing.T, result nodeopsapp.ReceiveDeliveredUnitResult, parcel string) {
	t.Helper()
	if got := result.Outcome(); got != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("outcome = %v, want INTAKE_FORMED", got)
	}
	record, has := result.Record()
	if !has || !record.Control.Active() || !slices.Equal(candidatesOf(record), []string{parcel}) {
		t.Fatalf("收寄记录 = %+v, want 唯一候选 %s 且控制建立", record, parcel)
	}
}

// assertContestedBy 断言这次送站答待识别、记身份冲突、控制建立、不关联，候选恰是 parcels（在册的那几件，声明不进候选）。
func assertContestedBy(t *testing.T, result nodeopsapp.ReceiveDeliveredUnitResult, parcels ...string) {
	t.Helper()
	if got := result.Outcome(); got != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("outcome = %v, want PENDING_IDENTIFICATION——号码另有未接受的声明，唯一命中不算唯一", got)
	}
	record, has := result.Record()
	if !has || !record.IdentityConflict || !record.Control.Active() {
		t.Fatalf("收寄记录 = %+v, want 身份冲突且控制建立", record)
	}
	if _, associated := record.Intake.Association(); associated {
		t.Fatalf("收寄记录带了关联：%+v", record.Intake)
	}
	if got := candidatesOf(record); !slices.Equal(got, parcels) {
		t.Fatalf("候选 = %v, want %v——声明不进候选", got, parcels)
	}
}

func candidatesOf(record noports.ReceptionRecord) []string {
	values := make([]string, len(record.Candidates))
	for index, candidate := range record.Candidates {
		values[index] = candidate.String()
	}
	slices.Sort(values)
	return values
}
