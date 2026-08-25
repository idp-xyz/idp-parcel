// 追踪与异常页的对外出口,供装配侧接导航。全程追踪页已接线(对 GET
// /tracking-projections 取数,渠道未登记前如实渲染 403 未配置态);其余两页仍以
// 「未配置」态呈现真实骨架:栏目按 visibility-exception CONTEXT 语义搭好,数据区
// 如实答未接线,不含合成数据。异常分诊队列在 governance 的 ExceptionTriagePage,
// 不在本目录。
export { TrackingProjectionPage } from './TrackingProjectionPage';
export { ExceptionCasesPage } from './ExceptionCasesPage';
export { ClaimsRecoveryPage } from './ClaimsRecoveryPage';
export { configureVisibilityApi } from './api';
