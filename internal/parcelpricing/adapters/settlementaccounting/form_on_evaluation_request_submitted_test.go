package settlementaccounting_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/settlementaccounting"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证 ppinbox.EvaluationRequestSubmittedConsumer 的处理方（票 sa-cc/11 裁决 5）：按信封引用向 SA 读口取那一份
// 请求 → 译成命令 → 交「按评价请求形成评价」入口 → 把入口的封闭结果折成消费两格：已形成 / 已存在入账不重投；
// 请求还看不见 / 未配置 / 适用冲突 / 输入不可得 / 未决各是一枚哨兵，消费门回滚重投，装配登进未决名单；引用坏了
// 与译不出的答案原样上抛、不译成未决。处理方不判评价结果——形成与否、结果分格全在入口与 EvaluatePricingHandler。

type requestSourceDouble struct {
	record saports.EvaluationRequestRecord
	found  bool
	err    error
	calls  []saports.EvaluationRequestKey
}

func (double *requestSourceDouble) FindByID(
	_ context.Context, key saports.EvaluationRequestKey,
) (saports.EvaluationRequestRecord, bool, error) {
	double.calls = append(double.calls, key)
	return double.record, double.found, double.err
}

type formerDouble struct {
	result   ppapplication.FormEvaluationFromRequestResult
	err      error
	commands []ppapplication.FormEvaluationFromRequestCommand
}

func (double *formerDouble) Handle(
	_ context.Context, command ppapplication.FormEvaluationFromRequestCommand,
) (ppapplication.FormEvaluationFromRequestResult, error) {
	double.commands = append(double.commands, command)
	return double.result, double.err
}

func submitted() ppinbox.SubmittedEvaluationRequest {
	return ppinbox.SubmittedEvaluationRequest{TenantID: "tenant-1", EvaluationRequestID: "EVREQ-SYN-1"}
}

func newFormOnSubmitted(t *testing.T, source *requestSourceDouble, former *formerDouble) *adapter.FormOnEvaluationRequestSubmittedAdapter {
	t.Helper()
	handler, err := adapter.NewFormOnEvaluationRequestSubmittedAdapter(source, former, ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	return handler
}

func foundRecord(t *testing.T) *requestSourceDouble {
	t.Helper()
	return &requestSourceDouble{record: synRecord(t), found: true}
}

// Covers: 主路——按（租户 + 请求 ID）向 SA 读口查一次，译出的命令交入口；已形成入账不重投。命令上的每一格来自
// 记录或装配方声明，处理方自己不发明任何一格。
func TestAFormedEvaluationIsDoneAndTheCommandCameFromTheRecord(t *testing.T) {
	source := foundRecord(t)
	former := &formerDouble{result: ppapplication.FormEvaluationFromRequestResult{Outcome: ppapplication.RequestEvaluationFormed}}

	if err := newFormOnSubmitted(t, source, former).HandleSubmittedEvaluationRequest(t.Context(), submitted()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(source.calls) != 1 || source.calls[0].TenantID.String() != "tenant-1" || source.calls[0].Request.String() != "EVREQ-SYN-1" {
		t.Fatalf("source calls = %+v, want one lookup by the envelope's tenant and request ID", source.calls)
	}
	if len(former.commands) != 1 {
		t.Fatalf("former calls = %d, want 1", len(former.commands))
	}
	command := former.commands[0]
	if command.Tenant.String() != "tenant-1" || command.Request.String() != "EVREQ-SYN-1" || command.Scope.String() != "scope-1" ||
		!command.BasisAt.Equal(occurredAt) || command.Evidence != ppdomain.EvidenceSynthetic ||
		command.Sources.Occurrence != "SYN-OCC-1" {
		t.Fatalf("command = %+v", command)
	}
}

// Covers: 做法 3「同请求重放 → 已存在」在消费侧的落法——已存在与已形成同格：入账不重投。
func TestAnExistingEvaluationIsDoneWithoutRedelivery(t *testing.T) {
	former := &formerDouble{result: ppapplication.FormEvaluationFromRequestResult{Outcome: ppapplication.RequestEvaluationExisting}}
	if err := newFormOnSubmitted(t, foundRecord(t), former).HandleSubmittedEvaluationRequest(t.Context(), submitted()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

// Covers: 请求与信封在 SA 同一事务落库，读不回只剩可见性滞后一种成因——未决重投，不当毒丸、不当提交矛盾；读口自己
// 答不出同格。入口不被调。
func TestARequestNotYetVisibleIsUndecided(t *testing.T) {
	for name, source := range map[string]*requestSourceDouble{
		"读不回":   {found: false},
		"读口答不出": {err: errors.New("connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			former := &formerDouble{}
			err := newFormOnSubmitted(t, source, former).HandleSubmittedEvaluationRequest(t.Context(), submitted())
			if !errors.Is(err, adapter.ErrEvaluationRequestNotVisible) {
				t.Fatalf("err = %v, want ErrEvaluationRequestNotVisible", err)
			}
			if len(former.commands) != 0 {
				t.Fatal("the former was called without a record")
			}
		})
	}
}

// Covers: 入口的三格「等谁」各成一枚哨兵，消息里带着入口给的东西（候选 / 缺项 / 停在哪一口）供运维读——不合成一枚：
// 恢复动作各不相同（登记价卡 / 人裁 / 等别的上下文的读口 / 等依赖），合成一枚运维就分不出该去找谁。
func TestEachWaitingOutcomeIsItsOwnUndecidedSentinel(t *testing.T) {
	candidate := pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-BUY-PLAN-B", "v1")
	for name, tc := range map[string]struct {
		result   ppapplication.FormEvaluationFromRequestResult
		sentinel error
		mentions string
	}{
		"未配置": {
			result:   ppapplication.FormEvaluationFromRequestResult{Outcome: ppapplication.RequestPriceCardNotConfigured},
			sentinel: adapter.ErrPriceCardNotConfigured,
			mentions: "scope-1",
		},
		"适用冲突": {
			result: ppapplication.FormEvaluationFromRequestResult{
				Outcome:    ppapplication.RequestPriceCardApplicabilityConflict,
				Candidates: []ppdomain.VersionReference{candidate},
			},
			sentinel: adapter.ErrPriceCardApplicabilityConflict,
			mentions: "SYN-BUY-PLAN-B@v1",
		},
		"输入不可得": {
			result: ppapplication.FormEvaluationFromRequestResult{
				Outcome: ppapplication.RequestPricingInputUnavailable,
				Missing: []string{"transport-fulfillment: no read-only view of the charge occurrence's member carried objects"},
			},
			sentinel: adapter.ErrPricingInputUnavailable,
			mentions: "transport-fulfillment",
		},
		"未决": {
			result: ppapplication.FormEvaluationFromRequestResult{
				Outcome: ppapplication.RequestEvaluationUndecided,
				Reason:  ppapplication.RequestEvaluationIdentityUnavailable,
			},
			sentinel: adapter.ErrEvaluationFormationUndecided,
			mentions: "EVALUATION_IDENTITY_UNAVAILABLE",
		},
	} {
		t.Run(name, func(t *testing.T) {
			former := &formerDouble{result: tc.result}
			err := newFormOnSubmitted(t, foundRecord(t), former).HandleSubmittedEvaluationRequest(t.Context(), submitted())
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("err = %v, want %v", err, tc.sentinel)
			}
			if !strings.Contains(err.Error(), tc.mentions) {
				t.Fatalf("err %q does not mention %q", err, tc.mentions)
			}
		})
	}
}

// Covers: 引用在 SA 词汇里构造不出（空白租户）是编程错误，原样上抛、不译成未决、不查读口。
func TestAnUntranslatableReferenceIsLoud(t *testing.T) {
	source := foundRecord(t)
	err := newFormOnSubmitted(t, source, &formerDouble{}).HandleSubmittedEvaluationRequest(t.Context(),
		ppinbox.SubmittedEvaluationRequest{TenantID: " ", EvaluationRequestID: "EVREQ-SYN-1"})
	if !errors.Is(err, adapter.ErrUntranslatableReference) {
		t.Fatalf("err = %v, want ErrUntranslatableReference", err)
	}
	if len(source.calls) != 0 {
		t.Fatal("the source was consulted with an untranslatable reference")
	}
}

// Covers: 入口答未受理说明译出来的命令在 PP 词汇里不成形——两侧词汇表分歧，不是等谁；与入口上抛的结构性错误同样
// 原样上抛，装配把它们留在哨兵名单之外让失败码落 publish_failed。
func TestANotAcceptedCommandOrAStructuralErrorIsLoud(t *testing.T) {
	notAccepted := &formerDouble{result: ppapplication.FormEvaluationFromRequestResult{Outcome: ppapplication.RequestEvaluationNotAccepted}}
	err := newFormOnSubmitted(t, foundRecord(t), notAccepted).HandleSubmittedEvaluationRequest(t.Context(), submitted())
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("not accepted: err = %v, want ErrUntranslatableAnswer", err)
	}

	boom := errors.New("unexpected formation outcome")
	failing := &formerDouble{err: boom}
	err = newFormOnSubmitted(t, foundRecord(t), failing).HandleSubmittedEvaluationRequest(t.Context(), submitted())
	if !errors.Is(err, boom) {
		t.Fatalf("structural: err = %v, want the former's error", err)
	}
	for _, sentinel := range []error{
		adapter.ErrEvaluationRequestNotVisible, adapter.ErrPriceCardNotConfigured,
		adapter.ErrPriceCardApplicabilityConflict, adapter.ErrPricingInputUnavailable, adapter.ErrEvaluationFormationUndecided,
	} {
		if errors.Is(err, sentinel) {
			t.Fatalf("a structural error was translated into %v", sentinel)
		}
	}
}

// Covers: 构造期拒 nil 与未声明的证据层级——证据层级不给默认，装配方必须说出这条路今天形成的评价是哪一级。
func TestTheAdapterRefusesNilDependenciesAndAnUndeclaredEvidenceKind(t *testing.T) {
	if _, err := adapter.NewFormOnEvaluationRequestSubmittedAdapter(nil, &formerDouble{}, ppdomain.EvidenceSynthetic); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := adapter.NewFormOnEvaluationRequestSubmittedAdapter(&requestSourceDouble{}, nil, ppdomain.EvidenceSynthetic); err == nil {
		t.Fatal("nil former accepted")
	}
	if _, err := adapter.NewFormOnEvaluationRequestSubmittedAdapter(&requestSourceDouble{}, &formerDouble{}, ""); err == nil {
		t.Fatal("undeclared evidence kind accepted")
	}
}
