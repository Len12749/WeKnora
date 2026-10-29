# 在 Linux 虚机构建 Lite 二进制

本机（macOS/arm64）只做前端；Go 二进制带 CGO（sqlite-vec），必须在 Linux 环境编。
虚机 `kb-vm` 是 x86_64、无 sudo、只能跑 docker，构建走容器。

## 前置

| 项 | 值 |
| --- | --- |
| 虚机目录 | `/webdata/program/weknora` |
| 构建镜像 | `weknora-gobuild:1.26`（已带 `libsqlite3-dev`、gcc） |
| 模块缓存 | `/webdata/program/.gomodcache` → 容器 `/go/pkg/mod` |
| Go 代理 | `https://goproxy.cn,direct`（虚机不通 proxy.golang.org） |

## 步骤

### 1. 同步源码

```bash
# 本机
./ship-to-vm.sh ~/Desktop/work/weknora weknora main
```

### 2. 同步前端产物

`web/` 在 `.gitignore` 里，bundle 不会带；而 `Dockerfile.lite` 是 `COPY web /app/web`，
镜像里的前端就是构建时那份。改过前端必须单独传：

```bash
# 本机：先构建
cd frontend && npm run build && cd ..
# 传到虚机（先备份旧的）
tar czf - web | ssh kb-vm 'cd /webdata/program/weknora && rm -rf web.pre && mv web web.pre && tar xzf -'
```

> 注意 `npm run build` 需要 Node 20.19+ / 22.12+（Vite 7）。系统 Node 是 20.11 会报
> `crypto.hash is not a function`。

### 3. 编译二进制

```bash
ssh kb-vm 'cd /webdata/program/weknora && \
  docker run --rm \
    -v /webdata/program/weknora:/src -w /src \
    -v /webdata/program/.gomodcache:/go/pkg/mod \
    -e GOPROXY=https://goproxy.cn,direct \
    -e GOFLAGS="-mod=mod -buildvcs=false" \
    weknora-gobuild:1.26 \
    sh -c "SKIP_FRONTEND=1 make build-lite"'
```

`SKIP_FRONTEND=1` 跳过前端（已在上一步传好）；产物就是仓库根目录的 `WeKnora-lite`。

### 4. 重建容器

```bash
ssh kb-vm 'cd /webdata/program/weknora && docker compose -f docker-compose.lite.yml up -d --build'
```

### 5. 验证

```bash
curl -s http://192.168.0.112:8080/health
docker ps --filter name=WeKnora-lite --format '{{.Status}}'
```

## 踩过的坑

- **不能用官方 `golang:1.26-bookworm`**：没有 `sqlite3.h`，编到
  `sqlite-vec-go-bindings/cgo` 直接 `fatal error: sqlite3.h: No such file or directory`。
  必须用 `weknora-gobuild:1.26`。
- **必须 `-buildvcs=false`**：容器内 `/src` 属主与 git 不一致，go 探测 VCS 时
  `error obtaining VCS status: exit status 128`。
- **必须挂 `/go/pkg/mod`**：不挂的话每次重建都要重下约 1.8G 依赖。
- **前端产物不在 git 里**：只 ship 源码，镜像里的 `web/` 还是旧的，页面不会变。
- **本机不能交叉编译**：CGO 需要 linux 的 gcc 与 sqlite 头文件，macOS 上编不出 linux 二进制。
