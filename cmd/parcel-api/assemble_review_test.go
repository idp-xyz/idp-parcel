package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psparty "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证票 09 复核完成编排的装配（ADR-0086 Decision 二的出站
// 半边，手法照 assemble_submission_test）：生产的 reviewCompletionBoundary 真把「复核
// 已完成」信封接上了——完成落库即恰好一封，重复完成不铸第二封；且生产发布侧铸的那一封
// 生产续办门真的译得出。整链闭环（暂停→队列→续办→接受）由 cmd/parcel-dispatch 的
// manual_review_resume_loop_test 在链形状上取证，这里钉的是本包装配点自己的壳。
//
// 票 wiring-baseline-remainder/04 之后多钉一件：复核授权真的去问了 party-commercial 的授权册
// ——留痕里的授权引用是那边所采用的授权规则版本，不是命令自报的；空册答`授权规则未配置`；
// 生产装配（映射留 nil）停在未形成。四样里只有请求映射是合成 S 替身（实例半边），其余全是
// 生产实现。

// resumeEnvelopeType 与发布侧适配器的类型常量同字面（手抄，理由同 submittedEnvelopeType）。
const resumeEnvelopeType = "parcel-shipment.shipment-request.manual-review-completed"

// 复核授权的合成坐标（`PAR-COM-14` 实例半边）：法人、权限等级、商业范围与业务时点。只出现在
// 本文件，不进生产装配；名字带 SYN 前缀是为了让它们与任何真实登记一眼分得开。
const (
	synReviewLegalEntity = "SYN-LEGAL-1"
	synReviewLevel       = "SYN-LEVEL-REVIEW"
	synReviewScope       = "SYN-SCOPE-REVIEW-1"
	synReviewRuleObject  = "SYN-REVIEW-RULE-1"
	synReviewRuleVersion = "v1"
)

var synReviewAt = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// synRReviewRequestSource 把复核授权询问折成一份 PC 授权请求。动作固定为人工复核；证据取询问
// 自带的那一份（PS 手上有），结构化原因是原因目录里的合成条目——PS 没有它，正是映射该补的那格。
type synRReviewRequestSource struct{ t *testing.T }

func (source synRReviewRequestSource) FormAuthorizationRequest(
	_ context.Context,
	query psports.ManualReviewAuthorizationQuery,
) (pcdomain.AuthorizationRequest, bool, error) {
	source.t.Helper()
	request, err := pcdomain.NewAuthorizationRequest(
		pcdomain.ManualReviewAction,
		mustValue(source.t, pcdomain.NewLegalEntityReference, synReviewLegalEntity),
		mustValue(source.t, pcdomain.NewAuthorityLevel, synReviewLevel),
		mustValue(source.t, pcdomain.NewCommercialScopeReference, synReviewScope),
		mustValue(source.t, pcdomain.NewStructuredReason, "SYN-MANUAL-REVIEW-COMPLETION"),
		mustValue(source.t, pcdomain.NewEvidenceReference, query.Evidence.String()),
		synReviewAt,
	)
	if err != nil {
		return pcdomain.AuthorizationRequest{}, false, err
	}
	return request, true, nil
}

var _ psparty.ManualReviewAuthorizationRequestSource = synRReviewRequestSource{}

// seedSYNReviewGrant 往真授权册里登记一条覆盖合成坐标的人工复核授权，租户取提交命令那一份
// 来源身份的租户——跨上下文只靠这个字面对上。经 PC 领域的重建门与 SaveGrant 走生产写口。
func seedSYNReviewGrant(t *testing.T, db *bentopg.DB) {
	t.Helper()

	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-REVIEW-RULE-1"),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-REVIEW-RULE-1"),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustValue(t, pcdomain.NewTenantID, submissionCommand(t).Identity.TenantID().String()),
		Kind:          pcdomain.AuthorizationRuleObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, synReviewRuleObject),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, synReviewRuleVersion),
		Scope:         mustValue(t, pcdomain.NewCommercialScopeReference, synReviewScope),
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:SYN-REVIEW-RULE-1"),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   approvedAt,
		EffectiveAt:   approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("重建已生效授权规则版本：%v", err)
	}
	grant, err := pcdomain.NewAuthorityGrant(
		version,
		pcdomain.ManualReviewAction,
		mustValue(t, pcdomain.NewLegalEntityReference, synReviewLegalEntity),
		mustValue(t, pcdomain.NewAuthorityLevel, synReviewLevel),
		mustValue(t, pcdomain.NewCommercialScopeReference, synReviewScope),
		interval,
	)
	if err != nil {
		t.Fatalf("授权授予：%v", err)
	}
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		t.Fatalf("构造授权册：%v", err)
	}
	var saved pcports.GrantSaveOutcome
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := grants.SaveGrant(txCtx, grant)
		if err != nil {
			return err
		}
		saved = outcome
		return nil
	})
	if err != nil {
		t.Fatalf("登记合成复核授权：%v", err)
	}
	if saved != pcports.GrantSaved {
		t.Fatalf("登记授权 = %v, want SAVED", saved)
	}
}

// submittedRequestOnRealAssembly 经真提交装出一份`已提交`委托（归属用放行替身，理由见
// envelopeMintingSubmission），交回读回的当前提交版本。
func submittedRequestOnRealAssembly(t *testing.T, db *bentopg.DB) domain.SubmissionVersionID {
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
	return request.CurrentSubmissionVersion().VersionID()
}

func reviewCompletionCommand(t *testing.T, version domain.SubmissionVersionID, suffix string) shipmentapp.CompleteManualReviewCommand {
	t.Helper()
	command := submissionCommand(t)
	return shipmentapp.CompleteManualReviewCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: version,
		Reviewer:          mustValue(t, domain.NewReviewerReference, "syn-reviewer-"+suffix),
		Evidence:          mustValue(t, domain.NewReviewEvidenceReference, "syn-review-evidence-"+suffix),
	}
}

// completedReviewOnRealAssembly 先装出一份`已提交`委托，往真授权册种一条复核授权，再经
// manualReviewOrchestrationWith 的生产编排（只有请求映射是合成替身）完成复核。交回读回的
// 当前提交版本与复核完成的处理结果。
func completedReviewOnRealAssembly(
	t *testing.T,
	db *bentopg.DB,
) (domain.SubmissionVersionID, shipmentapp.CompleteManualReviewResult) {
	t.Helper()

	version := submittedRequestOnRealAssembly(t, db)
	seedSYNReviewGrant(t, db)

	review, err := manualReviewOrchestrationWith(db, synRReviewRequestSource{t: t})
	if err != nil {
		t.Fatalf("装配复核完成编排：%v", err)
	}
	result, err := review.Handle(t.Context(), reviewCompletionCommand(t, version, "1"))
	if err != nil {
		t.Fatalf("完成复核：%v", err)
	}
	return version, result
}

// Covers: ADR-0086 Decision 二的装配证据——生产边界壳携真实 Outbox 意图适配器：复核
// 完成落库即恰好一封「复核已完成」信封；同一版本重复完成答`已有完成`且不铸第二封。
// 完成落了库而信封没入队，停等复核的委托就再也没有投递来续办——谁改坏本包的
// reviewCompletionBoundary，psinbox 与适配器各自的用例不会红，这里会。
//
// 同时钉 UC-PS-001 `AT-PS-034`「只由规则授权的角色按证据完成复核」的接线证据：留痕里的授权
// 引用是真授权册里那条 grant 的 objectID/version，命令没有任何一格能把它写成别的。
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
	completion, present := first.Completion()
	if !present {
		t.Fatal("已记录的完成没带留痕")
	}
	if got, want := completion.Authority().String(), synReviewRuleObject+"/"+synReviewRuleVersion; got != want {
		t.Fatalf("authority = %q, want %q——授权引用必须是 party-commercial 所采用的授权规则版本", got, want)
	}
	if got := envelopeCountOfType(t, db, resumeEnvelopeType); got != 1 {
		t.Fatalf("完成落库后「复核已完成」信封 = %d 封, want 恰好 1", got)
	}

	review, err := manualReviewOrchestrationWith(db, synRReviewRequestSource{t: t})
	if err != nil {
		t.Fatalf("装配复核完成编排：%v", err)
	}
	replay, err := review.Handle(t.Context(), reviewCompletionCommand(t, version, "2"))
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

// Covers: UC-PC-003「没有租户就没有任何授权规则，每一次询问都落在`授权规则未配置`——那是本
// 用例唯一走得到的真实成功路径」：映射在、真授权册为空，编排答未配置，不落库、不铸信封，也
// 不把它说成「你无权复核」。
func TestAReviewAgainstAnEmptyAuthorityBookStaysUndecided(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	version := submittedRequestOnRealAssembly(t, db)
	review, err := manualReviewOrchestrationWith(db, synRReviewRequestSource{t: t})
	if err != nil {
		t.Fatalf("装配复核完成编排：%v", err)
	}

	result, err := review.Handle(t.Context(), reviewCompletionCommand(t, version, "1"))
	if err != nil {
		t.Fatalf("完成复核：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.ManualReviewAuthorityRulesNotConfigured {
		t.Fatalf("outcome = %q, want AUTHORITY_RULES_NOT_CONFIGURED", got)
	}
	if got := envelopeCountOfType(t, db, resumeEnvelopeType); got != 0 {
		t.Fatalf("未配置时信封 = %d 封, want 0", got)
	}
	assertNoReviewCompletionRecorded(t, db)
}

// Covers: 生产装配的实例半边形状——复核授权的请求映射留 nil（`PAR-COM-14` 待提供），编排停在
// 未形成：不落库、不铸信封，也不冒充`授权规则未配置`或`不允许`。与 buildRejectionOrchestration
// 对拒绝授权映射的处置同款；谁把这里的 nil 换成一份「开发用」坐标，这条会先红。
func TestTheProductionReviewAssemblyStopsWithoutAnAuthorityMapping(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	version := submittedRequestOnRealAssembly(t, db)
	seedSYNReviewGrant(t, db)
	review, err := buildManualReviewOrchestration(db)
	if err != nil {
		t.Fatalf("装配复核完成编排：%v", err)
	}

	if _, err := review.Handle(t.Context(), reviewCompletionCommand(t, version, "1")); err == nil {
		t.Fatal("映射未配置时编排仍形成了答案——实例半边被默认掉了")
	}
	if got := envelopeCountOfType(t, db, resumeEnvelopeType); got != 0 {
		t.Fatalf("未形成时信封 = %d 封, want 0", got)
	}
	assertNoReviewCompletionRecorded(t, db)
}

func assertNoReviewCompletionRecorded(t *testing.T, db *bentopg.DB) {
	t.Helper()
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), submissionCommand(t).Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	if _, done := request.AcceptanceDecisionTask().ManualReviewCompletion(); done {
		t.Fatal("没拿到授权的复核完成落了库")
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
