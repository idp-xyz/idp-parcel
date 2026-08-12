package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var supersededAt = time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

// supersessionSpecFor 造一份成员集合与给定名单一致的新版本输入。来源身份刻意换了请求键：
// 补充请求必须以自己的来源身份到达，沿用原身份在来源保全层就是重放或冲突。
func supersessionSpecFor(t *testing.T, parcelIDs ...string) domain.NewSubmissionVersionSpec {
	t.Helper()
	declared := make([]domain.DeclaredParcelID, len(parcelIDs))
	for index, value := range parcelIDs {
		declared[index] = mustValue(t, domain.NewDeclaredParcelID, value)
	}
	return domain.NewSubmissionVersionSpec{
		VersionID:         mustValue(t, domain.NewSubmissionVersionID, "version-2"),
		TaskID:            mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-2"),
		SourceSubmission:  sourceFingerprint(t, "tenant-1", "customer-1", "source-a", "supplement-key-1", "supplement-digest-1"),
		DeclaredParcelIDs: declared,
		EstablishedAt:     supersededAt,
	}
}

// Covers: `AT-PS-036` 第一支「已提交委托补充普通资料 → 形成同一委托的新提交版本，保留
// 原版本」，与 CONTEXT「新版本成为唯一待判断版本，旧版本及其判断历史继续保留」「原任务和
// 判断历史不覆盖」（形状裁于 ADR-0045）。此前该支为真洞：聚合内没有任何历史承载点，探针
// 实测钉红于 `52a5add`。
func TestASupplementFormsANewSubmissionVersionKeepingHistory(t *testing.T) {
	superseded, err := submitted(t).FormNewSubmissionVersion(supersessionSpecFor(t, "parcel-2", "parcel-1"))
	if err != nil {
		t.Fatalf("form new submission version: %v", err)
	}

	if superseded.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED——换代不是决定", superseded.State())
	}
	current := superseded.CurrentSubmissionVersion()
	if current.VersionID().String() != "version-2" || !current.EstablishedAt().Equal(supersededAt) {
		t.Fatalf("current version = %q @ %s, want version-2 @ %s", current.VersionID(), current.EstablishedAt(), supersededAt)
	}
	task := superseded.AcceptanceDecisionTask()
	if task.SubmissionVersionID().String() != "version-2" || task.IsComplete() || task.IsStopped() {
		t.Fatalf("task = %#v; 新版本必须带着自己的运行中判断任务", task)
	}

	priors := superseded.PriorSubmissionVersions()
	if len(priors) != 1 || priors[0].VersionID().String() != "version-1" {
		t.Fatalf("prior versions = %#v, want the original version kept", priors)
	}
	priorTasks := superseded.PriorAcceptanceTasks()
	if len(priorTasks) != 1 || !priorTasks[0].IsStopped() || priorTasks[0].IsComplete() {
		t.Fatalf("prior tasks = %#v; 旧任务要停下而不是完成或消失", priorTasks)
	}
	if priorTasks[0].SubmissionVersionID().String() != "version-1" {
		t.Fatal("旧任务不再指着它当初判的那个版本")
	}

	// 新版本必须重新经过适用校验（UC-PS-001「不能成为绕过主入口规则的第二条接单路径」）：
	// 换代后的委托仍能正常走 Decide，且决定落在新版本上。
	decided, err := superseded.Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide after supersession: %v", err)
	}
	if decided.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state after decide = %q, want ACCEPTED", decided.State())
	}
}

// Covers: CONTEXT「成员重组或共享约束变化建立关联新委托」与 UC-PS-001「删除、拆分或重组
// 成员必须建立关联新委托」——这条转移只认成员集合等值（不看顺序），增删都拒。
func TestANewSubmissionVersionRefusesAChangedMemberSet(t *testing.T) {
	cases := map[string][]string{
		"a member added":   {"parcel-1", "parcel-2", "parcel-3"},
		"a member removed": {"parcel-1"},
		"a member swapped": {"parcel-1", "parcel-9"},
	}
	for name, members := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := submitted(t).FormNewSubmissionVersion(supersessionSpecFor(t, members...))
			if !errors.Is(err, domain.ErrSubmissionBoundaryChanged) {
				t.Fatalf("error = %v, want ErrSubmissionBoundaryChanged", err)
			}
		})
	}

	t.Run("the same set reordered is a supplement, not a reorganisation", func(t *testing.T) {
		if _, err := submitted(t).FormNewSubmissionVersion(supersessionSpecFor(t, "parcel-2", "parcel-1")); err != nil {
			t.Fatalf("form new submission version: %v", err)
		}
	})
}

// Covers: CONTEXT「已提交期间形成新提交版本：只允许仍属于同一委托边界的纠错或补充」的
// 决定边界半边——接受、拒绝、撤回越过边界后，同一信号分别走资料修订与关联新委托，不得
// 用新版本改写已决定的委托（`AT-PS-037` 的「提交后不追溯」与此同界）。
func TestANewSubmissionVersionCannotFormAfterTheDecisionBoundary(t *testing.T) {
	cases := map[string]func(*testing.T) domain.ShipmentRequest{
		"accepted": func(t *testing.T) domain.ShipmentRequest {
			t.Helper()
			decided, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
			if err != nil {
				t.Fatalf("decide: %v", err)
			}
			return decided
		},
		"rejected": func(t *testing.T) domain.ShipmentRequest {
			t.Helper()
			rejected, err := submitted(t).RejectByAuthority(activeRejectionSpec(t))
			if err != nil {
				t.Fatalf("reject: %v", err)
			}
			return rejected
		},
		"withdrawn": func(t *testing.T) domain.ShipmentRequest {
			t.Helper()
			withdrawn, err := submitted(t).WithdrawByCustomer(withdrawalSpec(t))
			if err != nil {
				t.Fatalf("withdraw: %v", err)
			}
			return withdrawn
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := arrange(t).FormNewSubmissionVersion(supersessionSpecFor(t, "parcel-1", "parcel-2"))
			if !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
				t.Fatalf("error = %v, want ErrDecisionAlreadyFormed", err)
			}
		})
	}
}

// Covers: ADR-0045「新版本要有自己的来源身份与指纹」与「版本编号不与任何一代重号」——
// 沿用旧身份或旧版本号的输入是装配错误，不是一种可以吞下的补充。
func TestANewSubmissionVersionNeedsItsOwnIdentityAndNumber(t *testing.T) {
	cases := map[string]func(domain.NewSubmissionVersionSpec) domain.NewSubmissionVersionSpec{
		"reused version number": func(spec domain.NewSubmissionVersionSpec) domain.NewSubmissionVersionSpec {
			spec.VersionID = mustValue(t, domain.NewSubmissionVersionID, "version-1")
			return spec
		},
		"reused source identity": func(spec domain.NewSubmissionVersionSpec) domain.NewSubmissionVersionSpec {
			spec.SourceSubmission = sourceFingerprint(t, "tenant-1", "customer-1", "source-a", "key-1", "digest-2")
			return spec
		},
		"missing version ID": func(spec domain.NewSubmissionVersionSpec) domain.NewSubmissionVersionSpec {
			spec.VersionID = domain.SubmissionVersionID{}
			return spec
		},
		"missing task ID": func(spec domain.NewSubmissionVersionSpec) domain.NewSubmissionVersionSpec {
			spec.TaskID = domain.AcceptanceDecisionTaskID{}
			return spec
		},
		"missing fingerprint": func(spec domain.NewSubmissionVersionSpec) domain.NewSubmissionVersionSpec {
			spec.SourceSubmission = domain.SourceSubmissionFingerprint{}
			return spec
		},
		"missing establishment time": func(spec domain.NewSubmissionVersionSpec) domain.NewSubmissionVersionSpec {
			spec.EstablishedAt = time.Time{}
			return spec
		},
		"no members": func(spec domain.NewSubmissionVersionSpec) domain.NewSubmissionVersionSpec {
			spec.DeclaredParcelIDs = nil
			return spec
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := submitted(t).FormNewSubmissionVersion(mutate(supersessionSpecFor(t, "parcel-1", "parcel-2")))
			if !errors.Is(err, domain.ErrInvalidSubmissionSupersession) {
				t.Fatalf("error = %v, want ErrInvalidSubmissionSupersession", err)
			}
		})
	}
}

// Covers: ADR-0045 的重建半边——历史随快照往返，不因重建而消失；历史任务仍在运行、
// 与历史版本对不上号或与另一代重号的快照是拼出来的，一律拒。
func TestRehydrationCarriesSupersessionHistoryAndRefusesForgeries(t *testing.T) {
	withHistory := func(t *testing.T) domain.RehydrateShipmentRequestSpec {
		t.Helper()
		snapshot := submittedSnapshot(t)
		snapshot.CurrentVersion.VersionID = mustValue(t, domain.NewSubmissionVersionID, "version-2")
		snapshot.CurrentVersion.SourceSubmission = sourceFingerprint(
			t, "tenant-1", "customer-1", "SOURCE-1", "supplement-key-1", "sha256:b")
		snapshot.AcceptanceTask.SubmissionVersionID = snapshot.CurrentVersion.VersionID
		snapshot.PriorVersions = []domain.RehydrateSubmissionVersionSpec{{
			VersionID:        mustValue(t, domain.NewSubmissionVersionID, "version-1"),
			SourceSubmission: sourceFingerprint(t, "tenant-1", "customer-1", "SOURCE-1", "key-1", "sha256:a"),
			DeclaredParcelIDs: []domain.DeclaredParcelID{
				mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
			},
			EstablishedAt: rehydratedAt,
		}}
		snapshot.PriorTasks = []domain.RehydrateAcceptanceTaskSpec{{
			TaskID:              mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-1"),
			SubmissionVersionID: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
			EstablishedAt:       rehydratedAt,
			State:               domain.AcceptanceTaskStopped,
		}}
		return snapshot
	}

	t.Run("history survives the round trip", func(t *testing.T) {
		request, err := domain.RehydrateShipmentRequest(withHistory(t))
		if err != nil {
			t.Fatalf("rehydrate: %v", err)
		}
		priors := request.PriorSubmissionVersions()
		if len(priors) != 1 || priors[0].VersionID().String() != "version-1" {
			t.Fatalf("prior versions = %#v", priors)
		}
		if tasks := request.PriorAcceptanceTasks(); len(tasks) != 1 || !tasks[0].IsStopped() {
			t.Fatalf("prior tasks = %#v", tasks)
		}
	})

	forgeries := map[string]func(domain.RehydrateShipmentRequestSpec) domain.RehydrateShipmentRequestSpec{
		"a prior task still running": func(s domain.RehydrateShipmentRequestSpec) domain.RehydrateShipmentRequestSpec {
			s.PriorTasks[0].State = domain.AcceptanceTaskRunning
			return s
		},
		"a prior task bound to another version": func(s domain.RehydrateShipmentRequestSpec) domain.RehydrateShipmentRequestSpec {
			s.PriorTasks[0].SubmissionVersionID = mustValue(t, domain.NewSubmissionVersionID, "version-9")
			return s
		},
		"a prior version renumbered as the current one": func(s domain.RehydrateShipmentRequestSpec) domain.RehydrateShipmentRequestSpec {
			s.PriorVersions[0].VersionID = s.CurrentVersion.VersionID
			s.PriorTasks[0].SubmissionVersionID = s.CurrentVersion.VersionID
			return s
		},
		"history halves out of step": func(s domain.RehydrateShipmentRequestSpec) domain.RehydrateShipmentRequestSpec {
			s.PriorTasks = nil
			return s
		},
	}
	for name, forge := range forgeries {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.RehydrateShipmentRequest(forge(withHistory(t))); !errors.Is(
				err, domain.ErrInvalidRehydratedShipmentRequest,
			) {
				t.Fatalf("error = %v, want ErrInvalidRehydratedShipmentRequest", err)
			}
		})
	}
}
