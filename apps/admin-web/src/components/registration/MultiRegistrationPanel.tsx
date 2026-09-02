import { useState } from 'react';
import { RegistrationPanel, type RegistrationPanelProps } from './RegistrationPanel';

/**
 * 一页装多本登记册时的「登记」签（ADR-0085，票 admin-write-faces/02 切片 02a/02d）。
 *
 * 计价那两页各只有一本册，直接摆 `RegistrationPanel` 即可；网络目录与 VE 目录的一页
 * 装着好几本册，各册的登记快照形状、端点与答案代数都不同，于是要先选册。选册这件事
 * 单独抽出来而不是每页各写一遍，是为了让**换册即换草稿**那条纪律只有一处：各册的快照
 * 形状互不相容，把上一册的草稿留在框里，下一次提交就会把一种册子的内容送进另一个
 * 端点，而服务端只会答一句说不清缘由的拒绝。
 *
 * 选册按钮不是「新建哪种对象」的菜单：登记册不可覆盖，更正翻旧插新、停用走状态推进，
 * 本签因此只有登记一个动作，没有行级编辑或删除面（判据同计价两页的登记签）。
 */

/** 一本册的登记面配置。除选册用的 `id`/`label` 外逐项透传给 `RegistrationPanel`。 */
export interface RegistrationTarget
  extends Omit<RegistrationPanelProps, 'moduleId' | 'problemNote'> {
  /** 选册键，取该上下文读面已有的族/种类原词——同一本册在读签与登记签同一个词。 */
  id: string;
  /** 选册按钮的中文，取该页读面已有的词表，不为登记签另造一套说法。 */
  label: string;
}

export interface MultiRegistrationPanelProps {
  moduleId: string;
  targets: RegistrationTarget[];
  /** problem+json 错误码的中文说明：一个上下文一份，各册共用。 */
  problemNote: (code: string) => string;
}

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

export function MultiRegistrationPanel({
  moduleId,
  targets,
  problemNote,
}: MultiRegistrationPanelProps) {
  const [selectedId, setSelectedId] = useState(targets[0].id);
  const selected = targets.find((target) => target.id === selectedId) ?? targets[0];

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="flex flex-wrap items-center gap-2 px-4 pt-4">
        {targets.map((target) => (
          <button
            key={target.id}
            type="button"
            className={chipClass(target.id === selected.id)}
            onClick={() => setSelectedId(target.id)}
          >
            {target.label}
          </button>
        ))}
      </div>
      <RegistrationPanel
        // key 按册取值以强制重挂：草稿是 RegistrationPanel 的内部状态，不换 key 就会
        // 跨册留存，而各册的快照形状互不相容。宁可换册清草稿，也不让一册的内容送进
        // 另一册的端点。
        key={selected.id}
        moduleId={moduleId}
        title={selected.title}
        endpoint={selected.endpoint}
        snapshotHint={selected.snapshotHint}
        submit={selected.submit}
        outcomeLabels={selected.outcomeLabels}
        refusalReasonLabels={selected.refusalReasonLabels}
        declarationLandingLabels={selected.declarationLandingLabels}
        problemNote={problemNote}
      />
    </div>
  );
}
