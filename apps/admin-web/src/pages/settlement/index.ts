// 结算与核算页的对外出口，供装配侧接导航。两页均已接真库读面，栏目取 settlement-accounting
// CONTEXT 原词；登记册当前为空，空态如实说「读取入口已配置、登记册为空」，不含合成数据。
// 对账单与收付款核销两页历史上落在 pages/governance/，不迁不重复导出；但四页同属本上下文，
// 读面类型统一放本目录 api.ts，那两页从此处引入。
export { ChargesBillingPage } from './ChargesBillingPage';
export { OperatingMetricsPage } from './OperatingMetricsPage';
