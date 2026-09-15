package domain

// 本文件是 PS CONTEXT「地址要素」词条的机制半边（pp-seams/03 裁决 1）：寄件与收件两个资料范围内，由本上下文定名的
// 一组封闭要素。要素仍是规范化业务内容里的 name/value 条目，条目名由资料范围原词与要素名两段组成——本文件只定名字
// 与「按名读值」的形，租户接单资料的字段怎么映到这些名字是接入侧的实例半边，这里一个渠道字段名都不认。
//
// 它不是地点：收件那一侧的「地点」词是收件地点引用（DeliveryPlaceReference），寄件那一侧不另造「寄件地点引用」
// （03 裁决 2）；地址要素是资料范围里的两格值，读口交值不交地点。

// senderPlaceDataGroupWord 是本上下文对「寄件」资料组的自有原词，与 deliveryPlaceDataGroupWord 同款：只用来给寄件
// 范围上的修订版本与地址要素条目一个稳定的名字，不是一个地点身份，也不派生任何引用。
const senderPlaceDataGroupWord = "SENDER_PLACE"

// SenderPlaceDataGroup 交回寄件资料组的原词引用。
func SenderPlaceDataGroup() SourceDataGroupReference {
	return SourceDataGroupReference{requiredValue{value: senderPlaceDataGroupWord}}
}

// AddressElementName 是地址要素的封闭名集。首批两个：邮编与国家 / 地区码——分区表（PP）与服务区域（NR）要的就是
// 这两格；再长一格要先回 CONTEXT 词条改封闭集。
type AddressElementName uint8

const (
	AddressElementNameInvalid AddressElementName = iota
	PostalCodeElement
	CountryCodeElement
)

func (name AddressElementName) String() string {
	switch name {
	case PostalCodeElement:
		return "POSTAL_CODE"
	case CountryCodeElement:
		return "COUNTRY_CODE"
	default:
		return ""
	}
}

func (name AddressElementName) valid() bool {
	return name == PostalCodeElement || name == CountryCodeElement
}

// addressElementEntrySeparator 沿用既有 name/value 条目的点号命名法（隔离形态词表 `sender.address` 一类）。
const addressElementEntrySeparator = "."

// AddressElementEntryName 交回某个资料范围上某个要素的条目名：`<资料范围原词>.<要素名>`。寄件与收件的同名要素是两条
// 不同的内容，名字里带范围才分得开；范围原词与要素名都是本上下文的封闭字面，拼出来的名字不必再过一道构造门。
func AddressElementEntryName(group SourceDataGroupReference, element AddressElementName) string {
	return group.String() + addressElementEntrySeparator + element.String()
}

// AddressElements 是一个资料范围（寄件或收件）上的地址要素取值：每个要素至多一格，缺席如实。值原样保全——邮编的格式
// 与前缀粒度属实例半边（ADR-0109「随首份真实分区表定」），本上下文不校验、不规范化、不去空白。
//
// 两格各带一个在场标志而不是用空串表缺席：客户给的串本上下文不去空白，一个全是空白的值与「没报」得分得开。「没报」
// 含显式清空，而显式清空按 CanonicalContentEntry 的定义就是一条值为空串的条目——值层面只认这一种缺席，空白串不是
// 空串，原样在场；同名多条互相矛盾那一路的缺席由 AddressElementsOf 自己的头注讲。
type AddressElements struct {
	postalCode          string
	postalCodeDeclared  bool
	countryCode         string
	countryCodeDeclared bool
}

// PostalCode 的第二个返回值为假即该范围上没报邮编（含显式清空），不是「读不到」。
func (elements AddressElements) PostalCode() (string, bool) {
	return elements.postalCode, elements.postalCodeDeclared
}

// CountryCode 的第二个返回值为假即该范围上没报国家 / 地区码。
func (elements AddressElements) CountryCode() (string, bool) {
	return elements.countryCode, elements.countryCodeDeclared
}

// Empty 报告两个要素都缺席——「要素缺席答缺」那一格的整体判据，读口据此不必逐格问。
func (elements AddressElements) Empty() bool {
	return !elements.postalCodeDeclared && !elements.countryCodeDeclared
}

// with 记下某个要素的取值；集外的名字不落任何一格。
func (elements AddressElements) with(element AddressElementName, value string) AddressElements {
	switch element {
	case PostalCodeElement:
		elements.postalCode, elements.postalCodeDeclared = value, true
	case CountryCodeElement:
		elements.countryCode, elements.countryCodeDeclared = value, true
	}
	return elements
}

// without 抹掉某个要素——同名多条互相矛盾时两条都不认走这里。
func (elements AddressElements) without(element AddressElementName) AddressElements {
	switch element {
	case PostalCodeElement:
		elements.postalCode, elements.postalCodeDeclared = "", false
	case CountryCodeElement:
		elements.countryCode, elements.countryCodeDeclared = "", false
	}
	return elements
}

// DeclaredAddressElements 是随一版提交版本申报的地址要素子段（pp-seams/05 裁决 1 / 2）：寄件与收件两个委托级资料范围
// 各一段 AddressElements，缺席如实。它是「内容随提交版本进快照」的第二种——画像（DeclaredParcelProfile）按成员，
// 要素按资料范围；与画像同一口径，允许整体缺席或只报一段。
//
// 只有这两段而不是按 SourceDataGroupReference 开放成一张表：地址要素只在寄 / 收两个范围上有词条（PS CONTEXT「地址
// 要素」），开放的键会让「别的范围上的同名要素」有地方落，而 AddressElementsOf 头注刻意不认它们。
type DeclaredAddressElements struct {
	sender   AddressElements
	delivery AddressElements
}

// NewDeclaredAddressElements 把寄件、收件两段包成一份子段；任一段可为零值（该范围上要素缺席）。
func NewDeclaredAddressElements(sender, delivery AddressElements) DeclaredAddressElements {
	return DeclaredAddressElements{sender: sender, delivery: delivery}
}

// InGroup 交回某个资料范围那一段：寄件与收件之外的范围没有地址要素，答零值——那是「本范围上没有这类要素」，
// 不是读不到。
func (elements DeclaredAddressElements) InGroup(group SourceDataGroupReference) AddressElements {
	switch group {
	case SenderPlaceDataGroup():
		return elements.sender
	case DeliveryPlaceDataGroup():
		return elements.delivery
	default:
		return AddressElements{}
	}
}

// Empty 报告两段都缺席——提交版本上整个子段缺席的判据，落库时据此省掉整段。
func (elements DeclaredAddressElements) Empty() bool {
	return elements.sender.Empty() && elements.delivery.Empty()
}
