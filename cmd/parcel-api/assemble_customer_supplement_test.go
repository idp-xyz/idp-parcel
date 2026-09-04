package main

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证受控补充编排的装配（ADR-0106 Decision 三的出站半边与 Decision 四
// 的生产入口，手法照 assemble_review_test）：生产的 supplementBoundary 真把「新提交版本已形成」
// 信封接上了——新版本落库即恰好一封，重放与基准过期不铸第二封；且生产发布侧铸的那一封生产续办
// 门真的译得出、签回的是新版本。整链闭环（停在受控补充→入账→队列→补充→续办→接受）由
// cmd/parcel-dispatch 的 customer_supplement_resume_loop_test 在链形状上取证，这里钉的是本包装配
// 点自己的壳。

// supplementEnvelopeType 与发布侧适配器的类型常量同字面（手抄，理由同 submittedEnvelopeType）。
const supplementEnvelopeType = "parcel-shipment.shipment-request.submission-version-formed"

// supplementCommand 是对 submissionCommand 那份委托的一次受控补充：补充请求有自己的来源身份
// 与载荷摘要，基准版本由调用方给（客户认为自己在补充的那一版），成员集合不变。
func supplementCommand(
	t *testing.T,
	basis domain.SubmissionVersionID,
	supplementKey, digest string,
) shipmentapp.FormNewSubmissionVersionCommand {
	t.Helper()
	original := submissionCommand(t)
	supplementIdentity, err := domain.NewSourceIdentity(
		original.Identity.TenantID(),
		original.Identity.CustomerAccountID(),
		original.Identity.Source(),
		mustValue(t, domain.NewSourceRequestKey, supplementKey),
	)
	if err != nil {
		t.Fatalf("new supplement identity: %v", err)
	}
	return shipmentapp.FormNewSubmissionVersionCommand{
		Identity:           original.Identity,
		SupplementIdentity: supplementIdentity,
		PayloadDigest:      mustValue(t, domain.NewPayloadDigest, digest),
		OccurredAt:         time.Date(2026, 8, 21, 11, 0, 0, 0, time.UTC),
		ReceivedAt:         time.Date(2026, 8, 21, 11, 0, 1, 0, time.UTC),
		ShipmentRequestID:  original.ShipmentRequestID,
		BasisVersion:       basis,
		DeclaredParcelIDs:  original.DeclaredParcelIDs,
	}
}

// supplementedOnRealAssembly 先经真提交装出一份`已提交`委托（归属用放行替身，理由见
// envelopeMintingSubmission），再经 buildCustomerSupplementOrchestration 的生产编排形成新提交
// 版本。交回补充前的首版、补充的处理结果。
func supplementedOnRealAssembly(
	t *testing.T,
	db *bentopg.DB,
) (domain.SubmissionVersionID, shipmentapp.FormNewSubmissionVersionResult) {
	t.Helper()

	submission, _ := envelopeMintingSubmission(t, db)
	command := submissionCommand(t)
	submitted, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	if got := submitted.Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("outcome = %v, want SUBMITTED", got)
	}

	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), command.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	basis := request.CurrentSubmissionVersion().VersionID()

	supplement, err := buildCustomerSupplementOrchestration(db)
	if err != nil {
		t.Fatalf("装配受控补充编排：%v", err)
	}
	result, err := supplement.Handle(t.Context(), supplementCommand(t, basis, "SYN-KEY-1-SUPPLEMENT", "syn-digest-2"))
	if err != nil {
		t.Fatalf("受控补充：%v", err)
	}
	return basis, result
}

// Covers: ADR-0106 Decision 三的装配证据——生产边界壳携真实 Outbox 意图适配器：新版本落库即恰好
// 一封「新提交版本已形成」信封；同一补充重放答`已处理`不铸第二封；基准过期的补充不形成版本也不
// 铸信封。版本落了库而信封没入队，停等补充的委托就再也没有投递来续办——谁改坏本包的
// supplementBoundary，psinbox 与适配器各自的用例不会红，这里会。
func TestAFormedSubmissionVersionHandsOffExactlyOneResumeEnvelope(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	basis, first := supplementedOnRealAssembly(t, db)
	if got := first.Outcome(); got != shipmentapp.SupplementRecorded {
		t.Fatalf("outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）", got, first.PendingReason())
	}
	formed, present := first.Version()
	if !present || formed.VersionID() == basis {
		t.Fatalf("新版本 = %v present = %v，want 一份不同于基准 %s 的新版本", formed.VersionID(), present, basis)
	}
	if got := envelopeCountOfType(t, db, supplementEnvelopeType); got != 1 {
		t.Fatalf("新版本落库后「新提交版本已形成」信封 = %d 封, want 恰好 1", got)
	}

	supplement, err := buildCustomerSupplementOrchestration(db)
	if err != nil {
		t.Fatalf("装配受控补充编排：%v", err)
	}
	replay, err := supplement.Handle(t.Context(), supplementCommand(t, basis, "SYN-KEY-1-SUPPLEMENT", "syn-digest-2"))
	if err != nil {
		t.Fatalf("重放同一补充：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.SupplementAlreadyHandled {
		t.Fatalf("outcome = %q, want SUPPLEMENT_ALREADY_HANDLED——同一补充身份以相同内容重试返回原结果", got)
	}
	if got := envelopeCountOfType(t, db, supplementEnvelopeType); got != 1 {
		t.Fatalf("重放后信封 = %d 封——同一版本至多形成一次，不得再驱一拍", got)
	}

	stale, err := supplement.Handle(t.Context(), supplementCommand(t, basis, "SYN-KEY-1-SUPPLEMENT-2", "syn-digest-3"))
	if err != nil {
		t.Fatalf("基准过期的补充：%v", err)
	}
	if got := stale.Outcome(); got != shipmentapp.SupplementBasisStale {
		t.Fatalf("outcome = %q, want SUPPLEMENT_BASIS_STALE——补充的是一个已被换代的版本", got)
	}
	if got := envelopeCountOfType(t, db, supplementEnvelopeType); got != 1 {
		t.Fatalf("基准过期后信封 = %d 封——没形成版本就没有可交的信封", got)
	}
}

// Covers: 发布侧铸的续办信封，生产续办门真的译得出，且签回的是**新**版本（缝的性质与
// TestTheMintedResumeEnvelopeDecodesIntoTheAcceptanceChainCommand 相同：字段名漂开的症状是静默
// 毒丸，停在受控补充的委托看起来只是「客户还没补」）。版本那一维单独钉：续办拿旧版本重跑正是
// ADR-0106 Context 第 3 条说的白烧或换代原因，链必须按新版本的任务走。
func TestTheMintedSupplementEnvelopeDecodesIntoTheAcceptanceChainCommand(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	basis, first := supplementedOnRealAssembly(t, db)
	if got := first.Outcome(); got != shipmentapp.SupplementRecorded {
		t.Fatalf("outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）", got, first.PendingReason())
	}
	formed, _ := first.Version()

	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	envelope := claimSupplementEnvelope(t, store)

	inboxStore, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	advancer := &recordingAdvancer{err: errAdvancerProbe}
	consumer, err := psinbox.NewSubmissionVersionFormedConsumer(db.Transactor(), inboxStore, advancer)
	if err != nil {
		t.Fatalf("构造受控补充续办门：%v", err)
	}

	err = consumer.Consume(t.Context(), envelope)
	if errors.Is(err, psinbox.ErrPoisonEnvelope) || err == nil {
		t.Fatalf("生产续办门译不出生产发布侧铸的信封：err = %v；两侧载荷字段名已漂开", err)
	}
	if !errors.Is(err, errAdvancerProbe) {
		t.Fatalf("err = %v, want 探针错误——译码之后该走到编排", err)
	}
	if len(advancer.commands) != 1 {
		t.Fatalf("推进次数 = %d, want 1", len(advancer.commands))
	}

	command := submissionCommand(t)
	got := advancer.commands[0]
	if got.Identity != command.Identity {
		t.Fatalf("来源身份 = %+v, want %+v", got.Identity, command.Identity)
	}
	if got.ShipmentRequestID != command.ShipmentRequestID {
		t.Fatalf("委托 = %q, want %q", got.ShipmentRequestID, command.ShipmentRequestID)
	}
	if got.SubmissionVersion != formed.VersionID() || got.SubmissionVersion == basis {
		t.Fatalf("提交版本 = %q, want 新版本 %q（基准 %q）——续办必须签回新版本", got.SubmissionVersion, formed.VersionID(), basis)
	}
	if len(got.DeclaredParcelIDs) != len(command.DeclaredParcelIDs) {
		t.Fatalf("声明成员 = %v, want %v", got.DeclaredParcelIDs, command.DeclaredParcelIDs)
	}
	for index, parcel := range command.DeclaredParcelIDs {
		if got.DeclaredParcelIDs[index] != parcel {
			t.Fatalf("声明成员[%d] = %q, want %q", index, got.DeclaredParcelIDs[index], parcel)
		}
	}
}

// claimSupplementEnvelope 按派发一拍的同一条认领路径取回「新提交版本已形成」信封（理由同
// claimSubmittedEnvelope）。提交信封与它同分区且先入队，先定稿头一封，认领才轮得到它。
func claimSupplementEnvelope(t *testing.T, store *outbox.Store) eventing.Envelope {
	t.Helper()

	now := time.Now().UTC().Add(time.Minute)
	deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{
		Now:         now,
		Limit:       10,
		LeaseFor:    time.Minute,
		MaxAttempts: 50,
	})
	if err != nil {
		t.Fatalf("认领待发信封：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.Type) == supplementEnvelopeType {
			return delivery.Envelope
		}
		if string(delivery.Envelope.Type) == submittedEnvelopeType {
			if err := store.MarkPublished(t.Context(), delivery.Ref, now); err != nil {
				t.Fatalf("定稿提交信封：%v", err)
			}
		}
	}
	deliveries, err = store.Claim(t.Context(), eventing.OutboxClaim{
		Now:         now.Add(2 * time.Minute),
		Limit:       10,
		LeaseFor:    time.Minute,
		MaxAttempts: 50,
	})
	if err != nil {
		t.Fatalf("二次认领：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.Type) == supplementEnvelopeType {
			return delivery.Envelope
		}
	}
	t.Fatalf("两轮认领都没有「新提交版本已形成」信封")
	return eventing.Envelope{}
}
