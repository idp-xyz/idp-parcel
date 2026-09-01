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

// 本文件对真实 PostgreSQL 16 证票 09 复核完成编排的装配（ADR-0086 Decision 二的出站
// 半边，手法照 assemble_submission_test）：生产的 reviewCompletionBoundary 真把「复核
// 已完成」信封接上了——完成落库即恰好一封，重复完成不铸第二封；且生产发布侧铸的那一封
// 生产续办门真的译得出。整链闭环（暂停→队列→续办→接受）由 cmd/parcel-dispatch 的
// manual_review_resume_loop_test 在链形状上取证，这里钉的是本包装配点自己的壳。

// resumeEnvelopeType 与发布侧适配器的类型常量同字面（手抄，理由同 submittedEnvelopeType）。
const resumeEnvelopeType = "parcel-shipment.shipment-request.manual-review-completed"

// completedReviewOnRealAssembly 先经真提交装出一份`已提交`委托（归属用放行替身，理由
// 见 envelopeMintingSubmission），再经 buildManualReviewOrchestration 的生产编排完成
// 复核。交回读回的当前提交版本与复核完成的处理结果。
func completedReviewOnRealAssembly(
	t *testing.T,
	db *bentopg.DB,
) (domain.SubmissionVersionID, shipmentapp.CompleteManualReviewResult) {
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
	version := request.CurrentSubmissionVersion().VersionID()

	review, err := buildManualReviewOrchestration(db)
	if err != nil {
		t.Fatalf("装配复核完成编排：%v", err)
	}
	result, err := review.Handle(t.Context(), shipmentapp.CompleteManualReviewCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: version,
		Authority:         mustValue(t, domain.NewReviewAuthorityReference, "syn-review-authority-1"),
		Reviewer:          mustValue(t, domain.NewReviewerReference, "syn-reviewer-1"),
		Evidence:          mustValue(t, domain.NewReviewEvidenceReference, "syn-review-evidence-1"),
	})
	if err != nil {
		t.Fatalf("完成复核：%v", err)
	}
	return version, result
}

// Covers: ADR-0086 Decision 二的装配证据——生产边界壳携真实 Outbox 意图适配器：复核
// 完成落库即恰好一封「复核已完成」信封；同一版本重复完成答`已有完成`且不铸第二封。
// 完成落了库而信封没入队，停等复核的委托就再也没有投递来续办——谁改坏本包的
// reviewCompletionBoundary，psinbox 与适配器各自的用例不会红，这里会。
func TestACompletedReviewHandsOffExactlyOneResumeEnvelope(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	version, first := completedReviewOnRealAssembly(t, db)
	if got := first.Outcome(); got != shipmentapp.ManualReviewCompletionRecorded {
		t.Fatalf("outcome = %q, want RECORDED", got)
	}
	if got := envelopeCountOfType(t, db, resumeEnvelopeType); got != 1 {
		t.Fatalf("完成落库后「复核已完成」信封 = %d 封, want 恰好 1", got)
	}

	review, err := buildManualReviewOrchestration(db)
	if err != nil {
		t.Fatalf("装配复核完成编排：%v", err)
	}
	command := submissionCommand(t)
	replay, err := review.Handle(t.Context(), shipmentapp.CompleteManualReviewCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: version,
		Authority:         mustValue(t, domain.NewReviewAuthorityReference, "syn-review-authority-2"),
		Reviewer:          mustValue(t, domain.NewReviewerReference, "syn-reviewer-2"),
		Evidence:          mustValue(t, domain.NewReviewEvidenceReference, "syn-review-evidence-2"),
	})
	if err != nil {
		t.Fatalf("重复完成：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.ManualReviewCompletionAlreadyDone {
		t.Fatalf("outcome = %q, want ALREADY_COMPLETED——先到的完成留痕在库", got)
	}
	if got := envelopeCountOfType(t, db, resumeEnvelopeType); got != 1 {
		t.Fatalf("重复完成后信封 = %d 封——补签不得再驱一拍", got)
	}
}

// Covers: 发布侧铸的续办信封，生产续办门真的译得出（缝的性质与
// TestTheMintedEnvelopeDecodesIntoTheAcceptanceChainCommand 相同：字段名漂开的症状是
// 静默毒丸，暂停的委托看起来只是「还没人复核完」）。两封信共用译码是消费侧自己的事实，
// 不减这条缝——发布侧是两个各自手抄载荷的适配器，谁改漂第二份都该在这里红。
func TestTheMintedResumeEnvelopeDecodesIntoTheAcceptanceChainCommand(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	version, first := completedReviewOnRealAssembly(t, db)
	if got := first.Outcome(); got != shipmentapp.ManualReviewCompletionRecorded {
		t.Fatalf("outcome = %q, want RECORDED", got)
	}

	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	envelope := claimResumeEnvelope(t, store)

	inboxStore, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	advancer := &recordingAdvancer{err: errAdvancerProbe}
	consumer, err := psinbox.NewManualReviewCompletedConsumer(db.Transactor(), inboxStore, advancer)
	if err != nil {
		t.Fatalf("构造续办门：%v", err)
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
	if got.SubmissionVersion != version {
		t.Fatalf("提交版本 = %q, want %q——续办必须签回复核针对的那份版本", got.SubmissionVersion, version)
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

// claimResumeEnvelope 按派发一拍的同一条认领路径取回续办信封（理由同
// claimSubmittedEnvelope）。提交信封与它同分区且先入队，先定稿头一封，认领才轮得到它。
func claimResumeEnvelope(t *testing.T, store *outbox.Store) eventing.Envelope {
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
		if string(delivery.Envelope.Type) == resumeEnvelopeType {
			return delivery.Envelope
		}
		if string(delivery.Envelope.Type) == submittedEnvelopeType {
			if err := store.MarkPublished(t.Context(), delivery.Ref, now); err != nil {
				t.Fatalf("定稿提交信封：%v", err)
			}
		}
	}
	// 头一封（提交）已定稿，再认领一次轮到续办那封。
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
		if string(delivery.Envelope.Type) == resumeEnvelopeType {
			return delivery.Envelope
		}
	}
	t.Fatalf("两轮认领都没有「复核已完成」信封")
	return eventing.Envelope{}
}

func envelopeCountOfType(t *testing.T, db *bentopg.DB, eventType string) int {
	t.Helper()

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	err = querier.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		eventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计信封行数：%v", err)
	}
	return count
}
