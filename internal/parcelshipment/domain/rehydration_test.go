package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var rehydratedAt = time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

// submittedSnapshot 是一份`已提交`委托在库里的样子：判断产物一个都还没有。
func submittedSnapshot(t *testing.T) domain.RehydrateShipmentRequestSpec {
	t.Helper()
	versionID := mustValue(t, domain.NewSubmissionVersionID, "version-1")
	return domain.RehydrateShipmentRequestSpec{
		Revision:          7,
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		BatchID:           mustValue(t, domain.NewSubmissionBatchID, "batch-1"),
		State:             domain.ShipmentRequestSubmitted,
		SubmittedAt:       rehydratedAt,
		CurrentVersion: domain.RehydrateSubmissionVersionSpec{
			VersionID:        versionID,
			SourceSubmission: sourceFingerprint(t, "tenant-1", "customer-1", "SOURCE-1", "key-1", "sha256:a"),
			DeclaredParcelIDs: []domain.DeclaredParcelID{
				mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
			},
			EstablishedAt: rehydratedAt,
		},
		AcceptanceTask: domain.RehydrateAcceptanceTaskSpec{
			TaskID:              mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-1"),
			SubmissionVersionID: versionID,
			EstablishedAt:       rehydratedAt,
			State:               domain.AcceptanceTaskRunning,
		},
	}
}

// reviewedSnapshot 是一份卡在人工复核上、复核已经做完的`已提交`委托在库里的样子：任务上
// 累了两轮处理记录，复核完成度已经写下，下一次 Decide 就该越过复核那道门。
func reviewedSnapshot(t *testing.T) domain.RehydrateShipmentRequestSpec {
	t.Helper()
	snapshot := submittedSnapshot(t)
	snapshot.AcceptanceTask.WaitingOn = domain.ResumeByManualReview
	snapshot.AcceptanceTask.ProcessingAttempts = []domain.ProcessingAttempt{
		internalAttempt(t, "COMMERCIAL_BASIS_UNAVAILABLE", firstAttemptAt),
		internalAttempt(t, "RECORDED_JUDGMENTS_UNAVAILABLE", firstAttemptAt.Add(time.Hour)),
	}
	snapshot.AcceptanceTask.ReviewCompletion = reviewCompletion(t)
	return snapshot
}

// Covers: ADR-0028「重建只校验，不重算」与「聚合携带版本」— 重建把已判定的产物当数据收下，
// 并把库里读到的版本原样带进聚合，供仓储按预期版本写入。
func TestRehydratingASubmittedRequestKeepsItsRevisionAndProducts(t *testing.T) {
	request, err := domain.RehydrateShipmentRequest(submittedSnapshot(t))
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}

	if request.Revision() != 7 {
		t.Fatalf("revision = %d, want 7", request.Revision())
	}
	if request.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", request.State())
	}
	if request.ShipmentRequestID().String() != "request-1" {
		t.Fatalf("shipment request ID = %q", request.ShipmentRequestID())
	}
	if request.CurrentSubmissionVersion().VersionID().String() != "version-1" {
		t.Fatalf("version ID = %q", request.CurrentSubmissionVersion().VersionID())
	}
	// 已提交意味着判断产物一个都不在场。重建不得凭空造出它们。
	if _, present := request.AcceptanceDecision(); present {
		t.Fatal("重建为一份已提交委托造出了接受决定")
	}
	if _, present := request.AcceptanceBaseline(); present {
		t.Fatal("重建为一份已提交委托造出了接受基线")
	}
}

// Covers: ADR-0028「重建要返回 error……库里读出的东西同样得过一遍不变量」— 逐条跨字段命题。
// 这些组合今天的构造路径造不出来，重建入口一开就全都造得出来，所以这里是它们头一回被写下。
func TestRehydrationRefusesStatesThatTheJudgmentPathCannotProduce(t *testing.T) {
	illegal := map[string]func(*domain.RehydrateShipmentRequestSpec){
		// 重建的聚合必然是从库里读出来的，而框架合同规定未持久化为 0、插入从 1 起。
		// 零版本重建出来的聚合一保存就会被当成插入，从而覆盖掉库里那一行。
		"未持久化的版本": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.Revision = 0
		},
		"负版本": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.Revision = -1
		},
		"无状态": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.State = domain.ShipmentRequestStateInvalid
		},
		"无委托标识": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.ShipmentRequestID = domain.ShipmentRequestID{}
		},
		"无批次标识": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.BatchID = domain.SubmissionBatchID{}
		},
		"无提交版本标识": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.CurrentVersion.VersionID = domain.SubmissionVersionID{}
			snapshot.AcceptanceTask.SubmissionVersionID = domain.SubmissionVersionID{}
		},
		"无任务标识": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.AcceptanceTask.TaskID = domain.AcceptanceDecisionTaskID{}
		},
		"提交版本无建立时间": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.CurrentVersion.EstablishedAt = time.Time{}
		},
		// 来源指纹是来源保全的锚点，构造路径一直在校验它，重建路径不能漏。
		"提交版本无来源指纹": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.CurrentVersion.SourceSubmission = domain.SourceSubmissionFingerprint{}
		},
		// 任务状态的零值是`未设`而不是`运行中`，所以适配器忘了填在这里拦得住。
		"任务状态未设": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.AcceptanceTask.State = domain.AcceptanceTaskStateInvalid
		},
		"任务无建立时间": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.AcceptanceTask.EstablishedAt = time.Time{}
		},
		"无提交时间": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.SubmittedAt = time.Time{}
		},
		"提交版本没有声明成员": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.CurrentVersion.DeclaredParcelIDs = nil
		},
		// 任务挂在另一个提交版本上，意味着这份聚合里两半各自属于不同的提交版本——
		// 而接受判断任务是「某一个提交版本上」的工作。
		"任务挂在别的提交版本上": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.AcceptanceTask.SubmissionVersionID = mustValue(
				t, domain.NewSubmissionVersionID, "version-other",
			)
		},
		// `已提交`配一个已完成的任务：任务完成只在决定越过提交边界时到达，而这份聚合
		// 没有决定。两者只能同真同假。
		"已提交却配已完成的任务": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.AcceptanceTask.State = domain.AcceptanceTaskComplete
		},
	}

	for name, breakIt := range illegal {
		t.Run(name, func(t *testing.T) {
			snapshot := submittedSnapshot(t)
			breakIt(&snapshot)
			if _, err := domain.RehydrateShipmentRequest(snapshot); !errors.Is(
				err, domain.ErrInvalidRehydratedShipmentRequest,
			) {
				t.Fatalf("error = %v, want ErrInvalidRehydratedShipmentRequest", err)
			}
		})
	}
}

// Covers: ADR-0030「今天只开`已提交`」— 任务那一层的两个字段是`已提交`可达的，因此它们在
// 快照里，且必须原样带进聚合。
//
// 处理记录漏了，用例要的「未决按原因分类统计」在重建之后从头数起；复核完成度漏了，后面两条
// 测的两处业务结果就会变。
func TestRehydrationCarriesTheProcessingAttemptsAndCompletedReview(t *testing.T) {
	request, err := domain.RehydrateShipmentRequest(reviewedSnapshot(t))
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}

	attempts := request.AcceptanceDecisionTask().ProcessingAttempts()
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2；重建把这份委托卡过的轮次丢了", len(attempts))
	}
	if attempts[0].Reason().String() != "COMMERCIAL_BASIS_UNAVAILABLE" {
		t.Fatalf("first attempt = %q, want COMMERCIAL_BASIS_UNAVAILABLE", attempts[0].Reason())
	}
	completion, done := request.AcceptanceDecisionTask().ManualReviewCompletion()
	if !done {
		t.Fatal("重建把一次已经完成的人工复核丢了")
	}
	if completion.Reviewer().String() != "OPERATOR-1" {
		t.Fatalf("reviewer = %q, want OPERATOR-1", completion.Reviewer())
	}
}

// Covers: `CompleteManualReview`「同一提交版本只接受一次完成，第二次要么是重复提交要么是
// 换人补签，两者都不该静默覆盖第一次的留痕」。
//
// 这条不变式由任务上的完成事实承载，因此重建丢掉那个事实就等于把它删掉——而删掉之后没有
// 任何东西变红：第二次补签会成功返回，留痕被覆盖，读的人看到的仍是一份完整的复核。
func TestARehydratedCompletedReviewStillRefusesASecondCompletion(t *testing.T) {
	request, err := domain.RehydrateShipmentRequest(reviewedSnapshot(t))
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}

	if _, err := request.CompleteManualReview(reviewCompletion(t)); !errors.Is(
		err, domain.ErrManualReviewAlreadyCompleted,
	) {
		t.Fatalf("error = %v, want ErrManualReviewAlreadyCompleted", err)
	}
}

// Covers: UC-PS-001 复核那道门 —— `manualReviewState` 由规则包的声明与本任务的完成事实合成。
//
// 完成事实在重建里丢掉，合成结果就从`已完成`退回`已要求`，于是一份规则要求复核、复核也确实
// 做完了的委托，重建之后会停在 `ResumeByManualReview` 上把同一次复核再要一遍。这是重建丢字段
// 里最难发现的一类：结果是「继续等」，看起来完全正常。
func TestARehydratedCompletedReviewSatisfiesTheReviewGate(t *testing.T) {
	request, err := domain.RehydrateShipmentRequest(reviewedSnapshot(t))
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}

	decided, err := request.Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-1"),
		Checks:     everyGroupPassingFor(t, "parcel-1"),
		Basis:      basisRequiringReview(t, allApplicableGroups...),
		DecidedAt:  decidedAt,
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if path, waiting := decided.AcceptanceDecisionTask().WaitingOn(); waiting {
		t.Fatalf("waiting on %q；复核已经做完了，这一轮却又要了一次", path)
	}
	if decided.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", decided.State())
	}
}

// Covers: UC-PS-001「追加判断与处理尝试」— 重建读回来的记录与此后新追加的记录属同一条历史。
// 重建丢掉旧记录时，下一次追加会从一条开始，而委托实际已经卡过三轮。
func TestAttemptsRecordedAfterRehydrationAppendToTheRehydratedOnes(t *testing.T) {
	request, err := domain.RehydrateShipmentRequest(reviewedSnapshot(t))
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}

	continued, err := request.RecordProcessingAttempt(
		internalAttempt(t, "MANUAL_REVIEW_PENDING", firstAttemptAt.Add(2*time.Hour)),
	)
	if err != nil {
		t.Fatalf("record processing attempt: %v", err)
	}

	attempts := continued.AcceptanceDecisionTask().ProcessingAttempts()
	if len(attempts) != 3 {
		t.Fatalf("attempts = %d, want 3", len(attempts))
	}
	if attempts[2].Reason().String() != "MANUAL_REVIEW_PENDING" {
		t.Fatalf("last attempt = %q, want MANUAL_REVIEW_PENDING", attempts[2].Reason())
	}
}

// Covers: ADR-0030「没开的状态由入口显式拒绝，且用一个与『这行数据不可能』不同的哨兵」。
//
// 另外三个状态在快照里少的是真字段——已接受少接受基线与预计承诺，已拒绝少决定，已撤回少
// 撤回记录。放它们进来重建出的聚合看起来与真的一样，而`已接受`那一份的空基线会把全部指名
// 成员的资料更正拒掉。两个哨兵必须互相分得开：一行`已接受`的数据本身没有任何毛病，报成
// 「这行不可能」会让人去查一处不存在的损坏。
func TestRehydrationRefusesAStateThisDoorDoesNotYetCover(t *testing.T) {
	unsupported := map[string]domain.ShipmentRequestState{
		"已接受": domain.ShipmentRequestAccepted,
		"已拒绝": domain.ShipmentRequestRejected,
		"已撤回": domain.ShipmentRequestWithdrawn,
	}

	for name, state := range unsupported {
		t.Run(name, func(t *testing.T) {
			snapshot := submittedSnapshot(t)
			snapshot.State = state

			_, err := domain.RehydrateShipmentRequest(snapshot)
			if !errors.Is(err, domain.ErrRehydrationStateNotSupported) {
				t.Fatalf("error = %v, want ErrRehydrationStateNotSupported", err)
			}
			if errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
				t.Fatalf("error = %v；这行数据没有毛病，缺的是门，两个哨兵不能互相冒充", err)
			}
		})
	}
}

// Covers: ADR-0028「校验拦一行坏数据、或适配器自己的 bug 变成一个看起来合法的聚合」。
//
// 半截的一条处理记录包外造不出来（字段未导出、构造器全校验），一批零值却造得出：
// `make([]ProcessingAttempt, n)` 忘了填就是。放过去会让「这份委托卡过几轮」多数出几轮
// 不存在的。
func TestRehydrationRefusesAnEmptyProcessingAttempt(t *testing.T) {
	snapshot := reviewedSnapshot(t)
	snapshot.AcceptanceTask.ProcessingAttempts = append(
		snapshot.AcceptanceTask.ProcessingAttempts, domain.ProcessingAttempt{},
	)

	if _, err := domain.RehydrateShipmentRequest(snapshot); !errors.Is(
		err, domain.ErrInvalidRehydratedShipmentRequest,
	) {
		t.Fatalf("error = %v, want ErrInvalidRehydratedShipmentRequest", err)
	}
}

// everyGroupPassingFor 建一份全部适用校验组都通过、且指名成员都判过的校验集合。它与
// allGroupsPassing 分开，只因为重建快照里的声明成员与 submitted 夹具不同。
func everyGroupPassingFor(t *testing.T, parcels ...string) []domain.AcceptanceCheck {
	t.Helper()
	checks := make([]domain.AcceptanceCheck, 0, len(allApplicableGroups)+len(parcels))
	for _, group := range allApplicableGroups {
		if group == domain.NetworkReachabilityCheck {
			continue
		}
		checks = append(checks, versionCheck(t, group, domain.CheckPassed, ""))
	}
	for _, parcel := range parcels {
		checks = append(checks, parcelCheck(t, domain.NetworkReachabilityCheck, parcel, domain.CheckPassed, ""))
	}
	return checks
}
