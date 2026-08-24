import { StatusBadge, type StatusBadgeProps } from '@idpxyz/ui-patterns';

/**
 * 状态徽标的纯数据描述。模板的数据入参一律不收 ReactNode 徽标，
 * 而收这个形状——让演示数据（demo.ts）与未来的接线数据都能保持纯数据，
 * 渲染成 StatusBadge 是模板自己的事。
 */
export interface TemplateStatus {
  label: string;
  /** 取 ui-patterns StatusBadge 的语气枚举。 */
  tone: NonNullable<StatusBadgeProps['status']>;
}

/** 把纯数据状态渲染成 StatusBadge。 */
export function TemplateStatusBadge({ status }: { status: TemplateStatus }) {
  return <StatusBadge status={status.tone}>{status.label}</StatusBadge>;
}

/**
 * 审计留痕条目。复核决定、状态变更等人工/系统动作的展示形状；
 * 只承载展示，不定义审计的持久化口径——那是后端上下文的所有权。
 */
export interface AuditEntry {
  id: string;
  /** 展示用时间字符串，格式由供数方决定，模板不做时区换算。 */
  time: string;
  /** 动作者（人名/系统名）。 */
  actor: string;
  /** 动作描述，用领域语言原词（如「批准」「驳回」「提交」）。 */
  action: string;
  /** 动作理由或补充说明，复核决定必有理由，此处展示它。 */
  detail?: string;
}

/**
 * 审计留痕区列表：时间 + 动作者 + 动作 + 理由。
 * 详情页「审计区」与复核工作流「审计留痕区」共用同一视觉。
 */
export function AuditTrailList({ entries }: { entries: AuditEntry[] }) {
  if (entries.length === 0) {
    return <p className="text-[12px] text-idpxyz-textMuted py-2">暂无审计记录。</p>;
  }
  return (
    <div>
      {entries.map((entry) => (
        <div
          key={entry.id}
          className="flex items-start gap-3 py-2 text-[12px] border-b border-idpxyz-border last:border-0"
        >
          <span className="text-idpxyz-textMuted font-mono shrink-0 text-[11px] mt-0.5">
            {entry.time}
          </span>
          <div className="flex-1 min-w-0">
            <span className="text-idpxyz-accent mr-1.5">{entry.actor}</span>
            <span className="text-idpxyz-text">{entry.action}</span>
            {entry.detail && (
              <p className="text-[11px] text-idpxyz-textMuted mt-0.5 break-all">{entry.detail}</p>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
