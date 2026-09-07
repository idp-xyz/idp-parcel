package pricinghttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// EvaluationReplayIntake 把一次已认证的接入请求翻译成评价回放命令（ADR-0124 决定一）。
//
// 它是接口，理由与序列复核那一口同源、与两个登记口不同：触发者是谁——谁发起了这次争议复核、
// 谁在 W02 执行——来自 ADR-0100 的 `OperatorEnvelope`，后端不采信自报身份；租户只从信封来。
// 载荷形状由产品定义、属机制半边（ADR-0101 决定一），已在 DecodeEvaluationReplayPayload；这一口
// 等的是操作者信封接线，不是渠道契约。
type EvaluationReplayIntake interface {
	IntakeEvaluationReplay(ctx context.Context, request *http.Request) (application.ReplayPricingEvaluationCommand, error)
}

// EvaluationReplayer 是本端点转交的回放编排，事务边界在编排侧给出（评价册的写口无环境事务即拒）。
type EvaluationReplayer interface {
	Handle(ctx context.Context, command application.ReplayPricingEvaluationCommand) (application.ReplayPricingEvaluationResult, error)
}

// NewReplayEvaluationEndpoint 交回评价回放的 HTTP 入口（POST /pricing-evaluation-replays；ADR-0124）。
//
// 路径叫 `-replays` 不叫 `-registrations`：本上下文这一格的动词是回放，答案代数说的也是回放
// （`已入册` / `原评价不在册` / `原方案版本不在册`……），叫成登记会与登记那一族的答案混为一谈
// ——判据同复核口叫 `-reviews`。
func NewReplayEvaluationEndpoint(intake EvaluationReplayIntake, replayer EvaluationReplayer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeEvaluationReplay(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		result, err := replayer.Handle(request.Context(), command)
		if err != nil {
			// 回放与否未知（依赖故障）不是业务答案：状态码只报「没形成答案」（ADR-0022），
			// 编排把`未决`连同错误一起交回，这里只认错误。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome := result.Outcome.String()
		if outcome == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		body := replayResponse{Outcome: outcome}
		if result.HasEvaluation {
			replay := replayEvaluationBodyOf(result.Evaluation)
			body.Replay = &replay
		}
		// `已入册`是新增一行，取 201；其余（重复返原、冒名冲突、不在册两格、规范化不支持、未受理）
		// 都是形成了的答案，取 200——状态码不替 outcome 说话（ADR-0022）。
		status := http.StatusOK
		if result.Outcome == application.ReplayRecorded {
			status = http.StatusCreated
		}
		writeJSON(response, status, body)
	})
}

// replayResponse 是回放端点的封闭响应形状。`outcome` 只说编排（ADR-0124 决定四）；回放评价重现了、
// 冲突了还是结构上算不出来，在 `replay` 里那份评价自己的状态与问题项上——面上不再折一遍。没形成
// 评价的答案（原评价不在册、原方案版本不在册……）`replay` 缺席。
type replayResponse struct {
	Outcome string                `json:"outcome"`
	Replay  *replayEvaluationBody `json:"replay,omitempty"`
}

// replayEvaluationBody 透出回放评价的身份与结论：新引用、回指的原评价、状态（封闭五格原词）、证据层级、
// 语义摘要（与原评价逐字相等即重现）与问题项。金额与费用行不透出——那是评价详情读法，归评价册的读面。
type replayEvaluationBody struct {
	EvaluationID   string            `json:"evaluationId"`
	ReplayOf       string            `json:"replayOf"`
	Status         string            `json:"status"`
	Evidence       string            `json:"evidence"`
	SemanticDigest string            `json:"semanticDigest"`
	Issues         []replayIssueBody `json:"issues"`
}

type replayIssueBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func replayEvaluationBodyOf(evaluation domain.PricingEvaluation) replayEvaluationBody {
	body := replayEvaluationBody{
		EvaluationID:   evaluation.ID().String(),
		Status:         string(evaluation.Status()),
		Evidence:       string(evaluation.Evidence()),
		SemanticDigest: evaluation.SemanticDigest(),
		Issues:         make([]replayIssueBody, 0, len(evaluation.Issues())),
	}
	if replayOf, isReplay := evaluation.ReplayOf(); isReplay {
		body.ReplayOf = replayOf.String()
	}
	// 空问题项交回空数组而不是 null：重现了的回放没有问题项，调用方判「没有」不该先判「有没有字段」。
	for _, issue := range evaluation.Issues() {
		body.Issues = append(body.Issues, replayIssueBody{Code: issue.Code(), Message: issue.Message()})
	}
	return body
}

// EvaluationReplayPayload 是运营操作者面的回放载荷线格式（ADR-0124 决定三；逐字段表单——回放低频、
// 结构简单，ADR-0101 决定八自裁）。三样都由触发方声明、一样不给默认：
//
//   - originalEvaluationId：要回放的那份评价。
//   - replayEvaluationId：**新**评价引用，由触发方铸（CONTEXT「重放必须使用新的评价引用」），同时是
//     幂等键——同一引用第二次到达返原记录。
//   - evidence：回放执行记录的证据层级（S / R / P，CONTEXT「回放执行记录按其证据来源标记」）。它是回放
//     数据来源的属性，执行器不知道来源是什么；面上不填 `S` 当默认——那个默认在今天走得到的每一个环境里
//     都对、在 W02 要去的那个环境里恰好错。`S` 不得升级由领域门守，这里不重写那条判据。
//
// **载荷里只有内容，没有身份。** 租户从 `OperatorEnvelope` 来、由 Intake 作为入参交进来；载荷里出现
// tenant 之类的键按未知键拒——严格解码不是挑剔，是不让自报身份有地方落。
type EvaluationReplayPayload struct {
	OriginalEvaluationID string `json:"originalEvaluationId"`
	ReplayEvaluationID   string `json:"replayEvaluationId"`
	Evidence             string `json:"evidence"`
}

// DecodeEvaluationReplayPayload 只做结构解码：JSON 合法、键都认识。字段值对不对留给 Command 里的领域
// 构造器——这里不另造一套校验。
func DecodeEvaluationReplayPayload(body io.Reader) (EvaluationReplayPayload, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var payload EvaluationReplayPayload
	if err := decoder.Decode(&payload); err != nil {
		return EvaluationReplayPayload{}, fmt.Errorf("%w: evaluation replay payload: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

// Command 把载荷连同信封给的租户翻成回放命令。每一格都过领域构造器或封闭集，拒了就是
// ErrMalformedRequest——同一份内容重发不会变好。证据层级缺席同样是畸形请求，不是「默认 S」。
func (payload EvaluationReplayPayload) Command(tenant domain.TenantID) (application.ReplayPricingEvaluationCommand, error) {
	if tenant.String() == "" {
		return application.ReplayPricingEvaluationCommand{}, ErrOperatorIdentityMissing
	}
	original, err := domain.NewEvaluationID(payload.OriginalEvaluationID)
	if err != nil {
		return application.ReplayPricingEvaluationCommand{}, fmt.Errorf("%w: originalEvaluationId: %v", ErrMalformedRequest, err)
	}
	replayID, err := domain.NewEvaluationID(payload.ReplayEvaluationID)
	if err != nil {
		return application.ReplayPricingEvaluationCommand{}, fmt.Errorf("%w: replayEvaluationId: %v", ErrMalformedRequest, err)
	}
	evidence := domain.EvidenceKind(payload.Evidence)
	switch evidence {
	case domain.EvidenceSynthetic, domain.EvidenceReplay, domain.EvidenceProduction:
	default:
		return application.ReplayPricingEvaluationCommand{}, fmt.Errorf("%w: evidence %q is not one of S / R / P", ErrMalformedRequest, payload.Evidence)
	}
	return application.ReplayPricingEvaluationCommand{
		Tenant:   tenant,
		Original: original,
		ReplayID: replayID,
		Evidence: evidence,
	}, nil
}
