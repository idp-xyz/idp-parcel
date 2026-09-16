package commercialhttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// codeUnnamedOutcome 同命令面先例：应用层交回没有名字的结果是编程错误，不是业务
// 答案，不能带空 `outcome` 上线——空字符串会被调用侧当成一种新的业务结果。
const codeUnnamedOutcome = "UNNAMED_OUTCOME"

// newRegistrationEndpoint 是本上下文全部登记端点共用的传输壳：方法门 → Intake →
// 编排 → 交给该族自己的答案转写。
//
// 三族（商业权威依据发布、参与方身份、产品渠道）共用这一层而各自带 write：方法门、
// Intake 分流与「编排交回 error 即没形成答案」逐字相同，抄三遍等于把同一条规则摊到
// 三处，收紧时改一处漏两处；而答案代数三族互不相同（发布多出未决与声明落点，身份
// 多出已停用与未找到），转写不能合并。
//
// 收函数值而不是收种类再自己分派：各族的命令类型互不相同，泛型参数由方法值推出，把
// 一族的编排接到另一族的端点上编译期就红。
func newRegistrationEndpoint[Command, Result any](
	intake func(context.Context, *http.Request) (Command, error),
	register func(context.Context, Command) (Result, error),
	write func(http.ResponseWriter, Result),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		result, err := register(request.Context(), command)
		if err != nil {
			// 登记与否未知（依赖故障）不是业务答案：状态码只报「没形成答案」
			// （ADR-0022），调用侧据以重试或转人工，成因去查服务端记录。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		write(response, result)
	})
}

// writeRegistrationIntakeProblem 与目录查阅侧的分流判据相同（403 未配置 / 400 畸形 /
// 5xx Intake 故障），单立一份不复用查阅侧助手：两侧各随自己的端点族演化，命令面的
// 分流将来要加载荷规范化摘要一格（ADR-0055 Decision 五），不该牵动查阅行。
func writeRegistrationIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	// 解码收齐的逐格问题（运营操作者面载荷，ADR-0126 Decision 四）仍是「请求畸形」——改载荷才会好——只是多告诉
	// 调用方哪几格；与光秃秃的 400 走同一格、同一个码，响应体多一节 problems。
	var problems *PublicationPayloadProblems
	if errors.As(err, &problems) {
		writeJSON(response, http.StatusBadRequest, problemWithFieldsResponse{Error: problemWithFields{
			Code:     codeMalformedRequest,
			Problems: payloadProblemAnswersOf(problems),
		}})
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		// 形状错的理由 Intake 已经写在错误里（自报租户、未知键、不恰一项、别的口的项、尾随内容……），只交 code 是把服务端
		// 知道的事丢在线的这一头；交 detail 与 `cause` 同一条理由：登记方拿一个没有指名的拒绝什么也补不了。
		writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest, malformedRequestDetail(err))
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

// malformedRequestDetail 从 ErrMalformedRequest 的包装散文里剪出哨兵之后那半。
//
// 按哨兵文本剪而不是 errors.Unwrap：`fmt.Errorf("%w: …", ErrMalformedRequest)` 解包得到的是哨兵本身，理由散文在
// 外层的 Error() 里、哨兵之后。哨兵自己那句英文原句不重复进 detail——它说的就是 code 已经说了的那件事。哨兵不在
// 文本里（自带措辞、经 Is 认哨兵的错误类型）时整句就是散文；剪完为空（错误只有哨兵、没带理由）时那一格缺席。
func malformedRequestDetail(err error) string {
	message := err.Error()
	sentinel := ErrMalformedRequest.Error()
	index := strings.Index(message, sentinel)
	if index < 0 {
		return message
	}
	return strings.TrimSpace(strings.TrimPrefix(message[index+len(sentinel):], ":"))
}

type problemWithFieldsResponse struct {
	Error problemWithFields `json:"error"`
}

type problemWithFields struct {
	Code     string                 `json:"code"`
	Problems []payloadProblemAnswer `json:"problems,omitempty"`
}

// registrationAnswer 是参与方身份族与产品渠道族共用的封闭响应形状。`outcome` 取应用
// 结果枚举原名——与受控登记 CLI 的答案代数同源：重放与内容冲突是治理答案不是失败，
// 传输层不合并、不改名，也没有「其他」这一格。
//
// `cause` 是散文不是格。两族的`未受理`把原因随结果交回（application 的
// PartyRegistryResult.Cause 与 ProductChannelResult.Cause），而那些原因今天没有封闭
// 代数——「修订必须连续」「参与方未登记，法人不钉悬空身份」「版本已收尾」各是一句话。
// 照原样过线而不是丢掉，是因为登记方拿一个没有指名的`未受理`什么也补不了；标成散文
// 而不给它判别子的地位，是因为调用侧一旦对着这些句子分支，用例改一个字就会拆掉它。
// 要可判别的理由代数得在用例侧立（先例：网络目录登记的 RefusalReason 是封闭枚举），
// 不是在传输层按字符串拼。
type registrationAnswer struct {
	Outcome string `json:"outcome"`
	Cause   string `json:"cause,omitempty"`
}
