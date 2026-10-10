package domain

import (
	"fmt"

	"go.idp.xyz/idp-parcel/referenceconfig"
)

// CatalogBasisReference 是版本化网络目录里一版定义行的登记依据：这一版凭什么登进来。采用参考配置时它是
// 采用路径写成的引用串（ADR-0147 决定三、四）；采用后改过的修订带租户自己的依据。零值是这一版登记时
// 没给依据——存量行与不带依据的直接登记都落这一格，不补默认。带参考配置引用前缀的串必须打得开已发布的
// 那一版，否则立不起来（ADR-0157 同一条），不退成一个不透明串。
type CatalogBasisReference struct{ value string }

func NewCatalogBasisReference(value string) (CatalogBasisReference, error) {
	if _, claimed, err := referenceconfig.OpenCitation(value); claimed && err != nil {
		return CatalogBasisReference{}, fmt.Errorf("catalog basis reference: %w", err)
	}
	required, err := newRequiredValue("catalog basis reference", value)
	if err != nil {
		return CatalogBasisReference{}, err
	}
	return CatalogBasisReference{value: required.value}, nil
}

func (reference CatalogBasisReference) String() string { return reference.value }

// Present 报告这一版登记时给没给依据。
func (reference CatalogBasisReference) Present() bool { return reference.value != "" }
