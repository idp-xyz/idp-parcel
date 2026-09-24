package domain

// RequestedServiceProductEntryName 是委托声明的服务产品在服务要求段里的条目名（票 psb/17）。它是本上下文的封闭
// 字面，理由与地址要素同一条：读口只认封闭名字，租户约定的条目名一律不认，认了就是把租户 schema 写死进代码。
// 取隔离形态词表既有的名字：换一个，既有载荷的摘要就会变。
const RequestedServiceProductEntryName = "service.requestedProduct"

// DeclaredServiceProduct 是客户在一份提交里声明请求的服务产品——对象身份，不是版本：选哪一版仍由 party-commercial
// 按锚点解（ADR-0080）。值原样保全，本上下文不校验、不规范化、不去空白，理由同地址要素：它是客户说的话，产品在
// 不在册由商业解析答，不由本上下文预判。零值即未声明。
type DeclaredServiceProduct struct {
	value string
}

func (product DeclaredServiceProduct) String() string {
	return product.value
}

// Declared 报告这份提交有没有声明服务产品。
func (product DeclaredServiceProduct) Declared() bool {
	return product.value != ""
}

// RequestedServiceProductOf 从服务要求段条目里读委托声明的服务产品，读法与 AddressElementsOf 同一套：只认封闭
// 条目名；值为空串（CanonicalContentEntry 定义的「显式清空」）读作未声明；同名多条时一条都不认，读口不替客户挑。
func RequestedServiceProductOf(entries []CanonicalContentEntry) DeclaredServiceProduct {
	var product DeclaredServiceProduct
	matched := 0
	for _, entry := range entries {
		if entry.Name() != RequestedServiceProductEntryName {
			continue
		}
		matched++
		product = DeclaredServiceProduct{value: entry.Value()}
	}
	if matched > 1 {
		return DeclaredServiceProduct{}
	}
	return product
}
