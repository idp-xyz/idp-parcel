import type { ReactNode } from 'react';

// 跨模板共享的展示形状。只放两个以上模板都要的；单模板专属类型留在各自文件，
// 避免这里长成新的「公共类型堆」。

/** 详情页「基本信息」区的一条字段。value 收 ReactNode 是为了让调用方放徽章或链接。 */
export interface DetailField {
  label: string;
  value: ReactNode;
}

/**
 * 审计留痕的一条记录。形状对齐 ui-primitives Timeline 的条目，
 * 模板直接透传渲染，不在展示层加工审计语义——留痕内容归后端事实。
 */
export interface AuditEntry {
  id: string;
  /** 动作名（如「提交复核决定」）。 */
  title: string;
  /** 操作者、依据等补充说明。 */
  description?: string;
  /** 已格式化的展示时间；格式化归调用方，模板不掺时区决策。 */
  timestamp?: string;
  variant?: 'default' | 'success' | 'warning' | 'error';
}
