package tfhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 外部承运轨迹事实有效时间显式判断的在线写面（label-channel/21；写面形状按 ADR-0085 两阶段接线）。
//
// 这是 ADR-0102 决定三第一种来源——「所有者就这一条显式给出」——的管理台入口。按 ADR-0101 决定八自裁形态：
// 一次判断只有两件事（指名哪条事实、从何时起有效），逐条判不批量导入，因此是**逐字段表单**。载荷只有内容
// 没有身份：租户从 ADR-0100 的操作者信封来，由 Intake 作为入参交进翻译（同 EffectiveTimeRuleRegistrationPayload）。
//
// 面上不解释状态词、不建议一个「推荐的有效时间」——那是替所有者判断（票 21 红线）；本文件只转交与映射。

// EffectiveTimeJudgmentIntake 把已认证的接入请求翻译成判断命令。接口而非本包内解析代码的理由同
// CredentialIntake：租户身份只能来自认证结果（ADR-0003），操作者渠道（ADR-0100）就位前本包不带任何实现，
// 装配点挂 UnconfiguredIntake。
type EffectiveTimeJudgmentIntake interface {
	IntakeEffectiveTimeJudgment(ctx context.Context, request *http.Request) (application.JudgeEffectiveTimeCommand, error)
}

// EffectiveTimeJudge 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type EffectiveTimeJudge interface {
	Judge(ctx context.Context, command application.JudgeEffectiveTimeCommand) (application.JudgeEffectiveTimeResult, error)
}

// EffectiveTimeJudgmentPayload 是载荷的线格式：指名的事实与所有者给出的有效时间（RFC 3339）。指名到事实
// 而不是版本——判断落在此刻的当前版之上、回指它，版本由编排读目录（application.JudgeEffectiveTimeCommand）。
type EffectiveTimeJudgmentPayload struct {
	Fact        string `json:"fact"`
	EffectiveAt string `json:"effectiveAt"`
}

// DecodeEffectiveTimeJudgmentPayload 只做结构解码：JSON 合法、键都认识。严格解码不是挑剔，是不让自报身份
// 有地方落——载荷里出现 tenant 之类的键一律按未知键拒。
func DecodeEffectiveTimeJudgmentPayload(body io.Reader) (EffectiveTimeJudgmentPayload, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var payload EffectiveTimeJudgmentPayload
	if err := decoder.Decode(&payload); err != nil {
		return EffectiveTimeJudgmentPayload{}, fmt.Errorf("%w: effective time judgment payload: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

// Command 把载荷连同信封给的租户翻成判断命令。时刻按 RFC 3339 解：解不出是坏报文（400，要改的是报文），
// 空着则交给编排答`未受理`（200，形成了的业务答案）——两者的续办不同，不折成一格。事实引用是否成形同样
// 留给编排：这里不过领域构造门。
func (payload EffectiveTimeJudgmentPayload) Command(tenant domain.TenantID) (application.JudgeEffectiveTimeCommand, error) {
	if tenant.String() == "" {
		return application.JudgeEffectiveTimeCommand{}, ErrOperatorIdentityMissing
	}
	command := application.JudgeEffectiveTimeCommand{TenantID: tenant, Fact: payload.Fact}
	if raw := strings.TrimSpace(payload.EffectiveAt); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return application.JudgeEffectiveTimeCommand{}, fmt.Errorf("%w: effectiveAt is not an RFC 3339 instant: %v", ErrMalformedRequest, err)
		}
		command.EffectiveAt = at.UTC()
	}
	return command, nil
}

// NewJudgeEffectiveTimeEndpoint 交回有效时间显式判断的 HTTP 入口。一条事实一次判断：当前版已按同一时间
// 显式判过即重放，其余情形换新版本回指前版并交 visibility-exception——分格由编排作答，这里只映射。
func NewJudgeEffectiveTimeEndpoint(intake EffectiveTimeJudgmentIntake, judge EffectiveTimeJudge) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.JudgeEffectiveTimeResult, error, bool) {
		command, err := intake.IntakeEffectiveTimeJudgment(request.Context(), request)
		if err != nil {
			return application.JudgeEffectiveTimeResult{}, err, false
		}
		result, err := judge.Judge(request.Context(), command)
		return result, err, true
	}, writeEffectiveTimeJudgmentOutcome)
}

// effectiveTimeJudgmentResponse 是封闭响应形状。`outcome` 取应用结果枚举原名；带记录的答案把那一版原样透出
// ——`已按同值判过`带回的是既有的判断版本及其依据，让面上摆得出「这一条已经判过、按哪个依据判的」；
// handoffReference 只在新版本已登记而意图没交出去时在场。三个时间三键归属不同（ADR-0102），有效时间只在
// 判断过的版本上在场。时间一律 RFC 3339 UTC。
type effectiveTimeJudgmentResponse struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	HandoffReference      string `json:"handoffReference,omitempty"`
	Fact                  string `json:"fact,omitempty"`
	Version               string `json:"version,omitempty"`
	Source                string `json:"source,omitempty"`
	Credential            string `json:"credential,omitempty"`
	Object                string `json:"object,omitempty"`
	SourceEvent           string `json:"sourceEvent,omitempty"`
	Status                string `json:"status,omitempty"`
	OccurredAt            string `json:"occurredAt,omitempty"`
	ReceivedAt            string `json:"receivedAt,omitempty"`
	EffectiveBasis        string `json:"effectiveBasis,omitempty"`
	EffectiveAt           string `json:"effectiveAt,omitempty"`
	EffectiveRule         string `json:"effectiveRule,omitempty"`
	EffectiveRuleVersion  string `json:"effectiveRuleVersion,omitempty"`
	Supersedes            string `json:"supersedes,omitempty"`
	RecordedAt            string `json:"recordedAt,omitempty"`
}

func writeEffectiveTimeJudgmentOutcome(response http.ResponseWriter, result application.JudgeEffectiveTimeResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := effectiveTimeJudgmentResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
		HandoffReference:      result.HandoffReference(),
	}
	if record, present := result.Record(); present {
		fact := record.Fact
		body.Fact = fact.Fact().String()
		body.Version = fact.Version().String()
		body.Source = fact.Source().String()
		body.Credential = fact.Credential().String()
		body.Object = fact.Object().String()
		if event, given := fact.SourceEvent(); given {
			body.SourceEvent = event.String()
		}
		body.Status = fact.Status().String()
		body.OccurredAt = fact.OccurredAt().UTC().Format(time.RFC3339)
		body.ReceivedAt = fact.ReceivedAt().UTC().Format(time.RFC3339)
		body.EffectiveBasis = fact.Effective().Basis().String()
		if at, judged := fact.EffectiveAt(); judged {
			body.EffectiveAt = at.UTC().Format(time.RFC3339)
		}
		if rule, byRule := fact.Effective().Rule(); byRule {
			body.EffectiveRule = rule.Rule()
			body.EffectiveRuleVersion = rule.Version()
		}
		if prior, has := fact.Supersedes(); has {
			body.Supersedes = prior.String()
		}
		body.RecordedAt = record.RecordedAt.UTC().Format(time.RFC3339)
	}

	// ADR-0022：状态码只报有没有新落一版。判断持久化了新版本用 201；同值重放、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.EffectiveTimeJudged {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
