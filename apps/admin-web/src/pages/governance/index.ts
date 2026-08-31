// 治理页的对外出口，供装配侧接导航。栏目与动作按各自 CONTEXT 语义搭好，不含合成数据。
// 对账单与收付款核销两页已接真库读面（主责上下文是 settlement-accounting，读面类型在
// pages/settlement/api.ts）；其余各页仍以「未配置」态呈现真实骨架，数据区如实答未接线。
export { AcceptanceReviewPage } from './AcceptanceReviewPage';
export { ExceptionTriagePage } from './ExceptionTriagePage';
export { ReconciliationPage } from './ReconciliationPage';
export { SettlementApplicationPage } from './SettlementApplicationPage';
export { StageAdmissionPage } from './StageAdmissionPage';
