package tfhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 总单登记册的在线登记口（ADR-0113 决定五；票 tf-carrier-master-document-register/01；写面形状按 ADR-0085
// 两阶段接线）。
//
// 按 ADR-0101 决定八自裁形态：这本册子结构简单（六件事一行一版、关联逐条）、低频（运营配置员登一份主运单
// 或改它的关联 / 适用关系），因此是**逐字段表单**候选，不走模板导入与草稿。登记口签收的就是命令的字段，
// Intake 逐字段翻译，不签收 JSON 快照本体。不开 `parcel-tf-register` CLI：TF 今天的登记全走 parcel-api
// 端点与 dispatch 家族，单为总单开一个 CLI 会让 TF 有两种登记入口。管理台页面归 admin-write-faces 那一族
// 的票，这里只落端点。

// MasterDocumentIntake 把已认证的接入请求翻译成总单首登 / 形成新版本两条命令。
//
// 接口而非本包内解析代码的理由同 DeliveryIntake：租户身份只能来自认证结果（ADR-0003），运营操作者渠道
// （ADR-0100）就位前本包不带任何实现，装配点挂 UnconfiguredIntake。方法名按事实具名，理由在 HandoverIntake。
type MasterDocumentIntake interface {
	IntakeMasterDocumentRegistration(ctx context.Context, request *http.Request) (application.RegisterMasterDocumentCommand, error)
	IntakeMasterDocumentRevision(ctx context.Context, request *http.Request) (application.ReviseMasterDocumentCommand, error)
}

// MasterDocumentRegistrar 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type MasterDocumentRegistrar interface {
	Register(
		ctx context.Context,
		command application.RegisterMasterDocumentCommand,
	) (application.RegisterMasterDocumentResult, error)
	Revise(
		ctx context.Context,
		command application.ReviseMasterDocumentCommand,
	) (application.RegisterMasterDocumentResult, error)
}

// NewRegisterCarrierMasterDocumentEndpoint 交回总单首登的 HTTP 入口。
func NewRegisterCarrierMasterDocumentEndpoint(intake MasterDocumentIntake, registrar MasterDocumentRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterMasterDocumentResult, error, bool) {
		command, err := intake.IntakeMasterDocumentRegistration(request.Context(), request)
		if err != nil {
			return application.RegisterMasterDocumentResult{}, err, false
		}
		result, err := registrar.Register(request.Context(), command)
		return result, err, true
	}, writeMasterDocumentOutcome)
}

// NewReviseCarrierMasterDocumentEndpoint 交回形成新版本（撤销 / 替代 / 关联重述）的 HTTP 入口。首登与新版本
// 分两个端点，理由同凭证：命令形状与恢复动作不同，合成一个入口就得靠请求体里的模式字段分路；三种新版本不再
// 各立端点——它们是同一条命令的三个取值，续办也同一个（去看当前版是谁、什么时候改的）。不叫 `-corrections`：
// 改变的是适用关系或关联，不是更正一个判断，原版本一字不动。
func NewReviseCarrierMasterDocumentEndpoint(intake MasterDocumentIntake, registrar MasterDocumentRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterMasterDocumentResult, error, bool) {
		command, err := intake.IntakeMasterDocumentRevision(request.Context(), request)
		if err != nil {
			return application.RegisterMasterDocumentResult{}, err, false
		}
		result, err := registrar.Revise(request.Context(), command)
		return result, err, true
	}, writeMasterDocumentOutcome)
}

// masterDocumentAssociationBody 是响应里的一条关联：类别词与引用原样透出。
type masterDocumentAssociationBody struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

// masterDocumentResponse 是两个总单端点共用的封闭响应形状。`outcome` 取应用结果枚举原名；带记录的答案把那一版
// 原样透出——`已有版本链`、`已不适用`与链被推进后的`内容冲突`带回的是撞上的当前版，登记方据以决定下一步是在它
// 之上形成新版本还是去看它最后一版是谁收的。时间一律 RFC 3339 UTC。关联是指针切片：没有记录的答案不带这一键，
// 带记录而零关联时是 `[]` 不是 null——零关联是内容（总单先签、集运单元后列），不是缺席。
type masterDocumentResponse struct {
	Outcome               string                           `json:"outcome"`
	UndecidedReason       string                           `json:"undecidedReason,omitempty"`
	ContinuationReference string                           `json:"continuationReference,omitempty"`
	Document              string                           `json:"document,omitempty"`
	Version               string                           `json:"version,omitempty"`
	Issuer                string                           `json:"issuer,omitempty"`
	Scope                 string                           `json:"scope,omitempty"`
	Commission            string                           `json:"commission,omitempty"`
	Booking               string                           `json:"booking,omitempty"`
	Standing              string                           `json:"standing,omitempty"`
	ChangedAt             string                           `json:"changedAt,omitempty"`
	Supersedes            string                           `json:"supersedes,omitempty"`
	ReplacedBy            string                           `json:"replacedBy,omitempty"`
	Associations          *[]masterDocumentAssociationBody `json:"associations,omitempty"`
}

func writeMasterDocumentOutcome(response http.ResponseWriter, result application.RegisterMasterDocumentResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := masterDocumentResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Record(); present {
		document := record.Document
		body.Document = document.Document().String()
		body.Version = document.Version().String()
		body.Issuer = document.Issuer().String()
		body.Scope = document.Scope().String()
		if commission, has := document.Commission(); has {
			body.Commission = commission.String()
		}
		if booking, has := document.Booking(); has {
			body.Booking = booking.String()
		}
		body.Standing = document.Standing().String()
		if changedAt, changed := document.ChangedAt(); changed {
			body.ChangedAt = changedAt.UTC().Format(time.RFC3339)
		}
		if prior, has := document.Supersedes(); has {
			body.Supersedes = prior.String()
		}
		if replacement, has := document.ReplacedBy(); has {
			body.ReplacedBy = replacement.String()
		}
		associations := document.Associations()
		bodies := make([]masterDocumentAssociationBody, 0, len(associations))
		for _, association := range associations {
			bodies = append(bodies, masterDocumentAssociationBody{
				Kind:      association.Kind().String(),
				Reference: association.Reference(),
			})
		}
		body.Associations = &bodies
	}

	// ADR-0022：状态码只报有没有新落一版。首登与新版本都持久化了新版本用 201；重放、冲突、已有版本链、未登记、
	// 已不适用、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.MasterDocumentRegistered ||
		result.Outcome() == application.MasterDocumentRevised {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
