package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
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
	seedSYNGrant(t, db, pcdomain.ManualReviewAction, synReviewLevel, synReviewScope, synReviewRuleObject)
}

// seedSYNGrant 是 seedSYNReviewGrant 的配方按动作展开：哪一种运营决定的授权规则都按同一条生产写口登记，
// 只有动作、权限等级、商业范围与规则对象各自不同，法人与版本沿用复核那一组合成坐标。
func seedSYNGrant(t *testing.T, db *bentopg.DB, action pcdomain.AuthorizedAction, level, scope, ruleObject string) {
	t.Helper()
	label := strings.TrimPrefix(ruleObject, "SYN-")

	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+label),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-"+label),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustValue(t, pcdomain.NewTenantID, submissionCommand(t).Identity.TenantID().String()),
		Kind:          pcdomain.AuthorizationRuleObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, ruleObject),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, synReviewRuleVersion),
		Scope:         mustValue(t, pcdomain.NewCommercialScopeReference, scope),
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:"+ruleObject),
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
		action,
		mustValue(t, pcdomain.NewLegalEntityReference, synReviewLegalEntity),
		mustValue(t, pcdomain.NewAuthorityLevel, level),
		mustValue(t, pcdomain.NewCommercialScopeReference, scope),
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
		t.Fatalf("登记合成授权：%v", err)
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

// channelOperatorAuthenticator 是操作者渠道认证那一段的替身：令牌即操作者 sub，租户取提交夹具那份委托的租户，只放行
// 构造时指名的那一种运营决定。令牌、操作者册与准入范围那一段与各口的答复代数无关，它们的答复格另由
// TestSwappedDecisionFacesAnswerFromTheOperatorChannel 一类用例在真装配上取证；这里要证的是 Intake 之后那一段。
type channelOperatorAuthenticator struct {
	tenant   domain.TenantID
	decision shipmenthttp.OperatorDecision
}

func (fake channelOperatorAuthenticator) AuthenticateOperatorDecision(
	_ context.Context,
	token string,
	decision shipmenthttp.OperatorDecision,
) (shipmenthttp.OperatorIdentity, error) {
	if token == "" {
		return shipmenthttp.OperatorIdentity{}, shipmenthttp.ErrOperatorCredentialRejected
	}
	if decision != fake.decision {
		return shipmenthttp.OperatorIdentity{}, shipmenthttp.ErrOperatorNotGranted
	}
	return shipmenthttp.OperatorIdentity{Tenant: fake.tenant, Operator: "https://id.syn.example/dex#" + token}, nil
}

// Covers: 票 operator-channel/34 与 ADR-0132 决定二「撞上换代答`版本已换代`」、ADR-0045 决定三（新版本携带自己的判断
// 任务）——经生产的操作者 Intake 与真寻址（委托查阅适配器），复核授权只有请求映射是合成替身：V1 记下复核之后受控补充形成
// V2，同一个指名 V1 的复核 POST 原样重投（超时重投，或操作员还开着 V1 那一页），答 200 `VERSION_SUPERSEDED` 并报出 V2；
// V2 的任务上不落完成，「复核已完成」信封仍是那一封。此前 Intake 拿服务端当前版顶替，这一投会答 201 `RECORDED`，把 V1
// 的复核人和证据签到 V2 的任务上，再铸一封信。编排直调、命令指名 V1 的对照在那时也答`版本已换代`，辨别不出这条缺陷，
// 所以这里只走 Intake。测试输入是合成数据，只记 `S`。
func TestAReviewNamingASupersededVersionIsNotSignedOntoTheNewVersionThroughTheOperatorIntake(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	version1 := submittedRequestOnRealAssembly(t, db)
	seedSYNReviewGrant(t, db)
	review, err := manualReviewOrchestrationWith(db, synRReviewRequestSource{t: t})
	if err != nil {
		t.Fatalf("装配复核完成编排：%v", err)
	}
	views, err := pspostgres.NewShipmentRequestViews(db)
	if err != nil {
		t.Fatalf("构造委托查阅读口：%v", err)
	}
	command := submissionCommand(t)
	intake, err := shipmenthttp.NewOperatorDecisionIntake(
		channelOperatorAuthenticator{tenant: command.Identity.TenantID(), decision: shipmenthttp.OperatorDecisionManualReviewCompletion},
		views, unwiredDecisionTargets{})
	if err != nil {
		t.Fatalf("构造操作者 Intake：%v", err)
	}
	endpoint := shipmenthttp.NewCompleteManualReviewEndpoint(intake, review)

	type answer struct {
		Outcome        string `json:"outcome"`
		Reviewer       string `json:"reviewer"`
		CurrentVersion string `json:"currentVersion"`
	}
	post := func(t *testing.T, body string) (int, answer) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/shipment-requests/manual-review-completions", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer SYN-OPERATOR-01")
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var got answer
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatalf("译响应：%v（%s）", err, response.Body.String())
		}
		return response.Code, got
	}
	namingV1 := `{"shipmentRequestId": "` + command.ShipmentRequestID.String() + `", "submissionVersionId": "` + version1.String() +
		`", "reason": "SYN-REVIEW-REASON-1"}`

	if status, got := post(t, namingV1); status != http.StatusCreated || got.Outcome != "RECORDED" {
		t.Fatalf("V1 上的复核答 %d %+v，want 201 RECORDED", status, got)
	}
	if got := envelopeCountOfType(t, db, resumeEnvelopeType); got != 1 {
		t.Fatalf("V1 复核落库后「复核已完成」信封 = %d 封, want 恰好 1", got)
	}

	supplement, err := buildCustomerSupplementOrchestration(db)
	if err != nil {
		t.Fatalf("装配受控补充编排：%v", err)
	}
	supplemented, err := supplement.Handle(t.Context(), supplementCommand(t, version1, "SYN-KEY-1-SUPPLEMENT-OC34", "syn-digest-oc34"))
	if err != nil {
		t.Fatalf("受控补充：%v", err)
	}
	formed, present := supplemented.Version()
	if supplemented.Outcome() != shipmentapp.SupplementRecorded || !present || formed.VersionID() == version1 {
		t.Fatalf("受控补充 = %q，新版本 %v present=%v——前提不成立，V2 没有形成", supplemented.Outcome(), formed.VersionID(), present)
	}
	version2 := formed.VersionID()

	status, replay := post(t, namingV1)
	if status != http.StatusOK || replay.Outcome != "VERSION_SUPERSEDED" || replay.CurrentVersion != version2.String() {
		t.Fatalf("V2 形成之后重投指名 V1 的复核答 %d %+v，want 200 VERSION_SUPERSEDED 且 currentVersion = %q", status, replay, version2)
	}
	if got := envelopeCountOfType(t, db, resumeEnvelopeType); got != 1 {
		t.Fatalf("重投之后「复核已完成」信封 = %d 封——V1 的复核又驱了 V2 一拍", got)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), command.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	task := request.AcceptanceDecisionTask()
	if task.SubmissionVersionID() != version2 {
		t.Fatalf("当前判断任务挂在 %q，want V2 %q", task.SubmissionVersionID(), version2)
	}
	if completion, done := task.ManualReviewCompletion(); done {
		t.Fatalf("V2 的任务上落了完成（复核人 %q，证据 %q）——V1 的复核签到了 V2 头上", completion.Reviewer(), completion.Evidence())
	}
}

// 拒绝授权的合成坐标（`PAR-COM-14` 实例半边），法人沿用复核那一组。
const (
	synRejectionLevel      = "SYN-LEVEL-REJECTION"
	synRejectionScope      = "SYN-SCOPE-REJECTION-1"
	synRejectionRuleObject = "SYN-REJECTION-RULE-1"
)

// synRRejectionRequestSource 把拒绝授权询问折成一份 PC 授权请求，形照 synRReviewRequestSource。询问不带证据，证据格取
// 合成条目。
type synRRejectionRequestSource struct{ t *testing.T }

func (source synRRejectionRequestSource) FormAuthorizationRequest(
	_ context.Context,
	query psports.ActiveRejectionAuthorizationQuery,
) (pcdomain.AuthorizationRequest, bool, error) {
	source.t.Helper()
	request, err := pcdomain.NewAuthorizationRequest(
		pcdomain.ActiveRejectionAction,
		mustValue(source.t, pcdomain.NewLegalEntityReference, synReviewLegalEntity),
		mustValue(source.t, pcdomain.NewAuthorityLevel, synRejectionLevel),
		mustValue(source.t, pcdomain.NewCommercialScopeReference, synRejectionScope),
		mustValue(source.t, pcdomain.NewStructuredReason, query.Reason.String()),
		mustValue(source.t, pcdomain.NewEvidenceReference, "SYN-REJECTION-EVIDENCE-1"),
		synReviewAt,
	)
	if err != nil {
		return pcdomain.AuthorizationRequest{}, false, err
	}
	return request, true, nil
}

var _ psparty.ActiveRejectionRequestSource = synRRejectionRequestSource{}

func seedSYNRejectionGrant(t *testing.T, db *bentopg.DB) {
	t.Helper()
	seedSYNGrant(t, db, pcdomain.ActiveRejectionAction, synRejectionLevel, synRejectionScope, synRejectionRuleObject)
}

// decisionAnswer 是运营决定各口响应里本组用例要读的那几格。
type decisionAnswer struct {
	Outcome         string `json:"outcome"`
	DecisionKind    string `json:"decisionKind"`
	Decider         string `json:"decider"`
	RejectionReason string `json:"rejectionReason"`
	PendingReason   string `json:"pendingReason"`
}

// postDecision 以 token 为操作者向 endpoint 投一份运营决定，交回状态码与读出的答复。
func postDecision(t *testing.T, endpoint http.Handler, path, token, body string) (int, decisionAnswer) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var got decisionAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("译响应：%v（%s）", err, response.Body.String())
	}
	return response.Code, got
}

// rejectionEndpointOnRealAssembly 经生产的操作者 Intake 与真寻址接上主动拒绝口，拒绝授权只有请求映射是合成替身。
func rejectionEndpointOnRealAssembly(t *testing.T, db *bentopg.DB) http.Handler {
	t.Helper()
	rejection, err := rejectionOrchestrationWith(db, synRRejectionRequestSource{t: t})
	if err != nil {
		t.Fatalf("装配主动拒绝编排：%v", err)
	}
	views, err := pspostgres.NewShipmentRequestViews(db)
	if err != nil {
		t.Fatalf("构造委托查阅读口：%v", err)
	}
	intake, err := shipmenthttp.NewOperatorDecisionIntake(
		channelOperatorAuthenticator{tenant: submissionCommand(t).Identity.TenantID(), decision: shipmenthttp.OperatorDecisionActiveRejection},
		views, unwiredDecisionTargets{})
	if err != nil {
		t.Fatalf("构造操作者 Intake：%v", err)
	}
	return shipmenthttp.NewRejectShipmentRequestEndpoint(intake, rejection)
}

func rejectionBody(t *testing.T, reason string) string {
	t.Helper()
	return `{"shipmentRequestId": "` + submissionCommand(t).ShipmentRequestID.String() + `", "reason": "` + reason + `"}`
}

const rejectionPath = "/shipment-requests/rejections"

// synFirstDecider 是令牌 SYN-OPERATOR-01 经 channelOperatorAuthenticator 认出的操作者，Intake 把它记作实际决定方。
const synFirstDecider = "https://id.syn.example/dex#SYN-OPERATOR-01"

// Covers: 票 operator-channel/36 与 ADR-0132 决定二「撞上已成立的决定答`任务已完结`并交回那一个」、ADR-0022「新建了用
// `201`，其余已形成的答案用 `200`」——经生产的操作者 Intake 与真库，同一拒绝原样重放、换人换原因的后到者都答 200
// `TASK_ALREADY_CLOSED`，回显先到那一份的决定方与原因，`acceptance_processing_attempt` 不增加；复核完成撞上这份已拒绝的
// 委托同样答`任务已完结`并交回那份决定。此前后到者答 201 `FORMED`，把没被采纳的拒绝读成成功。测试输入是合成数据，只记 `S`。
func TestARejectionReplayAnswersTaskAlreadyClosedThroughTheOperatorIntake(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	version := submittedRequestOnRealAssembly(t, db)
	seedSYNRejectionGrant(t, db)
	endpoint := rejectionEndpointOnRealAssembly(t, db)

	status, first := postDecision(t, endpoint, rejectionPath, "SYN-OPERATOR-01", rejectionBody(t, "SYN-REJECTION-REASON-1"))
	if status != http.StatusCreated || first.Outcome != "FORMED" || first.DecisionKind != "REJECTED" {
		t.Fatalf("首次拒绝答 %d %+v，want 201 FORMED REJECTED", status, first)
	}
	attempts := processingAttemptCount(t, db)

	for name, replay := range map[string]struct{ token, reason string }{
		"原样重放":  {"SYN-OPERATOR-01", "SYN-REJECTION-REASON-1"},
		"换人换原因": {"SYN-OPERATOR-02", "SYN-REJECTION-REASON-2"},
	} {
		status, got := postDecision(t, endpoint, rejectionPath, replay.token, rejectionBody(t, replay.reason))
		if status != http.StatusOK || got.Outcome != "TASK_ALREADY_CLOSED" || got.DecisionKind != "REJECTED" {
			t.Fatalf("%s答 %d %+v，want 200 TASK_ALREADY_CLOSED REJECTED", name, status, got)
		}
		if got.Decider != synFirstDecider || got.RejectionReason != "SYN-REJECTION-REASON-1" {
			t.Fatalf("%s回显决定方 %q 原因 %q，want 先到那一份 %q / SYN-REJECTION-REASON-1", name, got.Decider, got.RejectionReason, synFirstDecider)
		}
	}
	if got := processingAttemptCount(t, db); got != attempts {
		t.Fatalf("重放之后处理尝试 %d 行，want 仍是 %d——读回既有决定不是一轮未决", got, attempts)
	}

	seedSYNReviewGrant(t, db)
	review, err := manualReviewOrchestrationWith(db, synRReviewRequestSource{t: t})
	if err != nil {
		t.Fatalf("装配复核完成编排：%v", err)
	}
	completed, err := review.Handle(t.Context(), reviewCompletionCommand(t, version, "1"))
	if err != nil {
		t.Fatalf("完成复核：%v", err)
	}
	decision, present := completed.AcceptanceDecision()
	if completed.Outcome() != shipmentapp.ManualReviewTaskAlreadyClosed || !present || decision.Accepted() {
		t.Fatalf("复核撞上已拒绝委托 = %q（decision present=%v），want TASK_ALREADY_CLOSED 并交回那份拒绝", completed.Outcome(), present)
	}
}

// Covers: 票 operator-channel/36——自动接受先越过决定边界之后的主动拒绝，经生产的操作者 Intake 答 200 `TASK_ALREADY_CLOSED`
// 并交回那份接受；接受没有主动拒绝留痕，不回显决定方与原因。此前这一投答 201 `FORMED`。测试输入是合成数据，只记 `S`。
func TestARejectionAfterAnAutomaticAcceptanceAnswersTaskAlreadyClosed(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptedOnRealAssembly(t, db)
	seedSYNRejectionGrant(t, db)
	endpoint := rejectionEndpointOnRealAssembly(t, db)

	status, got := postDecision(t, endpoint, rejectionPath, "SYN-OPERATOR-01", rejectionBody(t, "SYN-REJECTION-REASON-1"))
	if status != http.StatusOK || got.Outcome != "TASK_ALREADY_CLOSED" || got.DecisionKind != "ACCEPTED" {
		t.Fatalf("接受之后的拒绝答 %d %+v，want 200 TASK_ALREADY_CLOSED ACCEPTED", status, got)
	}
	if got.Decider != "" || got.RejectionReason != "" {
		t.Fatalf("接受上回显了拒绝留痕：决定方 %q 原因 %q", got.Decider, got.RejectionReason)
	}
	if n := processingAttemptCount(t, db); n != 0 {
		t.Fatalf("处理尝试 %d 行，want 0", n)
	}
}

// Covers: 票 15 完成判据「生产装配如实停在该口今天的格」——拒绝授权映射为 nil 时授权仍排最前：对一份已拒绝的委托，生产
// 装配照旧答`未决`/`REJECTION_AUTHORITY_UNAVAILABLE`，不因委托已决就绕过授权去交回决定。没有授权的操作者不能拿重放探册
// （ADR-0183 复核记录二）。
func TestTheProductionRejectionAssemblyStaysUndecidedOnARejectedRequest(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	version := submittedRequestOnRealAssembly(t, db)
	seedSYNRejectionGrant(t, db)
	endpoint := rejectionEndpointOnRealAssembly(t, db)
	if status, got := postDecision(t, endpoint, rejectionPath, "SYN-OPERATOR-01", rejectionBody(t, "SYN-REJECTION-REASON-1")); status != http.StatusCreated {
		t.Fatalf("前提不成立：首次拒绝答 %d %+v", status, got)
	}

	production, err := buildRejectionOrchestration(db)
	if err != nil {
		t.Fatalf("装配主动拒绝编排：%v", err)
	}
	command := submissionCommand(t)
	result, err := production.Handle(t.Context(), shipmentapp.RejectShipmentRequestCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: version,
		Decider:           mustValue(t, domain.NewDeciderReference, "SYN-OPERATOR-02"),
		Reason:            mustValue(t, domain.NewRejectionReasonReference, "SYN-REJECTION-REASON-2"),
		Evidence:          mustValue(t, domain.NewRejectionEvidenceReference, "SYN-REJECTION-EVIDENCE-2"),
	})
	if err != nil {
		t.Fatalf("拒绝：%v", err)
	}
	if result.Outcome() != shipmentapp.ActiveRejectionUndecided || result.PendingReason() != shipmentapp.RejectionAuthorityUnavailable {
		t.Fatalf("生产装配答 %q / %q，want UNDECIDED / REJECTION_AUTHORITY_UNAVAILABLE", result.Outcome(), result.PendingReason())
	}
}

func processingAttemptCount(t *testing.T, db *bentopg.DB) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(), `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt`).Scan(&count); err != nil {
		t.Fatalf("统计处理尝试行数：%v", err)
	}
	return count
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
