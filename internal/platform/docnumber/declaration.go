// Package docnumber 是编号技术组件在本仓的共享内核（ADR-0355 决定四至八）：单据类型声明的汇总、单据编号规则册
// 的登记与读、取号编排与预览。
//
// 它不是限界上下文，不拥有任何单据（CONTEXT-MAP「公司级技术组件」）：只认类型代码，不读任何业务数据。单据号归
// 单据所属的上下文，各上下文经自己的消费侧端口调本包的取号编排，号随单据落在它们自己的表里。模板语法、加扰与
// 序列表只在平台库 `go.idp.xyz/idp-platform-go/numbering` 一处；本包只用它的公开原语编排，不复制其算法或 SQL。
//
// 本包不依赖 HTTP 与驱动：规则册的存储、序列分配与事务归属经端口注入，适配器在 docnumberpg 与 docnumberhttp。
package docnumber

import (
	"errors"
	"fmt"
	"regexp"
)

// TypeCode 是单据类型代码，例如 `SHIPMENT`。它是规则册与序列键里的那一维，由单据所属上下文声明。
type TypeCode string

// DateTimeZoneSource 声明一类单据的开立方有没有已登记的业务时区（ADR-0355 决定七）。取哪一个业务时区由所属
// 上下文在开立时交进来，本包只按声明分两种来路。
type DateTimeZoneSource string

const (
	// DateTimeZoneFromIssuer：开立方有业务时区（网点、结算账户）。开立时交进来就用它；交不出（开立方没登）
	// 时退到租户的单据编号时区。
	DateTimeZoneFromIssuer DateTimeZoneSource = "ISSUER_BUSINESS_TIME_ZONE"
	// DateTimeZoneFromTenant：开立方没有业务时区，日期一律按租户的单据编号时区。
	DateTimeZoneFromTenant DateTimeZoneSource = "TENANT_NUMBERING_TIME_ZONE"
)

func (source DateTimeZoneSource) valid() bool {
	return source == DateTimeZoneFromIssuer || source == DateTimeZoneFromTenant
}

// TypeDeclaration 是一类单据的编号声明：类型代码、产品默认模板与日期时区来路。每类只由所属上下文声明一处
// （ADR-0355 决定六），装配时经 NewCatalogue 汇总。
type TypeDeclaration struct {
	Code TypeCode
	// DefaultTemplate 是产品内置的默认模板（ADR-0355 决定五：内置策略，租户没登记自己的规则时就用它）。
	DefaultTemplate string
	DateTimeZone    DateTimeZoneSource
}

var (
	// ErrInvalidDeclaration 表示一条类型声明不成立；它是装配期的编程错误，进程带着原因退出。
	ErrInvalidDeclaration = errors.New("document numbering: invalid type declaration")
	// ErrTypeNotDeclared 表示类型代码没有任何上下文声明过。
	ErrTypeNotDeclared = errors.New("document numbering: document type is not declared")
)

// typeCodePattern 收大写字母开头的大写字母、数字与下划线。类型代码要进序列键与规则册的主键，也会被管理台与
// 日志引用，所以只收这一种写法，免得 `shipment` 与 `SHIPMENT` 成了两类。
var typeCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// Catalogue 是汇总后的类型声明表，装配时构造一次、之后只读。
type Catalogue struct {
	ordered []TypeDeclaration
	byCode  map[TypeCode]TypeDeclaration
}

// NewCatalogue 汇总各上下文的声明。类型代码重复、模板不合规、默认模板用了加扰，都是编程错误。
//
// 默认模板要过与租户登记同一道模板检查（checkTemplate）：产品默认不比租户的规则享有豁免，开立时才发现默认
// 模板渲染不出号就太晚了。默认模板不许用加扰——加扰密钥是租户取值、无默认（ADR-0355 决定五），一份依赖它的
// 默认模板对没配密钥的租户出不了号，「开箱即用」就不成立。
func NewCatalogue(groups ...[]TypeDeclaration) (*Catalogue, error) {
	catalogue := &Catalogue{byCode: map[TypeCode]TypeDeclaration{}}
	for _, group := range groups {
		for _, declaration := range group {
			if !typeCodePattern.MatchString(string(declaration.Code)) {
				return nil, fmt.Errorf("%w: type code %q", ErrInvalidDeclaration, declaration.Code)
			}
			if _, taken := catalogue.byCode[declaration.Code]; taken {
				return nil, fmt.Errorf("%w: type %s is declared twice", ErrInvalidDeclaration, declaration.Code)
			}
			if !declaration.DateTimeZone.valid() {
				return nil, fmt.Errorf("%w: type %s has no date time zone source", ErrInvalidDeclaration, declaration.Code)
			}
			checked, err := checkTemplate(declaration.Code, declaration.DefaultTemplate)
			if err != nil {
				return nil, fmt.Errorf("%w: type %s default template: %w", ErrInvalidDeclaration, declaration.Code, err)
			}
			if checked.scrambled {
				return nil, fmt.Errorf("%w: type %s default template uses a scrambled sequence", ErrInvalidDeclaration, declaration.Code)
			}
			catalogue.ordered = append(catalogue.ordered, declaration)
			catalogue.byCode[declaration.Code] = declaration
		}
	}
	if len(catalogue.ordered) == 0 {
		return nil, fmt.Errorf("%w: no type is declared", ErrInvalidDeclaration)
	}
	return catalogue, nil
}

// Declaration 交回一类单据的声明。
func (catalogue *Catalogue) Declaration(code TypeCode) (TypeDeclaration, bool) {
	declaration, found := catalogue.byCode[code]
	return declaration, found
}

// Declarations 按汇总顺序交回全部声明的一份拷贝。
func (catalogue *Catalogue) Declarations() []TypeDeclaration {
	return append([]TypeDeclaration(nil), catalogue.ordered...)
}
