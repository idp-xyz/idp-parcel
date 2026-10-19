package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var receptionDeliveredAt = time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)

// Covers: `/node-operations/receptions` 的第二参是真编排——收寄库、结果版本签发、
// Outbox 意图交付与身份核对（ADR-0184）在真实 PostgreSQL 上装得起来：号码不在关联册
// 里时明确接收答`待识别`，那是「查过了，查无此号」的业务答案，真实建立收寄与控制；
// 不经身份核对的明确拒收支真实落库——重放同一份输入走已有结果，证明首笔事务真的提交
// 了。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredReceptionAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}

	// 号码不在关联册里：身份核对查过了、零个候选，答待识别——真实建立收寄与控制，不是停下。
	unknown := receptionCommand(t, "SYN-DELIVERY-EXPLICIT-1", nodeopsapp.ExplicitReception)
	pending, err := reception.Handle(t.Context(), unknown)
	if err != nil {
		t.Fatalf("明确接收：%v", err)
	}
	if got := pending.Outcome(); got != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("outcome = %v, want PENDING_IDENTIFICATION——查无此号是业务答案", got)
	}
	if record, has := pending.Record(); !has || !record.Control.Active() {
		t.Fatal("待识别没有留下收寄与控制")
	}

	// 拒收换一件实物：同一件实物在同一节点已有当前收寄时，再来的拒收并入它、不形成未收寄（ADR-0199 决定四）。
	refusal := receptionCommand(t, "SYN-DELIVERY-REFUSED-1", nodeopsapp.ExplicitRefusal)
	refusal.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-REFUSED-1")
	first, err := reception.Handle(t.Context(), refusal)
	if err != nil {
		t.Fatalf("明确拒收：%v", err)
	}
	if got := first.Outcome(); got != nodeopsapp.NodeIntakeNotFormed {
		t.Fatalf("outcome = %v, want INTAKE_NOT_FORMED——拒收不经身份核对", got)
	}

	replay, err := reception.Handle(t.Context(), refusal)
	if err != nil {
		t.Fatalf("重放同一份拒收：%v", err)
	}
	if got := replay.Outcome(); got != nodeopsapp.ReceptionExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT——重放没走已有结果，首笔事务的落库没有提交", got)
	}
}

func receptionCommand(
	t *testing.T,
	sourceID string,
	claim nodeopsapp.ReceptionClaimKind,
) nodeopsapp.ReceiveDeliveredUnitCommand {
	t.Helper()
	command := nodeopsapp.ReceiveDeliveredUnitCommand{
		TenantID:    mustValue(t, nodomain.NewTenantID, "SYN-TENANT-1"),
		SourceID:    sourceID,
		Node:        mustValue(t, nodomain.NewNodeReference, "SYN-NODE-ORIGIN"),
		DeliveredBy: mustValue(t, nodomain.NewDeliveringPartyReference, "SYN-CUSTOMER-1"),
		Unit:        mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-1"),
		Mark:        noports.ExternalMarkObservation{Mark: "SYN-BARCODE-1"},
		Claim:       claim,
		OccurredAt:  receptionDeliveredAt,
	}
	switch claim {
	case nodeopsapp.ExplicitReception:
		command.Evidence = mustValue(t, nodomain.NewReceptionEvidenceReference, "SYN-SIGN-1")
	case nodeopsapp.ExplicitRefusal:
		command.Refusal = "SYN-REFUSED-DAMAGED"
	}
	return command
}

// Covers: 票 ps-external-mark-relations/02 完成判据「委托接受后，客户包裹件号逐字在节点识别出那一件包裹：收寄形成、
// 控制建立」的真库面——委托经真提交编排带着客户包裹件号建单、经真仓储接受落库之后，生产装配的收寄编排按这个号码唯一识别出那一件：`节点收寄已形成`、关联到 PS 的那件包裹、控制建立。
// 接受之前同一个号码答`待识别`。ADR-0212 决定四之后客户包裹件号的唯一识别要有可验证交付范围，送站带着那个客户账户的预报（决定十：
// 演示与夹具同一笔改成送站前先交预报）；不带预报的那一格见 TestTheWiredReceptionLeavesAnUnadvisedCustomerNumberPending。夹具全为
// 合成（S 级）。
func TestTheWiredReceptionIdentifiesAnAcceptedParcelByItsCustomerNumber(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	command := submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-0001")
	basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-0001")
	early := receptionCommand(t, "SYN-DELIVERY-EARLY-1", nodeopsapp.ExplicitReception)
	early.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-0001"}
	early.ScopeBasis = basis
	before, err := reception.Handle(t.Context(), early)
	if err != nil {
		t.Fatalf("接受前收寄：%v", err)
	}
	if got := before.Outcome(); got != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("接受前 outcome = %v, want PENDING_IDENTIFICATION——接受前的声明包裹不是正式包裹", got)
	}

	acceptSubmitted(t, db, command)

	arrived := receptionCommand(t, "SYN-DELIVERY-IDENTIFIED-1", nodeopsapp.ExplicitReception)
	arrived.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-2")
	arrived.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-0001"}
	arrived.ScopeBasis = basis
	formed, err := reception.Handle(t.Context(), arrived)
	if err != nil {
		t.Fatalf("接受后收寄：%v", err)
	}
	if got := formed.Outcome(); got != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("接受后 outcome = %v, want INTAKE_FORMED", got)
	}
	record, has := formed.Record()
	if !has || !record.Control.Active() || len(record.Candidates) != 1 || record.Candidates[0].String() != "syn-parcel-1" {
		t.Fatalf("收寄记录 = %+v, want 唯一候选 syn-parcel-1 且控制建立", record)
	}
}

// submitWithCustomerParcelNumber 经真提交编排建一份委托，首件声明包裹配上客户包裹件号 number。
func submitWithCustomerParcelNumber(t *testing.T, db *bentopg.DB, number string) shipmentapp.SubmitShipmentRequestCommand {
	t.Helper()
	command := submissionCommand(t)
	declaration, err := domain.DeclareCustomerParcelNumber(command.DeclaredParcelIDs[0],
		mustValue(t, domain.NewExternalIdentifierNumber, number))
	if err != nil {
		t.Fatalf("配对：%v", err)
	}
	command.CustomerParcelNumbers = []domain.CustomerParcelNumberDeclaration{declaration}
	submitOnRealAssembly(t, db, command)
	return command
}

// acceptSubmitted 经真仓储接受那份委托：接受之后，客户包裹件号才在节点识别得出正式包裹。
func acceptSubmitted(t *testing.T, db *bentopg.DB, command shipmentapp.SubmitShipmentRequestCommand) {
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
