package application

import (
	"context"
	"fmt"
	"slices"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// identityJudgment 是 UC-NO-002 步骤 4 的身份判断落下的那几格。收寄与重核共用这一份（ADR-0199 决定三「重核走与收寄步骤 4
// 同一份身份判断」）：两处各写一套，按包裹认、包裹锁、歧义信号与服务结果核对态迟早有一处漏改，同一个号在两条路上就判出
// 两种结果。
type identityJudgment struct {
	// candidates 是 PS 交回的包裹候选，照记在判断上，不论落成哪一格。
	candidates []domain.ParcelAssociationReference
	// parcel 只在 identified 时有意义：唯一命中、且那件包裹没有关联到别的作业实物。
	parcel     domain.ParcelAssociationReference
	identified bool
	// others 是号码唯一指向的那件包裹已关联到的别的作业实物的各行收寄（ADR-0196）；非空即一码两物。
	others []ports.ReceptionRecord
	// markers 与 check 只在 identified 时取（ADR-0186）。
	markers []string
	check   ports.ServiceResultCheck
}

// conflict 说这次判断落在身份冲突：号码标识多件包裹，或唯一那件带歧义信号（ADR-0195），或唯一指向的那件包裹已关联到另一件
// 作业实物（ADR-0196）。零候选——带不带歧义信号——不是冲突，是待识别。
func (judgment identityJudgment) conflict() bool {
	return !judgment.identified && len(judgment.candidates) > 0
}

// identityJudge 持身份判断要问的三个口。excluded 只在被重核的是身份冲突处置确认过「不是」的那一版时有值（ADR-0210 决定九）：PS 唯一
// 命中其中一件时照零候选办，不据此关联——关联回去，那件包裹若此刻关联在别的实物上，一码两物还会给那件追加冲突版，推翻对它的处置。
// 收寄不带它：新的一次送站是新的判断，没有处置过的排除可依。
type identityJudge struct {
	identity   ports.ParcelIdentityView
	services   ports.ParcelServiceResultView
	receptions ports.ReceptionStore
	excluded   []domain.ParcelAssociationReference
}

// judge 拿一次外部标识观察问 PS，判这件作业实物的身份。须在落册那一笔事务里调：唯一命中那一支按（租户、包裹）取锁（见
// ports.ReceptionStore.FindOtherUnitIntakes），从读别的实物关联到那一笔落册之间，同一件包裹上的另一次判断等着。
//
// 身份核对读不动不拿空候选顶替（ADR-0089）：那会把可能已知的包裹落成查无此号。错误带 ErrParcelIdentityUnreadable：收寄据此
// 落身份核对态未核对的待识别（ADR-0199 决定五），重核的消费门据此把这一格记成等依赖，而不是发布失败。
func (judge identityJudge) judge(
	ctx context.Context,
	tenant domain.TenantID,
	mark ports.ExternalMarkObservation,
	unit domain.HandlingUnitID,
) (identityJudgment, error) {
	answer, err := judge.identity.ResolveParcelIdentity(ctx, tenant, mark)
	if err != nil {
		return identityJudgment{}, fmt.Errorf("resolve parcel identity: %w: %w", ErrParcelIdentityUnreadable, err)
	}
	judgment := identityJudgment{candidates: answer.Candidates}
	// 号码唯一指向一件包裹，还要问这件包裹是否已关联到另一件作业实物：是，就是「同一外部条码出现在多个实物」（NO
	// CONTEXT「实物身份与事实完整性」、ADR-0196）。按关联到的包裹认而不按号码认，不论先到那件此刻是否仍在节点控制下；
	// 同一作业实物再来不在此列。这一问读不动就不形成答案，也不带 ErrParcelIdentityUnreadable：那是本方册读不动，不是没问到 PS，
	// 不能落成未核对。恰一个候选却带歧义信号的不是唯一命中（ADR-0195），
	// 与多候选同走身份冲突，不进这一问。
	parcel, unique := answer.Unique()
	if !unique {
		return judgment, nil
	}
	if slices.Contains(judge.excluded, parcel) {
		return identityJudgment{}, nil
	}
	judgment.others, err = judge.receptions.FindOtherUnitIntakes(ctx, tenant, parcel, unit)
	if err != nil {
		return identityJudgment{}, fmt.Errorf("find other units associated with the parcel: %w", err)
	}
	if len(judgment.others) > 0 {
		return judgment, nil
	}
	judgment.parcel, judgment.identified = parcel, true

	// 身份唯一成立才有可问的包裹：向 PS 取它此刻的服务结果，落成标记（UC-NO-002「对已取消、终局、无路由或受限实物
	// 标记相应引用」）。读不动不挡已经发生的物理事实——照常落册，核对态记读不动、标记留空，不补查（ADR-0186）；空标记
	// 因此只在已查时才说「没有取消与终局」。
	results, err := judge.services.FindParcelServiceResults(ctx, tenant, parcel)
	if err != nil {
		judgment.check = ports.ServiceResultsUnreadable
		return judgment, nil
	}
	judgment.check = ports.ServiceResultsChecked
	judgment.markers = results.Markers()
	return judgment, nil
}
