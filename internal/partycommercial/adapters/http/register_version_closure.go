package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 按后继收尾旧版的两个在线口（ADR-0233）：截止 `/commercial-version-successor-cuts` 与同起点退役
// `/commercial-version-retirements`。两口都是登记册配置写（ADR-0151 的定族判据：提交方是管理商业登记册的操作者，授予
// 按登记册），挂操作者渠道的登记册 Intake；载荷与受控 CLI register-version-closures 的一项逐字同形
// （registrationjson.VersionClosureBatchDocument），在线口各收自己那一节恰一项。
//
// 两种收尾分两口而不合一口由服务端按起点挑：截止留着旧版的前一段、退役让旧版整版不再参选，对已经指名旧版的消费方
// 是两种不同的答复（未生效与已终止）；让服务端替登记方挑，登记方就看不出自己落下的是哪一种。也不并进
// `/commercial-publications`：发布口收的是一份新版本，这两口不立任何版本。

// SuccessorCutIntake 把一次已认证的接入请求翻译成截止命令。租户格只来自认证结果，同本包其余 Intake。
type SuccessorCutIntake interface {
	IntakeSuccessorCut(ctx context.Context, request *http.Request) (application.CutCommercialVersionAtSuccessorCommand, error)
}

// SameStartRetirementIntake 同上，翻译一次同起点退役。
type SameStartRetirementIntake interface {
	IntakeSameStartRetirement(ctx context.Context, request *http.Request) (application.RetireCommercialVersionAtSameStartCommand, error)
}

// VersionClosureRegistrar 是两口转交的编排，判据同使用授权族：Intake 逐口、编排一个用例对象一套答案代数。事务边界在
// 编排侧给出。
type VersionClosureRegistrar interface {
	Cut(ctx context.Context, command application.CutCommercialVersionAtSuccessorCommand) (application.VersionClosureResult, error)
	RetireAtSameStart(ctx context.Context, command application.RetireCommercialVersionAtSameStartCommand) (application.VersionClosureResult, error)
}

var _ VersionClosureRegistrar = (*application.CloseCommercialVersionHandler)(nil)

// NewCutCommercialVersionAtSuccessorEndpoint 交回截止的 HTTP 入口。
func NewCutCommercialVersionAtSuccessorEndpoint(intake SuccessorCutIntake, registrar VersionClosureRegistrar) http.Handler {
	return newRegistrationEndpoint(intake.IntakeSuccessorCut, registrar.Cut, writeVersionClosureAnswer)
}

// NewRetireCommercialVersionAtSameStartEndpoint 交回同起点退役的 HTTP 入口。
func NewRetireCommercialVersionAtSameStartEndpoint(intake SameStartRetirementIntake, registrar VersionClosureRegistrar) http.Handler {
	return newRegistrationEndpoint(intake.IntakeSameStartRetirement, registrar.RetireAtSameStart, writeVersionClosureAnswer)
}

// versionClosureAnswer 在登记族共用的两格之外回显落下（或重放认出）的那一次收尾：截止回显旧版截止后的选用区间，退役
// 回显后继与退役时刻。登记方据它核对，不必另去查册；`未受理`时这几格缺席。
type versionClosureAnswer struct {
	Outcome           string `json:"outcome"`
	Cause             string `json:"cause,omitempty"`
	CorrectedStartsAt string `json:"correctedStartsAt,omitempty"`
	CorrectedEndsAt   string `json:"correctedEndsAt,omitempty"`
	Successor         string `json:"successorVersion,omitempty"`
	RetiredAt         string `json:"retiredAt,omitempty"`
}

// writeVersionClosureAnswer 判据同本包其余登记族：`已登记`取 201，重放与`未受理`是治理答案取 200，原因随 `cause` 过线。
func writeVersionClosureAnswer(response http.ResponseWriter, result application.VersionClosureResult) {
	outcome := result.Outcome()
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := versionClosureAnswer{Outcome: name}
	if cause := result.Cause(); cause != nil {
		answer.Cause = cause.Error()
	}
	if correction, held := result.Correction(); held {
		interval := correction.CorrectedInterval()
		answer.CorrectedStartsAt = rfc3339(interval.StartsAt())
		if end, bounded := interval.EndsAt(); bounded {
			answer.CorrectedEndsAt = rfc3339(end)
		}
	}
	if retirement, held := result.Retirement(); held {
		answer.Successor = retirement.Successor().String()
		answer.RetiredAt = rfc3339(retirement.RetiredAt())
	}
	status := http.StatusOK
	if outcome == application.VersionClosureRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
