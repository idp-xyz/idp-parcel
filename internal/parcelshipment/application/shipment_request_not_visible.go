package application

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ErrShipmentRequestNotVisible 是命令指名的委托在请求方的来源身份下查不到。端口以完整来源身份为键，否定结果本来就不分
// 「不存在」与「属于别的租户或客户账户」，所以这一格就是`统一不可见结果`：接 HTTP 答 404（ADR-0022），不拆成「未找到」与
// 「无权」两格。它是调用方的错，不是没形成答案——答成 5xx 会让离线客户端把它留在队里无限重投。
//
// 它仍是 domain.ErrInvalidShipmentRequest 的一种：领域上说的同样是「这份委托上没有这件事可做」，按旧名判的调用方不受影响；
// HTTP 适配器只认这一个具名错误，别处包着 ErrInvalidShipmentRequest 交上来的领域拒绝照旧是 5xx。
var ErrShipmentRequestNotVisible = fmt.Errorf("%w: not visible to this source identity", domain.ErrInvalidShipmentRequest)
