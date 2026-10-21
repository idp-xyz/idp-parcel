package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证提交判得的归属答案（票 docs-consistency/code/39）：重发要答「原归属结果」，存下的决定读回来就
// 必须与判得的那一份逐字段相同——暂停引用、停写证据、交接评估（含部分确认那种被读口收起的确认引用）一格都不能丢。

var answerAnchor = time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)

// Covers: UC-PS-001「必须持久保留的业务记录」生产归属结果一条——权威 × 准入控制各组合、四种交接评估与三格拒绝读回逐字段相同。
func TestSubmissionAnswersRoundTripWholly(t *testing.T) {
	answers, sources, transactor := newSubmissionAnswers(t)
	ctx := t.Context()

	confirmed := ports.ProductionHandoffObservation{
		Observation: domain.HandoffObservationCompleteConfirmation,
		EffectiveAt: answerAnchor.Add(-30 * time.Second),
	}
	partial := ports.ProductionHandoffObservation{
		Observation:          domain.HandoffObservationPartialConfirmation,
		ConfirmedScopeDigest: mustBuild(t, domain.NewAdmissionScopeDigest, "half-of-scope"),
		ConfirmationRef:      mustBuild(t, domain.NewHandoffConfirmationReference, "CONF-PARTIAL"),
	}
	unconfigured := ports.ProductionHandoffObservation{Observation: domain.HandoffObservationChannelUnconfigured}
	timedOut := ports.ProductionHandoffObservation{Observation: domain.HandoffObservationTimedOut}
	cases := map[string]ports.SubmissionAnswer{
		"输入未受理":  {Refusal: ports.SubmissionRefusedInputNotAccepted},
		"原委托不可见": {Refusal: ports.SubmissionRefusedPriorRequestNotFound},
		"关联不适用":  {Refusal: ports.SubmissionRefusedLinkIneligible},
		"本产品放行": {
			Decision:          ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, nil),
			ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "REQ-1"),
		},
		"准入暂停": {
			Decision:     ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, nil),
			BlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAdmissionPaused},
		},
		"归属未决": {
			Decision:     ownershipDecision(t, domain.ProductionAuthorityUnresolved, domain.AdmissionControlOpen, nil),
			BlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionDecisionStale, domain.FutureSubmissionAuthorityUnresolved},
		},
		"已交给他方": {
			Decision:     ownershipDecision(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen, &confirmed),
			BlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionOtherAuthority},
		},
		"他方只部分确认": {
			Decision:     ownershipDecision(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen, &partial),
			BlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionOtherAuthority},
		},
		"通往他方的通道未配置": {
			Decision:     ownershipDecision(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen, &unconfigured),
			BlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionOtherAuthority},
		},
		"他方超时未答": {
			Decision:     ownershipDecision(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen, &timedOut),
			BlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionOtherAuthority},
		},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			source := identity(t, "tenant-a", "customer-a", "portal", "key-"+name)
			preserveFor(t, sources, transactor, source)
			mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
				return answers.RecordSubmissionAnswer(txCtx, source, answer)
			})

			found, exists, err := answers.FindLatestSubmissionAnswer(ctx, source)
			if err != nil || !exists {
				t.Fatalf("读回最近一次答案：exists = %v, err = %v", exists, err)
			}
			if !reflect.DeepEqual(found.Decision, answer.Decision) {
				t.Errorf("决定读回 %+v，记下的是 %+v", found.Decision, answer.Decision)
			}
			if found.Refusal != answer.Refusal {
				t.Errorf("拒绝读回 %v，记下的是 %v", found.Refusal, answer.Refusal)
			}
			if !slices.Equal(found.BlockReasons, answer.BlockReasons) || found.ShipmentRequestID != answer.ShipmentRequestID {
				t.Errorf("门禁读回 %v / %q，记下的是 %v / %q",
					found.BlockReasons, found.ShipmentRequestID, answer.BlockReasons, answer.ShipmentRequestID)
			}
		})
	}
}

// Covers: 只追加——重判留下每一次；最近一次与「建这份委托所据的那一次」各答各的，作用域外一律不可见。
func TestSubmissionAnswersAppendAndAnswerByRequest(t *testing.T) {
	answers, sources, transactor := newSubmissionAnswers(t)
	ctx := t.Context()
	source := identity(t, "tenant-a", "customer-a", "portal", "key-1")
	preserveFor(t, sources, transactor, source)

	unresolved := ports.SubmissionAnswer{
		Decision:     ownershipDecision(t, domain.ProductionAuthorityUnresolved, domain.AdmissionControlOpen, nil),
		BlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAuthorityUnresolved},
	}
	winner := ports.SubmissionAnswer{
		Decision:          ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, nil),
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "REQ-WINNER"),
	}
	loser := ports.SubmissionAnswer{
		Decision:          ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, nil),
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "REQ-LOSER"),
	}
	for _, answer := range []ports.SubmissionAnswer{unresolved, winner, loser} {
		mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			return answers.RecordSubmissionAnswer(txCtx, source, answer)
		})
	}

	latest, _, err := answers.FindLatestSubmissionAnswer(ctx, source)
	if err != nil || latest.ShipmentRequestID != loser.ShipmentRequestID {
		t.Fatalf("最近一次 = %q（err %v），want 最后记下的 REQ-LOSER", latest.ShipmentRequestID, err)
	}
	built, exists, err := answers.FindSubmissionAnswerForRequest(ctx, source, winner.ShipmentRequestID)
	if err != nil || !exists || built.ShipmentRequestID != winner.ShipmentRequestID {
		t.Fatalf("建 REQ-WINNER 所据 = %q / %v（err %v）", built.ShipmentRequestID, exists, err)
	}
	if _, exists, _ := answers.FindSubmissionAnswerForRequest(ctx, source, mustBuild(t, domain.NewShipmentRequestID, "REQ-NONE")); exists {
		t.Fatal("没放行过的委托号读到了答案")
	}

	for name, elsewhere := range map[string]domain.SourceIdentity{
		"另一个租户":  identity(t, "tenant-b", "customer-a", "portal", "key-1"),
		"另一个客户":  identity(t, "tenant-a", "customer-b", "portal", "key-1"),
		"另一个来源":  identity(t, "tenant-a", "customer-a", "api", "key-1"),
		"另一个请求键": identity(t, "tenant-a", "customer-a", "portal", "key-2"),
	} {
		if _, exists, err := answers.FindLatestSubmissionAnswer(ctx, elsewhere); err != nil || exists {
			t.Errorf("%s：exists = %v, err = %v，want 不可见", name, exists, err)
		}
	}
}

// Covers: 放行与「有将要建的委托」两格错配不落库；没保全的来源记不了答案。
func TestSubmissionAnswersRefuseWhatCannotHold(t *testing.T) {
	answers, sources, transactor := newSubmissionAnswers(t)
	ctx := t.Context()
	source := identity(t, "tenant-a", "customer-a", "portal", "key-1")
	preserveFor(t, sources, transactor, source)

	allowedWithoutRequest := ports.SubmissionAnswer{
		Decision: ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, nil),
	}
	blockedWithRequest := ports.SubmissionAnswer{
		Decision:          ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, nil),
		BlockReasons:      []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAdmissionPaused},
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "REQ-1"),
	}
	refusalWithGate := ports.SubmissionAnswer{
		Refusal:      ports.SubmissionRefusedLinkIneligible,
		BlockReasons: []domain.FutureSubmissionBlockReason{domain.FutureSubmissionAdmissionPaused},
	}
	for name, answer := range map[string]ports.SubmissionAnswer{
		"放行却不指名委托": allowedWithoutRequest,
		"挡下却指名委托":  blockedWithRequest,
		"拒绝却带门禁原因": refusalWithGate,
	} {
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return answers.RecordSubmissionAnswer(txCtx, source, answer)
		})
		if err == nil {
			t.Errorf("%s：记下了", name)
		}
	}

	unpreserved := identity(t, "tenant-a", "customer-a", "portal", "never-preserved")
	valid := ports.SubmissionAnswer{
		Decision:          ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, nil),
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "REQ-1"),
	}
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return answers.RecordSubmissionAnswer(txCtx, unpreserved, valid)
	}); err == nil {
		t.Error("没保全的来源记下了答案")
	}
	if _, exists, _ := answers.FindLatestSubmissionAnswer(ctx, source); exists {
		t.Error("被拒的写入留下了行")
	}
}

// Covers: 写口在无事务上下文被 RequireExecutor 拒绝：答案要与它的事务边界同生共死，事务边界归装配点。
func TestSubmissionAnswerWritesRefuseToRunOutsideATransaction(t *testing.T) {
	answers, sources, transactor := newSubmissionAnswers(t)
	ctx := t.Context()
	source := identity(t, "tenant-a", "customer-a", "portal", "key-1")
	preserveFor(t, sources, transactor, source)

	answer := ports.SubmissionAnswer{
		Decision:          ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, nil),
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "REQ-1"),
	}
	if err := answers.RecordSubmissionAnswer(ctx, source, answer); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记答案应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := answers.LockSubmission(ctx, source); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务取续办锁应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, exists, err := answers.FindLatestSubmissionAnswer(ctx, source); err != nil || exists {
		t.Errorf("被拒的写入：exists = %v, err = %v", exists, err)
	}
}

// ownershipDecision 按权威 × 准入控制拼一份决定；handoff 非空时按它记上一次交接评估。完整确认的范围摘要与两个引用在这里
// 补齐：它们要与决定的范围对得上，调用方手上还没有那份范围。
func ownershipDecision(
	t *testing.T,
	authority domain.ProductionAuthorityKind,
	control domain.AdmissionControl,
	handoff *ports.ProductionHandoffObservation,
) domain.ProductionOwnershipDecision {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustBuild(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustBuild(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("准入范围：%v", err)
	}
	validity, err := domain.NewOwnershipValidityInterval(answerAnchor.Add(-time.Hour), answerAnchor.Add(time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	spec := domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustBuild(t, domain.NewProductionOwnershipDecisionID, "DEC-"+authority.String()+"-"+control.String()),
		Scope:            scope,
		Authority:        authority,
		AdmissionControl: control,
		RuleVersion:      mustBuild(t, domain.NewProductionOwnershipRuleVersion, "RULE-1"),
		AsOf:             answerAnchor.Add(-time.Minute),
		Validity:         validity,
		Revision:         mustBuild(t, domain.NewProductionOwnershipRevision, "REV-1"),
		DecisionAt:       answerAnchor.Add(-time.Minute),
	}
	switch authority {
	case domain.ProductionAuthorityOther:
		spec.OtherAuthorityRef = mustBuild(t, domain.NewProductionAuthorityReference, "OTHER-1")
		spec.HandoffRef = mustBuild(t, domain.NewHandoffConfirmationReference, "STOP-1")
	case domain.ProductionAuthorityUnresolved:
		spec.UnresolvedReason = domain.OwnershipUnresolvedRuleUnavailable
		spec.ContinuationRef = mustBuild(t, domain.NewOwnershipContinuationReference, "CONT-OWN-1")
	}
	if control == domain.AdmissionControlPaused {
		spec.SuspensionRef = mustBuild(t, domain.NewOwnershipSuspensionReference, "SUSP-1")
	}
	decision, err := domain.NewProductionOwnershipDecision(spec)
	if err != nil {
		t.Fatalf("归属决定：%v", err)
	}
	if handoff == nil {
		return decision
	}
	observed := *handoff
	if observed.Observation == domain.HandoffObservationCompleteConfirmation {
		observed.ConfirmedScopeDigest = scope.Digest()
		observed.ConfirmationRef = mustBuild(t, domain.NewHandoffConfirmationReference, "CONF-1")
		observed.QueryRef = mustBuild(t, domain.NewHandoffQueryReference, "QUERY-1")
	}
	assessment, err := domain.AssessSafeHandoff(domain.SafeHandoffAssessmentSpec{
		AttemptID:            mustBuild(t, domain.NewHandoffAttemptID, "ATTEMPT-1"),
		Scope:                scope,
		TargetAuthority:      spec.OtherAuthorityRef,
		Observation:          observed.Observation,
		ConfirmedScopeDigest: observed.ConfirmedScopeDigest,
		ConfirmationRef:      observed.ConfirmationRef,
		QueryRef:             observed.QueryRef,
		ContinuationRef:      mustBuild(t, domain.NewOwnershipContinuationReference, "CONT-HANDOFF-1"),
		EffectiveAt:          observed.EffectiveAt,
		AssessedAt:           answerAnchor,
	})
	if err != nil {
		t.Fatalf("交接评估：%v", err)
	}
	withHandoff, err := decision.WithSafeHandoff(assessment)
	if err != nil {
		t.Fatalf("记上交接评估：%v", err)
	}
	return withHandoff
}

func newSubmissionAnswers(t *testing.T) (*adapter.SubmissionAnswers, *adapter.SourceSubmissions, bentoapp.Transactor) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	answers, err := adapter.NewSubmissionAnswers(db)
	if err != nil {
		t.Fatalf("构造归属答案仓储：%v", err)
	}
	sources, err := adapter.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源仓储：%v", err)
	}
	return answers, sources, db.Transactor()
}

func preserveFor(t *testing.T, sources *adapter.SourceSubmissions, transactor bentoapp.Transactor, source domain.SourceIdentity) {
	t.Helper()
	occurredAt := answerAnchor.Add(-2 * time.Minute)
	preserved, err := domain.NewSourceSubmissionFingerprint(
		source, mustBuild(t, domain.NewPayloadDigest, "digest-1"), occurredAt, occurredAt.Add(time.Second))
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	mustWithinTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		return sources.Preserve(txCtx, preserved)
	})
}

// Covers: 续办锁按完整来源身份互斥（ports.SubmissionContinuationGuard 在本库的那一半）——同一身份的第二个事务等到第一个结束，
// 别的身份不受影响。
func TestSubmissionLockSerializesOneSourceIdentity(t *testing.T) {
	answers, _, transactor := newSubmissionAnswers(t)
	ctx := t.Context()
	source := identity(t, "tenant-a", "customer-a", "portal", "key-1")

	held := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := answers.LockSubmission(txCtx, source); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	// 外部键相同、作用域任一段不同的来源身份是别的身份，锁互不相干。
	for name, other := range map[string]domain.SourceIdentity{
		"另一个租户":  identity(t, "tenant-b", "customer-a", "portal", "key-1"),
		"另一个客户":  identity(t, "tenant-a", "customer-b", "portal", "key-1"),
		"另一个来源":  identity(t, "tenant-a", "customer-a", "api", "key-1"),
		"另一个请求键": identity(t, "tenant-a", "customer-a", "portal", "key-2"),
	} {
		if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return answers.LockSubmission(txCtx, other)
		}); err != nil {
			t.Errorf("%s取锁：%v", name, err)
		}
	}

	waiting, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if err := transactor.WithinTransaction(waiting, func(txCtx context.Context) error {
		return answers.LockSubmission(txCtx, source)
	}); err == nil {
		t.Error("同一来源身份在持锁期间又取到了锁")
	}

	close(release)
	if err := <-holderDone; err != nil {
		t.Fatalf("持锁事务：%v", err)
	}
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return answers.LockSubmission(txCtx, source)
	}); err != nil {
		t.Fatalf("持锁事务结束后取锁：%v", err)
	}
}
