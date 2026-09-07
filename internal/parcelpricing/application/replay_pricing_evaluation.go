package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ReplayPricingEvaluationOutcome 是回放请求的应用处理结果。它说的是**编排**发生了什么，不替评价说话：
// 重放本身重现了还是冲突了、还是结构上算不出来，在交回的评价的 Status 与问题项里
// （COMPLETED / CONFLICT + REPLAY_RESULT_MISMATCH 等 / FAILED + CANONICALIZATION_VERSION_UNSUPPORTED），
// 这里不再折一遍——折了就有两处权威。
type ReplayPricingEvaluationOutcome uint8

const (
	ReplayPricingEvaluationOutcomeInvalid ReplayPricingEvaluationOutcome = iota
	// ReplayRecorded：回放评价已以新引用入册。
	ReplayRecorded
	// ReplayExistingResult：同回放引用已在册，且重算后语义相同——重复请求返原记录，不重入册。
	ReplayExistingResult
	// ReplayIdentityConflict：同回放引用已在册，但它回放的不是这份原评价、或语义不同——同标识装了
	// 不同内容，原记录不顶替。
	ReplayIdentityConflict
	// ReplayOriginalNotFound：原评价不在本租户的评价册上。越权探针与真不存在同答（ADR-0029）。
	ReplayOriginalNotFound
	// ReplayPlanVersionNotOnRegister：原评价记录的方案版本不在价卡登记册上——**结构上重放不了**，
	// 不形成评价、不退回在用版本。恢复动作是把那一版登进登记册。
	ReplayPlanVersionNotOnRegister
	// ReplayPlanCanonicalizationUnsupported：原方案的登记快照按另一套规范化形状折装、本构建重建不了
	// （ADR-0014）——同样结构上重放不了，但没有任何登记动作能推动它，只有支持该形状的构建；与上一格
	// 分开，因为恢复动作不同。
	ReplayPlanCanonicalizationUnsupported
	// ReplayNotAccepted：请求不合法——引用为空、新引用等于原引用、证据层级不合法或想把 `S` 重放成
	// 更高层级（领域门 NewReplayEvaluationRequest 拒）。恢复动作是改请求。
	ReplayNotAccepted
	// ReplayUndecided：依赖故障，回放与否未知，原因随错误交回。
	ReplayUndecided
)

func (outcome ReplayPricingEvaluationOutcome) String() string {
	switch outcome {
	case ReplayRecorded:
		return "RECORDED"
	case ReplayExistingResult:
		return "EXISTING_RESULT"
	case ReplayIdentityConflict:
		return "IDENTITY_CONFLICT"
	case ReplayOriginalNotFound:
		return "ORIGINAL_NOT_FOUND"
	case ReplayPlanVersionNotOnRegister:
		return "PLAN_VERSION_NOT_ON_REGISTER"
	case ReplayPlanCanonicalizationUnsupported:
		return "PLAN_CANONICALIZATION_UNSUPPORTED"
	case ReplayNotAccepted:
		return "NOT_ACCEPTED"
	case ReplayUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// ReplayPricingEvaluationCommand 携带一次回放请求。
//
// 新评价引用由调用方铸（CONTEXT「重放必须使用新的评价引用，不得复用原评价 ID」）：它同时是幂等键
// ——同一引用第二次到达返原记录，与评价请求由请求带标识是同一纪律。证据层级也由调用方声明、不给
// 默认：它是回放数据来源的属性（W02 隔离执行的脱敏历史事实记 `R`、合成输入只能记 `S`），编排
// 不知道来源是什么，领域门只守「`S` 不得升级」。租户从信封来（ADR-0100），原评价必须属该租户。
type ReplayPricingEvaluationCommand struct {
	Tenant   domain.TenantID
	Original domain.EvaluationID
	ReplayID domain.EvaluationID
	Evidence domain.EvidenceKind
}

// ReplayPricingEvaluationResult 是回放的答复。字段导出、不设访问器，形状随 ReferenceSeriesPreview：
// 传输层要把它逐字透出，测试要能造出任一格。
type ReplayPricingEvaluationResult struct {
	Outcome ReplayPricingEvaluationOutcome
	// Evaluation 是回放评价——刚入册那一份，或同引用在册返回的原记录；HasEvaluation 为假时它是零值：
	// 没形成评价的答案（原评价不在册、原方案版本不在册……）没有评价可交。
	Evaluation    domain.PricingEvaluation
	HasEvaluation bool
}

// ReplayPricingEvaluationDeps 是回放编排的依赖。
//
// **没有 EvaluationHandoff 这一格，是有意的，不是漏装。** 回放结果不是新费用：SA 的费用采用按评价
// 标识幂等（AT-SA-173 只对同一评价成立），一份带新引用的回放交出去就是一笔重复的费用采用。这一格
// 不在结构上，装配点就装不进去——比一句「不要调」的注释硬。
type ReplayPricingEvaluationDeps struct {
	Store ports.EvaluationStore
	// Plans 按原评价记录的方案版本引用取回原方案——「原评价用的那一版」，不是此刻适用的那一版。
	Plans ports.PriceCardVersionLoader
}

// ReplayPricingEvaluationHandler 是评价重放门的执行编排（票 wiring-baseline-remainder/06 件②）——
// PN-08 W02「历史回放」按对象重放候选规则、CONTEXT 点名的「争议复核」走的都是这一扇门，只是触发者
// 不同。计算全在领域（ReplayPricingEvaluation 纯函数），这里只做取原评价、取原方案、入册与答案翻译。
//
// 别把它与 EvaluatePricingHandler.settleAgainstExisting 认成一回事：那是同标识重复请求的冒名比对——
// 纯函数重算、比语义摘要、不铸新引用、不入册；这里守的是 CONTEXT 那条「重放必须使用新的评价引用……
// 重算结果与原评价不一致时，结果为冲突」，回放评价自己是一条入册的版本化事实。
type ReplayPricingEvaluationHandler struct {
	deps ReplayPricingEvaluationDeps
}

func NewReplayPricingEvaluationHandler(deps ReplayPricingEvaluationDeps) *ReplayPricingEvaluationHandler {
	return &ReplayPricingEvaluationHandler{deps: deps}
}

// Handle 把一次回放请求推进到评价册答案：取原评价（租户不符视同不在册）→ 按原评价记录的方案版本引用
// 取原方案（不在册 / 重建不了各自成格，不形成评价）→ ReplayPricingEvaluation 纯函数（原输入快照、原版本
// 清单、原方案内容摘要与规范化版本都在原评价里，编排不重新解析任何在用版本——missingSeriesBindings 与
// missingCatalogueLinks 对重放请求答「不缺」，这里也不绕过它们）→ 入册；同回放引用已在册时重算比语义
// 摘要分辨重复与冒名。
//
// 依赖故障不吞：回放是治理动作，触发者必须拿到失败原因才能续办，答一个没有成因的`未决`等于没答。
func (handler *ReplayPricingEvaluationHandler) Handle(
	ctx context.Context,
	command ReplayPricingEvaluationCommand,
) (ReplayPricingEvaluationResult, error) {
	if command.Tenant.String() == "" || command.Original.String() == "" || command.ReplayID.String() == "" {
		return ReplayPricingEvaluationResult{Outcome: ReplayNotAccepted}, nil
	}

	original, found, err := handler.deps.Store.FindByID(ctx, command.Original)
	if err != nil {
		return ReplayPricingEvaluationResult{Outcome: ReplayUndecided}, fmt.Errorf("replay pricing evaluation: find original: %w", err)
	}
	// 评价标识全局唯一，读口不按租户过滤；租户在这里核——别人租户的评价对本租户就是不存在
	//（ADR-0003 隔离；越权探针与真不存在同答，ADR-0029）。
	if !found || original.Input().TenantID() != command.Tenant {
		return ReplayPricingEvaluationResult{Outcome: ReplayOriginalNotFound}, nil
	}

	plan, onRegister, err := handler.deps.Plans.FindByReference(ctx, command.Tenant, original.PlanReference())
	switch {
	case errors.Is(err, domain.ErrCanonicalizationVersionUnsupported):
		return ReplayPricingEvaluationResult{Outcome: ReplayPlanCanonicalizationUnsupported}, nil
	case err != nil:
		return ReplayPricingEvaluationResult{Outcome: ReplayUndecided}, fmt.Errorf("replay pricing evaluation: find original plan: %w", err)
	case !onRegister:
		return ReplayPricingEvaluationResult{Outcome: ReplayPlanVersionNotOnRegister}, nil
	}

	replayed, err := domain.ReplayPricingEvaluation(command.ReplayID, original, plan, command.Evidence)
	if err != nil {
		// 只有两种：新引用等于原引用、请求不合法（含 `S` 升级）。两种恢复动作都是改请求。
		return ReplayPricingEvaluationResult{Outcome: ReplayNotAccepted}, nil
	}

	saved, err := handler.deps.Store.Save(ctx, replayed)
	if err != nil {
		return ReplayPricingEvaluationResult{Outcome: ReplayUndecided}, fmt.Errorf("replay pricing evaluation: save: %w", err)
	}
	switch saved {
	case ports.EvaluationSaved:
		return ReplayPricingEvaluationResult{Outcome: ReplayRecorded, Evaluation: replayed, HasEvaluation: true}, nil
	case ports.EvaluationAlreadyRecorded:
		existing, found, err := handler.deps.Store.FindByID(ctx, command.ReplayID)
		if err != nil {
			return ReplayPricingEvaluationResult{Outcome: ReplayUndecided}, fmt.Errorf("replay pricing evaluation: read back existing replay: %w", err)
		}
		if !found {
			// 写时说在、读时说不在——评价行只增不改，这一格在正常运行里到不了；照实答未决，不折成成功。
			return ReplayPricingEvaluationResult{Outcome: ReplayUndecided}, fmt.Errorf("replay pricing evaluation: %s was reported recorded but cannot be read back", command.ReplayID)
		}
		// 同引用在册：回放的是同一份原评价、同一证据层级、且语义摘要相同才是重复请求；否则是同标识
		// 装了别的东西。语义摘要刻意不含 replayOf 与证据层级（重现与否要靠它与原评价逐字相等），所以
		// 那两格在这里单独比。
		if existingOf, isReplay := existing.ReplayOf(); !isReplay || existingOf != command.Original ||
			existing.Evidence() != replayed.Evidence() || existing.SemanticDigest() != replayed.SemanticDigest() {
			return ReplayPricingEvaluationResult{Outcome: ReplayIdentityConflict}, nil
		}
		return ReplayPricingEvaluationResult{Outcome: ReplayExistingResult, Evaluation: existing, HasEvaluation: true}, nil
	default:
		return ReplayPricingEvaluationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedEvaluationSave, saved)
	}
}
