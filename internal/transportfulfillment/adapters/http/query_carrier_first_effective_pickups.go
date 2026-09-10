package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// CarrierPickupChainReader 是本端点消费的读口：一个载运对象的实际承运商首次有效收寄整条链（label-channel/31）。
type CarrierPickupChainReader interface {
	ListByObject(ctx context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) ([]ports.CarrierFirstEffectivePickupRecord, error)
}

// 编译期锁缝：读口形状与登记册端口保持一致——本端点不新造查询语义。
var _ CarrierPickupChainReader = ports.CarrierFirstEffectivePickupRegistry(nil)

const outcomeCarrierPickupChainListed = "CARRIER_FIRST_EFFECTIVE_PICKUP_CHAIN_LISTED"

// NewQueryCarrierFirstEffectivePickupsEndpoint 交回收寄链查阅的 HTTP 入口
// （GET /transport-fulfillment-carrier-first-effective-pickups?object=…）。
//
// 它是判断面的读半边：判断人看这个对象的链走到哪一版、凭什么依据，再到写口判下一条。查阅零登记零编辑零披露，
// 消费本上下文自己的存储读面，走目录查阅那条准入（CatalogueQueryIntake，ADR-0077/0078）；写半边在
// judge_carrier_first_effective_pickup.go，挂命令面的准入，两边的 Intake 不可互换。
//
// 对象是必备维：一对象一链，缺席不猜「全部对象」。没有链的对象如实交回空列表。
func NewQueryCarrierFirstEffectivePickupsEndpoint(intake CatalogueQueryIntake, reader CarrierPickupChainReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		object, err := domain.NewCarriedObjectReference(request.URL.Query().Get("object"))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		records, err := reader.ListByObject(request.Context(), query.Scope.Tenant(), object)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]carrierPickupBody, 0, len(records))
		for _, record := range records {
			bodies = append(bodies, carrierPickupBodyOf(record))
		}
		writeJSON(response, http.StatusOK, carrierPickupChainResponse{Outcome: outcomeCarrierPickupChainListed, Pickups: bodies})
	})
}

type carrierPickupChainResponse struct {
	Outcome string              `json:"outcome"`
	Pickups []carrierPickupBody `json:"pickups"`
}
