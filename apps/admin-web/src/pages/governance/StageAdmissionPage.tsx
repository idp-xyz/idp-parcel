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
        title: '治理模块尚未接线',
        description: '业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '对应查询与决定端点经 ADR-0017 准入闸门放行后接线',
        },
      }}
    />
  );
}
