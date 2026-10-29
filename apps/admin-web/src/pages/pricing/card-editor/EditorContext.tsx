import { createContext, useContext } from 'react';
import type { PriceCardTemplateSpec } from '../api';
import type { CardFormState } from '../card-form/model';
import type { EditorEnv } from './use-editor-env';

export interface FieldError {
  where: string;
  text: string;
  /** 夹在服务端话里的英文原文，折叠显示。 */
  original?: string;
}

export interface EditorContextValue {
  state: CardFormState;
  /** 改状态：传一个从旧状态算新状态的函数。 */
  update: (change: (draft: CardFormState) => CardFormState) => void;
  spec: PriceCardTemplateSpec;
  env: EditorEnv;
  /** 控件定位串 → 这个控件上的问题（本地校验与预览返回的）。 */
  errors: ReadonlyMap<string, FieldError[]>;
}

export const EditorContext = createContext<EditorContextValue | null>(null);

export function useEditor(): EditorContextValue {
  const value = useContext(EditorContext);
  if (value === null) throw new Error('useEditor 要在价卡编辑器内使用');
  return value;
}
