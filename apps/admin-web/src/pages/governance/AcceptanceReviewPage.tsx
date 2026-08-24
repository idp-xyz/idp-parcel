import { useState } from 'react';
import { ReviewFlowTemplate } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['acceptance-review'];

/**
 * 接受前人工复核。队列语义取 parcel-shipment CONTEXT.md 的「等待人工复核」态：
 * 只有适用规则显式要求人工业务判断时，接受判断任务才进入该态，续办方是授权复核角色。
 *
 * 动作命名为「复核通过 / 复核不通过」而不是「接受 / 拒绝」——CONTEXT.md 明确
 * 「复核完成本身不形成决定，决定仍由判断任务按适用规则形成」；把按钮叫「接受」
 * 会让复核角色以为自己在替判断任务作决定。
 */
export function AcceptanceReviewPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  return (
    <ReviewFlowTemplate
      title={info.title}
      description={info.owner}
      queueTitle="等待人工复核"
      // 队列、详情字段与审计留痕接线时由 parcel-shipment 应用端口供数。
      // 详情届时至少呈现判断任务保存的：采用版本、当前阶段、已形成结果、仍缺权威结果
      // （字段清单出处 parcel-shipment CONTEXT.md 接受判断任务）。
      queue={[]}
      selectedId={selectedId}
      onSelect={setSelectedId}
      approveLabel="复核通过"
      rejectLabel="复核不通过"
      // unconfigured 态不渲染决定区，本回调当前不可达；接线时替换为应用端口调用，
      // 理由必填的门槛已由模板在提交前守住。
      onDecide={() => {}}
      viewState={{
        kind: 'unconfigured',
        title: '治理模块尚未接线',
        description: `业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
