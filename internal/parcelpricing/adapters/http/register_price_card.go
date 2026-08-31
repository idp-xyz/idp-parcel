package pricinghttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// codeUnnamedOutcome 同命令面先例：应用层交回没有名字的结果是编程错误，不是业务
// 答案，不能带空 `outcome` 上线——空字符串会被客户端当成一种新的业务结果。
const codeUnnamedOutcome = "UNNAMED_OUTCOME"

// PriceCardRegistrationIntake 把一次已认证的接入请求翻译成价卡登记命令。
//
// 它是接口而不是本包内的解析代码（ADR-0085 Decision 二，机制同 ADR-0055）：登记
// 快照的重建门在领域侧已实现（RehydratePriceCardRegistration），但「渠道原始载荷 →
// 登记快照」的翻译与操作者认证属渠道接入契约，`PAR-INT-01` 待提供；采信自报租户会
// 穿透 ADR-0003 的隔离边界。未决期间本包不带任何实现，包括「开发用」的采信头部版本。
type PriceCardRegistrationIntake interface {
	IntakePriceCardRegistration(ctx context.Context, request *http.Request) (application.RegisterPriceCardCommand, error)
}

// PriceCardRegistrar 是本端点转交的登记编排。事务边界在编排侧给出（登记写口无环境
// 事务即拒，先例同登记 CLI 的 execute），适配器只转交与映射，不判断任何业务结果。
type PriceCardRegistrar interface {
	Handle(ctx context.Context, command application.RegisterPriceCardCommand) (application.RegisterPriceCardOutcome, error)
}

// NewRegisterPriceCardEndpoint 交回价卡登记的 HTTP 入口（ADR-0085 Decision 一：登记
// 写面进端点表带未配置格；在线口与登记 CLI 消费同一登记用例，答案代数一致）。
func NewRegisterPriceCardEndpoint(intake PriceCardRegistrationIntake, registrar PriceCardRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakePriceCardRegistration(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		outcome, err := registrar.Handle(request.Context(), command)
		if err != nil {
			// 登记与否未知（依赖故障）不是业务答案：状态码只报「没形成答案」
			//（ADR-0022），客户端据以重试或转人工，成因去查服务端记录。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRegistrationAnswer(response, outcome.String(), outcome == application.PriceCardRecorded)
	})
}

// registrationResponse 是登记端点的封闭响应形状。`outcome` 取应用结果枚举原名——与
// 登记 CLI 的答案代数同源：治理答案（内容冲突/形状不可比）是答案不是失败，传输层
// 不合并、不改名，也没有「其他」这一格。
type registrationResponse struct {
	Outcome string `json:"outcome"`
}

// writeRegistrationIntakeProblem 与目录查阅侧的分流判据相同（403 未配置 / 400 畸形 /
// 5xx Intake 故障），单立一份不复用查阅侧助手：两侧各随自己的端点族演化，命令面的
// 分流将来要加载荷规范化摘要一格（ADR-0055 Decision 五），不该牵动查阅行。
func writeRegistrationIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

func writeRegistrationAnswer(response http.ResponseWriter, outcome string, recorded bool) {
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if recorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, registrationResponse{Outcome: outcome})
}
