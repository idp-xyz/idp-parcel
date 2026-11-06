package shipmenthttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件按端点逐个证 ADR-0144 迁移后的传输形状（票 catalogue-read-pagination-rollout/02，spec 取舍 8）：答复带 page 且 size 取
// Intake 给的页大小；配置了的渠道上集外参数答 400 带理由散文；渠道未配置时同一个坏请求答 403——解码在 Intake 之后，答复不随请求
// 内容变（ADR-0055 决定三）。各册怎么取页由 postgres 读面的用例证，这里的读口一律是答一页空行的替身。

type postIntakeRequestIntakeDouble struct{ limit int }

func (double postIntakeRequestIntakeDouble) IntakePostIntakeDispositionRequestQuery(context.Context, *http.Request) (shipmenthttp.PostIntakeDispositionRequestQuery, error) {
	return shipmenthttp.PostIntakeDispositionRequestQuery{Tenant: domain.TenantID{}, Limit: double.limit}, nil
}

type postIntakeRequestReaderDouble struct{}

func (postIntakeRequestReaderDouble) ListUnansweredPostIntakeDispositionRequests(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.UnansweredPostIntakeDispositionRequests, error) {
	return ports.UnansweredPostIntakeDispositionRequests{}, nil
}

type manualItemIntakeDouble struct{ limit int }

func (double manualItemIntakeDouble) IntakePostIntakeManualItemQuery(context.Context, *http.Request) (shipmenthttp.PostIntakeManualItemQuery, error) {
	return shipmenthttp.PostIntakeManualItemQuery{Tenant: domain.TenantID{}, Limit: double.limit}, nil
}

type manualItemReaderDouble struct{}

func (manualItemReaderDouble) ListPendingPostIntakeManualItems(context.Context, domain.TenantID, int, cataloguepage.Query) (ports.PendingPostIntakeManualItems, error) {
	return ports.PendingPostIntakeManualItems{}, nil
}

func (manualItemReaderDouble) ListPostIntakeManualItemsOfParcel(context.Context, domain.TenantID, domain.DeclaredParcelID) ([]ports.PostIntakeManualItem, error) {
	return nil, nil
}

type pagedEndpoint struct {
	configured, unconfigured http.Handler
	// good 是一问合规的列表请求（选择器照端点要求带上），bad 在它上面多一个集外参数。
	good, bad string
	size      string
}

func pagedEndpoints(t *testing.T) map[string]pagedEndpoint {
	t.Helper()
	tenant := mustValue(t, domain.NewTenantID, "TENANT-1")
	return map[string]pagedEndpoint{
		"面单交易": {
			configured:   shipmenthttp.NewQueryLabelTransactionsEndpoint(labelTransactionIntake(t), &labelTransactionsReaderDouble{}),
			unconfigured: shipmenthttp.NewQueryLabelTransactionsEndpoint(shipmenthttp.UnconfiguredIntake{}, &labelTransactionsReaderDouble{}),
			good:         "/label-transactions", bad: "/label-transactions?sort=parcelId", size: "50",
		},
		"委托查阅": {
			configured:   shipmenthttp.NewQueryShipmentRequestViewsEndpoint(&viewsIntakeDouble{}, &viewsReaderDouble{}),
			unconfigured: shipmenthttp.NewQueryShipmentRequestViewsEndpoint(shipmenthttp.UnconfiguredIntake{}, &viewsReaderDouble{}),
			good:         "/shipment-request-views", bad: "/shipment-request-views?state=ARCHIVED", size: "50",
		},
		"复核队列": {
			configured:   shipmenthttp.NewQueryAcceptanceReviewQueueEndpoint(&viewsIntakeDouble{}, &reviewQueueReaderDouble{}, &judgmentsReaderDouble{}),
			unconfigured: shipmenthttp.NewQueryAcceptanceReviewQueueEndpoint(shipmenthttp.UnconfiguredIntake{}, &reviewQueueReaderDouble{}, &judgmentsReaderDouble{}),
			good:         "/acceptance-review-queue", bad: "/acceptance-review-queue?state=SUBMITTED", size: "50",
		},
		"授权处置队列": {
			configured:   shipmenthttp.NewQueryAuthorizedDispositionQueueEndpoint(&viewsIntakeDouble{}, &dispositionQueueReaderDouble{}),
			unconfigured: shipmenthttp.NewQueryAuthorizedDispositionQueueEndpoint(shipmenthttp.UnconfiguredIntake{}, &dispositionQueueReaderDouble{}),
			good:         "/authorized-disposition-queue", bad: "/authorized-disposition-queue?sort=decidedAt", size: "50",
		},
		"渠道择优并列冲突": {
			configured:   shipmenthttp.NewQueryChannelSelectionDecisionsEndpoint(channelSelectionIntake(t), &channelSelectionReaderDouble{}),
			unconfigured: shipmenthttp.NewQueryChannelSelectionDecisionsEndpoint(shipmenthttp.UnconfiguredIntake{}, &channelSelectionReaderDouble{}),
			good:         "/channel-selection-decisions?view=tied", bad: "/channel-selection-decisions?view=tied&conclusion=TIED", size: "50",
		},
		"释放未受理册": {
			configured:   shipmenthttp.NewQueryControlReleaseRefusalsEndpoint(refusalIntakeDouble{tenant: tenant, limit: 2}, &refusalCatalogueDouble{}),
			unconfigured: shipmenthttp.NewQueryControlReleaseRefusalsEndpoint(shipmenthttp.UnconfiguredIntake{}, &refusalCatalogueDouble{}),
			good:         "/control-release-refusals", bad: "/control-release-refusals?tenant=TENANT-9", size: "2",
		},
		"取消先后冲突册": {
			configured:   shipmenthttp.NewQueryCancellationOrderConflictsEndpoint(conflictIntakeDouble{tenant: tenant, limit: 2}, &conflictCatalogueDouble{}),
			unconfigured: shipmenthttp.NewQueryCancellationOrderConflictsEndpoint(shipmenthttp.UnconfiguredIntake{}, &conflictCatalogueDouble{}),
			good:         "/cancellation-order-conflicts", bad: "/cancellation-order-conflicts?boundaryKind=ARCHIVED", size: "2",
		},
		"等回应的处置请求": {
			configured:   shipmenthttp.NewQueryPostIntakeDispositionRequestsEndpoint(postIntakeRequestIntakeDouble{limit: 25}, postIntakeRequestReaderDouble{}),
			unconfigured: shipmenthttp.NewQueryPostIntakeDispositionRequestsEndpoint(shipmenthttp.UnconfiguredIntake{}, postIntakeRequestReaderDouble{}),
			good:         "/post-intake-disposition-requests", bad: "/post-intake-disposition-requests?sort=requestKey", size: "25",
		},
		"待人工处理项": {
			configured:   shipmenthttp.NewQueryPostIntakeManualItemsEndpoint(manualItemIntakeDouble{limit: 25}, manualItemReaderDouble{}),
			unconfigured: shipmenthttp.NewQueryPostIntakeManualItemsEndpoint(shipmenthttp.UnconfiguredIntake{}, manualItemReaderDouble{}),
			good:         "/post-intake-disposition-manual-items", bad: "/post-intake-disposition-manual-items?owner=SOMEONE", size: "25",
		},
	}
}

func serve(handler http.Handler, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

// Covers: ADR-0144 决定四、五与 spec 取舍 8——九个端点各自：合规的一问带 page、size 取 Intake 的页大小；集外参数答 400 带理由散文；
// 渠道未配置时同一个坏请求答 403。
func TestEveryMigratedEndpointCarriesThePageAndDecodesAfterTheIntake(t *testing.T) {
	for name, endpoint := range pagedEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			good := serve(endpoint.configured, endpoint.good)
			if good.Code != http.StatusOK {
				t.Fatalf("合规的一问 status = %d；body = %s", good.Code, good.Body)
			}
			if size := string(pageOf(t, good)["size"]); size != endpoint.size {
				t.Fatalf("page.size = %s, want %s（取 Intake 的页大小）", size, endpoint.size)
			}
			assertPageQueryRefused(t, serve(endpoint.configured, endpoint.bad))
			if unconfigured := serve(endpoint.unconfigured, endpoint.bad); unconfigured.Code != http.StatusForbidden {
				t.Fatalf("渠道未配置时的坏请求 status = %d, want 403；body = %s", unconfigured.Code, unconfigured.Body)
			}
		})
	}
}

// Covers: 有界查找与单条分支在解码之前分派走（02 票「做法与判据」）：按运单号查委托、按包裹读回人工项都不带 page，也不因为带了
// 别的参数被当成目录查询拒掉。
func TestBoundedLookupsBypassTheCatalogueDecoding(t *testing.T) {
	views := serve(shipmenthttp.NewQueryShipmentRequestViewsEndpoint(&viewsIntakeDouble{}, &viewsReaderDouble{}),
		"/shipment-request-views?documentNumber=SYN-WB-1")
	if views.Code != http.StatusOK || containsKey(t, views, "page") {
		t.Fatalf("按运单号查 = %d %s，want 200 且不带 page", views.Code, views.Body)
	}
	items := serve(shipmenthttp.NewQueryPostIntakeManualItemsEndpoint(manualItemIntakeDouble{limit: 25}, manualItemReaderDouble{}),
		"/post-intake-disposition-manual-items?parcel=SYN-PARCEL-1")
	if items.Code != http.StatusOK || containsKey(t, items, "page") {
		t.Fatalf("按包裹读回 = %d %s，want 200 且不带 page", items.Code, items.Body)
	}
}
