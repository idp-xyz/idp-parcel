package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// DeclaredSourceDataAmendmentAllowance 把 party-commercial 的资料修订允许声明（接单规则包版本下的
// 第三族阶段内容声明，ADR-0120）接到 ports.SourceDataRuleDeclaration 上——票 ps-port-remainder/02
// 余下的那段；它替下的 UnconfiguredSourceDataRuleDeclaration 随声明读口立起退场。
//
// 只翻译不判断（ADR-0025 消费方侧）：拥有版本经 AdoptedStageOwner 回指接受时固定的接单规则包
// （ADR-0062，与 DeclaredStageContent 同一条回指路径），声明经 PC 读口取回，「这一格能不能改」由 PC 的
// AllowanceFor 连同父行「封闭」标记一起算出（ADR-0120 Decision 四），这里只把 PC 三值一对一译成 PS
// 三值。阶段与意图两个封闭集的词是 PS 的原词、PC 只镜像（ADR-0120 Decision 五）；译法是逐格显式的
// switch 而不是按 String() 对字——对字会让两侧词漂移变成静默的译错，钉相等的地方是旁边的跨侧测试。
//
// 两处 found=false 都译 NotDeclared：采用版本尚未固定（闭包不在），与声明未登记（无父行）一样都是
// 「还没人说这处资料能不能改」，编排据以停在待复核；两者的恢复动作都在提供方（固定闭包 / 按 PAR-COM-13
// 登记），不是重试。读不回与声明立不住原样上抛，编排落 SourceDataRuleUnavailable——那一格的恢复动作才是
// 重试，与前两格合成一格续办方就分不清该催人还是该重试。
type DeclaredSourceDataAmendmentAllowance struct {
	owners     AdoptedStageOwner
	allowances pcports.SourceDataAmendmentAllowanceView
}

func NewDeclaredSourceDataAmendmentAllowance(
	owners AdoptedStageOwner,
	allowances pcports.SourceDataAmendmentAllowanceView,
) (*DeclaredSourceDataAmendmentAllowance, error) {
	if owners == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: adopted stage owner is nil")
	}
	if allowances == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: source data amendment allowance view is nil")
	}
	return &DeclaredSourceDataAmendmentAllowance{owners: owners, allowances: allowances}, nil
}

var _ psports.SourceDataRuleDeclaration = (*DeclaredSourceDataAmendmentAllowance)(nil)

// DeclareSourceDataAmendment 先把查询译成提供方的键，再回指采用版本、读声明、查那一格、译三值。
// 译不过去的查询在读任何东西之前就上抛：判不出阶段的查询按端口头注不该发出，收到零值是编排的错
// 不是业务答案，拒答而不是当最早阶段查——用一个判不出的阶段去问矩阵，与用它去放行是同一个错。
func (adapter *DeclaredSourceDataAmendmentAllowance) DeclareSourceDataAmendment(
	ctx context.Context,
	query psports.SourceDataAmendmentQuery,
) (psports.SourceDataAmendmentAllowance, error) {
	none := psports.SourceDataAmendmentNotDeclared
	stage, err := declaredStageOf(query.Stage)
	if err != nil {
		return none, err
	}
	intent, err := declaredIntentOf(query.Intent)
	if err != nil {
		return none, err
	}
	group, err := pcdomain.NewSourceDataGroupReference(query.Scope.DataGroup().String())
	if err != nil {
		return none, fmt.Errorf("%w: data group: %v", ErrUntranslatableAnswer, err)
	}

	owner, found, err := adapter.owners.AcceptanceRulePackageFor(ctx, query.Identity)
	if err != nil {
		return none, fmt.Errorf("adopted rule package: %w", err)
	}
	if !found {
		return none, nil
	}
	tenant, err := pcTenantOf(query.Identity)
	if err != nil {
		return none, err
	}
	content, found, err := adapter.allowances.LoadSourceDataAmendmentAllowance(ctx, tenant, owner)
	if err != nil {
		return none, fmt.Errorf("load source data amendment allowance: %w", err)
	}
	if !found {
		return none, nil
	}
	return allowanceOf(content.AllowanceFor(group, stage, intent))
}

// declaredStageOf 是 PS 六格到 PC 镜像六格的逐格翻译。全函数：零值`判不出`与集外值都上抛，没有任何
// 一格会被折成最早阶段。
func declaredStageOf(stage psdomain.AmendmentStage) (pcdomain.DeclaredAmendmentStage, error) {
	switch stage {
	case psdomain.StageAcceptedNotYetReceived:
		return pcdomain.DeclaredAcceptedNotYetReceived, nil
	case psdomain.StageReceivedOrMeasured:
		return pcdomain.DeclaredReceivedOrMeasured, nil
	case psdomain.StageLabelledOrBagged:
		return pcdomain.DeclaredLabelledOrBagged, nil
	case psdomain.StageCustomsDataFormingNotSubmitted:
		return pcdomain.DeclaredCustomsDataFormingNotSubmitted, nil
	case psdomain.StageCustomsSubmitted:
		return pcdomain.DeclaredCustomsSubmitted, nil
	case psdomain.StageCaseClosedOrServiceCompleted:
		return pcdomain.DeclaredCaseClosedOrServiceCompleted, nil
	default:
		return pcdomain.DeclaredAmendmentStageInvalid,
			fmt.Errorf("%w: amendment stage %d is not a determined stage", ErrUntranslatableAnswer, uint8(stage))
	}
}

// declaredIntentOf 是 PS 三格意图到 PC 镜像三格的逐格翻译。零值是「没声明意图」，编排在它之前就该
// 已经拒掉；到了这里仍是编程错误。
func declaredIntentOf(intent psdomain.AmendmentIntent) (pcdomain.DeclaredAmendmentIntent, error) {
	switch intent {
	case psdomain.SupplementIntent:
		return pcdomain.DeclaredSupplementIntent, nil
	case psdomain.CorrectionIntent:
		return pcdomain.DeclaredCorrectionIntent, nil
	case psdomain.ExplicitClearIntent:
		return pcdomain.DeclaredExplicitClearIntent, nil
	default:
		return pcdomain.DeclaredAmendmentIntentInvalid,
			fmt.Errorf("%w: amendment intent %d is not a declared intent", ErrUntranslatableAnswer, uint8(intent))
	}
}

// allowanceOf 是 PC 三值到 PS 三值的一对一翻译。集外上抛：两侧封闭集应当一致，读到集外是分叉，
// 静默吸收会把「不允许」读成「未声明」或反过来。
func allowanceOf(allowance pcdomain.AmendmentAllowance) (psports.SourceDataAmendmentAllowance, error) {
	switch allowance {
	case pcdomain.AmendmentAllowanceNotDeclared:
		return psports.SourceDataAmendmentNotDeclared, nil
	case pcdomain.AmendmentAllowed:
		return psports.SourceDataAmendmentAllowed, nil
	case pcdomain.AmendmentDisallowed:
		return psports.SourceDataAmendmentDisallowed, nil
	default:
		return psports.SourceDataAmendmentNotDeclared,
			fmt.Errorf("%w: amendment allowance %d is outside the provider's closed set", ErrUntranslatableAnswer, uint8(allowance))
	}
}
