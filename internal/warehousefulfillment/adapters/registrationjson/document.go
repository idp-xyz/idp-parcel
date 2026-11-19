// Package registrationjson 是仓配履约登记报文的 JSON 译装：把调用方交来的一笔修订译成应用层的登记命令（ADR-0375 决定四）。
//
// 租户与登记责任方只从接入信封来（UC-WF-001「输入」），所以译装收调用方传入的两个值，报文里不许有这两格。本包只管形状：
// 国家码在不在内置目录、业务时区认不认得出、修订号接不接得上，都是应用层在判过重放与冲突之后另判的事，译装不提前把它们
// 当坏载荷拒掉，否则同一份内容在「原样重投」时会先于「已登记（重放）」被拒。
//
// 各册的报文共用本文件里的件：坏载荷的错误格、一格文本过领域构造门、按「一个对象、不认识的键即拒」解码、自带租户与登记责任方
// 的拒绝。各册只写自己的文档形状与命令译法。
package registrationjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ErrMalformedDocument 是报文形状不成立：改载荷才会好，原样重投不会。
var ErrMalformedDocument = errors.New("warehouse fulfillment registrationjson: malformed document")

// MalformedDocumentError 带一句只谈调用方自己这份载荷错在哪的理由。理由只点名格，不回显任何取值——地址正文与仓编码出现在
// 错误正文里，日志与界面都得再防一次（UC-WF-001「权限、隔离与审计」）。
type MalformedDocumentError struct {
	Reason string
}

func (err *MalformedDocumentError) Error() string {
	return fmt.Sprintf("%s: %s", ErrMalformedDocument, err.Reason)
}

func (err *MalformedDocumentError) Is(target error) bool {
	return target == ErrMalformedDocument
}

func malformed(format string, arguments ...any) error {
	return &MalformedDocumentError{Reason: fmt.Sprintf(format, arguments...)}
}

// carriedIdentity 是各册报文共有的两格，它们**只为拒而存在**：用 json.RawMessage 是因为 `"tenantId": null` 也是自报，键在场
// 就要拒，指针认不出 null。只靠 DisallowUnknownFields 也会拒，但那样的理由是「未知键」，登记方读不出这是一条规则而不是拼写错误。
type carriedIdentity struct {
	TenantID   json.RawMessage `json:"tenantId"`
	Registrant json.RawMessage `json:"registrant"`
}

// refuseCarriedIdentity 在报文自带租户或登记责任方时交回坏载荷：键在场即拒，不看值。
func (carried carriedIdentity) refuseCarriedIdentity() error {
	if len(carried.TenantID) > 0 {
		return malformed("tenantId must not be carried in the payload; the tenant comes from the access channel, not from the request")
	}
	if len(carried.Registrant) > 0 {
		return malformed("registrant must not be carried in the payload; the registrant comes from the operator envelope, not from the request")
	}
	return nil
}

// textCell 过一格文本的领域构造门。构造器的错误正文带着格名而不带取值，但这里仍自己写理由：理由只点名 JSON 里的键路径，
// 与报文的写法对得上。
func textCell[Value any](path, raw string, construct func(string) (Value, error)) (Value, error) {
	value, err := construct(raw)
	switch {
	case err == nil:
		return value, nil
	case errors.Is(err, domain.ErrUnstorableValue):
		return value, malformed("%s carries a character that cannot be stored (NUL or invalid UTF-8)", path)
	default:
		return value, malformed("%s must not be blank", path)
	}
}

// decodeDocument 把请求体解成一份文档：只许一个 JSON 对象，不认识的键即拒，尾随的第二个 JSON 值也拒（放过它就是无声丢掉一段
// 输入）。subject 是理由里「不属于哪种修订」的说法，如 "warehouse revision"。
func decodeDocument[Document any](reader io.Reader, subject string) (Document, error) {
	var document, none Document
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return none, decodeFailure(err, subject)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return none, malformed("the body must carry exactly one JSON object and nothing after it")
	}
	return document, nil
}

// decodeFailure 把解码器的错译成一句不带取值的理由。解码器的错误正文里有的带着报文里的片段（类型错的取值、读错的原文），
// 所以只取它的类别与键路径，不转抄它的正文。
func decodeFailure(err error, subject string) error {
	var syntax *json.SyntaxError
	var typed *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		return malformed("the body is empty")
	case errors.Is(err, io.ErrUnexpectedEOF), errors.As(err, &syntax):
		return malformed("the body is not valid JSON")
	case errors.As(err, &typed):
		if typed.Field == "" {
			return malformed("the body must be a JSON object")
		}
		return malformed("%s must be %s", typed.Field, expectedKind(typed.Type))
	}
	if quoted, isUnknownKey := strings.CutPrefix(err.Error(), "json: unknown field "); isUnknownKey {
		if key, unquoteErr := strconv.Unquote(quoted); unquoteErr == nil && len(key) <= 64 {
			return malformed("the key %q is not part of a %s", key, subject)
		}
		return malformed("the payload carries a key that is not part of a %s", subject)
	}
	return malformed("the body could not be read as JSON")
}

func expectedKind(kind reflect.Type) string {
	switch kind.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Int, reflect.Int64:
		return "an integer"
	case reflect.Bool:
		return "a boolean"
	case reflect.Struct:
		return "an object"
	default:
		return "of the declared type"
	}
}
