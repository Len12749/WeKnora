# 本机部署说明（非官方默认配置）

WeKnora Lite 0.8.3，单二进制 + SQLite，服务于 `知识库Harness` 的入库验证。

## 启动 / 停止

两个服务都由 launchd 托管，开机自启、崩溃自动拉起：

```bash
launchctl kickstart -k gui/$(id -u)/com.sen.kbharness.weknora   # 重启 WeKnora
launchctl bootout  gui/$(id -u)/com.sen.kbharness.weknora       # 停止
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.sen.kbharness.weknora.plist  # 启动
```

Ollama 同理，label 为 `com.sen.kbharness.ollama`（`bge-m3`，开 flash attention + q8_0 KV cache）。

**运行时不在本仓库**，在 `~/Library/Application Support/kbharness/weknora`（二进制 + `.env.lite` + `config/` + `web/` + `data/`）。
原因：launchd 进程没有「桌面」目录的 TCC 权限，直接跑本目录会报
`operation not permitted: .env.lite`，环境变量读不进来就 panic。

- UI / API：<http://127.0.0.1:8080>
- 账号：用户名 `kjlmadmin` / 登录邮箱 `kjlmadmin@local.test`（密码见 kjlm-weknora 的凭据文件）
- 数据：`~/Library/Application Support/kbharness/weknora/data/`（`weknora.db`、`files/`）
- 日志：`~/Library/Logs/kbharness/{weknora,ollama}.log`

本仓库下的 `data/` 是迁移前的旧快照，服务已不再使用，可确认后删除。

## 与官方默认的差异（踩过的坑）

0. **MCP `ask` 超时 4 分钟 → 8 分钟（本地补丁，`git diff internal/mcpserver/tools_ask.go`）**
   ReAct 循环下宽问题会跑十几轮检索，4 分钟常在答案流式中途被砍，返回
   `answer timed out: context deadline exceeded`（日志 `latency=4m0.0s`）。
   改 `internal/mcpserver/tools_ask.go` 的 `askTimeout = 8 * time.Minute`，
   `make build` 后替换 `~/Library/Application Support/kbharness/weknora/WeKnora-lite`
   并用 launchctl 重启；换机/升级时必须重打。

1. **`SSRF_WHITELIST_EXTRA`**
   VPN 是 TUN/fake-IP 模式，`api.deepseek.com` 会被解析到 `198.18.0.0/15`，
   WeKnora 的 SSRF 校验直接拒绝。同时本地 Ollama 的 `127.0.0.1` 也在黑名单里。
   ```
   SSRF_WHITELIST_EXTRA=api.deepseek.com,127.0.0.1,localhost
   ```

2. **云端模型必须用 `source=remote`**
   `CreateModel` 只把 `source=remote` 当远程模型；其他 source 一律走 Ollama 拉模型，
   状态会变成 `download_failed`，之后所有调用都被拒。
   接 DeepSeek 官方 API：
   ```
   type=KnowledgeQA, source=remote,
   parameters={base_url: https://api.deepseek.com/v1,
               provider: deepseek,
               interface_type: openai,
               api_key: sk-...}
   ```

3. **向量模型走本地 Ollama**
   DeepSeek 官方 API 没有 embedding 接口。本地拉 `bge-m3`（1024 维）：
   ```
   type=Embedding, source=local,
   parameters={base_url: http://127.0.0.1:11434,
               provider: ollama,
               embedding_parameters: {dimension: 1024}}
   ```
   注意 `source=local` 才会触发 Ollama 拉取并把状态置为 active。

## 已验证

- `/api/v1/chunker/preview`：切块预览可用，`strategy=auto` 才会带 `context_header`
- 端到端入库：6 篇样例上传 → 解析完成 → 混合检索命中正确
- `POST /models/:id/debug` 需要 **multipart**（字段 `input`），传 JSON 会报 `model download failed`
