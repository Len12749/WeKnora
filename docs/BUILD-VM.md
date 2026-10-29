# 在 Linux 虚机构建与部署 Lite

虚机 `kb-vm` 是 x86_64、无 sudo、只能跑 docker。先分清改了什么，两条路径耗时差一个数量级。

## 场景 A：只改了前端（约 1 分钟）

前端产物是 `COPY web /app/web` 打进镜像的，**不需要碰 Go 二进制**，重建镜像即可。

```bash
# 1. 本机构建（需要 Node 20.19+ / 22.12+，系统 20.11 会报 crypto.hash is not a function）
cd ~/Desktop/work/weknora/frontend && npm run build && cd ..

# 2. 传产物（web/ 在 .gitignore，bundle 不会带）
tar czf - web | ssh kb-vm 'cd /webdata/program/weknora && rm -rf web.pre && mv web web.pre && tar xzf -'

# 3. 重建镜像并重启
ssh kb-vm 'cd /webdata/program/weknora && docker compose -f docker-compose.lite.yml up -d --build'
```

## 场景 B：改了 Go 代码（约 5 分钟）

CGO 依赖（sqlite-vec）只能在 Linux 编，macOS 不能交叉编译。

### 1. 同步源码

```bash
./ship-to-vm.sh ~/Desktop/work/weknora weknora main
```

### 2. 编译二进制

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

`SKIP_FRONTEND=1`：前端按场景 A 单独传，编二进制时跳过。产物是仓库根的 `WeKnora-lite`。

### 3. 重建容器

```bash
ssh kb-vm 'cd /webdata/program/weknora && docker compose -f docker-compose.lite.yml down && docker compose -f docker-compose.lite.yml up -d --build'
```

## 验证

```bash
curl -s http://192.168.0.112:8080/health              # 200
docker ps --filter name=WeKnora-lite --format '{{.Status}}'   # healthy
# 确认镜像里的前端是新的：与 frontend/dist/index.html 的 assets/index-*.js 一致
docker exec WeKnora-lite cat /app/web/index.html | grep -o 'assets/index-[^"]*\.js'
```

启动后要等 20–40 秒才 `healthy`，期间 `/health` 可能是 000。

## 踩过的坑

- **不能只 ship 源码**。前端产物不在 git 里，只同步源码时镜像里的 `web/` 还是旧的，页面不会变。
- **不能用官方 `golang:1.26-bookworm`**：没有 `sqlite3.h`，编到 `sqlite-vec-go-bindings/cgo` 直接
  `fatal error: sqlite3.h: No such file or directory`。必须用 `weknora-gobuild:1.26`。
- **必须 `-buildvcs=false`**：容器内 `/src` 属主与 git 不一致，go 探测 VCS 报
  `error obtaining VCS status: exit status 128`。
- **必须挂 `/go/pkg/mod`**：不挂每次重下约 1.8G 依赖。
- **`up -d --build` 报容器名冲突**（`Conflict. The container name ... is already in use`）：
  残留容器卡住了，先 `down` 再 `up -d`。数据在 `./data` 挂载，不会丢，中断约 2 秒。
- **只改前端时别编二进制**：白等 5 分钟，二进制根本没变。
