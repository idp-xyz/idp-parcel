// 试点治理页的对外出口，供装配侧接导航。本目录只装主责上下文为 pilotgovernance 的页；
// 曾与它同放在这里的接受前人工复核、异常分诊、对账单、收付款核销已各归所属上下文目录
// （shipment-request / visibility / settlement），目录从此按上下文分而不按「治理与复核」这个
// 呈现分组分。阶段决定页接 GET /governance-registers 取数，接入渠道未配置时如实呈现「未配置」
// 态，不含合成数据。
export { StageAdmissionPage } from './StageAdmissionPage';
