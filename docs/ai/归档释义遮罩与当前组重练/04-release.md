# 2.1.0 发布进度

时间：2026-11-20 07:53 +08:00。用户调用 quick-release，明确选择生产环境、无任务号；飞书文档仍不归档。

## 结果

- 版本：2.0.0 → 2.1.0，依据是新增兼容的当前组重练能力，另含归档词中文释义遮罩。
- 代码提交：`f83c667eb2690d8e259c377dcfbca363a0b2a294`，标题 `feat: 增加归档释义遮罩和闪卡当前组重练`；已推送 origin/main，并通过 git ls-remote 核对一致。
- 生产发布尚未完成。已同步代码快照并成功构建远端 frontend-builder；随后应用容器更新命令的 SSH 握手被服务器重置，未进入远端执行。两次只读状态检查（含等待 30 秒后的检查）同样失败，当前服务器和容器状态未能确认。

## 已完成步骤与证据

1. fetch 后 main 与 origin/main 领先/落后均为 0；本次仅暂存任务代码、测试、文档、README、CHANGELOG 和 npm 版本元数据。已有未跟踪 frontend/pnpm-lock.yaml 保留，不提交、不进入快照。
2. `npm version 2.1.0 --no-git-tag-version --ignore-scripts` 完成；package-lock 只同步根版本，不升级依赖、创建 tag 或运行版本脚本。
3. 发布前 SSH 可用，远端源码为 2.0.0，backend 与 mysql 容器均运行，MySQL healthy。内存总量 1673 MiB、available 632 MiB、Swap 1024 MiB（未用），磁盘可用约 9.8 GiB；数据库卷与音频卷均存在。
4. 从已推送代码提交 `git archive HEAD` 导出 `/tmp/vocabulary-release.mh276T`，按 DEPLOYMENT.md rsync，不加 --delete，排除凭据、Git、索引、依赖、备份与迁移记录；退出码 0。
5. 服务器上的 package.json、package-lock.json 及三处功能文件 SHA-256 均与本地提交快照一致。此次无后端/数据库变更，无须新增迁移备份。
6. 远端 `docker build --target frontend-builder -f backend/Dockerfile .` 退出码 0：安装 271 个锁定依赖，frontend@2.1.0 构建 2212 个模块，镜像层导出成功，ID 为 `sha256:0b3e2a96d091af61b998367cd5d08c61ec22c61300d9fc218a64a9bc88e60513`。有常见的大体积 chunk 提示，无构建失败。

## 中断位置

待执行的应用更新命令：

```bash
ssh -p 7339 root@123.56.219.4 'cd /root/vocabulary-memorization && docker compose -f docker-compose.yml -f docker-compose.production.yml up -d --no-deps --build backend'
```

实际连接结果（退出码 255）：

```text
kex_exchange_identification: read: Connection reset by peer
Connection reset by 123.56.219.4 port 7339
```

握手即失败，未在结果不明时重复执行容器更新；随后仅尝试只读 uptime/free/docker ps 也失败。不能根据此错误认定内存耗尽或服务器宕机，根因未确认。未自动重启服务器、回滚、改配置或清理镜像/数据。

## 恢复条件与后续

- 先恢复 SSH 7339 的可用性；必要时由用户在云控制台检查服务器及 SSH 服务。
- 恢复后先读取容器、内存、磁盘、现有镜像和源码版本，确认发布状态，再继续上述应用构建与更新。
- 不再次升版；前端缓存构建已完成且代码已推送。运行容器的最终镜像、启动结果与生产版本仍待确认。
- 本发布阶段未重复本机构建/测试，未执行线上 HTTP 冒烟或业务验收；开发阶段验证详见 03-review.md。

本文件为发布过程记录，后续单独提交不改变已同步代码提交对应的运行时代码。用户实际功能验收保持待执行。
