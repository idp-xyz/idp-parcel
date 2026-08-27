#!/usr/bin/env bash
# 第二十七轮全绿戳：真库全量 -race，锚提交写在日志首行。跑法与 r26 同源（WSL go + cgo，
# Windows 侧无 gcc 跑不了 -race）；日志格式沿用 r26 的 SHA/CMD/GO/START…EXIT/END 头尾。
set -u
cd /mnt/d/tops/idp-parcel || exit 1
export IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable'
LOG=.scratch/mechanism-reinventory-r27/raw/race-run-2026-08-27-f6f0029.log
{
  echo "SHA:$(git rev-parse --short HEAD)"
  echo "CMD:go test -race -p 1 -count=1 ./..."
  echo "GO:$(go version)"
  echo "START:$(date -Iseconds)"
} > "$LOG"
go test -race -p 1 -count=1 ./... >> "$LOG" 2>&1
echo "EXIT:$?" >> "$LOG"
echo "END:$(date -Iseconds)" >> "$LOG"
