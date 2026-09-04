package tfhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 外部承运凭证登记册的在线登记口（label-channel/18；写面形状按 ADR-0085 两阶段接线）。
//
// 按 ADR-0101 决定八自裁形态：这本册子结构简单（五件事、一行一版）、低频（运营配置员登一份
// 订舱号或班次凭证，或改它的适用关系；载运对象那一类的凭证大批到来时走渠道适配器，不走这里），
// 因此是**逐字段表单**候选，不走模板导入与草稿。登记口签收的就是命令的字段，Intake 逐字段翻译，
// 不签收 JSON 快照本体。管理台页面归 admin-write-faces 那一族的票，这里只落端点。

// CredentialIntake 把已认证的接入请求翻译成凭证首登 / 改变适用关系两条命令。
//
// 接口而非本包内解析代码的理由同 DeliveryIntake：租户身份只能来自认证结果（ADR-0003），运营操作者
// 渠道（ADR-0100）就位前本包不带任何实现，装配点挂 UnconfiguredIntake。方法名按事实具名，理由在
// HandoverIntake。
type CredentialIntake interface {
	IntakeCredentialRegistration(ctx context.Context, request *http.Request) (application.RegisterExternalCarrierCredentialCommand, error)
	IntakeCredentialApplicabilityChange(ctx context.Context, request *http.Request) (application.ChangeCredentialApplicabilityCommand, error)
}

// CredentialRegistrar 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type CredentialRegistrar interface {
	Register(
		ctx context.Context,
		command application.RegisterExternalCarrierCredentialCommand,
	) (application.RegisterExternalCarrierCredentialResult, error)
	ChangeApplicability(
		ctx context.Context,
		command application.ChangeCredentialApplicabilityCommand,
	) (application.RegisterExternalCarrierCredentialResult, error)
}

// NewRegisterExternalCarrierCredentialEndpoint 交回凭证首登的 HTTP 入口。
func NewRegisterExternalCarrierCredentialEndpoint(intake CredentialIntake, registrar CredentialRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterExternalCarrierCredentialResult, error, bool) {
		command, err := intake.IntakeCredentialRegistration(request.Context(), request)
		if err != nil {
			return application.RegisterExternalCarrierCredentialResult{}, err, false
		}
		result, err := registrar.Register(request.Context(), command)
		return result, err, true
	}, writeCredentialOutcome)
}

// NewChangeExternalCarrierCredentialApplicabilityEndpoint 交回改变适用关系（作废 / 失效 / 替代）
// 的 HTTP 入口。首登与改变分两个端点，理由同交接：命令形状与恢复动作不同，合成一个入口就得靠
// 请求体里的模式字段分路；三种改变不再各立端点——它们是同一条命令的三个取值，续办也同一个
// （去看当前版是谁、什么时候收的）。
func NewChangeExternalCarrierCredentialApplicabilityEndpoint(intake CredentialIntake, registrar CredentialRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterExternalCarrierCredentialResult, error, bool) {
		command, err := intake.IntakeCredentialApplicabilityChange(request.Context(), request)
		if err != nil {
			return application.RegisterExternalCarrierCredentialResult{}, err, false
		}
		result, err := registrar.ChangeApplicability(request.Context(), command)
		return result, err, true
	}, writeCredentialOutcome)
}

// credentialResponse 是两个凭证端点共用的封闭响应形状。`outcome` 取应用结果枚举原名；带记录的
// 答案把那一版的五件事原样透出——`已有版本链`与`已不适用`带回的是撞上的当前版，登记方据以决定
// 下一步是改它还是去看它最后一版是谁收的。时间一律 RFC 3339 UTC。
type credentialResponse struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	Credential            string `json:"credential,omitempty"`
	Version               string `json:"version,omitempty"`
	Assigner              string `json:"assigner,omitempty"`
	IdentifiedKind        string `json:"identifiedKind,omitempty"`
	IdentifiedRef         string `json:"identifiedRef,omitempty"`
	EffectiveFrom         string `json:"effectiveFrom,omitempty"`
	EffectiveUntil        string `json:"effectiveUntil,omitempty"`
	Standing              string `json:"standing,omitempty"`
	ChangedAt             string `json:"changedAt,omitempty"`
	Supersedes            string `json:"supersedes,omitempty"`
	ReplacedBy            string `json:"replacedBy,omitempty"`
}

func writeCredentialOutcome(response http.ResponseWriter, result application.RegisterExternalCarrierCredentialResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := credentialResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Record(); present {
		credential := record.Credential
		body.Credential = credential.Credential().String()
		body.Version = credential.Version().String()
		body.Assigner = credential.Assigner().String()
		body.IdentifiedKind = credential.Identifies().Kind().String()
		body.IdentifiedRef = credential.Identifies().Reference()
		body.EffectiveFrom = credential.Applicability().From().UTC().Format(time.RFC3339)
		if until, closed := credential.Applicability().Until(); closed {
			body.EffectiveUntil = until.UTC().Format(time.RFC3339)
		}
		body.Standing = credential.Standing().String()
		if changedAt, changed := credential.ChangedAt(); changed {
			body.ChangedAt = changedAt.UTC().Format(time.RFC3339)
		}
		if prior, has := credential.Supersedes(); has {
			body.Supersedes = prior.String()
		}
		if replacement, has := credential.ReplacedBy(); has {
			body.ReplacedBy = replacement.String()
		}
	}

	// ADR-0022：状态码只报有没有新落一版。首登与改变都持久化了新版本用 201；重放、冲突、已有版本链、
	// 未登记、已不适用、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.CredentialRegistered ||
		result.Outcome() == application.CredentialApplicabilityChanged {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
