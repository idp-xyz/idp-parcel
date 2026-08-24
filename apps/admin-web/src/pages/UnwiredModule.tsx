import { CircleDashed } from 'lucide-react';
import { moduleInfoById } from '../navigation';

// 未接线模块的统一占位页：讲清楚该模块归谁、依据什么文档、为何还没有数据。
// 不模拟列表或表单——那会把「看起来能用」误当成「已交付」。
export function UnwiredModule({ moduleId }: { moduleId: string }) {
  const info = moduleInfoById[moduleId];
  if (!info) return null;

  return (
    <div className="flex-1 flex items-center justify-center">
      <div className="max-w-[560px] px-6">
        <div className="flex items-center gap-2 mb-4">
          <CircleDashed className="h-5 w-5 text-idpxyz-textMuted" />
          <span className="text-[18px] text-idpxyz-textBright">{info.title}</span>
          <span className="text-[11px] px-2 py-0.5 rounded border border-idpxyz-border text-idpxyz-textMuted">
            未接线
          </span>
        </div>
        <dl className="text-[13px] leading-6">
          <div className="flex gap-2">
            <dt className="text-idpxyz-textMuted shrink-0">主责上下文</dt>
            <dd className="text-idpxyz-text">{info.owner}</dd>
          </div>
          <div className="flex gap-2">
            <dt className="text-idpxyz-textMuted shrink-0">场景出处</dt>
            <dd className="text-idpxyz-text break-all">{info.source}</dd>
          </div>
        </dl>
        <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-4">
          业务端点按 ADR-0017 的准入闸门尚未放行，本页为骨架占位：不发请求、
          不含任何未确认参数的默认值。闸门开启后由对应上下文的应用端口供数。
        </p>
      </div>
    </div>
  );
}
