package domain

import "errors"

// ContinuedAttemptRegister 是某一件包裹的`面单继续尝试决定`登记册，键为租户加包裹。
//
// **按包裹而不按面单交易。** CONTEXT 把决定定义为「针对明确包裹**当前完整面单服务范围**形成」
// ——一个包裹可以关联多笔具有重试、替代、作废或换单关系的交易，挂在交易上会让同一个包裹在不同
// 交易下各有一套关闭状态，而那正是「受控关闭以包裹为范围」要排除的。
//
// 决定只增不改，判断不存。`包裹级继续尝试判断`由 Judge 现算——CONTEXT 说它「只由有效的关闭、
// 重开决定及当前有效终局结果派生」，存一列就有了第二个来源，而终局有效性后来变化时那一列不会
// 跟着变（CONTEXT 明写这种情形要重新派生）。
type ContinuedAttemptRegister struct {
	// revision 同 LabelTransaction：读出时所在的持久化版本，未持久化为零。转移以值接收者
	// 复制整份，版本天然带下去。
	revision  int64
	tenant    TenantID
	parcel    DeclaredParcelID
	decisions []ContinuedAttemptDecision
}

// OpenContinuedAttemptRegister 开一册空的。
//
// 空册是**有意义的状态**而不是「还没建」：CONTEXT 说无生效关闭且无当前有效终局时派生为`开放`，
// 空册正是那一格的常见来源。读面要分辨「没有人作过决定」与「最近适用决定为重开」，靠的是
// HasAnyDecision，不是给判断加一格。
func OpenContinuedAttemptRegister(tenant TenantID, parcel DeclaredParcelID) (ContinuedAttemptRegister, error) {
	if !tenant.valid() || !parcel.valid() {
		return ContinuedAttemptRegister{}, ErrInvalidContinuedAttemptDecision
	}
	return ContinuedAttemptRegister{tenant: tenant, parcel: parcel}, nil
}

func (register ContinuedAttemptRegister) Revision() int64 {
	return register.revision
}

func (register ContinuedAttemptRegister) Tenant() TenantID {
	return register.tenant
}

func (register ContinuedAttemptRegister) Parcel() DeclaredParcelID {
	return register.parcel
}

// Decisions 交回副本，顺序即追加顺序。这个顺序是「最近适用决定是哪一条」的全部依据，因此
// CONTEXT 要求「同一业务时点的冲突必须保存稳定、可审计的领域顺序」。
func (register ContinuedAttemptRegister) Decisions() []ContinuedAttemptDecision {
	decisions := make([]ContinuedAttemptDecision, len(register.decisions))
	copy(decisions, register.decisions)
	return decisions
}

// HasAnyDecision 说这一册有没有人作过决定。
//
// 它服务于读面上一句必须说得出的话：`开放`可能来自「没有人作过决定」，也可能来自「关过又重开
// 了」，两者派生出同一格而现场处置完全不同。判断本身不加格（那是新造领域语言），区别由这里交代。
func (register ContinuedAttemptRegister) HasAnyDecision() bool {
	return len(register.decisions) != 0
}

// Append 追加一条决定。
//
// currentFinalPresent 由调用方从包裹当前有效终局取得——终局属本上下文但不属本册，塞进本册就是
// 把一份派生自别处的事实复制成第二个来源。它只影响重开这一格：CONTEXT「只有当前不存在有效终局
// 服务结果时，才能依据适用授权追加重开决定」。
//
// **本方法不判断授权够不够格。** 授权规则属 party-commercial，本上下文只记所采用的那一份
// （同主动拒绝、取消那几处的分工）。它也不会自动形成任何决定——CONTEXT 明写「系统不得自动形成」，
// 而这里没有任何不带 spec 的入口。
func (register ContinuedAttemptRegister) Append(
	spec ContinuedAttemptDecisionSpec,
	currentFinalPresent bool,
) (ContinuedAttemptRegister, error) {
	decision, err := register.decisionFrom(spec, currentFinalPresent)
	if err != nil {
		return ContinuedAttemptRegister{}, err
	}
	// 显式复制再追加，理由同本包其余追加式清单：append 到内部切片上，两份聚合值会共享同一
	// 底层数组。
	decisions := make([]ContinuedAttemptDecision, 0, len(register.decisions)+1)
	decisions = append(decisions, register.decisions...)
	register.decisions = append(decisions, decision)
	return register, nil
}

func (register ContinuedAttemptRegister) decisionFrom(
	spec ContinuedAttemptDecisionSpec,
	currentFinalPresent bool,
) (ContinuedAttemptDecision, error) {
	if !spec.ID.valid() ||
		!spec.Kind.valid() ||
		!spec.Decider.valid() ||
		!spec.AuthorityRole.valid() ||
		!spec.AuthoritySnapshot.valid() ||
		!spec.Reason.valid() ||
		spec.EffectiveAt.IsZero() {
		return ContinuedAttemptDecision{}, ErrInvalidContinuedAttemptDecision
	}
	if register.holds(spec.ID) {
		return ContinuedAttemptDecision{}, ErrInvalidContinuedAttemptDecision
	}

	decision := ContinuedAttemptDecision{
		id:                spec.ID,
		kind:              spec.Kind,
		requester:         spec.Requester,
		decider:           spec.Decider,
		authorityRole:     spec.AuthorityRole,
		authoritySnapshot: spec.AuthoritySnapshot,
		reason:            spec.Reason,
		effectiveAt:       spec.EffectiveAt.UTC(),
	}

	switch spec.Kind {
	case ControlledClosureDecision:
		// 截断边界与关闭责任来源是关闭独有的必备项；关闭不指向此前关闭——那是重开才有的关系。
		if !spec.CutoffBoundary.valid() ||
			!spec.ClosureResponsibilitySource.valid() ||
			spec.RelatedPriorClosure.valid() {
			return ContinuedAttemptDecision{}, ErrInvalidContinuedAttemptDecision
		}
		decision.cutoffBoundary = spec.CutoffBoundary
		decision.closureResponsibilitySource = spec.ClosureResponsibilitySource
	case ReopeningDecision:
		// 重开不带截断边界，也不带关闭责任来源：它「只允许未来形成新交易」，不裁决任何并发尝试的
		// 合法性；要核的那份来源在它所关联的关闭上。
		if !spec.RelatedPriorClosure.valid() ||
			spec.CutoffBoundary.valid() ||
			spec.ClosureResponsibilitySource.valid() {
			return ContinuedAttemptDecision{}, ErrInvalidContinuedAttemptDecision
		}
		if currentFinalPresent {
			return ContinuedAttemptDecision{}, ErrContinuedAttemptDecisionNotAdmitted
		}
		standing, found := register.standingClosure()
		// 指不到一份仍然生效的关闭时不成立：重开一个没关过的包裹说不出它在重开什么，而指向
		// 一份已被更早的重开解掉的关闭，会让同一份关闭被解两次。
		if !found || standing.id != spec.RelatedPriorClosure {
			return ContinuedAttemptDecision{}, ErrContinuedAttemptDecisionNotAdmitted
		}
		// 只面向未来生效：与所解的那份关闭同刻或更早生效的重开，会让「边界后的新尝试被拒绝」
		// 这条在时间上自相矛盾。
		if !spec.EffectiveAt.After(standing.effectiveAt) {
			return ContinuedAttemptDecision{}, ErrInvalidContinuedAttemptDecision
		}
		decision.relatedPriorClosure = spec.RelatedPriorClosure
	}
	return decision, nil
}

func (register ContinuedAttemptRegister) holds(id ContinuedAttemptDecisionID) bool {
	for _, decision := range register.decisions {
		if decision.id == id {
			return true
		}
	}
	return false
}

// standingClosure 交回当前仍然生效的那份关闭。
//
// 按追加顺序走而不是按生效时间排序：CONTEXT 要求「同一业务时点的冲突必须保存稳定、可审计的
// 领域顺序」，而追加顺序正是那个顺序。按时间排会让同刻的两条决定谁在后取决于排序实现。
func (register ContinuedAttemptRegister) standingClosure() (ContinuedAttemptDecision, bool) {
	var standing ContinuedAttemptDecision
	var found bool
	for _, decision := range register.decisions {
		switch decision.kind {
		case ControlledClosureDecision:
			standing, found = decision, true
		case ReopeningDecision:
			standing, found = ContinuedAttemptDecision{}, false
		}
	}
	return standing, found
}

// StandingClosure 交回当前仍然生效的那份受控关闭决定（若有）。它是 Judge 的`受控关闭`一格
// 背后的那份决定本体——包裹终局的关闭路径要把它作为证据引用（JudgeLabelServiceFinal），
// 只有判断值不够指。
func (register ContinuedAttemptRegister) StandingClosure() (ContinuedAttemptDecision, bool) {
	return register.standingClosure()
}

// Judge 现算`包裹级继续尝试判断`。
//
// 规则逐字取自 CONTEXT：「当前无有效终局且没有生效关闭时派生为开放，仍有生效关闭时保持受控
// 关闭」。终局在场时不派生`开放`——那一格的含义是「允许申请新的重试、替代或换单」，而终局已经
// 关掉了这件事。
//
// 它不缓存也不落库。CONTEXT 明写终局有效性后来变化时判断要「依据剩余有效决定历史和更新后的
// 终局结果重新派生」，存一列就会在那一刻变成旧话。
func (register ContinuedAttemptRegister) Judge(currentFinalPresent bool) ContinuedAttemptJudgment {
	if _, closed := register.standingClosure(); closed {
		return ContinuedAttemptControlledClosed
	}
	if currentFinalPresent {
		return ContinuedAttemptControlledClosed
	}
	return ContinuedAttemptOpen
}

// RehydrateContinuedAttemptRegisterSpec 携带从一行重建登记册所需的全部字段。
type RehydrateContinuedAttemptRegisterSpec struct {
	Revision  int64
	Tenant    TenantID
	Parcel    DeclaredParcelID
	Decisions []ContinuedAttemptDecisionSpec
}

// RehydrateContinuedAttemptRegister 逐条过与写入时同一套校验把一行读回登记册。
//
// **重建时终局一律按不在场传入。** 终局有效性是读回之后由调用方现取的事实，用它去卡重建会让
// 一行完全合法的历史（关闭 → 重开 → 其后才形成终局）读不回来——重建门以结果自证一致，不逆推
// 形成过程。
func RehydrateContinuedAttemptRegister(
	spec RehydrateContinuedAttemptRegisterSpec,
) (ContinuedAttemptRegister, error) {
	if spec.Revision < 1 {
		return ContinuedAttemptRegister{}, ErrInvalidRehydratedContinuedAttemptRegister
	}
	register, err := OpenContinuedAttemptRegister(spec.Tenant, spec.Parcel)
	if err != nil {
		return ContinuedAttemptRegister{}, ErrInvalidRehydratedContinuedAttemptRegister
	}
	register.revision = spec.Revision
	for _, decision := range spec.Decisions {
		appended, err := register.Append(decision, false)
		if err != nil {
			return ContinuedAttemptRegister{}, ErrInvalidRehydratedContinuedAttemptRegister
		}
		register = appended
	}
	return register, nil
}

// ErrInvalidRehydratedContinuedAttemptRegister 与 ErrInvalidContinuedAttemptDecision 分开
// （ADR-0028/0030 的纪律）：后者说「此刻要发生的这件事不合规则」，前者说「这份已经发生过的
// 东西不可能是本上下文判出来的」——要查的是库里那一行或写它的适配器。
var ErrInvalidRehydratedContinuedAttemptRegister = errors.New(
	"parcel shipment: invalid rehydrated continued attempt register",
)
