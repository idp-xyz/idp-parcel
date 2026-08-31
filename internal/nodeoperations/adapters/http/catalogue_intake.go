package nodeopshttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件承载节点作业查阅页的目录查阅接入形状（ADR-0077，票
// admin-skeleton-closure-batch/05）。查阅不收寄、不集拆、不改控制——查询端点只消费
// 存储读面，不接应用编排（ADR-0077 Decision 一）；命令面的收寄端点在
// receive_delivered_unit.go，两面共用本包的传输层错误码与答复助手，但接入形状各自
// 成型：命令要从请求体收设备签发的事实身份与时间（ADR-0023），查阅对请求零采信。

// CatalogueQuery 是一次已授权的节点作业目录查阅（ADR-0077）：作用域来自认证与授权
// 结果，授权边界只有租户——物流节点与作业位置是作业事实的归属维，不是查阅方身份
// （见 domain.OperationsQueryScope）；页大小由接入面按渠道契约裁决。两样都不采信
// 调用方自报。
type CatalogueQuery struct {
	Scope domain.OperationsQueryScope
	Limit int
}

// CatalogueQueryIntake 把一次已认证的运营查阅请求翻译成查询。
//
// 它是接口而非解析代码：请求方身份与租户必须同时核对，运营接入面的认证方式属
// `PAR-INT-01` 待提供；采信自报租户会穿透 ADR-0003 的隔离边界（ADR-0077 Decision
// 三）。未决期间本包不带任何采信实现，包括「开发用」的采信头部版本。
type CatalogueQueryIntake interface {
	IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error)
}

func writeCatalogueIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

// 查询端点的门次序照 settlementhttp：方法 → 册名 → 准入。册名不认识是请求形状的事，
// 判它不需要先知道调用方是谁。两个助手交回 false 时答复已写出，调用方直接返回。

func guardGet(response http.ResponseWriter, request *http.Request) bool {
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
		return false
	}
	return true
}

func intakeCatalogueQuery(
	response http.ResponseWriter,
	request *http.Request,
	intake CatalogueQueryIntake,
) (CatalogueQuery, bool) {
	query, err := intake.IntakeCatalogueQuery(request.Context(), request)
	if err != nil {
		writeCatalogueIntakeProblem(response, err)
		return CatalogueQuery{}, false
	}
	return query, true
}

func rfc3339(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

// optionalInstant 把可空时刻转写成串，未发生时交回空串配 omitempty 整键不出现。
// 缺席是真话不是缺陷：不为「未转出」「未封装」「未关闭」编造一个零时刻——零时刻是
// 合法时间值，用它兼表没发生会让两态在报文上分不开。
func optionalInstant(at *time.Time) string {
	if at == nil {
		return ""
	}
	return rfc3339(*at)
}
