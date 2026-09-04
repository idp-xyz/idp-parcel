package tfhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 轨迹源有效时间规则目录的在线登记口（label-channel/19；写面形状按 ADR-0085 两阶段接线）。
//
// 按 ADR-0101 决定八自裁形态：这本目录结构极简（一源一链、一版三件正文）、低频（运营配置员接一家源时
// 登一版，判错了再登一版），因此是**逐字段表单**，不走模板导入与草稿。载荷形状由产品定义、属机制半边
// （ADR-0101 决定一），本文件把它落成 EffectiveTimeRuleRegistrationPayload——**载荷里只有内容，没有身份**，
// 租户从 ADR-0100 的操作者信封来，由 Intake 作为入参交进翻译。

// ErrOperatorIdentityMissing 表示调用方没交来租户——那不是载荷的错（载荷本来就不该带它），是 Intake 没拿
// 到操作者信封就来翻译。与 ErrMalformedRequest 分开：前者改载荷没用。
var ErrOperatorIdentityMissing = errors.New("transport fulfillment http: operator identity (tenant) is required")

// EffectiveTimeRuleIntake 把已认证的接入请求翻译成规则登记命令。接口而非本包内解析代码的理由同
// CredentialIntake：租户身份只能来自认证结果（ADR-0003），操作者渠道（ADR-0100）就位前本包不带任何实现，
// 装配点挂 UnconfiguredIntake。
type EffectiveTimeRuleIntake interface {
	IntakeEffectiveTimeRuleRegistration(ctx context.Context, request *http.Request) (application.RegisterEffectiveTimeRuleCommand, error)
}

// EffectiveTimeRuleRegistrar 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type EffectiveTimeRuleRegistrar interface {
	Register(
		ctx context.Context,
		command application.RegisterEffectiveTimeRuleCommand,
	) (application.RegisterEffectiveTimeRuleResult, error)
}

// EffectiveTimeRuleRegistrationPayload 是载荷的线格式。字段与 application.RegisterEffectiveTimeRuleCommand
// 一一对应，只少租户一格；词取 domain 封闭集原词，对不对留给领域构造门。偏移以整秒传——规则的偏移是所有者
// 登记的业务量，秒是它最细的单位，ISO 8601 时长在这里只会多一层解析且没有额外表达力。
type EffectiveTimeRuleRegistrationPayload struct {
	Source            string `json:"source"`
	Version           string `json:"version"`
	SourceTimeMeaning string `json:"sourceTimeMeaning"`
	Anchor            string `json:"anchor"`
	OffsetSeconds     int64  `json:"offsetSeconds"`
}

// DecodeEffectiveTimeRuleRegistrationPayload 只做结构解码：JSON 合法、键都认识。严格解码不是挑剔，是不让自报
// 身份有地方落——载荷里出现 tenant 之类的键一律按未知键拒。
func DecodeEffectiveTimeRuleRegistrationPayload(body io.Reader) (EffectiveTimeRuleRegistrationPayload, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var payload EffectiveTimeRuleRegistrationPayload
	if err := decoder.Decode(&payload); err != nil {
		return EffectiveTimeRuleRegistrationPayload{}, fmt.Errorf("%w: effective time rule payload: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

// Command 把载荷连同信封给的租户翻成登记命令。这里不过领域构造门：词在不在集合内由编排答`未受理`，
// 那是形成了的业务答案（200），不是坏请求（400）——两者的续办不同，前者要改词，后者要改报文。
func (payload EffectiveTimeRuleRegistrationPayload) Command(tenant domain.TenantID) (application.RegisterEffectiveTimeRuleCommand, error) {
	if tenant.String() == "" {
		return application.RegisterEffectiveTimeRuleCommand{}, ErrOperatorIdentityMissing
	}
	return application.RegisterEffectiveTimeRuleCommand{
		TenantID:          tenant,
		Source:            payload.Source,
		Version:           payload.Version,
		SourceTimeMeaning: payload.SourceTimeMeaning,
		Anchor:            payload.Anchor,
		Offset:            time.Duration(payload.OffsetSeconds) * time.Second,
	}, nil
}

// NewRegisterEffectiveTimeRuleEndpoint 交回规则登记的 HTTP 入口。首登与换版是同一个入口：一源一链，该源
// 有没有当前版由编排看目录，登记方只登「这一版正文」。
func NewRegisterEffectiveTimeRuleEndpoint(intake EffectiveTimeRuleIntake, registrar EffectiveTimeRuleRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterEffectiveTimeRuleResult, error, bool) {
		command, err := intake.IntakeEffectiveTimeRuleRegistration(request.Context(), request)
		if err != nil {
			return application.RegisterEffectiveTimeRuleResult{}, err, false
		}
		result, err := registrar.Register(request.Context(), command)
		return result, err, true
	}, writeEffectiveTimeRuleOutcome)
}

// effectiveTimeRuleResponse 是封闭响应形状。`outcome` 取应用结果枚举原名；带记录的答案把那一版三件正文
// 与前版原样透出——`内容冲突`带回的是撞上的既有版本。偏移用指针：零偏移是登记出来的值，必须在场；
// 没有记录的答案才缺席。时间一律 RFC 3339 UTC。
type effectiveTimeRuleResponse struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	Source                string `json:"source,omitempty"`
	Version               string `json:"version,omitempty"`
	SourceTimeMeaning     string `json:"sourceTimeMeaning,omitempty"`
	Anchor                string `json:"anchor,omitempty"`
	OffsetSeconds         *int64 `json:"offsetSeconds,omitempty"`
	Supersedes            string `json:"supersedes,omitempty"`
	RecordedAt            string `json:"recordedAt,omitempty"`
}

func writeEffectiveTimeRuleOutcome(response http.ResponseWriter, result application.RegisterEffectiveTimeRuleResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := effectiveTimeRuleResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Record(); present {
		rule := record.Rule
		content := rule.Content()
		offset := int64(content.Offset / time.Second)
		body.Source = rule.Source().String()
		body.Version = rule.Version().String()
		body.SourceTimeMeaning = content.SourceTimeMeaning.String()
		body.Anchor = content.Anchor.String()
		body.OffsetSeconds = &offset
		if prior, has := rule.Supersedes(); has {
			body.Supersedes = prior.String()
		}
		body.RecordedAt = record.RecordedAt.UTC().Format(time.RFC3339)
	}

	// ADR-0022：状态码只报有没有新落一版。首登与换版都持久化了新版本用 201；重放、冲突、未受理、未决都是
	// 形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.EffectiveTimeRuleRegistered ||
		result.Outcome() == application.EffectiveTimeRuleRevised {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
