package application

import (
	"context"
	"fmt"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// identityJudgment 是 UC-NO-002 步骤 4 的身份判断落下的那几格。收寄与重核共用这一份（ADR-0199 决定三「重核走与收寄步骤 4
// 同一份身份判断」）：两处各写一套，按包裹认、包裹锁、歧义信号、交付范围与服务结果核对态迟早有一处漏改，同一个号在两条路上就判出
// 两种结果。
type identityJudgment struct {
	// candidates 是 PS 交回的全部包裹候选，范围外的也在，照记在判断上，不论落成哪一格（ADR-0212 决定四）。
	candidates []domain.ParcelAssociationReference
	// verdict 是答复照 ADR-0212 决定四那张表判下的一格，冲突来由由它分（决定八）。
	verdict ports.ParcelIdentityVerdict
	// scopeCheck 是这次判断推出的交付范围核对态（ADR-0212 决定五），做成了身份核对的每一版都记。
	scopeCheck domain.DeliveryScopeCheck
	// parcel 只在 identified 时有意义：唯一命中、且那件包裹没有关联到别的作业实物。
	parcel     domain.ParcelAssociationReference
	identified bool
	// others 是号码唯一指向的那件包裹已关联到的别的作业实物的各行收寄（ADR-0196）；非空即一码两物。
	others []ports.ReceptionRecord
	// markers 与 check 只在 identified 时取（ADR-0186）。
	markers []string
	check   ports.ServiceResultCheck
}

// conflict 说这次判断落在身份冲突：号码标识多件（有范围时数范围相容的）包裹，或唯一那件带（范围内的）歧义信号（ADR-0195、ADR-0212
// 决定八），或唯一指向的那件包裹已关联到另一件作业实物（ADR-0196）。零候选、有范围而范围内一个候选也没有、没有范围而只经客户包裹
// 件号命中，都不是冲突，是待识别（ADR-0212 决定四、五）。
func (judgment identityJudgment) conflict() bool {
	return judgment.conflictCause() != ports.IdentityConflictCauseAbsent
}

// conflictCause 是这次判断落下身份冲突的来由（ADR-0211 决定一），不是冲突时缺席。一码两物只从唯一命中那一支来；其余两格取判断表的
// 格子——多候选压过歧义信号，因为信号消失之后它仍是多候选，交给自动解开就解错了。有范围时两格都只数范围相容的候选、只看范围内的
// 信号（ADR-0212 决定八），所以范围外另有候选、范围内只差信号的，记只因歧义信号。
func (judgment identityJudgment) conflictCause() ports.IdentityConflictCause {
	switch {
	case len(judgment.others) > 0:
		return ports.ConflictByOneMarkOnTwoUnits
	case judgment.verdict == ports.IdentityVerdictMultipleCandidates:
		return ports.ConflictByMultipleCandidates
	case judgment.verdict == ports.IdentityVerdictAmbiguityOnly:
		return ports.ConflictByAmbiguityOnly
	default:
		return ports.IdentityConflictCauseAbsent
	}
}

// identitySubject 是一次身份判断所判的那件实物与那一次送站：外部标识观察、作业实物、收寄节点、原收寄的业务时间与来源里的交付范围
// 依据引用。afterReception 只在重核时为真：此后归入的交接单只可能在收寄之后才有（ADR-0212 决定三、六），新的一次送站不去读它——
// 那件实物在别的送站里归入过的交接单不是这一次的证据。
type identitySubject struct {
	mark           ports.ExternalMarkObservation
	unit           domain.HandlingUnitID
	node           domain.NodeReference
	receivedAt     time.Time
	basis          domain.DeliveryScopeBasisReference
	afterReception bool
}

// identityJudge 持身份判断要问的各个口。excluded 只在被重核的是身份冲突处置确认过「不是」的那一版时有值（ADR-0210 决定九）：PS 唯一
// 命中其中一件时照零候选办，不据此关联——关联回去，那件包裹若此刻关联在别的实物上，一码两物还会给那件追加冲突版，推翻对它的处置。
// 收寄不带它：新的一次送站是新的判断，没有处置过的排除可依。
type identityJudge struct {
	identity   ports.ParcelIdentityView
	services   ports.ParcelServiceResultView
	receptions ports.ReceptionStore
	scope      ports.DeliveryScopeEvidence
	excluded   []domain.ParcelAssociationReference
}

// judge 先按依据推导这件实物的可验证交付范围，拿范围给出的客户账户与外部标识观察问 PS，再按答复值上的判断格判身份。须在落册那一笔
// 事务里调：唯一命中那一支按（租户、包裹）取锁（见 ports.ReceptionStore.FindOtherUnitIntakes），从读别的实物关联到那一笔落册之间，
// 同一件包裹上的另一次判断等着。
//
// 证据册读不动是本方册读不动：不形成答案，也不带 ErrParcelIdentityUnreadable——没推出范围就去问 PS，范围内本可唯一的一件会落成
// 范围未证。身份核对读不动不拿空候选顶替（ADR-0089）：那会把可能已知的包裹落成查无此号。错误带 ErrParcelIdentityUnreadable：收寄
// 据此落身份核对态未核对的待识别（ADR-0199 决定五），重核的消费门据此把这一格记成等依赖，而不是发布失败。
func (judge identityJudge) judge(
	ctx context.Context,
	tenant domain.TenantID,
	subject identitySubject,
) (identityJudgment, error) {
	scope, err := judge.deriveScope(ctx, tenant, subject)
	if err != nil {
		return identityJudgment{}, fmt.Errorf("derive the verifiable delivery scope: %w", err)
	}
	answer, err := judge.identity.ResolveParcelIdentity(ctx, tenant, subject.mark, scope.Accounts())
	if err != nil {
		return identityJudgment{}, fmt.Errorf("resolve parcel identity: %w: %w", ErrParcelIdentityUnreadable, err)
	}
	verdict, parcel := answer.Verdict()
	judgment := identityJudgment{candidates: answer.Parcels(), verdict: verdict, scopeCheck: scope.Check()}
	// 号码唯一指向一件包裹，还要问这件包裹是否已关联到另一件作业实物：是，就是「同一外部条码出现在多个实物」（NO
	// CONTEXT「实物身份与事实完整性」、ADR-0196）。按关联到的包裹认而不按号码认，不论先到那件此刻是否仍在节点控制下；
	// 同一作业实物再来不在此列。这一问读不动就不形成答案，也不带 ErrParcelIdentityUnreadable：那是本方册读不动，不是没问到 PS，
	// 不能落成未核对。不是唯一命中的各格（冲突、待识别、范围未证）不进这一问，一码两物只在唯一命中之后才谈得上（ADR-0212 决定五
	// 「不进 ADR-0196 那一支」）。
	if verdict != ports.IdentityVerdictUnique {
		return judgment, nil
	}
	if slices.Contains(judge.excluded, parcel) {
		return identityJudgment{verdict: ports.IdentityVerdictUnknown, scopeCheck: scope.Check()}, nil
	}
	judgment.others, err = judge.receptions.FindOtherUnitIntakes(ctx, tenant, parcel, subject.unit)
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

// deriveScope 按依据读证据状态，交领域推导（domain.DeriveVerifiableDeliveryScope，收寄与重核同一份）。依据取最后归入的交接单，没有
// 归入过的取来源里的引用（ADR-0212 决定三）；证据状态是此刻的，预报的有效时窗按原收寄的业务时间取——重核晚来，时窗不跟着重核时刻
// 走（决定六）。时窗只在预报在册时才读：预报不在册时答依据不在册，与时窗无关。
func (judge identityJudge) deriveScope(
	ctx context.Context,
	tenant domain.TenantID,
	subject identitySubject,
) (domain.VerifiableDeliveryScope, error) {
	basis := domain.UnitDeliveryScopeBasis{Source: subject.basis}
	if subject.afterReception {
		note, assigned, err := judge.scope.FindLastAssignedHandoverNote(ctx, tenant, subject.unit)
		if err != nil {
			return domain.VerifiableDeliveryScope{}, fmt.Errorf("find the last assigned handover note: %w", err)
		}
		if assigned {
			basis.LastAssignedNote = note
		}
	}
	input := domain.DeliveryScopeDerivation{
		TenantID:   tenant,
		Basis:      basis,
		Node:       subject.node,
		ReceivedAt: subject.receivedAt,
		Mark:       subject.mark.Mark,
	}
	effective := basis.Effective()
	switch effective.Kind() {
	case domain.BasisDeliveryPreAdvice:
		id, err := domain.NewDeliveryPreAdviceID(effective.Identifier())
		if err != nil {
			break
		}
		state, found, err := judge.scope.FindDeliveryPreAdvice(ctx, tenant, id)
		if err != nil {
			return domain.VerifiableDeliveryScope{}, fmt.Errorf("find the delivery pre-advice: %w", err)
		}
		if !found {
			break
		}
		input.PreAdvice = state
		window, effectiveThen, err := judge.scope.FindPreAdviceWindowAt(ctx, tenant, subject.receivedAt)
		if err != nil {
			return domain.VerifiableDeliveryScope{}, fmt.Errorf("find the pre-advice validity window: %w", err)
		}
		if effectiveThen {
			input.Window = window
		}
	case domain.BasisDeliveryHandoverNote:
		id, err := domain.NewDeliveryHandoverNoteID(effective.Identifier())
		if err != nil {
			break
		}
		state, found, err := judge.scope.FindDeliveryHandoverNote(ctx, tenant, id)
		if err != nil {
			return domain.VerifiableDeliveryScope{}, fmt.Errorf("find the delivery handover note: %w", err)
		}
		if found {
			input.HandoverNote = state
		}
	}
	return domain.DeriveVerifiableDeliveryScope(input), nil
}
