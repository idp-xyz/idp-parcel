package domain

import (
	"errors"
	"fmt"
)

// 本文件是接单规则包的提交条件在受理判断里的判法（`BD-PS-006`；ADR-0374 决定三，ADR-0376 决定一；票 declared-goods-lines/09）。
// 条件本身随产品发布在参考配置里（`REFCFG-1:parcel-shipment/submission-conditions/…`），服务产品所采用的接单规则包以
// `CROSS_FIELD_CONDITION` 引用它才生效；判法在这里，参考配置只写条件码与它指的格。结果归「必要资料」组：未满足是`未定`，
// 续办方是客户——头部系统缺平台号、缺箱唛是「待补全」，客户补齐即继续，不退单。

var ErrInvalidSubmissionCondition = errors.New("parcel shipment: invalid submission condition")

// SubmissionCondition 是本上下文认得的提交条件，封闭集合：参考配置里出现集合外的码，构造门答错，那一版不能发布。
type SubmissionCondition uint8

const (
	SubmissionConditionInvalid SubmissionCondition = iota
	// PlatformShipmentDeclaredCondition：委托报了平台货件（销售平台与平台货件号）。
	PlatformShipmentDeclaredCondition
	// EveryParcelHasPlatformBoxLabelCondition：当前提交版本的每件包裹都报了平台箱标号。
	EveryParcelHasPlatformBoxLabelCondition
	// PackingDeclaredCondition：当前提交版本报了货物明细与装箱分配。报了就是报全，由提交口的形状规则保证，这一条只要求在场。
	PackingDeclaredCondition
)

var submissionConditionWords = map[SubmissionCondition]string{
	PlatformShipmentDeclaredCondition:       "PLATFORM_SHIPMENT_DECLARED",
	EveryParcelHasPlatformBoxLabelCondition: "EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL",
	PackingDeclaredCondition:                "PACKING_DECLARED",
}

func (condition SubmissionCondition) String() string {
	return submissionConditionWords[condition]
}

func ParseSubmissionCondition(word string) (SubmissionCondition, bool) {
	for condition, known := range submissionConditionWords {
		if known == word {
			return condition, true
		}
	}
	return SubmissionConditionInvalid, false
}

// SubmissionConditions 是一份依据快照上要判的条件集。用位集而不是切片：快照要能整值比较（重建时比对决定与预计承诺的
// 依据），条件的次序也不是内容。零值即没有条件。
type SubmissionConditions struct {
	bits uint8
}

func NewSubmissionConditions(conditions ...SubmissionCondition) (SubmissionConditions, error) {
	var set SubmissionConditions
	for _, condition := range conditions {
		if _, known := submissionConditionWords[condition]; !known {
			return SubmissionConditions{}, fmt.Errorf("%w: %d", ErrInvalidSubmissionCondition, condition)
		}
		set.bits |= 1 << condition
	}
	return set, nil
}

func (set SubmissionConditions) Has(condition SubmissionCondition) bool {
	return set.bits&(1<<condition) != 0
}

// Conditions 按条件码的定义次序交回，次序稳定，落库与显示都用它。
func (set SubmissionConditions) Conditions() []SubmissionCondition {
	var conditions []SubmissionCondition
	for condition := PlatformShipmentDeclaredCondition; condition <= PackingDeclaredCondition; condition++ {
		if set.Has(condition) {
			conditions = append(conditions, condition)
		}
	}
	return conditions
}

func (set SubmissionConditions) Union(other SubmissionConditions) SubmissionConditions {
	return SubmissionConditions{bits: set.bits | other.bits}
}

func (set SubmissionConditions) Empty() bool {
	return set.bits == 0
}

// RequiredDocumentChecks 按依据快照上的提交条件判「必要资料」组。对象是当前提交版本与委托上的平台声明。
//
// 委托级条件（平台货件、装箱分配）未满足出一条版本级`未定`；逐件条件（箱标号）按缺的包裹各出一条——查阅面要说得出缺的是
// 哪几箱。全部满足、或快照上没有条件，出一条版本级通过，这一组算判过：规则包声明了这一组适用而没引用任何读得懂的条件，
// 它要求的就只有这些。
func (request ShipmentRequest) RequiredDocumentChecks(conditions SubmissionConditions) ([]AcceptanceCheck, error) {
	var checks []AcceptanceCheck
	unmet := func(parcel DeclaredParcelID, condition SubmissionCondition) error {
		reason, err := NewCheckReason(condition.String())
		if err != nil {
			return err
		}
		check, err := NewUndeterminedAcceptanceCheck(RequiredDocumentCheck, parcel, reason, ResumeByCustomerSupplement)
		if err != nil {
			return err
		}
		checks = append(checks, check)
		return nil
	}
	if conditions.Has(PlatformShipmentDeclaredCondition) && !request.platformShipment.Declared() {
		if err := unmet(DeclaredParcelID{}, PlatformShipmentDeclaredCondition); err != nil {
			return nil, err
		}
	}
	if conditions.Has(EveryParcelHasPlatformBoxLabelCondition) {
		labelled := make(map[DeclaredParcelID]struct{}, len(request.platformBoxLabels))
		for _, label := range request.platformBoxLabels {
			labelled[label.parcel] = struct{}{}
		}
		for _, parcel := range request.currentVersion.DeclaredParcelIDs() {
			if _, ok := labelled[parcel]; !ok {
				if err := unmet(parcel, EveryParcelHasPlatformBoxLabelCondition); err != nil {
					return nil, err
				}
			}
		}
	}
	if conditions.Has(PackingDeclaredCondition) && len(request.currentVersion.goods.Packing()) == 0 {
		if err := unmet(DeclaredParcelID{}, PackingDeclaredCondition); err != nil {
			return nil, err
		}
	}
	if len(checks) > 0 {
		return checks, nil
	}
	passed, err := NewAcceptanceCheck(RequiredDocumentCheck, DeclaredParcelID{}, CheckPassed, CheckReason{})
	if err != nil {
		return nil, err
	}
	return []AcceptanceCheck{passed}, nil
}
