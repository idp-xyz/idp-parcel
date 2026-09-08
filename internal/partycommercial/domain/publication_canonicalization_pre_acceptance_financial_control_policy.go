package domain

import "fmt"

// 本文件是接受前财务控制策略册接进服务端规范化的那一格（加册不换号，仍是 PCC-1——ADR-0126 Decision 一；票
// admin-write-faces/13）。策略版本的正文是父行一格共同通过条件加子表逐行的控制项（0024，ADR-0115 Decision 二）；
// 受控批文把它写成 declarations 下的 preAcceptanceFinancialControlPolicyBody{jointPassCondition, controls[…]}，
// 这里键名镜像它。

// PreAcceptanceFinancialControlPolicyBody 是接受前财务控制策略版本的正文输入面，以领域值对象给出：它就是
// NewPreAcceptanceFinancialControlPolicy 收的那两项，只是不带拥有它们的（已生效）版本——预览与录入发生在发布之前。
//
// Items 是已过 NewPreAcceptanceControlItem 的行；跨行的门（至少一项、顺序唯一、（种类 × 范围）唯一）在折成文档前
// 由 validate 答，与发布时同一条规则。零项不是「显式无控制」——那一句由客户合同声明（ADR-0115 Decision 一），本正文说不了它。
type PreAcceptanceFinancialControlPolicyBody struct {
	JointPass JointPassCondition
	Items     []PreAcceptanceControlItem
}

// validate 在折成文档前把正文过一遍与发布时相同的门。不另造校验——预览要在录入之前就把「顺序撞了」「同一范围同一种
// 控制两行」「一项都没有」答给操作者，不能等到发布那一刻，而那条规则只在 filedPreAcceptanceControlItems 一处。
func (body PreAcceptanceFinancialControlPolicyBody) validate() error {
	_, err := filedPreAcceptanceControlItems(body.JointPass, body.Items)
	return err
}

// canonicalPreAcceptanceFinancialControlPolicyBody 镜像批文 preAcceptanceFinancialControlPolicyBody 的键名。
// 控制项按判断顺序写出：顺序版本内唯一，因此它就是这张表唯一的自然序——表单里换行序不是换正文，摘要不该跟着变。
type canonicalPreAcceptanceFinancialControlPolicyBody struct {
	JointPassCondition string                              `json:"jointPassCondition"`
	Controls           []canonicalPreAcceptanceControlItem `json:"controls"`
}

// canonicalPreAcceptanceControlItem 是一行控制项，五格都必在场：行的形状由 NewPreAcceptanceControlItem 守，文档不给
// 缺格留位置。责任引用与费用范围引用是开放引用（0024 头注），收串不校验存在性。
type canonicalPreAcceptanceControlItem struct {
	Control        string `json:"control"`
	ChargeScope    string `json:"chargeScope"`
	Order          int    `json:"order"`
	OnFailure      string `json:"onFailure"`
	Responsibility string `json:"responsibility"`
}

// canonicalizePreAcceptanceFinancialControlPolicy 是 CanonicalizePublicationContent 在本册的那一支：正文缺席答
// ErrPublicationContentAbsent，正文过不了门答构造门的原话，过了门折成文档算摘要。
func canonicalizePreAcceptanceFinancialControlPolicy(content PublicationContent) (CanonicalPublicationContent, error) {
	none := CanonicalPublicationContent{}
	if content.PreAcceptanceFinancialControlPolicy == nil {
		return none, ErrPublicationContentAbsent
	}
	document, err := canonicalPreAcceptanceFinancialControlPolicyBodyOf(*content.PreAcceptanceFinancialControlPolicy)
	if err != nil {
		return none, err
	}
	return canonicalDigestOf(canonicalPublicationDocument{
		Canonicalization:                    publicationCanonicalizationVersion,
		Kind:                                content.Kind.String(),
		PreAcceptanceFinancialControlPolicy: document,
	})
}

func canonicalPreAcceptanceFinancialControlPolicyBodyOf(body PreAcceptanceFinancialControlPolicyBody) (*canonicalPreAcceptanceFinancialControlPolicyBody, error) {
	filed, err := filedPreAcceptanceControlItems(body.JointPass, body.Items)
	if err != nil {
		return nil, err
	}
	document := &canonicalPreAcceptanceFinancialControlPolicyBody{
		JointPassCondition: body.JointPass.String(),
		Controls:           make([]canonicalPreAcceptanceControlItem, 0, len(filed)),
	}
	for _, item := range filed {
		document.Controls = append(document.Controls, canonicalPreAcceptanceControlItem{
			Control:        item.Kind().String(),
			ChargeScope:    item.Scope().String(),
			Order:          item.EvaluationOrder(),
			OnFailure:      item.FailureDisposition().String(),
			Responsibility: item.Responsibility().String(),
		})
	}
	return document, nil
}

// body 把文档里的一节折回领域正文。每一行过 NewPreAcceptanceControlItem，三个封闭集按 String() 原词反查；跨行的门
// 留给 validate——快照是数据，正文立不立得住仍由构造门说。
func (document canonicalPreAcceptanceFinancialControlPolicyBody) body() (PreAcceptanceFinancialControlPolicyBody, error) {
	jointPass, known := JointPassConditionNamed(document.JointPassCondition)
	if !known {
		return PreAcceptanceFinancialControlPolicyBody{}, fmt.Errorf("jointPassCondition: %w: %q",
			ErrInvalidPreAcceptanceFinancialControlPolicy, document.JointPassCondition)
	}
	body := PreAcceptanceFinancialControlPolicyBody{JointPass: jointPass, Items: make([]PreAcceptanceControlItem, 0, len(document.Controls))}
	for index, row := range document.Controls {
		item, err := row.item()
		if err != nil {
			return PreAcceptanceFinancialControlPolicyBody{}, fmt.Errorf("controls[%d]: %w", index, err)
		}
		body.Items = append(body.Items, item)
	}
	if err := body.validate(); err != nil {
		return PreAcceptanceFinancialControlPolicyBody{}, err
	}
	return body, nil
}

func (row canonicalPreAcceptanceControlItem) item() (PreAcceptanceControlItem, error) {
	kind, known := PreAcceptanceControlKindNamed(row.Control)
	if !known {
		return PreAcceptanceControlItem{}, fmt.Errorf("control: %w: %q", ErrInvalidPreAcceptanceControlItem, row.Control)
	}
	scope, err := NewChargeScopeReference(row.ChargeScope)
	if err != nil {
		return PreAcceptanceControlItem{}, fmt.Errorf("chargeScope: %w", err)
	}
	disposition, known := ControlFailureDispositionNamed(row.OnFailure)
	if !known {
		return PreAcceptanceControlItem{}, fmt.Errorf("onFailure: %w: %q", ErrInvalidPreAcceptanceControlItem, row.OnFailure)
	}
	responsibility, err := NewControlResponsibilityReference(row.Responsibility)
	if err != nil {
		return PreAcceptanceControlItem{}, fmt.Errorf("responsibility: %w", err)
	}
	return NewPreAcceptanceControlItem(kind, scope, row.Order, disposition, responsibility)
}

// PreAcceptanceControlKindNamed 按 String() 的原词反查控制种类：规范化文档里的、运营操作者面载荷里的都是那一个词，
// 名单只在 String() 一处，这里只是反查（判据同 CommercialObjectKindNamed）。集合外答 false——含空串，也含「无控制」
// 那类词：它不在集合里不是漏，是 ADR-0115 Decision 一。
func PreAcceptanceControlKindNamed(name string) (PreAcceptanceControlKind, bool) {
	for kind := PrepaidFreezeControl; kind.valid(); kind++ {
		if kind.String() == name {
			return kind, true
		}
	}
	return PreAcceptanceControlKindInvalid, false
}

// ControlFailureDispositionNamed 按 String() 的原词反查失败处置，判据同上。集合外答 false——把打错的处置折进某一格，
// 等于替租户改了失败时委托的去向。
func ControlFailureDispositionNamed(name string) (ControlFailureDisposition, bool) {
	for disposition := RejectOnControlFailure; disposition.valid(); disposition++ {
		if disposition.String() == name {
			return disposition, true
		}
	}
	return ControlFailureDispositionInvalid, false
}

// JointPassConditionNamed 按 String() 的原词反查共同通过条件，判据同上。首发只有一值，空串仍答 false：CONTEXT 要求
// 策略明确它，缺席不折成「全部通过」（ADR-0115 Decision 三）。
func JointPassConditionNamed(name string) (JointPassCondition, bool) {
	for condition := AllControlsPass; condition.valid(); condition++ {
		if condition.String() == name {
			return condition, true
		}
	}
	return JointPassConditionInvalid, false
}
