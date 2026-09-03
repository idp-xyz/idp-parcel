package tfhttp

import (
	"errors"
	"net/http"
)

// commandEndpoint 收拢命令端点共同的传输层纪律：方法门、4xx/5xx 分法与结果映射。
//
// 它是交付端点里 `endpoint` 那段逻辑的泛型形状——交付那份先落，按具体结果类型写死；控制事实
// 三个入口各有自己的结果类型，再抄三遍就是四份同一条纪律。此处不改交付那份：把它挪过来是
// 评审阶段的事，不是红绿循环里的事。
//
// 分法与 ADR-0022 镜像应用签名：Intake 没交出命令是调用方或接入层的事（未配置 403、构造不出
// 400、其他 500 INTAKE_FAILED）；编排交回 error 是没形成答案（500 NO_ANSWER_FORMED，**不带
// outcome**）；编排交回结果就交给 write 映射，`未决`在这里已经是形成了的答案。
func commandEndpoint[R any](
	invoke func(*http.Request) (R, error, bool),
	write func(http.ResponseWriter, R),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		result, err, intakeDone := invoke(request)
		if err != nil {
			if !intakeDone {
				if errors.Is(err, ErrAccessChannelNotConfigured) {
					writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
					return
				}
				if errors.Is(err, ErrMalformedRequest) {
					writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
					return
				}
				writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
				return
			}
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		write(response, result)
	})
}
