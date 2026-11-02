package domain

// ShipmentDocumentNumberReference 是客户按运单号追踪时给的那个号：委托的单据号（GLOSSARY「单据号」；ADR-0356 决定一）。委托与它的
// 单据号都归 parcel-shipment，本上下文只拿它当查询键交过去，不存、不随视图留（ADR-0356 决定四）。
//
// 逐字保存，不去空白、不折大小写：号由编号规则渲染，折算就是另一个号；只拒全空白——那立不起一个号。
type ShipmentDocumentNumberReference struct{ requiredValue }

func NewShipmentDocumentNumberReference(value string) (ShipmentDocumentNumberReference, error) {
	required, err := newRequiredValue("shipment document number reference", value)
	return ShipmentDocumentNumberReference{required}, err
}

// CustomerParcelReference 是客户为一件包裹声明的客户包裹件号，按运单号追踪时随列出的那一件带回，客户系统据它把自己的件对上
// 包裹内部引用（ADR-0356 决定三；ADR-0223 决定一）。值归 parcel-shipment 的委托，本上下文只转述。
type CustomerParcelReference struct{ requiredValue }

func NewCustomerParcelReference(value string) (CustomerParcelReference, error) {
	required, err := newRequiredValue("customer parcel reference", value)
	return CustomerParcelReference{required}, err
}
