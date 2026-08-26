// 追踪与异常页的对外出口,供装配侧接导航。全程追踪页已接线(对 GET
// /tracking-projections 取数,渠道未登记前如实渲染 403 未配置态);异常案件与索赔
// 追偿两页仍以「未配置」态呈现真实骨架:栏目按 visibility-exception CONTEXT 语义
// 搭好,数据区如实答未接线,不含合成数据。异常分诊队列在 governance 的
// ExceptionTriagePage,不在本目录。
//
// 六类规则与策略目录页(对 GET /visibility-catalogues 取数,票
// admin-web-page-wiring-frontier/02)也从这里出:页面归导航主数据区,文件归本目录
// ——目录的主责上下文是 visibility-exception,呈现分组不改变所有权。
export { TrackingProjectionPage } from './TrackingProjectionPage';
export { ExceptionCasesPage } from './ExceptionCasesPage';
export { ClaimsRecoveryPage } from './ClaimsRecoveryPage';
export { TrackingJudgmentRulesPage } from './TrackingJudgmentRulesPage';
export { DisclosurePoliciesPage } from './DisclosurePoliciesPage';
export { ClaimPrerequisitesPage } from './ClaimPrerequisitesPage';
export { configureVisibilityApi } from './api';
