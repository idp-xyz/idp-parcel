// 网络目录的界面词。族键与登记口 kind 同词（代码引用用），界面上说一线的叫法：节点叫网点，网络连接叫运输连接，
// 网络可用性调整叫停运与恢复（network-routing CONTEXT 的原词在代码与注释里）。

import type { NetworkCatalogFamily } from './api';

export const familyLabels: Record<NetworkCatalogFamily, string> = {
  node: '网点',
  connection: '运输连接',
  line: '线路',
  'service-area': '服务区域',
  'service-calendar': '服务日历',
  'availability-adjustment': '停运与恢复',
  'route-strategy': '路线策略',
};

// 检索框的提示:各族的 q 落在哪几格(ADR-0144 决定四:只在该册点名的文本列上做包含匹配),照 networkrouting postgres 读面
// 各族 catalogFamily 的 keyword 列写。只写那边点了名的格,不写「代码等」这种兜底——检索已下推,提示里多说一格,人就会去搜一格搜不到的。
export const keywordPlaceholders: Record<NetworkCatalogFamily, string> = {
  node: '搜网点编码',
  connection: '搜连接编码或两端网点',
  line: '搜线路编码',
  'service-area': '搜服务区域编码',
  'service-calendar': '搜网点、连接或线路编码',
  'availability-adjustment': '搜调整编码、对象编码或来源',
  'route-strategy': '搜策略编码',
};

/** 网络与网点页的六个签:服务区域另有专页,这里不露。 */
export const networkCatalogFamilies: NetworkCatalogFamily[] = [
  'node',
  'connection',
  'line',
  'service-calendar',
  'availability-adjustment',
  'route-strategy',
];

// ——以下为登记签的页面口径(ADR-0085)。标题说这个任务做什么,不叫「登记 ×× 版本」。

export const registrationTitles: Record<NetworkCatalogFamily, string> = {
  node: '新增或更新网点',
  connection: '新增或更新运输连接',
  line: '新增或更新线路',
  'service-area': '新增或更新服务区域',
  'service-calendar': '新增或更新服务日历',
  'availability-adjustment': '登记停运或恢复',
  'route-strategy': '新增或更新路线策略',
};

// 填写提示:只说填什么、怎么改。形状与受控登记命令 parcel-network-register -kind <族> 吃的同一份,这一句留给对接的人。
// 各族只差那个族词,所以由一处拼出:抄七遍会让同一句在其中一遍被改动时悄悄分叉。
function snapshotHint(family: NetworkCatalogFamily, fields: string): string {
  return (
    `请粘贴一份 JSON，${fields}` +
    '要改一条已有的，就用同一个编码登一个更大的版本号、填新的生效时间，旧版到那时自动截止；已登的版本不会被覆盖。' +
    `（格式与命令行 parcel-network-register -kind ${family} 用的同一份。）`
  );
}

export const registrationSnapshotHints: Record<NetworkCatalogFamily, string> = {
  node: snapshotHint('node', '写明网点编码、版本号、所在时区（如 Asia/Shanghai）和从何时起生效。'),
  connection: snapshotHint('connection', '写明连接编码、版本号、出发网点与到达网点（两端不能相同）、时区和从何时起生效；编码里不要带「+」。'),
  line: snapshotHint('line', '写明线路编码、版本号、按顺序排列的运输连接、适用服务、时区和从何时起生效。'),
  'service-area': snapshotHint('service-area', '写明区域编码、版本号和从何时起生效；可以带覆盖的国家（两位代码）、邮编前缀，以及揽收、派送网点。'),
  'service-calendar': snapshotHint(
    'service-calendar',
    '写明适用对象（NODE 网点 / CONNECTION 运输连接 / LINE 线路）与它的编码、版本号、截单时间与各段时长，以及从何时起生效。',
  ),
  'availability-adjustment': snapshotHint(
    'availability-adjustment',
    '写明调整编码、版本号、适用对象（NODE 网点 / CONNECTION 运输连接 / LINE 线路）与编码、调整种类（SUSPENSION 临时停运 / CLOSURE 关闭 / RESUMPTION 恢复 / SCOPE_ADJUSTMENT 范围调整）、消息来源和开始时间；已知何时解除可一并填上。',
  ),
  'route-strategy': snapshotHint('route-strategy', '写明策略编码、版本号、适用服务、排序方式（COST_SINGLE_DIMENSION 成本最低优先，可不填）和从何时起生效。'),
};

// ——以下为路由判断两册的词表(票 admin-skeleton-closure-batch/03 阶段二)。
// 库面原词见 migrations/network_routing/0002 与 0005 的 CHECK;中文取 CONTEXT 原词。

/** register 查询参数封闭两册,词与查阅口、库表同(端点纪律:同一册同一个词)。 */
export type RoutePlanRegister = 'initial-route' | 'reassessment';

export const routePlanRegisters: RoutePlanRegister[] = ['initial-route', 'reassessment'];

export const routePlanRegisterLabels: Record<RoutePlanRegister, string> = {
  'initial-route': '初始路由判断',
  reassessment: '路由复核',
};

// 初始判断结论封闭两格。「无当前有效路由」是明确判断而非空行(CONTEXT 硬句),
// 词表给它完整的一格,不折进成计划行。
export const initialRouteConclusionLabels: Record<string, string> = {
  ROUTE_FORMED: '已成计划',
  NO_CURRENT_ROUTE: '无当前有效路由',
};

// 计划适用性封闭四态,中文取 CONTEXT 生命周期原词(当前有效/已被替代/已失效/已结束)。
export const applicabilityStateLabels: Record<string, string> = {
  CURRENTLY_EFFECTIVE: '当前有效',
  SUPERSEDED: '已被替代',
  LAPSED: '已失效',
  CONCLUDED: '已结束',
};

// 复核走向封闭四格。
export const reassessmentConclusionLabels: Record<string, string> = {
  STILL_APPLICABLE: '仍适用',
  PLAN_LAPSED: '计划失效',
  FIRST_PLAN_FORMED: '首次成计划',
  REROUTED: '已改路',
};

// 候选评估封闭三态;缺席(NULL)表示本走向不评估,缺席的呈现归页面,词表不设假格。
export const candidateStateLabels: Record<string, string> = {
  CANDIDATES_AVAILABLE: '有可用候选',
  NO_QUALIFIED_CANDIDATES: '无合格候选',
  CANDIDATE_REVIEW_UNDECIDED: '候选评审未决',
};

// 改路判定封闭三态;缺席(NULL)表示没评估过改路。
export const rerouteStateLabels: Record<string, string> = {
  AUTOMATIC_ALLOWED: '允许自动改路',
  SUGGESTION_ONLY: '仅出建议',
  BARRED: '禁止改路',
};

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。',
  MALFORMED_REQUEST: '这次查询或提交的内容系统读不懂；请刷新页面后重试，仍然不行请联系系统管理员。',
  INTAKE_FAILED: '登录信息没能核对通过，这次什么都没有保存；请稍后重试。',
  NO_ANSWER_FORMED: '系统暂时没能处理，结果还不确定；请稍后刷新查看，必要时重新提交同一份。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? `这次没有成功（错误码 ${code}）；请记下错误码联系系统管理员。`;
}

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}
