import { wallMomentOf, wallMomentProblem } from './publication-form-shared';
import { versionHintLines, versionHintOf, type KnownVersion } from './known-versions';

// 发布表单里「这个对象已有哪些版本」的提示（票 party-commercial-uiux/15），摆在版本信息一节里。规则与措辞在 known-versions.ts。
//
// `known` 为 null 即还在读或读不到：不提示，不报错——提示是帮人认的，读不到不该吵人。`startsAt` / `endsAt` 是草稿里日期时间
// 控件的本地墙钟；读面没有状态的册不给（`startsAt` 为 undefined），那几册不判重叠。

export function ExistingVersionsNote({
  known,
  objectId,
  version,
  startsAt,
  endsAt,
  timeZone,
}: {
  known: readonly KnownVersion[] | null;
  objectId: string;
  version: string;
  startsAt?: string;
  endsAt?: string;
  timeZone: string;
}) {
  if (known === null) return null;
  const hint = versionHintOf(known, {
    objectId,
    version,
    startsAt: utcOf(startsAt, timeZone),
    endsAt: utcOf(endsAt, timeZone),
  });
  if (hint === null) return null;
  const lines = versionHintLines(hint, objectId);
  if (lines.length === 0) return null;
  return (
    <div className="flex flex-col gap-1" aria-live="polite">
      {lines.map((line) => (
        <p
          key={line.text}
          className={`text-[12px] ${line.tone === 'warn' ? 'font-medium text-idpxyz-danger' : 'text-idpxyz-breadcrumb'}`}
        >
          {line.text}
        </p>
      ))}
    </div>
  );
}

/** 本地墙钟换成 UTC；没填、没给或换不成时刻都是 null（那一端当没有——起点没有就不判重叠，止点没有即持续有效）。 */
function utcOf(wall: string | undefined, timeZone: string): string | null {
  if (wall === undefined || wall.trim() === '' || wallMomentProblem(wall, timeZone) !== null) return null;
  return wallMomentOf(wall, timeZone);
}
