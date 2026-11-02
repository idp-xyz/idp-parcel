import { isOperatorRefusal } from '../../auth/access-refusal';
import { httpRows } from '../../components/TechnicalDetails';
import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import type { ApiResult } from '../catalogue-api';
import { exceptionProblemNote } from './case-presentation';

// 异常与索赔区各列表的取数答案 → 模板页面态，话按「说哪里出了事、下一步怎么办」写（照物流动态页的 viewStateOf）。
// 共享的 catalogueViewState 由多区共用、措辞不归本区改，本区四页改走这一个。
//
// 诚实态三格分开：读不到（接入没配置）、无权查看、出错各说各的，空册另走空态——都不合并成「无」。
// 筛出为空不是空态：viewState 按总数判，表格区另显无匹配的话。

export interface ExceptionViewOptions {
  module: ModuleInfo;
  /** 读口路径，只进技术详情。 */
  endpoint: string;
  /** 这页查的是什么（「异常案件」「客户索赔」），拼进错误标题。 */
  subject: string;
  emptyTitle: string;
  emptyDescription: string;
}

export function exceptionViewState<Body>(
  answer: ApiResult<Body> | null,
  rowCount: number,
  retry: () => void,
  options: ExceptionViewOptions,
): TemplateViewState {
  if (answer === null) return { kind: 'loading', shape: 'table' };
  if (answer.kind === 'outcome') {
    return rowCount === 0 ? { kind: 'empty', title: options.emptyTitle, description: options.emptyDescription } : { kind: 'ready' };
  }
  if (answer.kind === 'unconfigured') {
    return {
      kind: 'unconfigured',
      title: `${options.subject}还读不到`,
      description: `${options.subject}的数据通道还没有配置好，这不是「没有数据」：重试不会改变结果，需要先由系统管理员完成配置。`,
      facts: { owner: options.module.owner, source: options.module.source, endpoint: options.endpoint },
    };
  }
  if (isOperatorRefusal(answer)) {
    return { kind: answer.kind, facts: { owner: options.module.owner, source: options.module.source } };
  }
  if (answer.kind === 'callerProblem') {
    return {
      kind: 'error',
      title: `这次没有查到${options.subject}`,
      description: exceptionProblemNote(answer.code),
      technical: httpRows(answer.status, answer.code),
    };
  }
  if (answer.kind === 'noAnswer') {
    return {
      kind: 'error',
      title: `这次没有取到${options.subject}`,
      description: exceptionProblemNote(answer.code),
      onRetry: retry,
      technical: httpRows(answer.status, answer.code),
    };
  }
  return {
    kind: 'error',
    title: '连不上服务',
    description: '请求没有到达后台；请确认网络正常后重试。',
    onRetry: retry,
    technical: [{ label: '网络错误', value: answer.message }],
  };
}
