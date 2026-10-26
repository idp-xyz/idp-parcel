package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 参与方关系的三个结束口（票 ps-external-mark-relations/17）：撤销、到期、替代。形照身份停用（RegisterPartyIdentityHandler.
// Deactivate）：取最新修订 → 领域转换 → 存下一笔修订；三口各走领域既有的结束方法，本文件不另立「能不能结束」的判断。
//
// 比停用多一道门：结束时刻不得晚于登记时刻。关系的终止是状态事实——终止修订一落册，AppliesAt 就按状态答否，
// 授权交付判断随之即时答否（票 10 只读最新修订）。写一个未来的结束时刻，册面就会说「到那时才结束」而判断此刻已经不认它，
// 两处说法对不上。要按期结束，登一版带区间终点的修订（RegisterRelationship），到期之后再登记到期。
//
// 答案代数同本能力面的停用口：落下终止修订答`已结束`；同修订同内容答`重复`；同修订异内容答`内容冲突`；关系不在册答
// `未找到`；其余被拒答`未受理`并带原因。

// RevokePartyRelationshipCommand 撤销一段关系：即时结束，带撤销依据。Revision 是终止修订的落点（= 册上最新修订 + 1），
// 判据同 DeactivatePartyIdentityCommand：操作者声明自己看到的册面，错位说明册面已被并发推进或意图已陈旧，拒绝而不是
// 替操作者猜。
type RevokePartyRelationshipCommand struct {
	Tenant   domain.TenantID
	ID       domain.RelationshipID
	Revision int
	Basis    domain.RelationshipBasisReference
	At       time.Time
}

// ExpirePartyRelationshipCommand 登记一段关系已到期，带到期依据。At 是到期时刻：不早于关系的区间终点（领域门），
// 也不晚于登记时刻（本用例的门）。开区间的关系没有终点可到期，只能撤销或替代。
type ExpirePartyRelationshipCommand struct {
	Tenant   domain.TenantID
	ID       domain.RelationshipID
	Revision int
	Basis    domain.RelationshipBasisReference
	At       time.Time
}

// SupersedePartyRelationshipCommand 以接替的持有方结束一段关系，并在同一笔里登记接替关系。
//
// 接替关系的正文不由调用方重述：持有方是 Successor，相对方、角色与范围照原关系，区间接住原关系余下的那一段
// （domain.PartyRelationship.SuccessorSpec）。调用方只给它的标识、依据与批准——让调用方重述正文，就等于给了它一次不经
// 撤销改写关系内容的机会。接替关系必须带批准：原关系在替代时刻就不再支持新决定，接替关系若只是候选，中间就空出一段
// 谁都不持有的时间。
type SupersedePartyRelationshipCommand struct {
	Tenant                domain.TenantID
	ID                    domain.RelationshipID
	Revision              int
	Successor             domain.PartyID
	At                    time.Time
	SuccessorRelationship domain.RelationshipID
	Basis                 domain.RelationshipBasisReference
	Approval              RelationshipApproval
}

// PartyRelationshipEndingHandler 是参与方关系三个结束口的写侧编排。调用方逐命令各起事务（批不是聚合，AT-PC-011）；
// 替代的两笔（接替关系的首笔修订与原关系的终止修订）必须在同一事务里，本用例在原关系那一笔落不下时交回 error，让调用方
// 整笔回滚，不留「接替关系立了、原关系还生效」的半份。
type PartyRelationshipEndingHandler struct {
	registry ports.PartyIdentityRegistry
	clock    ports.Clock
	// registrations 登记接替关系：接替关系过的是关系登记的全部门（双方在册且届时已生效、授权交付关系的账户门、修订槽），
	// 不在这里抄第二份。
	registrations *RegisterPartyIdentityHandler
}

// NewPartyRelationshipEndingHandler 的 clock 给「结束时刻不得晚于登记时刻」那道门。关系登记不判注册号，注册号类型目录不接。
func NewPartyRelationshipEndingHandler(
	registry ports.PartyIdentityRegistry,
	clock ports.Clock,
) *PartyRelationshipEndingHandler {
	return &PartyRelationshipEndingHandler{
		registry:      registry,
		clock:         clock,
		registrations: NewRegisterPartyIdentityHandler(registry, nil),
	}
}

// Revoke 撤销一段已生效的关系（domain.PartyRelationship.Revoke）。
func (handler *PartyRelationshipEndingHandler) Revoke(
	ctx context.Context,
	command RevokePartyRelationshipCommand,
) (PartyRegistryResult, error) {
	const operation = "revoke party relationship"
	latest, verdict, err := handler.loadForEnding(ctx, operation, endingTarget{
		tenant:   command.Tenant,
		id:       command.ID,
		revision: command.Revision,
		at:       command.At,
		status:   domain.RelationshipRevoked,
		sameFacts: func(relationship domain.PartyRelationship) bool {
			basis, has := relationship.EndBasis()
			return has && basis == command.Basis
		},
	})
	if err != nil || verdict.done {
		return verdict.result, err
	}
	if verdict.replayed {
		return PartyRegistryResult{outcome: PartyIdentityAlreadyRegistered}, nil
	}
	return handler.endFresh(ctx, operation, latest, command.Revision, command.At,
		func(relationship domain.PartyRelationship) (domain.PartyRelationship, error) {
			return relationship.Revoke(command.Basis, command.At)
		})
}

// Expire 登记一段关系已到期（domain.PartyRelationship.Expire）。
func (handler *PartyRelationshipEndingHandler) Expire(
	ctx context.Context,
	command ExpirePartyRelationshipCommand,
) (PartyRegistryResult, error) {
	const operation = "expire party relationship"
	latest, verdict, err := handler.loadForEnding(ctx, operation, endingTarget{
		tenant:   command.Tenant,
		id:       command.ID,
		revision: command.Revision,
		at:       command.At,
		status:   domain.RelationshipExpired,
		sameFacts: func(relationship domain.PartyRelationship) bool {
			basis, has := relationship.EndBasis()
			return has && basis == command.Basis
		},
	})
	if err != nil || verdict.done {
		return verdict.result, err
	}
	if verdict.replayed {
		return PartyRegistryResult{outcome: PartyIdentityAlreadyRegistered}, nil
	}
	return handler.endFresh(ctx, operation, latest, command.Revision, command.At,
		func(relationship domain.PartyRelationship) (domain.PartyRelationship, error) {
			return relationship.Expire(command.Basis, command.At)
		})
}

// Supersede 以接替的持有方结束一段关系（domain.PartyRelationship.SupersededBy），并在同一笔里登记接替关系。
//
// 先登接替关系、后落原关系的终止修订：接替关系被拒（双方不在册、账户门、同标识异内容）时一个字节没写，原关系照旧生效；
// 原关系那一笔落不下时接替关系已经写进了事务，交回 error 让调用方整笔回滚。
func (handler *PartyRelationshipEndingHandler) Supersede(
	ctx context.Context,
	command SupersedePartyRelationshipCommand,
) (PartyRegistryResult, error) {
	const operation = "supersede party relationship"
	if command.SuccessorRelationship == command.ID {
		return notAccepted(errors.New("接替关系要另立标识：同一标识上的下一修订是这段关系自己的终止，不是另一段关系")), nil
	}
	latest, verdict, err := handler.loadForEnding(ctx, operation, endingTarget{
		tenant:   command.Tenant,
		id:       command.ID,
		revision: command.Revision,
		at:       command.At,
		status:   domain.RelationshipSuperseded,
		sameFacts: func(relationship domain.PartyRelationship) bool {
			successor, has := relationship.Successor()
			return has && successor == command.Successor
		},
	})
	if err != nil || verdict.done {
		return verdict.result, err
	}
	successorCommandOf := func() (RegisterPartyRelationshipCommand, error) {
		spec, err := latest.Relationship().SuccessorSpec(command.Successor, command.Basis, command.At)
		if err != nil {
			return RegisterPartyRelationshipCommand{}, fmt.Errorf("接替关系不成立：%w", err)
		}
		return RegisterPartyRelationshipCommand{
			Tenant:   command.Tenant,
			ID:       command.SuccessorRelationship,
			Revision: 1,
			Spec:     spec,
			Approval: &command.Approval,
		}, nil
	}

	if verdict.replayed {
		successorCommand, refusal := successorCommandOf()
		if refusal != nil {
			return notAccepted(refusal), nil
		}
		return handler.replaySuccessor(ctx, operation, successorCommand)
	}

	// 原关系的结束门（时刻、转换、修订槽）先于接替关系判：原关系结束不了，接替关系一个字节都不该写。
	next, refusal := handler.checkEnding(latest, command.Revision, command.At,
		func(relationship domain.PartyRelationship) (domain.PartyRelationship, error) {
			return relationship.SupersededBy(command.Successor, command.At)
		})
	if refusal != nil {
		return notAccepted(refusal), nil
	}
	successorCommand, refusal := successorCommandOf()
	if refusal != nil {
		return notAccepted(refusal), nil
	}

	successorResult, err := handler.registrations.RegisterRelationship(ctx, successorCommand)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	switch successorResult.Outcome() {
	case PartyIdentityRegistered, PartyIdentityAlreadyRegistered:
	case PartyIdentityNotAccepted:
		return notAccepted(fmt.Errorf("接替关系 %s：%w", command.SuccessorRelationship, successorResult.Cause())), nil
	default:
		// 同标识异内容：册上已有一段同名的关系，它不是这一次要接替的那段。
		return successorResult, nil
	}

	outcome, err := handler.registry.SaveRelationship(ctx, next)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if outcome != ports.PartyRegistrySaved && successorResult.Outcome() == PartyIdentityRegistered {
		return PartyRegistryResult{}, fmt.Errorf(
			"%s: 接替关系 %s 已写入，而原关系 %s 的终止修订 r%d 落点已被并发占住（%s）；整笔回滚，不留半份",
			operation, command.SuccessorRelationship, command.ID, next.Revision(), outcome,
		)
	}
	return endingResult(outcome), nil
}

// endingTarget 是三口共用的那一半：哪一段关系、落在哪个修订、什么时刻、要落成哪种终止，以及册上已结束的最新修订算不算
// 这一次结束本身（sameFacts 比各口自己那一格：撤销与到期比依据，替代比接替方；状态与时刻由 loadForEnding 比）。
type endingTarget struct {
	tenant    domain.TenantID
	id        domain.RelationshipID
	revision  int
	at        time.Time
	status    domain.RelationshipStatus
	sameFacts func(domain.PartyRelationship) bool
}

// endingVerdict 是装载那一步的分类：done 即 result 就是答复；replayed 即册上最新修订就是这一次结束本身；两者都假即一段
// 尚未结束的关系，交给调用方走转换。
type endingVerdict struct {
	result   PartyRegistryResult
	done     bool
	replayed bool
}

// loadForEnding 取要结束的那段关系的最新修订，并把已结束的册面分成重放、冲突与拒三格，判据同 deactivationReplay：
// 同修订同内容是重放；同修订异内容（另一种终止、另一个时刻、另一份依据或另一个接替方）是冲突；落点不是那一笔就拒——
// 一段关系只结束一次，第二次结束会把「何时起不再支持新决定」改掉，而已经形成的决定正靠那一刻定依据。
func (handler *PartyRelationshipEndingHandler) loadForEnding(
	ctx context.Context,
	operation string,
	target endingTarget,
) (domain.PartyRelationshipRegistration, endingVerdict, error) {
	latest, found, err := handler.registry.LoadLatestRelationship(ctx, target.tenant, target.id)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, endingVerdict{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return domain.PartyRelationshipRegistration{}, endingVerdict{
			result: PartyRegistryResult{outcome: PartyIdentityNotFound},
			done:   true,
		}, nil
	}
	relationship := latest.Relationship()
	endedAt, ended := relationship.EndedAt()
	if !ended {
		return latest, endingVerdict{}, nil
	}
	if target.revision != latest.Revision() {
		return latest, endingVerdict{
			result: notAccepted(fmt.Errorf(
				"关系 %s 已于 r%d 结束（%s），一段关系只结束一次", target.id, latest.Revision(), relationship.Status(),
			)),
			done: true,
		}, nil
	}
	if relationship.Status() == target.status && endedAt.Equal(target.at.UTC()) && target.sameFacts(relationship) {
		return latest, endingVerdict{replayed: true}, nil
	}
	return latest, endingVerdict{
		result: PartyRegistryResult{outcome: PartyIdentityContentConflict},
		done:   true,
	}, nil
}

// checkEnding 是三口共用的转换段：时刻门 → 领域转换 → 修订槽，交回待落的终止修订。非空的 refusal 是给登记方看的拒因，
// 不是技术失败。
func (handler *PartyRelationshipEndingHandler) checkEnding(
	latest domain.PartyRelationshipRegistration,
	revision int,
	at time.Time,
	transit func(domain.PartyRelationship) (domain.PartyRelationship, error),
) (domain.PartyRelationshipRegistration, error) {
	if now := handler.clock.Now(); at.After(now) {
		return domain.PartyRelationshipRegistration{}, fmt.Errorf(
			"结束时刻 %s 晚于登记时刻 %s：终止修订一落册，关系即不再支持新的商业决定，未来的结束时刻会让册面与判断对不上；"+
				"要按期结束，登一版带区间终点的修订，到期之后再登记到期",
			at.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339),
		)
	}
	relationship := latest.Relationship()
	ended, err := transit(relationship)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, endingRefusal(err, relationship)
	}
	next := latest.Revision() + 1
	if revision != next {
		return domain.PartyRelationshipRegistration{}, fmt.Errorf(
			"结束落点修订错位：命令声明 %d，册面推进到 %d——册面已被并发推进或意图已陈旧", revision, next,
		)
	}
	return domain.NewPartyRelationshipRegistration(latest.Tenant(), latest.ID(), next, ended)
}

// endFresh 落一段尚未结束的关系的终止修订（撤销与到期；替代另有接替关系一笔，见 Supersede）。
func (handler *PartyRelationshipEndingHandler) endFresh(
	ctx context.Context,
	operation string,
	latest domain.PartyRelationshipRegistration,
	revision int,
	at time.Time,
	transit func(domain.PartyRelationship) (domain.PartyRelationship, error),
) (PartyRegistryResult, error) {
	next, refusal := handler.checkEnding(latest, revision, at, transit)
	if refusal != nil {
		return notAccepted(refusal), nil
	}
	outcome, err := handler.registry.SaveRelationship(ctx, next)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	return endingResult(outcome), nil
}

// replaySuccessor 认一次替代的重放：原关系的终止修订已对上，接替关系也得是当初登的那一段。接替关系查无即冲突——当初接替
// 的是另一个标识，这一次照登就会多出第二段接替关系；查得到就照原样再登一遍，修订 1 已在册，登记册只会答重放或冲突。
func (handler *PartyRelationshipEndingHandler) replaySuccessor(
	ctx context.Context,
	operation string,
	successorCommand RegisterPartyRelationshipCommand,
) (PartyRegistryResult, error) {
	_, found, err := handler.registry.LoadLatestRelationship(ctx, successorCommand.Tenant, successorCommand.ID)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return PartyRegistryResult{outcome: PartyIdentityContentConflict}, nil
	}
	result, err := handler.registrations.RegisterRelationship(ctx, successorCommand)
	if err != nil {
		return PartyRegistryResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	switch result.Outcome() {
	case PartyIdentityRegistered:
		return PartyRegistryResult{}, fmt.Errorf("%s: 重放替代时接替关系 %s 的修订 1 竟不在册，整笔回滚", operation, successorCommand.ID)
	case PartyIdentityNotAccepted:
		return notAccepted(fmt.Errorf("接替关系 %s：%w", successorCommand.ID, result.Cause())), nil
	default:
		return result, nil
	}
}

// endingRefusal 把领域转换的拒绝连同册面一起交回：领域那一格只说「转换不成立」，登记方要看得到是哪一格不成立——关系还在
// 候选、已经结束、区间没有终点或终点未到、接替方就是原持有方或相对方。判据只在领域一处，这里只把册面摆出来，不复述规则。
func endingRefusal(err error, relationship domain.PartyRelationship) error {
	interval := relationship.Effective()
	end := "开区间（无终点）"
	if endsAt, bounded := interval.EndsAt(); bounded {
		end = endsAt.UTC().Format(time.RFC3339)
	}
	return fmt.Errorf("%w：册上最新修订状态为 %s，持有方 %s，相对方 %s，有效区间自 %s 至 %s",
		err, relationship.Status(), relationship.Holder(), relationship.Counterparty(),
		interval.StartsAt().UTC().Format(time.RFC3339), end)
}

func endingResult(outcome ports.PartyRegistrySaveOutcome) PartyRegistryResult {
	switch outcome {
	case ports.PartyRegistrySaved:
		return PartyRegistryResult{outcome: PartyRelationshipEnded}
	case ports.PartyRegistryAlreadyRegistered:
		return PartyRegistryResult{outcome: PartyIdentityAlreadyRegistered}
	case ports.PartyRegistryContentConflict:
		return PartyRegistryResult{outcome: PartyIdentityContentConflict}
	default:
		return PartyRegistryResult{}
	}
}
