package commercialhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// PublicationVocabularyIntake 是词表读口的准入：把一次已认证的接入请求翻译成「可以读」或拒绝。
//
// 它只答准入、不交出作用域：词表是产品级封闭集（各册正文的领域枚举），没有租户维也没有页大小，Intake 没有
// 东西可翻译。但它仍是运营操作者面的一口——词表只为发布表单服务，等的与五步路径四口是同一样东西（ADR-0100
// 信封里的操作者），所以准入形跟着那四口走，不跟着目录查阅走：它不消费任何存储读面，不满足隔离读放行
// （ADR-0078）入格的判据；单独成一种接口，隔离读的 Intake 在编译期就装不进来（同预览口）。
type PublicationVocabularyIntake interface {
	IntakePublicationVocabularyQuery(ctx context.Context, request *http.Request) error
}

// outcomePublicationVocabularyListed 是本端点唯一的业务成格。空集合列表也是这一格：kind 合法只是没词。
const outcomePublicationVocabularyListed = "PUBLICATION_VOCABULARY_LISTED"

// NewQueryPublicationVocabularyEndpoint 交回商业发布词表读口的 HTTP 入口（GET /commercial-publication-vocabularies?kind=；
// 票 admin-write-faces/20，通道 1 代裁）：一口按 kind 答该册正文里全部封闭集的码，发布表单据此供下拉、不内置枚举。
//
// 一口按 kind 答而不是一册一口：五步路径已经是一套载荷 CommercialPublicationPayload 带 kind 分册正文（ADR-0126），
// 词表跟着同一条分派走是同一形；一册一口要在装配表加好几行、几张表单票改同一文件，正是第 1 波刻意避开的撞点。
// 只答码不答中文：中文留在 admin-web 各页词表。
//
// kind 属传输形状（与方法检查同级、先于 Intake，判据同 /commercial-policies：未配置 Intake 对全部类别同答 403，
// 分支选择不泄露任何东西）。集合外或缺席答 400 + MALFORMED_REQUEST 并把 kind 那一格的问题带在响应体里——与命令口
// 的逐格问题同一形状——不静默答空：「没这一册」与「这一册没词」要人做的事不同。kind 合法而没词答 200 + 空 sets。
func NewQueryPublicationVocabularyEndpoint(intake PublicationVocabularyIntake) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		name := request.URL.Query().Get("kind")
		kind, known := domain.CommercialObjectKindNamed(name)
		if !known {
			problems := &PublicationPayloadProblems{}
			problems.add("kind", fmt.Errorf("集合外的商业对象类别 %q", name))
			writeJSON(response, http.StatusBadRequest, problemWithFieldsResponse{Error: problemWithFields{
				Code:     codeMalformedRequest,
				Problems: payloadProblemAnswersOf(problems),
			}})
			return
		}

		if err := intake.IntakePublicationVocabularyQuery(request.Context(), request); err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		sets, err := domain.PublicationVocabulary(kind)
		if err != nil {
			// kind 已过反查，领域再拒只能是编程错误；不带 outcome 上线，判据同 codeUnnamedOutcome。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, publicationVocabularyAnswer{
			Outcome: outcomePublicationVocabularyListed,
			Kind:    kind.String(),
			Sets:    vocabularySetAnswersOf(sets),
		})
	})
}

// publicationVocabularyAnswer 是本口的封闭响应形状：kind 回显原词，sets 一律在场——没词是空数组不是缺键，
// 调用方按 sets 迭代，不必特判哪一册。
type publicationVocabularyAnswer struct {
	Outcome string                `json:"outcome"`
	Kind    string                `json:"kind"`
	Sets    []vocabularySetAnswer `json:"sets"`
}

// vocabularySetAnswer 镜像 domain.VocabularySet：name 是载荷里那格的键名，codes 按领域枚举顺序。
type vocabularySetAnswer struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
}

func vocabularySetAnswersOf(sets []domain.VocabularySet) []vocabularySetAnswer {
	answers := make([]vocabularySetAnswer, 0, len(sets))
	for _, set := range sets {
		answers = append(answers, vocabularySetAnswer{Name: set.Name, Codes: append([]string{}, set.Codes...)})
	}
	return answers
}
