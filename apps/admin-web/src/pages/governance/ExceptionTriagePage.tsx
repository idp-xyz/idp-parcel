import { useState } from 'react';
import { Ban, FolderPlus, Link2, Search } from 'lucide-react';
import { ReviewFlowTemplate, type ReviewDecisionOption } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['exception-triage'];

/** 分诊结果的决定 id。四结果名单出自 visibility-exception CONTEXT.md「异常分诊」定义。 */
type TriageDecisionId =
  | 'link-existing-case'
  | 'auto-establish-case'
  | 'route-manual-review'
  | 'no-case';

/**
 * 分诊决定集。标签逐字取 CONTEXT.md「异常分诊」的四结果原词，不自造变体、
 * 不折叠为二元「建立案件 / 不建案」。
 *
 * 「关联既有案件」需要目标案件选择交互；既有案件查询端点尚未建立，该决定
 * 如实呈现为不可用并注明依赖——不用假案件列表撑起选择器（零合成数据红线）。
 * 端点就绪后随接线一并开放，选择交互届时再定形状。
 */
const triageDecisions: ReviewDecisionOption<TriageDecisionId>[] = [
  {
    id: 'link-existing-case',
    label: '关联既有案件',
    variant: 'secondary',
    icon: <Link2 size={13} />,
    disabled: true,
    disabledReason: '目标案件选择依赖既有案件查询端点；端点未建，就绪后随接线一并开放。',
  },
  {
    id: 'auto-establish-case',
    label: '自动建立案件',
    variant: 'default',
    icon: <FolderPlus size={13} />,
  },
  {
    id: 'route-manual-review',
    label: '进入人工复核',
    variant: 'secondary',
    icon: <Search size={13} />,
  },
  {
    id: 'no-case',
    label: '不建案',
    variant: 'danger',
    icon: <Ban size={13} />,
  },
];

/**
 * 异常分诊与处置协调。队列语义取 visibility-exception CONTEXT.md「异常识别、分诊与分级」：
 * 低可信、资料不足、可能重复或关联不明确的信号必须先进入分诊，本页即该分诊队列；
 * 决定集为「异常分诊」定义的四结果，其中「进入人工复核」把信号转给更深的人工复核，
 * 不在本队列内自我循环。
 */
export function ExceptionTriagePage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  return (
    <ReviewFlowTemplate
      title={info.title}
      description={info.owner}
      queueTitle="进入分诊的信号"
      // 队列与详情接线时由 visibility-exception 应用端口供数。详情届时至少呈现
      // 信号保存的：对象、类型、规则版本、判断时间、事实依据、可信度、当前发作期
      // （字段清单出处 visibility-exception CONTEXT.md「异常识别、分诊与分级」）。
      queue={[]}
      selectedId={selectedId}
      onSelect={setSelectedId}
      decisions={triageDecisions}
      // unconfigured 态不渲染决定区，本回调当前不可达；接线时替换为应用端口调用，
      // 理由必填的门槛已由模板在提交前守住。
      onDecide={() => {}}
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
