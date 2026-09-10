// 本目录的对外出口,供装配侧接导航用。ShipmentRequestPage 是提交/撤回单导航位的
// 组合入口;各页面组件也单独导出,导航位怎么拆由装配侧定,不必进目录内部找。
export { ShipmentRequestPage } from './ShipmentRequestPage';
export { SubmitShipmentRequestPage } from './SubmitShipmentRequestPage';
export { WithdrawShipmentRequestPage } from './WithdrawShipmentRequestPage';
// 委托查阅面:列表页内含钻取到详情的组合,可独立作一个导航位;详情页也单独导出。
// 查阅面的行/详情形状是查询契约的镜像,住 api.ts,从下方 api 类型块一并导出。
export { ShipmentRequestListPage } from './ShipmentRequestListPage';
export { ShipmentRequestDetailPage } from './ShipmentRequestDetailPage';
// UC-PS-006 接受后取消入口(已接线:逐件分发到 POST /shipment-requests/parcel-cancellations)。
export { CancelParcelPage } from './CancelParcelPage';
// 面单交易查阅面(行粒度交易×包裹,已接线:GET /label-transactions)。行形状是查询契约的
// 镜像,与其余查阅面同住 api.ts,从下方 api 类型块一并导出。
export { LabelTransactionsPage } from './LabelTransactionsPage';
// 接受前人工复核工作流(队列与单案读 GET /acceptance-review-queue,复核完成与拒绝两个命令)。
// 曾落在 pages/governance/,现已归位本目录——主责上下文是小包托运,读写面与其余页同住 api.ts。
export { AcceptanceReviewPage } from './AcceptanceReviewPage';
// 授权处置工作流(队列读 GET /authorized-disposition-queue,单份复用复核队列分支,两去向打
// POST /shipment-requests/authorized-dispositions;票 sa-preacceptance-policy-view/05,ADR-0132)。
export { AuthorizedDispositionPage } from './AuthorizedDispositionPage';
export { configureShipmentRequestApi } from './api';
export type {
  ApiResult,
  DeclaredParcelDraft,
  ProductionOwnershipView,
  ShipmentRequestDraft,
  SubmitOutcome,
  SubmitResponseBody,
  WithdrawalDraft,
  WithdrawalOutcome,
  WithdrawalResponseBody,
  CancellationDraft,
  CancellationOutcome,
  CancellationResponseBody,
  ShipmentRequestSummary,
  ShipmentRequestDetail,
  DeclaredParcelRecord,
  ParcelDimensions,
  AcceptanceTaskRecord,
  DecisionRecord,
  ViewsListResponseBody,
  ViewDetailResponseBody,
  LabelTransactionRow,
  LabelTransactionsListResponseBody,
} from './api';
