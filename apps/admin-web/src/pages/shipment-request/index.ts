// 本目录的对外出口,供装配侧接导航用。ShipmentRequestPage 是提交/撤回单导航位的
// 组合入口;各页面组件也单独导出,导航位怎么拆由装配侧定,不必进目录内部找。
export { ShipmentRequestPage } from './ShipmentRequestPage';
export { SubmitShipmentRequestPage } from './SubmitShipmentRequestPage';
export { WithdrawShipmentRequestPage } from './WithdrawShipmentRequestPage';
// 委托查阅面:列表页内含钻取到详情的组合,可独立作一个导航位;详情页也单独导出。
// 查阅面的行/详情形状是查询契约的镜像,住 api.ts,从下方 api 类型块一并导出。
export { ShipmentRequestListPage } from './ShipmentRequestListPage';
export { ShipmentRequestDetailPage } from './ShipmentRequestDetailPage';
// UC-PS-006 接受后取消入口(未配置骨架,端点形状落地前不受理请求)。
export { CancelParcelPage } from './CancelParcelPage';
// 面单交易查阅面(行粒度交易×包裹,查询端点未建为未配置骨架)。
export { LabelTransactionsPage, type LabelTransactionRow } from './LabelTransactionsPage';
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
  ShipmentRequestSummary,
  ShipmentRequestDetail,
  DeclaredParcelRecord,
  ParcelDimensions,
  AcceptanceTaskRecord,
  DecisionRecord,
  ViewsListResponseBody,
  ViewDetailResponseBody,
} from './api';
