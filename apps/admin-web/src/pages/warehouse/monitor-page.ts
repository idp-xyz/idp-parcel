// 仓库监控翻目录读口的游标。条件一变就回到第一页；上一页只退自己走过的游标，不另问一份「上一页」的号。

export interface MonitorCursor {
  after: string | null;
  trail: readonly (string | null)[];
}

export function monitorCursorStart(): MonitorCursor {
  return { after: null, trail: [null] };
}

export function monitorCursorNext(cursor: MonitorCursor, next: string): MonitorCursor {
  return { after: next, trail: [...cursor.trail, next] };
}

export function monitorCursorPrevious(cursor: MonitorCursor): MonitorCursor {
  if (cursor.trail.length <= 1) return cursor;
  const trail = cursor.trail.slice(0, -1);
  return { after: trail[trail.length - 1] ?? null, trail };
}

export function monitorCursorCanPrevious(cursor: MonitorCursor): boolean {
  return cursor.trail.length > 1;
}
