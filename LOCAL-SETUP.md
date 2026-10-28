# 本机部署说明（非官方默认配置）

WeKnora Lite 0.8.3，单二进制 + SQLite，服务于 `知识库Harness` 的入库验证。

## 启动 / 停止

由 launchd 托管，开机自启、崩溃自动拉起：

```bash
launchctl kickstart -k gui/$(id -u)/com.sen.kbharness.weknora   # 重启 WeKnora
launchctl bootout  gui/$(id -u)/com.sen.kbharness.weknora       # 停止
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.sen.kbharness.weknora.plist  # 启动
```

**运行时不在本仓库**，在 `~/Library/Application Support/kbharness/weknora`（二进制 + `.env.lite` + `config/` + `web/` + `data/`）。
原因：launchd 进程没有「桌面」目录的 TCC 权限，直接跑本目录会报
`operation not permitted: .env.lite`，环境变量读不进来就 panic。

- UI / API：<http://127.0.0.1:8080>
- 账号：用户名 `kjlmadmin` / 登录邮箱 `kjlmadmin@local.test`（密码见 kjlm-weknora 的凭据文件）
- 数据：`~/Library/Application Support/kbharness/weknora/data/`（`weknora.db`、`files/`）
- 日志：`~/Library/Logs/kbharness/weknora.log`（stdout/stderr 都指向这里，`LOG_PATH` 未设置）

本仓库下的 `data/` 是迁移前的旧快照（约 12M），服务已不再使用，可确认后删除。

## 编译与升级

本机没装 Homebrew，Go 工具链单独解压在 `~/sdk/go1.26.10`：

```bash
export PATH=$HOME/sdk/go1.26.10/bin:$PATH
cd ~/Desktop/work/weknora
GOPROXY=https://goproxy.cn,direct GOFLAGS=-buildvcs=false SKIP_FRONTEND=1 make build-lite
cp WeKnora-lite "$HOME/Library/Application Support/kbharness/weknora/WeKnora-lite"
launchctl kickstart -k gui/$(id -u)/com.sen.kbharness.weknora
```

`-buildvcs=false` 是必需的，否则 git 状态探测会让构建失败；`SKIP_FRONTEND=1`
是因为前端产物已随发行包放在运行时目录的 `web/`，源码树里没有再构建。

## 与官方默认的差异（踩过的坑）

0. **MCP `ask` 超时 4 分钟 → 8 分钟**
   ReAct 循环下宽问题会跑十几轮检索，4 分钟常在答案流式中途被砍，返回
   `answer timed out: context deadline exceeded`（日志 `latency=4m0.0s`）。
   改为 `internal/mcpserver/tools_ask.go` 的 `askTimeout = 8 * time.Minute`，
   已提交在 `6757a69d7`，升级时不再需要另行重打。

1. **`SSRF_WHITELIST_EXTRA`**
   VPN 是 TUN/fake-IP 模式，`api.deepseek.com` 会被解析到 `198.18.0.0/15`，
   WeKnora 的 SSRF 校验直接拒绝；`api.siliconflow.cn` 同理。
   ```
   SSRF_WHITELIST_EXTRA=api.deepseek.com,api.siliconflow.cn,127.0.0.1,localhost
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

3. **向量 / 重排走硅基流动**
   DeepSeek 官方 API 没有 embedding 接口，本地 Ollama 已停用。当前两个模型都是远程：
   ```
   type=Embedding, source=remote, name=BAAI/bge-m3,
   parameters={base_url: https://api.siliconflow.cn/v1, provider: siliconflow,
               embedding_parameters: {dimension: 1024}}

   type=Rerank, source=remote, name=BAAI/bge-reranker-v2-m3,
   parameters={base_url: https://api.siliconflow.cn/v1, provider: siliconflow}
   ```
   维度必须是 1024，改维度等于换向量空间，历史切片全部失效。

4. **网络搜索用 DeepSeek 官方搜索 API**
   provider 类型 `deepseek`（本仓新增，`internal/infrastructure/web_search/deepseek.go`），
   端点固定 `https://api.deepseek.com/v1/tools/search`，按成功请求次数计费。
   本机已建「DeepSeek 搜索」并设为默认，智能推理已选中。

## 已验证

- `/api/v1/chunker/preview`：切块预览可用，`strategy=auto` 才会带 `context_header`
- 端到端入库：6 篇样例上传 → 解析完成 → 混合检索命中正确
- `POST /models/:id/debug` 需要 **multipart**（字段 `input`），传 JSON 会报 `model download failed`
- `POST /web-search-providers/:id/test`：DeepSeek 搜索连通（返回 1 条结果）
