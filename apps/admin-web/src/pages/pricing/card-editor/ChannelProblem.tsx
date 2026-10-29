import { Button } from '@idpxyz/ui-primitives';
import { ReloginButton } from '../../../auth/ReloginButton';
import type { ApiResult } from '../../catalogue-api';
import { problemNote } from '../presentation';

/** 没拿到业务答复时说人话：接入没配置、没登录或没授予、请求没成、暂时没答出来，各说各的，不画成「没有数据」。 */
export function ChannelProblem({ answer, what, onRetry }: { answer: ApiResult<unknown>; what: string; onRetry?: () => void }) {
  const retry = onRetry ? (
    <Button variant="outline" size="sm" className="mt-2" onClick={onRetry}>
      重试
    </Button>
  ) : null;
  switch (answer.kind) {
    case 'outcome':
      return null;
    case 'unconfigured':
      return (
        <div className="text-[12px] text-idpxyz-textMuted">
          <p>{what}暂时用不了：这个租户的操作入口还没有配置完。配置好之后刷新本页再试，重试和改内容都不会改变结果。</p>
        </div>
      );
    case 'loginRequired':
      return (
        <p className="text-[12px] text-idpxyz-textMuted">
          {what}需要先登录：登录已经过期或还没有登录。重新登录后再试。
          <ReloginButton size="inline" />
        </p>
      );
    case 'notGranted':
      return (
        <p className="text-[12px] text-idpxyz-textMuted">
          {what}被拒绝：当前账号没有维护价卡的权限。请联系管理员授权，换一个有权限的账号再试。
        </p>
      );
    case 'callerProblem':
      return (
        <div className="text-[12px] text-[color:var(--ds-status-danger-fg)]">
          <p>
            {what}没有成功：{problemNote(answer.code)}
            {answer.detail ? `（${answer.detail}）` : ''}
          </p>
        </div>
      );
    case 'noAnswer':
      return (
        <div className="text-[12px] text-[color:var(--ds-status-danger-fg)]">
          <p>
            {what}暂时没有得到结果：{problemNote(answer.code)}。可以稍后重试。
          </p>
          {retry}
        </div>
      );
    case 'transport':
      return (
        <div className="text-[12px] text-[color:var(--ds-status-danger-fg)]">
          <p>
            {what}没有发出去：{answer.message}
          </p>
          {retry}
        </div>
      );
  }
}
