import { DetailPageTemplate } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['stage-admission'];

/**
 * 阶段决定与暂停恢复。选 DetailPageTemplate 而不是 ReviewFlowTemplate：
 * 阶段决定不是从队列里逐条消化的复核件——同一时刻只有一个「当前阶段」，
 * 治理者看的是这一个对象的证据与历史，然后作 Go / No-Go / 暂停 / 恢复决定。
 * 队列形态会暗示「处理完这条还有下一条」，与 PN-08 的阶段语义相悖。
 *
 * 接线时基本信息区至少呈现：当前阶段、生效阶段决定、暂停状态；
 * 业务区块呈现证据登记册引用与阶段历史；决定动作放 headerActions。
 * 字段与动作口径出处见 moduleInfoById 引用的 PN-08 交接文档，此处不复述。
 *
 * 未接线的原因写成「归属未定」而非「端点未放行」，是一次裁决的结果，不是措辞选择：
 * 治理登记册是产品级机制、无租户维（见 migrations/pilot_governance/0001 抬头），
 * 而本管理台的隔离读准入按租户放行——形状对不上不是工期问题。写「尚未接线」
 * 会让人以为在排期，那是假信息。裁决与重开判据见
 * .scratch/admin-web-page-wiring-frontier/issues/03。
 */
export function StageAdmissionPage() {
  return (
    <DetailPageTemplate
      title={info.title}
      description={info.owner}
      // unconfigured 态替换整个内容区，接线前不预设任何字段值。
      basicFields={[]}
      viewState={{
        kind: 'unconfigured',
        title: '本页数据的归属尚未裁定，不在本管理台接线',
        description:
          '试点治理登记的是产品级事实（本产品此刻拿什么去评审、谁在写生产），没有租户维；'
          + '本管理台的查阅面按租户隔离放行，两者形状对不上。本页不发请求、不填任何默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '待裁：这些产品级记录该由哪个承载面呈现（不默认是租户管理台）',
        },
      }}
    />
  );
}
