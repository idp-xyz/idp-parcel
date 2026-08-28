package domain

import "fmt"

// 本文件是「合规候选口岸」与「申报路径」两本目录登记册的领域词形（票
// admin-remainder-mechanism-batch/03）。所有权取 CONTEXT 所有权句：本上下文拥有合规
// 候选区域、口岸、申报路径和关务适用性判断；network-routing 只在合格候选中选择。
// 目录因此只登事实——哪个口岸、哪条路径在哪个生效区间内是合规候选，路径经哪个口岸
// 按哪个方向以哪种申报模式申报——不做路由选择（NR 的事），也不做案件判断（案件链
// 已有归属）。生效区间不在这里建模：区间随登记给出、终点在换版时落定，那半边的词形
// 循 ObligationRegistration / InterpretationRuleEntry 先例落在端口层。

// CustomsPortReference 指名一个口岸。真实口岸标识属实例半边（PAR-NET-02 /
// PAR-CUS-01 待提供），这里只是引用词形，不内置任何国家或口岸默认。
type CustomsPortReference struct{ requiredValue }

func NewCustomsPortReference(value string) (CustomsPortReference, error) {
	required, err := newRequiredValue("customs port reference", value)
	return CustomsPortReference{required}, err
}

// DeclarationPathReference 指名一条申报路径。
type DeclarationPathReference struct{ requiredValue }

func NewDeclarationPathReference(value string) (DeclarationPathReference, error) {
	required, err := newRequiredValue("declaration path reference", value)
	return DeclarationPathReference{required}, err
}

// DeclarationModeReference 指名申报模式。模式是监管规则里的真实词表（受控跨客户
// 合报的兼容维之一），本产品今天拿不到任何真实模式集，所以它是引用不是封闭枚举
// ——把具体模式写成 Go 常量就是替租户拟实例默认值。
type DeclarationModeReference struct{ requiredValue }

func NewDeclarationModeReference(value string) (DeclarationModeReference, error) {
	required, err := newRequiredValue("declaration mode reference", value)
	return DeclarationModeReference{required}, err
}

// DeclarationPathRoute 是一条申报路径的三维目录事实：经哪个口岸、按哪个方向、以
// 哪种申报模式。方向沿用进出口封闭二值（ManifestDirection）；口岸以标识引用而不带
// 外键语义——路径与口岸的引用关系裁量记在票面（03），口岸在册与否是读侧核对的事，
// 目录行不替它作判断。
type DeclarationPathRoute struct {
	port      CustomsPortReference
	direction ManifestDirection
	mode      DeclarationModeReference
}

// NewDeclarationPathRoute 三维缺一构造不出：缺格落库会静默变成一条没人登记过的
// 路径事实，而三维正是 NR 拿去做候选选择的输入。
func NewDeclarationPathRoute(
	port CustomsPortReference,
	direction ManifestDirection,
	mode DeclarationModeReference,
) (DeclarationPathRoute, error) {
	if !port.valid() {
		return DeclarationPathRoute{}, fmt.Errorf("%w: customs port reference", ErrBlankValue)
	}
	if !direction.valid() {
		return DeclarationPathRoute{}, fmt.Errorf(
			"declaration path route: unknown manifest direction %d", direction)
	}
	if !mode.valid() {
		return DeclarationPathRoute{}, fmt.Errorf("%w: declaration mode reference", ErrBlankValue)
	}
	return DeclarationPathRoute{port: port, direction: direction, mode: mode}, nil
}

func (route DeclarationPathRoute) Port() CustomsPortReference { return route.port }

func (route DeclarationPathRoute) Direction() ManifestDirection { return route.direction }

func (route DeclarationPathRoute) Mode() DeclarationModeReference { return route.mode }
