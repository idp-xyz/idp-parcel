package pilotgovernance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/pilotgovernance"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 生产装配要往三个窄口里插的就是这三个治理侧仓储。编译期钉住，形状不合当场编不过——
// 靠装配时才发现，表现出来是一个看不出原因的`权威未确定`。
var (
	_ adapter.AuthorityIntervalSource   = (*pgpostgres.AuthorityIntervals)(nil)
	_ adapter.OwnershipHandoffSource    = (*pgpostgres.Takeovers)(nil)
	_ adapter.AdmissionSuspensionSource = (*pgpostgres.Suspensions)(nil)
)

const (
	objectScope   = "OBJ-SCOPE-A"
	capability    = "CAP-PRODUCE-SHIPMENT"
	factKind      = "FACT-SHIPMENT-ACCEPTANCE"
	selfAuthority = "AUTH-SELF"
	peerAuthority = "AUTH-PEER"
	pilotScope    = "PILOT-SCOPE/v1"
)

var (
	asOf         = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	intervalFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

// ── 装配期：答复视界补不上就装不出诚实的答复 ──────────────────────────────

func TestAdapterRefusesToBuildWithoutAnAnswerValidity(t *testing.T) {
	deps := completeDeps(t)
	deps.AnswerValidity = 0

	_, err := adapter.NewProductionOwnershipAdapter(deps)
	if !errors.Is(err, adapter.ErrAnswerValidityNotStated) {
		t.Fatalf("error = %v, want ErrAnswerValidityNotStated", err)
	}
}

func TestAdapterRefusesToBuildWithoutAClock(t *testing.T) {
	deps := completeDeps(t)
	deps.Clock = nil

	if _, err := adapter.NewProductionOwnershipAdapter(deps); err == nil {
		t.Fatal("没有时钟也装得起来——判断时点会变成零值")
	}
}

// ── 实例半边缺席：一律`权威未确定`，绝不默认「本产品承接」 ─────────────────

// 四种缺席各测一次而不是合成一个表：它们缺的是不同的东西（目录、自身权威串、目录里
// 那一行、那一行的完整性），而红线要的是四种都不会滑成认领。
func TestUnconfiguredCollaboratorsAllAnswerAuthorityUnresolved(t *testing.T) {
	cases := map[string]func(*adapter.ProductionOwnershipAdapterDeps){
		"目录未装": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Directory = nil
		},
		"自身权威串未配": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.SelfAuthority = ""
		},
		"区间读口未装": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Intervals = nil
		},
		"暂停读口未装": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Suspensions = nil
		},
		"目录查无此范围": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Directory = &directoryDouble{found: false}
		},
		"目录交回残缺坐标": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Directory = &directoryDouble{
				scope: adapter.GovernanceScope{ObjectScope: objectScope},
				found: true,
			}
		},
	}

	for name, breakOne := range cases {
		t.Run(name, func(t *testing.T) {
			deps := completeDeps(t)
			breakOne(&deps)

			decision := decide(t, deps)

			if decision.Authority() != psdomain.ProductionAuthorityUnresolved {
				t.Fatalf("authority = %s, want UNRESOLVED", decision.Authority())
			}
			reason, continuation, ok := decision.UnresolvedDetails()
			if !ok {
				t.Fatal("`权威未确定`没带停摆原因与续办引用")
			}
			if reason != psdomain.OwnershipUnresolvedRuleUnavailable {
				t.Fatalf("reason = %s, want RULE_UNAVAILABLE", reason)
			}
			if continuation.String() == "" {
				t.Fatal("续办引用为空——回去补哪一件说不出来")
			}
		})
	}
}

// 未配置时不去问登记册。问了也拿不到能用的坐标，而一次不该发生的查询会在日后被当成
// 「这个范围确实查过」的证据。
func TestUnconfiguredDirectoryDoesNotQueryTheRegister(t *testing.T) {
	deps := completeDeps(t)
	intervals := &intervalSourceDouble{err: errors.New("不该被问到")}
	deps.Intervals = intervals
	deps.Directory = &directoryDouble{found: false}

	decide(t, deps)

	if intervals.calls != 0 {
		t.Fatalf("ListCurrent 被调用 %d 次，未配置时不该问登记册", intervals.calls)
	}
}

// ── 权威区间：命中恰一条才有权威可答 ─────────────────────────────────────

func TestSelfAuthorityIntervalYieldsIDPParcel(t *testing.T) {
	decision := decide(t, completeDeps(t))

	if decision.Authority() != psdomain.ProductionAuthorityIDPParcel {
		t.Fatalf("authority = %s, want IDP_PARCEL", decision.Authority())
	}
	if _, _, ok := decision.UnresolvedDetails(); ok {
		t.Fatal("`本产品承接`仍带了停摆原因")
	}
	if _, ok := decision.OtherAuthorityReference(); ok {
		t.Fatal("`本产品承接`仍带了他方权威引用")
	}
}

// 红线本体：登记册在、能读、但这个范围一条区间都没有。此时必须如实答`权威未确定`，
// 「没人登记」不等于「那就是我们」。
func TestNoRegisteredIntervalNeverFallsBackToSelf(t *testing.T) {
	deps := completeDeps(t)
	other := selfInterval()
	other.ObjectScope = "OBJ-SCOPE-OTHER"
	deps.Intervals = &intervalSourceDouble{intervals: []pgdomain.AuthorityInterval{other}}

	decision := decide(t, deps)

	if decision.Authority() != psdomain.ProductionAuthorityIDPParcel {
		reason, _, _ := decision.UnresolvedDetails()
		if decision.Authority() != psdomain.ProductionAuthorityUnresolved ||
			reason != psdomain.OwnershipUnresolvedRuleUnavailable {
			t.Fatalf("authority = %s / reason = %s, want UNRESOLVED / RULE_UNAVAILABLE",
				decision.Authority(), reason)
		}
		return
	}
	t.Fatal("无权威区间登记时认领了本产品——红线")
}

// 同维两条区间是登记错误。不按时间或行序任选一条：治理侧的冲突预检拦的正是这件事，
// 消费侧替它挑一条，等于把一次记录冲突静默判成一个确定归属。
func TestOverlappingIntervalsAnswerAuthorityNotUnique(t *testing.T) {
	deps := completeDeps(t)
	peer := selfInterval()
	peer.Authority = peerAuthority
	deps.Intervals = &intervalSourceDouble{
		intervals: []pgdomain.AuthorityInterval{selfInterval(), peer},
	}

	decision := decide(t, deps)

	reason, _, _ := decision.UnresolvedDetails()
	if decision.Authority() != psdomain.ProductionAuthorityUnresolved ||
		reason != psdomain.OwnershipUnresolvedAuthorityNotUnique {
		t.Fatalf("authority = %s / reason = %s, want UNRESOLVED / AUTHORITY_NOT_UNIQUE",
			decision.Authority(), reason)
	}
}

// 区间是左闭右开 [From, To)，与 OwnershipValidityInterval.Contains 同一口径。两个端点
// 各测一次：端点归属写反时，交接当天会出现两个权威或零个权威。
func TestIntervalBoundsAreHalfOpen(t *testing.T) {
	t.Run("起点当刻已在区间内", func(t *testing.T) {
		deps := completeDeps(t)
		interval := selfInterval()
		interval.From = asOf
		deps.Intervals = &intervalSourceDouble{intervals: []pgdomain.AuthorityInterval{interval}}

		if got := decide(t, deps).Authority(); got != psdomain.ProductionAuthorityIDPParcel {
			t.Fatalf("authority = %s, want IDP_PARCEL（起点应含在内）", got)
		}
	})

	t.Run("终点当刻已在区间外", func(t *testing.T) {
		deps := completeDeps(t)
		interval := selfInterval()
		interval.To = asOf
		deps.Intervals = &intervalSourceDouble{intervals: []pgdomain.AuthorityInterval{interval}}

		if got := decide(t, deps).Authority(); got != psdomain.ProductionAuthorityUnresolved {
			t.Fatalf("authority = %s, want UNRESOLVED（终点当刻已出区间）", got)
		}
	})
}

// ── 他方权威：没有停写证据就不能宣布交接完成 ─────────────────────────────

func TestOtherAuthorityRequiresATakeoverRecord(t *testing.T) {
	takeover := takeoverRecord(t, "STOP-EVIDENCE-1")

	t.Run("有接管记录则答他方权威", func(t *testing.T) {
		deps := peerAuthorityDeps(t)
		deps.Handoffs = &handoffSourceDouble{takeover: takeover, found: true}

		decision := decide(t, deps)

		if decision.Authority() != psdomain.ProductionAuthorityOther {
			t.Fatalf("authority = %s, want OTHER", decision.Authority())
		}
		authority, ok := decision.OtherAuthorityReference()
		if !ok || authority.String() != peerAuthority {
			t.Fatalf("other authority = %q, want %q", authority, peerAuthority)
		}
		handoff, ok := decision.HandoffReference()
		if !ok || handoff.String() != "STOP-EVIDENCE-1" {
			t.Fatalf("handoff = %q, want 原权威停写证据", handoff)
		}
	})

	t.Run("接管读口未装则答交接不可读", func(t *testing.T) {
		deps := peerAuthorityDeps(t)
		deps.Handoffs = nil

		reason, _, _ := decide(t, deps).UnresolvedDetails()
		if reason != psdomain.OwnershipUnresolvedHandoffUnavailable {
			t.Fatalf("reason = %s, want HANDOFF_UNAVAILABLE", reason)
		}
	})

	t.Run("查无接管记录则答交接未完成", func(t *testing.T) {
		deps := peerAuthorityDeps(t)
		deps.Handoffs = &handoffSourceDouble{found: false}

		decision := decide(t, deps)

		reason, _, _ := decision.UnresolvedDetails()
		if reason != psdomain.OwnershipUnresolvedHandoffIncomplete {
			t.Fatalf("reason = %s, want HANDOFF_INCOMPLETE", reason)
		}
		if _, ok := decision.OtherAuthorityReference(); ok {
			t.Fatal("交接未完成却已经带上他方权威引用——等于替对方宣布交接完成")
		}
	})
}

// ── 准入控制是独立一维 ───────────────────────────────────────────────────

func TestAdmissionControlIsIndependentOfAuthority(t *testing.T) {
	suspension := suspensionDecision(t, "SUSP-1")

	t.Run("暂停生效时与`本产品承接`并存", func(t *testing.T) {
		deps := completeDeps(t)
		deps.Suspensions = &suspensionSourceDouble{suspension: suspension, ground: pgdomain.AdmissionSuspendedByNamedScope}

		decision := decide(t, deps)

		if decision.Authority() != psdomain.ProductionAuthorityIDPParcel {
			t.Fatalf("authority = %s, want IDP_PARCEL——暂停不改变这个范围归谁", decision.Authority())
		}
		if decision.AdmissionControl() != psdomain.AdmissionControlPaused {
			t.Fatalf("control = %s, want PAUSED", decision.AdmissionControl())
		}
		reference, ok := decision.SuspensionReference()
		if !ok || reference.String() != "SUSP-1" {
			t.Fatalf("suspension reference = %q, want SUSP-1", reference)
		}
	})

	t.Run("暂停生效时与`权威未确定`并存", func(t *testing.T) {
		deps := completeDeps(t)
		deps.Intervals = &intervalSourceDouble{}
		deps.Suspensions = &suspensionSourceDouble{suspension: suspension, ground: pgdomain.AdmissionSuspendedByNamedScope}

		decision := decide(t, deps)

		if decision.Authority() != psdomain.ProductionAuthorityUnresolved {
			t.Fatalf("authority = %s, want UNRESOLVED", decision.Authority())
		}
		if decision.AdmissionControl() != psdomain.AdmissionControlPaused {
			t.Fatal("权威已判未决就把暂停丢了——两维互相吞掉")
		}
	})

	t.Run("无生效暂停则准入开放且不带暂停引用", func(t *testing.T) {
		decision := decide(t, completeDeps(t))

		if decision.AdmissionControl() != psdomain.AdmissionControlOpen {
			t.Fatalf("control = %s, want OPEN", decision.AdmissionControl())
		}
		if _, ok := decision.SuspensionReference(); ok {
			t.Fatal("未暂停却带了暂停引用")
		}
	})

	// 治理侧凡答「拦」，本包给出的都是同一个`暂停`加同一条暂停引用：命中与保守的差别
	// 决定的是治理侧的运维动作，不是本上下文的答复。零值一并钉住——它必须跟着拦，漏填
	// 一处不能在这里表现为一次默认放行。
	for name, ground := range map[string]pgdomain.AdmissionSuspensionGround{
		"命中所问范围版本": pgdomain.AdmissionSuspendedByNamedScope,
		"覆盖关系读不出":  pgdomain.AdmissionSuspendedByUnreadableScopeRelation,
		"零值":       pgdomain.AdmissionSuspensionGroundInvalid,
	} {
		t.Run("凡拦即`暂停`·"+name, func(t *testing.T) {
			deps := completeDeps(t)
			deps.Suspensions = &suspensionSourceDouble{suspension: suspension, ground: ground}

			decision := decide(t, deps)

			if decision.AdmissionControl() != psdomain.AdmissionControlPaused {
				t.Fatalf("control = %s, want PAUSED", decision.AdmissionControl())
			}
			reference, ok := decision.SuspensionReference()
			if !ok || reference.String() != "SUSP-1" {
				t.Fatalf("suspension reference = %q, %t；引用指向作数的那条暂停本身", reference, ok)
			}
		})
	}
}

// ── 依赖调不通一律上抛，绝不折成某一格答复（ADR-0029） ────────────────────

// 四个读口各一次。折错方向的后果不对称：把读不到读成「没有登记」是`权威未确定`（看着
// 像业务结论），把读不到读成「没暂停」则是一次默认放行——两者的运维动作都与故障不同。
func TestDependencyFailuresAreNeverFoldedIntoAnAnswer(t *testing.T) {
	unavailable := errors.New("登记册不可读")

	cases := map[string]func(*adapter.ProductionOwnershipAdapterDeps){
		"目录读不通": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Directory = &directoryDouble{err: unavailable}
		},
		"区间读不通": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Intervals = &intervalSourceDouble{err: unavailable}
		},
		"暂停读不通": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Suspensions = &suspensionSourceDouble{err: unavailable}
		},
		"接管读不通": func(deps *adapter.ProductionOwnershipAdapterDeps) {
			deps.Intervals = &intervalSourceDouble{intervals: []pgdomain.AuthorityInterval{peerInterval()}}
			deps.Handoffs = &handoffSourceDouble{err: unavailable}
		},
	}

	for name, breakOne := range cases {
		t.Run(name, func(t *testing.T) {
			deps := completeDeps(t)
			breakOne(&deps)
			built, err := adapter.NewProductionOwnershipAdapter(deps)
			if err != nil {
				t.Fatalf("build adapter: %v", err)
			}

			_, err = built.DecideProductionOwnership(context.Background(), admissionScope(t))
			if !errors.Is(err, unavailable) {
				t.Fatalf("error = %v, want wrapped %v", err, unavailable)
			}
		})
	}
}

// ── 有效期间 ─────────────────────────────────────────────────────────────

func TestValidityNeverOutlivesTheRegisterOrTheStatedHorizon(t *testing.T) {
	t.Run("开放区间以声明的答复视界收口", func(t *testing.T) {
		deps := completeDeps(t)
		deps.AnswerValidity = 2 * time.Hour

		validity := decide(t, deps).Validity()

		if want := asOf.Add(2 * time.Hour); !validity.ValidUntil().Equal(want) {
			t.Fatalf("validUntil = %s, want %s", validity.ValidUntil(), want)
		}
		if !validity.ValidFrom().Equal(intervalFrom) {
			t.Fatalf("validFrom = %s, want 区间起点 %s", validity.ValidFrom(), intervalFrom)
		}
	})

	t.Run("登记终点更早时以登记终点收口", func(t *testing.T) {
		deps := completeDeps(t)
		deps.AnswerValidity = 48 * time.Hour
		interval := selfInterval()
		interval.To = asOf.Add(time.Hour)
		deps.Intervals = &intervalSourceDouble{intervals: []pgdomain.AuthorityInterval{interval}}

		validity := decide(t, deps).Validity()

		if want := asOf.Add(time.Hour); !validity.ValidUntil().Equal(want) {
			t.Fatalf("validUntil = %s, want 登记终点 %s", validity.ValidUntil(), want)
		}
	})

	t.Run("判断时点落在有效期间内", func(t *testing.T) {
		if !decide(t, completeDeps(t)).Validity().Contains(asOf) {
			t.Fatal("判断时点不在自己答复的有效期间内")
		}
	})
}

// ── 标识与修订由内容派生 ─────────────────────────────────────────────────

// 不持久化决定，因此同一份登记册问两次必须得到同一个标识；否则调用方每问一次都拿到
// 「新的一次决定」，`决定已过期`那一格就永远判不出来。
func TestSameRegisterYieldsTheSameDecisionIdentity(t *testing.T) {
	first := decide(t, completeDeps(t))
	second := decide(t, completeDeps(t))

	if first.DecisionID() != second.DecisionID() {
		t.Fatalf("decision ID 不稳定：%q vs %q", first.DecisionID(), second.DecisionID())
	}
	if first.Revision() != second.Revision() {
		t.Fatalf("revision 不稳定：%q vs %q", first.Revision(), second.Revision())
	}
}

// 登记册变一处修订就要变一次，否则调用方带着旧修订来问，门禁不会判`决定已过期`。
func TestRevisionMovesWithEveryRegisteredChange(t *testing.T) {
	baseline := decide(t, completeDeps(t)).Revision()

	t.Run("权威方换人", func(t *testing.T) {
		deps := peerAuthorityDeps(t)
		deps.Handoffs = &handoffSourceDouble{takeover: takeoverRecord(t, "STOP-1"), found: true}

		if decide(t, deps).Revision() == baseline {
			t.Fatal("权威方换了修订没动")
		}
	})

	t.Run("准入被暂停", func(t *testing.T) {
		deps := completeDeps(t)
		deps.Suspensions = &suspensionSourceDouble{suspension: suspensionDecision(t, "SUSP-9"), ground: pgdomain.AdmissionSuspendedByNamedScope}

		if decide(t, deps).Revision() == baseline {
			t.Fatal("准入暂停了修订没动")
		}
	})

	t.Run("区间边界移动", func(t *testing.T) {
		deps := completeDeps(t)
		interval := selfInterval()
		interval.To = asOf.Add(72 * time.Hour)
		deps.Intervals = &intervalSourceDouble{intervals: []pgdomain.AuthorityInterval{interval}}

		if decide(t, deps).Revision() == baseline {
			t.Fatal("区间边界移动了修订没动")
		}
	})
}

// ── 与未来建单门禁合起来看 ───────────────────────────────────────────────

// 未配置时准入控制答`开放`不是一次放行：`开放`说的是本产品此刻接纳新准入，而这份范围
// 归谁仍是未决，门禁照样拦。两格分开各答各的，合起来才是对的——这一条把它钉住，免得
// 日后有人看见 OPEN 就以为提交能过。
func TestUnconfiguredDecisionStillBlocksTheFutureSubmissionGate(t *testing.T) {
	deps := completeDeps(t)
	deps.Directory = nil
	decision := decide(t, deps)

	if decision.AdmissionControl() != psdomain.AdmissionControlOpen {
		t.Fatalf("control = %s, want OPEN", decision.AdmissionControl())
	}

	gate, err := psdomain.EvaluateFutureSubmissionGate(
		decision, decision.Scope().Digest(), decision.Revision(), asOf)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	if gate.IsAllowed() {
		t.Fatal("权威未决却放行了未来建单")
	}
	if !hasReason(gate.BlockReasons(), psdomain.FutureSubmissionAuthorityUnresolved) {
		t.Fatalf("block reasons = %v, want 含 AUTHORITY_UNRESOLVED", gate.BlockReasons())
	}
}

func TestSelfAuthorityWithOpenAdmissionPassesTheFutureSubmissionGate(t *testing.T) {
	decision := decide(t, completeDeps(t))

	gate, err := psdomain.EvaluateFutureSubmissionGate(
		decision, decision.Scope().Digest(), decision.Revision(), asOf)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	if !gate.IsAllowed() {
		t.Fatalf("归属为本产品且准入开放仍被拦：%v", gate.BlockReasons())
	}
}

func TestPausedAdmissionBlocksEvenWhenOwnershipIsOurs(t *testing.T) {
	deps := completeDeps(t)
	deps.Suspensions = &suspensionSourceDouble{suspension: suspensionDecision(t, "SUSP-2"), ground: pgdomain.AdmissionSuspendedByNamedScope}
	decision := decide(t, deps)

	gate, err := psdomain.EvaluateFutureSubmissionGate(
		decision, decision.Scope().Digest(), decision.Revision(), asOf)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	if !hasReason(gate.BlockReasons(), psdomain.FutureSubmissionAdmissionPaused) {
		t.Fatalf("block reasons = %v, want 含 ADMISSION_PAUSED", gate.BlockReasons())
	}
}

// ── 替身与构造助手 ───────────────────────────────────────────────────────

type intervalSourceDouble struct {
	intervals []pgdomain.AuthorityInterval
	err       error
	calls     int
}

func (source *intervalSourceDouble) ListCurrent(context.Context) ([]pgdomain.AuthorityInterval, error) {
	source.calls++
	if source.err != nil {
		return nil, source.err
	}
	return source.intervals, nil
}

type suspensionSourceDouble struct {
	suspension pgdomain.SuspensionDecision
	ground     pgdomain.AdmissionSuspensionGround
	err        error
}

func (source *suspensionSourceDouble) FindUnresumedSuspension(
	context.Context, pgdomain.ScopeVersionReference, time.Time,
) (pgdomain.SuspensionDecision, pgdomain.AdmissionSuspensionGround, error) {
	if source.err != nil {
		return pgdomain.SuspensionDecision{}, pgdomain.AdmissionSuspensionGroundInvalid, source.err
	}
	return source.suspension, source.ground, nil
}

type handoffSourceDouble struct {
	takeover pgdomain.TakeoverRecord
	found    bool
	err      error
}

func (source *handoffSourceDouble) FindByInterval(
	context.Context, pgdomain.AuthorityInterval,
) (pgdomain.TakeoverRecord, bool, error) {
	if source.err != nil {
		return pgdomain.TakeoverRecord{}, false, source.err
	}
	return source.takeover, source.found, nil
}

type directoryDouble struct {
	scope adapter.GovernanceScope
	found bool
	err   error
}

func (source *directoryDouble) FindGovernanceScope(
	context.Context, psdomain.AdmissionScope,
) (adapter.GovernanceScope, bool, error) {
	if source.err != nil {
		return adapter.GovernanceScope{}, false, source.err
	}
	return source.scope, source.found, nil
}

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

// completeDeps 是「实例半边全都配齐、登记册答本产品承接、无暂停」的那一档。各用例只
// 打坏其中一件，因此每条断言指向的缺席是唯一的。
func completeDeps(t *testing.T) adapter.ProductionOwnershipAdapterDeps {
	t.Helper()
	return adapter.ProductionOwnershipAdapterDeps{
		Intervals:      &intervalSourceDouble{intervals: []pgdomain.AuthorityInterval{selfInterval()}},
		Suspensions:    &suspensionSourceDouble{ground: pgdomain.AdmissionNotSuspended},
		Handoffs:       &handoffSourceDouble{},
		Directory:      &directoryDouble{scope: governanceScope(t), found: true},
		SelfAuthority:  selfAuthority,
		Clock:          fixedClock{now: asOf},
		AnswerValidity: 24 * time.Hour,
	}
}

func peerAuthorityDeps(t *testing.T) adapter.ProductionOwnershipAdapterDeps {
	t.Helper()
	deps := completeDeps(t)
	deps.Intervals = &intervalSourceDouble{intervals: []pgdomain.AuthorityInterval{peerInterval()}}
	return deps
}

func decide(t *testing.T, deps adapter.ProductionOwnershipAdapterDeps) psdomain.ProductionOwnershipDecision {
	t.Helper()
	built, err := adapter.NewProductionOwnershipAdapter(deps)
	if err != nil {
		t.Fatalf("build adapter: %v", err)
	}
	decision, err := built.DecideProductionOwnership(context.Background(), admissionScope(t))
	if err != nil {
		t.Fatalf("decide production ownership: %v", err)
	}
	return decision
}

func selfInterval() pgdomain.AuthorityInterval {
	return pgdomain.AuthorityInterval{
		ObjectScope: objectScope,
		Capability:  capability,
		FactKind:    factKind,
		Authority:   selfAuthority,
		From:        intervalFrom,
	}
}

func peerInterval() pgdomain.AuthorityInterval {
	interval := selfInterval()
	interval.Authority = peerAuthority
	return interval
}

func governanceScope(t *testing.T) adapter.GovernanceScope {
	t.Helper()
	reference, err := pgdomain.NewScopeVersionReference(pilotScope)
	if err != nil {
		t.Fatalf("scope version reference: %v", err)
	}
	return adapter.GovernanceScope{
		ObjectScope: objectScope,
		Capability:  capability,
		FactKind:    factKind,
		PilotScope:  reference,
	}
}

func admissionScope(t *testing.T) psdomain.AdmissionScope {
	t.Helper()
	reference, err := psdomain.NewAdmissionScopeReference("ADM-SCOPE-1")
	if err != nil {
		t.Fatalf("admission scope reference: %v", err)
	}
	digest, err := psdomain.NewAdmissionScopeDigest("sha256:adm-scope-1")
	if err != nil {
		t.Fatalf("admission scope digest: %v", err)
	}
	scope, err := psdomain.NewAdmissionScope(reference, digest)
	if err != nil {
		t.Fatalf("admission scope: %v", err)
	}
	return scope
}

func suspensionDecision(t *testing.T, id string) pgdomain.SuspensionDecision {
	t.Helper()
	suspensionID, err := pgdomain.NewSuspensionID(id)
	if err != nil {
		t.Fatalf("suspension ID: %v", err)
	}
	reference, err := pgdomain.NewScopeVersionReference(pilotScope)
	if err != nil {
		t.Fatalf("scope version reference: %v", err)
	}
	suspension, err := pgdomain.RecordSuspension(pgdomain.SuspensionDecisionSpec{
		ID:            suspensionID,
		TriggerSource: "TRIGGER-1",
		Basis:         "BASIS-1",
		Evidence:      "EVIDENCE-1",
		Scope:         reference,
		ExecutedBy:    "OPERATOR-1",
		OccurredAt:    intervalFrom,
		EffectiveAt:   intervalFrom,
		InTransitNote: "在途对象不受本次暂停影响",
	})
	if err != nil {
		t.Fatalf("record suspension: %v", err)
	}
	return suspension
}

func takeoverRecord(t *testing.T, stopEvidence string) pgdomain.TakeoverRecord {
	t.Helper()
	inventory, err := pgdomain.TakeInventory([]pgdomain.InventoryEntry{{
		ObjectIdentity:   "OBJ-1",
		CurrentFacts:     "FACTS-1",
		CurrentAuthority: peerAuthority,
		ResponsibleParty: "PARTY-1",
		NextAction:       "ACTION-1",
		ReviewBy:         asOf.Add(72 * time.Hour),
	}}, intervalFrom)
	if err != nil {
		t.Fatalf("take inventory: %v", err)
	}
	record, err := pgdomain.RecordTakeover(pgdomain.TakeoverRecordSpec{
		StopEvidence:     stopEvidence,
		Interval:         peerInterval(),
		AcceptedFacts:    "ACCEPTED-1",
		PendingExternals: "PENDING-1",
		ActualControl:    "CONTROL-1",
		Responsibilities: "RESP-1",
		NextAction:       "ACTION-1",
		Inventory:        inventory,
		EffectiveAt:      intervalFrom,
	})
	if err != nil {
		t.Fatalf("record takeover: %v", err)
	}
	return record
}

func hasReason(reasons []psdomain.FutureSubmissionBlockReason, want psdomain.FutureSubmissionBlockReason) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
