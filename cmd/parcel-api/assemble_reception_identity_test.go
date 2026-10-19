package main

import (
	"slices"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件经生产装配 buildReceptionOrchestration 在真库上钉 ADR-0184 决定六「核对不按范围收窄」落到节点上的两格：
// 委托经真提交编排带号建单、经真仓储接受落库，节点按现场号码核对。夹具全为合成（S 级）。

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

// Covers: 这是已知代价的刻画，不是要守的规则。它刻画的是 ADR-0184 复核记录「决定六与 UC-NO-002「权限、隔离与审计」」
// 记下的代价「其二」：号只在一份已接受委托里在册时，别的实物贴着同号送站会唯一命中那一件，形成`节点收寄已形成`并建立
// 控制。票 ps-external-mark-relations/05 裁定之后，本用例按裁定有意翻转，不是回归。
//
// (a) 跨客户账户：客户账户二的委托用同号只提交、还没接受，它的实物送站关联到客户账户一的包裹。
// (b) 同一客户账户的两份委托重复用号：提交入口只在同一草案内查重，第二份照常建单；它的实物唯一关联到第一份的包裹，
// 第二份也接受之后才答身份冲突。
func TestTheWiredReceptionAssociatesTheOnlyRegisteredHolderOfANumber(t *testing.T) {
	t.Run("跨客户账户", func(t *testing.T) {
		db, reception := wiredReceptionOnRealDatabase(t)
		accepted := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-HOLDER-1", "SYN-PARCEL-HOLDER-1", "SYN-HOLDER-NUMBER-0001")
		submitOnRealAssembly(t, db, accepted)
		acceptNumbered(t, db, accepted)
		submitOnRealAssembly(t, db,
			numberedSubmission(t, "SYN-CUSTOMER-2", "SYN-KEY-HOLDER-2", "SYN-PARCEL-HOLDER-2", "SYN-HOLDER-NUMBER-0001"))

		result := deliverMarked(t, reception, "HOLDER-OTHER-ACCOUNT", "SYN-CUSTOMER-2", "SYN-HOLDER-NUMBER-0001")
		assertAssociatedWith(t, result, "SYN-PARCEL-HOLDER-1")
	})

	t.Run("同一客户账户", func(t *testing.T) {
		db, reception := wiredReceptionOnRealDatabase(t)
		first := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-REUSE-1", "SYN-PARCEL-REUSE-1", "SYN-REUSE-NUMBER-0001")
		second := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-REUSE-2", "SYN-PARCEL-REUSE-2", "SYN-REUSE-NUMBER-0001")
		submitOnRealAssembly(t, db, first)
		acceptNumbered(t, db, first)
		submitOnRealAssembly(t, db, second)

		early := deliverMarked(t, reception, "REUSE-SECOND-EARLY", "SYN-CUSTOMER-1", "SYN-REUSE-NUMBER-0001")
		assertAssociatedWith(t, early, "SYN-PARCEL-REUSE-1")

		acceptNumbered(t, db, second)
		late := deliverMarked(t, reception, "REUSE-SECOND-LATE", "SYN-CUSTOMER-1", "SYN-REUSE-NUMBER-0001")
		if got := late.Outcome(); got != nodeopsapp.UnitPendingIdentification {
			t.Fatalf("第二份也接受之后 outcome = %v, want PENDING_IDENTIFICATION", got)
		}
		record, has := late.Record()
		if !has || !record.IdentityConflict ||
			!slices.Equal(candidatesOf(record), []string{"SYN-PARCEL-REUSE-1", "SYN-PARCEL-REUSE-2"}) {
			t.Fatalf("第二份也接受之后收寄记录 = %+v, want 身份冲突、两个候选", record)
		}
	})
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

func candidatesOf(record noports.ReceptionRecord) []string {
	values := make([]string, len(record.Candidates))
	for index, candidate := range record.Candidates {
		values[index] = candidate.String()
	}
	slices.Sort(values)
	return values
}
