package bentocontract

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件是 `PBC-02/03/04/05/07` 取证用例共用的 Parcel 侧夹具。
//
// 委托夹具走生产构造路径（SubmitShipmentRequest），不走重建门也不手搓字段：合同要证的
// 是「Parcel 真实聚合经真实适配器过框架合同」，夹具若绕开生产路径，证出来的就不是那个
// 聚合。时间一律取固定 UTC 值：领域时间经快照 JSON 往返后按同一瞬间同一 Location 读回，
// 夹具若带单调钟读数，往返前后的相等性会在比较层碎掉。

var contractInstant = time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)

// contractScope 点名一份夹具属于哪个（租户, 客户账户）作用域。作用域隔离是合同的
// 考点之一，所以它在夹具签名上显式出现，不藏在常量里。
type contractScope struct {
	tenant   string
	customer string
}

// parcelDB 与 frameworkDB 同源：独立数据库、真实迁移计划、框架技术表在 bento schema。
// 另交回连接池，供取证用例在事务外直接数行——「恰好一份委托」这类断言要数库里的行，
// 不能只信编排的答复。
func parcelDB(t *testing.T) (*bentopg.DB, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db, pool
}

func mustBuild[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

func contractIdentity(t *testing.T, scope contractScope, key string) domain.SourceIdentity {
	t.Helper()
	built, err := domain.NewSourceIdentity(
		mustBuild(t, domain.NewTenantID, scope.tenant),
		mustBuild(t, domain.NewCustomerAccountID, scope.customer),
		mustBuild(t, domain.NewSource, "portal"),
		mustBuild(t, domain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	return built
}

func contractFingerprint(t *testing.T, scope contractScope, key, digest string) domain.SourceSubmissionFingerprint {
	t.Helper()
	built, err := domain.NewSourceSubmissionFingerprint(
		contractIdentity(t, scope, key),
		mustBuild(t, domain.NewPayloadDigest, digest),
		contractInstant.Add(-time.Hour),
		contractInstant.Add(-time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	return built
}

// contractAdmissionScope 是夹具共用的完整拟受理范围。
func contractAdmissionScope(t *testing.T) domain.AdmissionScope {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustBuild(t, domain.NewAdmissionScopeReference, "contract-scope-ref"),
		mustBuild(t, domain.NewAdmissionScopeDigest, "contract-scope-digest"),
	)
	if err != nil {
		t.Fatalf("准入范围：%v", err)
	}
	return scope
}

// contractOwnershipDecision 造一份「本产品已被明确选为当前生产权威」的归属决定，
// 有效区间覆盖 contractInstant。它是 Parcel 自有端口（ProductionOwnershipAuthority）
// 的确定性取值，简报明写这类替身替的是 Parcel 的端口而非框架合同，不属「本地框架替身」。
func contractOwnershipDecision(t *testing.T) domain.ProductionOwnershipDecision {
	t.Helper()
	scope := contractAdmissionScope(t)
	validity, err := domain.NewOwnershipValidityInterval(
		contractInstant.Add(-24*time.Hour),
		contractInstant.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	decision, err := domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustBuild(t, domain.NewProductionOwnershipDecisionID, "contract-ownership-1"),
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      mustBuild(t, domain.NewProductionOwnershipRuleVersion, "contract-rule-1"),
		AsOf:             contractInstant,
		Validity:         validity,
		Revision:         mustBuild(t, domain.NewProductionOwnershipRevision, "contract-rev-1"),
		DecisionAt:       contractInstant,
	})
	if err != nil {
		t.Fatalf("归属决定：%v", err)
	}
	return decision
}

// submittedContractRequest 经生产路径建一份带两个成员、一张画像的`已提交`委托。
// 成员与身份都从 requestID 派生，保证不同夹具互不相碰。
func submittedContractRequest(t *testing.T, scope contractScope, key, requestID string) domain.ShipmentRequest {
	t.Helper()

	decision := contractOwnershipDecision(t)
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		contractAdmissionScope(t).Digest(),
		mustBuild(t, domain.NewProductionOwnershipRevision, "contract-rev-1"),
		contractInstant,
	)
	if err != nil {
		t.Fatalf("建单门禁：%v", err)
	}
	candidate, err := domain.NewSubmissionCandidate(
		contractFingerprint(t, scope, key, "digest-1"),
		mustBuild(t, domain.NewSubmissionBatchID, requestID+"-batch"),
		mustBuild(t, domain.NewShipmentRequestID, requestID),
		[]domain.DeclaredParcelID{
			mustBuild(t, domain.NewDeclaredParcelID, requestID+"-p1"),
			mustBuild(t, domain.NewDeclaredParcelID, requestID+"-p2"),
		},
	)
	if err != nil {
		t.Fatalf("提交候选：%v", err)
	}

	weight, err := domain.NewDeclaredWeight(
		mustBuild(t, domain.NewMeasurementValue, "2.5"),
		mustBuild(t, domain.NewMeasurementUnitReference, "kg"))
	if err != nil {
		t.Fatalf("申报重量：%v", err)
	}
	measurement, err := domain.NewDeclaredMeasurement(weight, domain.DeclaredDimensions{})
	if err != nil {
		t.Fatalf("申报测量：%v", err)
	}
	profile, err := domain.NewDeclaredParcelProfile(
		mustBuild(t, domain.NewDeclaredParcelID, requestID+"-p1"), measurement)
	if err != nil {
		t.Fatalf("成员画像：%v", err)
	}

	request, err := domain.SubmitShipmentRequest(domain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   mustBuild(t, domain.NewSubmissionVersionID, requestID+"-ver-1"),
		TaskID:      mustBuild(t, domain.NewAcceptanceDecisionTaskID, requestID+"-task-1"),
		SubmittedAt: contractInstant,
		Profiles:    []domain.DeclaredParcelProfile{profile},
	})
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	return request
}

// withProcessingAttempt 在已提交委托上记一轮处理未决，作为「同一键上的更新后聚合」。
// 它是重建门今天放行的状态里唯一既改内容又不离开`已提交`的领域转移，正合合同
// Update 腿的需要。
func withProcessingAttempt(t *testing.T, base domain.ShipmentRequest) domain.ShipmentRequest {
	t.Helper()
	attempt, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:       mustBuild(t, domain.NewProcessingAttemptReason, "COMMERCIAL_BASIS_UNAVAILABLE"),
		ResumePath:   domain.ResumeByInternalRetry,
		Continuation: mustBuild(t, domain.NewOwnershipContinuationReference, "contract-cont-1"),
		AttemptedAt:  contractInstant.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("处理记录：%v", err)
	}
	updated, err := base.RecordProcessingAttempt(attempt)
	if err != nil {
		t.Fatalf("记处理未决：%v", err)
	}
	return updated
}

// requestSourceIdentity 从聚合自身取回它的来源身份键。合同的 Inserter 只收聚合不收键，
// 而委托聚合本就携带产生它的来源身份——键从聚合派生，两者不可能错位。
func requestSourceIdentity(request domain.ShipmentRequest) domain.SourceIdentity {
	return request.CurrentSubmissionVersion().SourceSubmission().Identity()
}

// contractRequestSpec 从聚合的公开访问器摊回重建规格，Revision 由调用方指定。
//
// 它只覆盖合同夹具会出现的形状：无历史、无关联出处、无判断产物的`已提交`委托——这是
// 本包自用的取证工具，不是第二份 documentOf；越界的形状直接 Fatal，免得比较函数对
// 没摊到的字段静默答「相等」。
func contractRequestSpec(t *testing.T, request domain.ShipmentRequest, revision int64) domain.RehydrateShipmentRequestSpec {
	t.Helper()

	if request.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("合同夹具只该是已提交委托，实得 %d", uint8(request.State()))
	}
	if len(request.PriorSubmissionVersions()) != 0 || len(request.PriorAcceptanceTasks()) != 0 {
		t.Fatal("合同夹具不该携带受控补充历史")
	}
	if _, established := request.PriorRequestLink(); established {
		t.Fatal("合同夹具不该携带关联出处")
	}
	if _, present := request.AcceptanceDecision(); present {
		t.Fatal("合同夹具不该携带接受决定")
	}

	current := request.CurrentSubmissionVersion()
	task := request.AcceptanceDecisionTask()

	taskSpec := domain.RehydrateAcceptanceTaskSpec{
		TaskID:              task.TaskID(),
		SubmissionVersionID: task.SubmissionVersionID(),
		EstablishedAt:       task.EstablishedAt(),
		State:               domain.AcceptanceTaskRunning,
		ProcessingAttempts:  task.ProcessingAttempts(),
	}
	switch {
	case task.IsComplete():
		taskSpec.State = domain.AcceptanceTaskComplete
	case task.IsStopped():
		taskSpec.State = domain.AcceptanceTaskStopped
	}
	if waiting, present := task.WaitingOn(); present {
		taskSpec.WaitingOn = waiting
	}
	if completion, done := task.ManualReviewCompletion(); done {
		taskSpec.ReviewCompletion = completion
	}

	return domain.RehydrateShipmentRequestSpec{
		Revision:          revision,
		ShipmentRequestID: request.ShipmentRequestID(),
		BatchID:           request.BatchID(),
		State:             request.State(),
		SubmittedAt:       request.SubmittedAt(),
		CurrentVersion: domain.RehydrateSubmissionVersionSpec{
			VersionID:         current.VersionID(),
			SourceSubmission:  current.SourceSubmission(),
			DeclaredParcelIDs: current.DeclaredParcelIDs(),
			Profiles:          current.DeclaredProfiles(),
			EstablishedAt:     current.EstablishedAt(),
		},
		AcceptanceTask: taskSpec,
	}
}
