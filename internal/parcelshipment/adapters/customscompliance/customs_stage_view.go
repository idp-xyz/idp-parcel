// Package customscompliance 是 parcel-shipment 消费 customs-compliance 事实的适配器（ADR-0025
// 消费方侧；缝的登记见 ADR-0118）。它只翻译不判断：关务出「形成了没有、提交了没有、关闭了
// 没有」，阶段由 PS 的资料修订编排判。
package customscompliance

import (
	"context"
	"errors"
	"fmt"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：消费方话语译不成提供方的
// 键，是编程错误不是业务答案，不折成`不知道`。
var ErrUntranslatableAnswer = errors.New("parcel shipment customscompliance adapter: untranslatable answer")

// ParcelDeclarationFactsLookup 是本适配器向 customs-compliance 取包裹申报事实的窄口。真实装配
// 交给 customscompliance/adapters/postgres.ParcelDeclarationFactsView；这里只声明读的那一口，
// 理由同 nodeoperations 包的 ExecutionFactLookup——测试替身不必背上提供方的整个端口面。
type ParcelDeclarationFactsLookup interface {
	LoadParcelDeclarationFacts(
		ctx context.Context,
		tenant ccdomain.TenantID,
		parcel ccdomain.DeclaredParcelReference,
	) (ccports.ParcelDeclarationFacts, error)
}

// CustomsStageView 读关务按包裹键交出的三件事实，译成 ports.CustomsStageFacts 的三格三态
// （票 ps-port-remainder/05 接线的那一只；它替下的 UnconnectedCustomsStageView 已随读面立起
// 退场）。
//
// 翻译是逐格的布尔到三态：关务说「有」译`在`、说「没有」译`不在`。`不知道`在这里没有出口
// ——关务三本册子都是它自己的，读回来了就是知道；读面调不通是错误，上抛让编排落在
// `SourceDataAmendmentStageFactUnavailable`（等依赖恢复），不折成任何一格。哪一格压过哪一格、
// 「形成中」与「已提交」同时在时算哪个阶段，一律留给 domain.JudgeAmendmentStage。
type CustomsStageView struct {
	facts ParcelDeclarationFactsLookup
}

func NewCustomsStageView(facts ParcelDeclarationFactsLookup) (CustomsStageView, error) {
	if facts == nil {
		return CustomsStageView{}, fmt.Errorf("parcel shipment customscompliance adapter: parcel declaration facts lookup is nil")
	}
	return CustomsStageView{facts: facts}, nil
}

var _ psports.CustomsStageView = CustomsStageView{}

// LoadCustomsStageFacts 把（租户 + 正式包裹）译成关务的键、读三件事实、逐格译三态。租户与包裹
// 引用在两个上下文里是同一串字面——与 adopt_on_node_intake 把关联引用直接当包裹标识用同一条
// 约定；译不过去上抛，不猜。
func (view CustomsStageView) LoadCustomsStageFacts(
	ctx context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (psports.CustomsStageFacts, error) {
	ccTenant, err := ccdomain.NewTenantID(tenant.String())
	if err != nil {
		return psports.CustomsStageFacts{}, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	ccParcel, err := ccdomain.NewDeclaredParcelReference(parcel.String())
	if err != nil {
		return psports.CustomsStageFacts{}, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	facts, err := view.facts.LoadParcelDeclarationFacts(ctx, ccTenant, ccParcel)
	if err != nil {
		return psports.CustomsStageFacts{}, fmt.Errorf("load customs stage facts: %w", err)
	}
	return psports.CustomsStageFacts{
		DataForming: stageFactOf(facts.MemberOfUnsubmittedUnit),
		Submitted:   stageFactOf(facts.InFixedSubmissionVersion),
		CaseClosed:  stageFactOf(facts.InClosedCase),
	}, nil
}

// stageFactOf 是布尔到三态的那一格翻译：两个取值各有落点，没有第三个值可落进`不知道`。
func stageFactOf(present bool) psdomain.StageFact {
	if present {
		return psdomain.StageFactPresent
	}
	return psdomain.StageFactAbsent
}
