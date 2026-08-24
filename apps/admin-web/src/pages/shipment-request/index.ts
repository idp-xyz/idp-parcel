// 本目录的对外出口,供装配侧接导航用。ShipmentRequestPage 是单导航位的组合入口;
// 两个分页组件也单独导出,想拆成两个导航位时不必进目录内部找。
export { ShipmentRequestPage } from './ShipmentRequestPage';
export { SubmitShipmentRequestPage } from './SubmitShipmentRequestPage';
export { WithdrawShipmentRequestPage } from './WithdrawShipmentRequestPage';
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
} from './api';
