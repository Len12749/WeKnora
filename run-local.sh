#!/bin/bash
# 本机启动 WeKnora Lite（lite 版不自动读 .env 文件，需先注入环境变量）
set -e
cd "$(dirname "$0")"
set -a
. ./.env.lite
set +a
mkdir -p logs
exec ./WeKnora-lite >> logs/local.log 2>&1
