import { useState } from 'react';
import { ReviewFlowTemplate } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['exception-triage'];

/**
 * 异常分诊与处置协调。队列语义取 visibility-exception CONTEXT.md「异常分诊」定义：
 * 低可信、资料不足、可能重复或关联不明确的信号必须先进入分诊，其中需要人工判断的
 * 进入人工复核；本页即该人工复核队列。
 *
 * 已知边界：CONTEXT.md 的分诊结果有四种（关联既有案件、自动建立案件、进入人工复核、
 * 不建案），而 ReviewFlowTemplate 的决定区是二元动作。这里先承载「建立案件 / 不建案」
 * 两个结果；「关联既有案件」在接线时需要扩展模板动作区或另定交互，
 * 不得把它硬塞进二元按钮。
 */
export function ExceptionTriagePage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  return (
    <ReviewFlowTemplate
      title={info.title}
      description={info.owner}
      queueTitle="进入人工复核的信号"
      // 队列与详情接线时由 visibility-exception 应用端口供数。详情届时至少呈现
      // 信号保存的：对象、类型、规则版本、判断时间、事实依据、可信度、当前发作期
      // （字段清单出处 visibility-exception CONTEXT.md「异常识别、分诊与分级」）。
      queue={[]}
      selectedId={selectedId}
      onSelect={setSelectedId}
      approveLabel="建立案件"
      rejectLabel="不建案"
      // unconfigured 态不渲染决定区，本回调当前不可达；接线时替换为应用端口调用。
      onDecide={() => {}}
      viewState={{
        kind: 'unconfigured',
        title: '治理模块尚未接线',
        description: `业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
