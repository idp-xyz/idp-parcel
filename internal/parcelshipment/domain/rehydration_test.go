package domain_test

import (
	"errors"
	"strings"
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
		// 声明成员逐条校 + 去重，与构造路径 NewSubmissionCandidate 那三条对齐。只校「非空」
		// 时，一个空成员或一份重复成员会原样进接受基线，而基线「恒覆盖该提交版本的完整声明
		// 成员」——错的基线此后与真的无从分辨。
		"声明成员立不起来": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.CurrentVersion.DeclaredParcelIDs = []domain.DeclaredParcelID{{}}
		},
		"声明成员里混了一个空值": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.CurrentVersion.DeclaredParcelIDs = append(
				snapshot.CurrentVersion.DeclaredParcelIDs, domain.DeclaredParcelID{},
			)
		},
		// `make([]DeclaredParcelID, 3)` 忘了填就是这个形状。它与本文件为处理记录写下的
		// 「半截的造不出来，一批零值造得出」一字不差地成立。
		"一批零值声明成员": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.CurrentVersion.DeclaredParcelIDs = make([]domain.DeclaredParcelID, 3)
		},
		"声明成员重复": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			first := snapshot.CurrentVersion.DeclaredParcelIDs[0]
			snapshot.CurrentVersion.DeclaredParcelIDs = []domain.DeclaredParcelID{first, first}
		},
		// 等待态越界不校的话会被 `WaitingOn()` 报成**缺席**——「这任务不等任何人」——
		// 而不是被拒成一行坏数据。同一个 ResumePath 作为处理记录的字段时是校了值域的。
		"等待态越界": func(snapshot *domain.RehydrateShipmentRequestSpec) {
			snapshot.AcceptanceTask.WaitingOn = domain.ResumePath(7)
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
				// 本条同时是那笔分期债的**到期提醒**。谁把这扇门开到某个状态，这里当天变红，
				// 而这段文字是它唯一会被读到的时刻——写在 ADR 里的话，正好是没人会在那一刻
				// 翻开的地方。
				t.Fatalf("error = %v, want ErrRehydrationStateNotSupported。\n"+
					"若你正是来把这扇门开到这个状态的，本条变红是预期的；但同一天有三笔债一起到期：\n"+
					"（1）ADR-0028 点名的三组跨字段命题今天一条都没写，因为它们在`已提交`下根本造不"+
					"出来：`state = 已接受` 配 `decisionFormed = false`、`decision.accepted = true` "+
					"配任务未完成、`waitingOn` 有值而决定已形成。门一开它们全都造得出来，而那份记录"+
					"说这是「最花时间、也最容易写漏的部分」。\n"+
					"（2）`validForRehydration` 今天唯一那条跨字段命题（`已提交` ⇒ 任务运行中）对新开"+
					"的状态没有对应物：`已接受`/`已拒绝` 该配任务`已完成`、`已撤回` 该配`已停止`，"+
					"今天一条都没写。\n"+
					"（3）`absentInSubmitted` 里那几个字段要挪进 `carriedByRehydration` 并各自补上快照"+
					"表达，见 revision_test.go 的分类用例——它会同时变红，别只修这一条。", err)
			}
			if errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
				t.Fatalf("error = %v；这行数据没有毛病，缺的是门，两个哨兵不能互相冒充", err)
			}
		})
	}
}

// Covers: 同一条分格规则的**反方向**——一行坏数据不得被报成「本期不支持」。
//
// 上一条守的是「一行`已接受`没有毛病，别报成坏数据」；这一条守的是「那一列存了 99 是真坏了，
// 别报成缺一扇门」。两者一起丢进 default 时，运维会照着一行损坏的数据去等一扇永远不会为它
// 而开的门；而那时报文里的状态名还是**空串**——`ShipmentRequestState(99).String()` 交回空串，
// 唯一的诊断线索也一并没了，光看哨兵与报文都分不出这是哪一种。
//
// `admitRehydratedState` 的注释本来就点名了这个失败（「否则 default 分支会把一行坏数据报成
// 『本期不支持』」），当时只修了零值那一个实例。
func TestRehydrationTellsAnImpossibleStateFromAnUnopenedOne(t *testing.T) {
	snapshot := submittedSnapshot(t)
	snapshot.State = domain.ShipmentRequestState(99)

	_, err := domain.RehydrateShipmentRequest(snapshot)
	if !errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
		t.Fatalf("error = %v, want ErrInvalidRehydratedShipmentRequest；一行坏数据报成缺一扇门，运维会去等一扇不会开的门", err)
	}
	if errors.Is(err, domain.ErrRehydrationStateNotSupported) {
		t.Fatalf("error = %v；两个哨兵互相冒充了", err)
	}
	// 拒绝要说得出是哪个值。名字这条路对越界值走不通（`String()` 交回空串），所以印数字。
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("error = %v；拒绝没说出是哪个值，适配器作者只能靠猜", err)
	}
}

// Covers: 任务状态的值域要由**它自己**那条检查回答，不能挂在另一条命题的前件上。
//
// 越界的任务状态还会被 `已提交 ⇒ 任务运行中` 那条撞上，而两者交回同一个哨兵——所以一条只断言
// 哨兵的表项分不出是谁拦的：把值域检查整段拆掉，那条表项照样全绿。分得开的是报文，值域检查
// 说得出是哪个值，另一条说的是「已收工」，而那句话对一个垃圾值本身就是错的。
//
// 这条断言的是「守卫在，而且守卫的守卫也在」。挂在前件上的那一下会在门开到别的状态那天消失，
// 届时不会有任何东西变红。
func TestRehydrationRefusesATaskStateOnItsOwnValueDomain(t *testing.T) {
	snapshot := submittedSnapshot(t)
	snapshot.AcceptanceTask.State = domain.AcceptanceTaskState(99)

	_, err := domain.RehydrateShipmentRequest(snapshot)
	if !errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
		t.Fatalf("error = %v, want ErrInvalidRehydratedShipmentRequest", err)
	}
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("error = %v；拒绝没说出是哪个值，说明拦住它的是「已提交配已收工的任务」那条，"+
			"而那条的前件是`已提交`，门一开到别的状态就没了", err)
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

// Covers: 关联出处随快照往返（`AT-PS-036`/`AT-PS-076` 的重建半边）——有出处而不带回，
// 一份关联新委托重建后看起来像首次委托；半截或自指的出处是一行坏数据，不是首次委托。
func TestRehydrationCarriesThePriorRequestLink(t *testing.T) {
	snapshot := submittedSnapshot(t)
	snapshot.PriorLink = domain.RehydratePriorRequestLinkSpec{
		PriorRequestID: mustValue(t, domain.NewShipmentRequestID, "request-0"),
		Kind:           domain.LinkRejectedCorrection,
	}

	request, err := domain.RehydrateShipmentRequest(snapshot)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	link, present := request.PriorRequestLink()
	if !present || link.PriorRequestID().String() != "request-0" || link.Kind() != domain.LinkRejectedCorrection {
		t.Fatalf("link = %#v present = %v; 关联出处没有随快照回来", link, present)
	}

	plain, err := domain.RehydrateShipmentRequest(submittedSnapshot(t))
	if err != nil {
		t.Fatalf("rehydrate first request: %v", err)
	}
	if _, present := plain.PriorRequestLink(); present {
		t.Fatal("首次委托重建后凭空长出了关联出处")
	}
}

func TestRehydrationRefusesAHalfOrSelfPointingLink(t *testing.T) {
	cases := map[string]domain.RehydratePriorRequestLinkSpec{
		"kind without a prior": {Kind: domain.LinkRejectedCorrection},
		"prior without a kind": {PriorRequestID: mustValue(t, domain.NewShipmentRequestID, "request-0")},
		"kind out of range": {
			PriorRequestID: mustValue(t, domain.NewShipmentRequestID, "request-0"),
			Kind:           domain.RequestLinkKind(99),
		},
		"pointing at itself": {
			PriorRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
			Kind:           domain.LinkWithdrawnResubmission,
		},
	}
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			snapshot := submittedSnapshot(t)
			snapshot.PriorLink = link
			if _, err := domain.RehydrateShipmentRequest(snapshot); !errors.Is(
				err, domain.ErrInvalidRehydratedShipmentRequest,
			) {
				t.Fatalf("error = %v, want ErrInvalidRehydratedShipmentRequest", err)
			}
		})
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
