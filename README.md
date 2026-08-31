# 背单词 · Vocabulary Memorization

一个极简的自托管背单词应用：录入单词即自动查词，得到带词性、音标、例句、词根词缀、近反义词和形近词的完整释义，配合间隔重复（SRS）闪卡和豆包语音朗读巩固记忆，多用户各自隔离，超管后台统一管理。

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)
![Vue](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vuedotjs&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-8-4479A1?logo=mysql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-blue)

## 特性

- 📝 **智能查词**：录入英文单词，自动查词得到中文释义（含词性，同一词性的多条释义自动合并）。附带音标、英文例句（含中文翻译）、词根词缀、近义词、反义词、形近词。
- 🌐 **全站查词只查一次**：同一个单词无论被哪个用户输入，全站只向大模型查询一次，结果缓存进全局词库，后续任何人复用，秒回。
- 🃏 **间隔重复闪卡**：简化版 SM-2 排期，三档自评（记住 / 模糊 / 不认识），反复忘的词优先出现，点「记住」自动归档。
- 🔊 **语音朗读**：豆包（火山引擎）语音合成，查词成功后后台预生成发音，单词卡和闪卡正面一键点读。
- 📊 **统计可视化**：ECharts 绘制背诵趋势、次数分布、近 7 天词云、开头字母统计，数据全部由后端 SQL 聚合。
- 👥 **多用户 + 后台**：账号之间数据隔离；超管可增删用户、重置密码、禁用账号、管理全局词库与查词/语音配置。
- 🌙 **深色主题**：手动切换深浅色，记忆在浏览器本地，不跟随系统。

## 核心功能

### 录入与查词

- 登录后输入英文单词回车（或点「添加」）即记录，重复输入同一个词累计背诵次数。
- 查词是**异步**的：录入立即入库返回，释义在后台查到后自动写回，期间显示「查询中...」。
- 查词来源为 DeepSeek（可在后台配置 API Key / Base URL / 模型），会先校验拼写：拼写错误显示「请检查单词拼写是否正常」。
- 失败按退避间隔自动重试（共 3 次尝试），仍失败则显示「查询失败，请稍后重试」；这类词显示「重试」按钮而非「归档」，点一下重新触发一次查词。
- **全局词库缓存**：查到的释义存进独立于用户单词表的 `word_dictionary` 表，同一个单词全站只查一次。用户删除自己的单词只影响自己的列表；超管清空缓存也只清缓存，不影响任何人已保存的记录——下次再输入才重新查词。

### 间隔重复闪卡

- 到期单词进入闪卡队列，每组 30 张，本地翻卡自评，自评档位只有三档：

| 档位 | 排期 | 结果 |
|---|---|---|
| 记住（good） | 间隔按难度系数倍增 | 直接归档 |
| 模糊（hard） | 间隔 ×1.2，难度 −0.15 | 保持未归档，稍后重现 |
| 不认识（again） | 间隔重置 1 天，难度 −0.20 | 保持未归档，次日再见 |

- 难度系数收敛在 `[1.30, 2.50]`；队列按背诵次数降序优先，反复背反复忘的词最先出现。
- 点「不认识」时，若该词已有释义但缺强化信息（老数据），后台会自动补一次查词。

### 强化信息与点击查询

- 释义按词性拆开展示；例句、近义词、反义词、形近词里的单词可以**点击查询**：弹出浮层展示该词的释义、音标、读音与例句，若不在你的词库里则自动录入一次。

### 统计页

总词汇量、累计背诵次数、今日背诵次数、次数分布、近 14 天新增趋势、近 7 天词云（按累计背诵次数加权）、开头字母统计，全部由 `GET /api/stats` 用 SQL 聚合，不下载全量单词到前端。

### 用户体系与后台

- 应用整体需登录，**没有自助注册入口**；首次启动自动创建超管账号（见 `.env`）。
- 超管在 `/admin` 可：新增用户（可设超管）、查看创建/最后登录时间与录入数、重置密码、禁用/启用、删除（级联清理单词）；管理全局词库（过滤、删除、批量删除、对「暂无释义」词条重新查询、导出 CSV）；修改 DeepSeek 查词与豆包语音配置。
- 普通用户在 `/profile` 可改密码、重置背诵次数，查看自己的注册/最后登录时间与录入数。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.24+（标准库 `net/http` 方法路由，无第三方 web 框架）、`go-sql-driver/mysql`、`golang.org/x/crypto/bcrypt` |
| 前端 | Vue 3 + Vite、Vue Router（history 模式）、Pinia、Element Plus、ECharts + echarts-wordcloud |
| 数据库 | MySQL 8，数据存 Docker volume，结构变更写成幂等迁移随容器启动自动执行 |
| 认证 | Bearer Token（登录返回 `{ token, user }`，前端存 localStorage，请求带 `Authorization` 头），会话 30 天滑动续期，重置密码/禁用账号即时失效 |
| 语音 | 豆包（火山引擎 BytePlus Seed Speech）语音合成，音频落盘 `audio/` 目录并由 volume 持久化 |

## 快速开始

### 方式一：Docker Compose（一条命令跑整套）

```bash
# 仓库根目录，首次可直接跑 ./deploy.sh 自动生成 .env（随机数据库密码、超管密码）
docker compose up -d --build
```

- 访问 `http://localhost:39100`，用 `.env` 里的 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 登录。
- `.env` 已加入 `.gitignore`（含数据库密码、超管密码、DeepSeek Key），换机器部署记得一起拷贝。
- 前端产物构建进镜像，改前端代码需 `--build` 重建才能看到效果。

### 方式二：本地开发（前端热更新）

完整步骤（起 MySQL 容器、`go run` 后端、`npm run dev` 前端）、环境变量表、测试命令、代码约定与发版流程，见 [CONTRIBUTING.md](CONTRIBUTING.md)。

```bash
# 后端
cd backend
DB_HOST=127.0.0.1 DB_PORT=3306 DB_USER=vocab DB_PASSWORD=devpass DB_NAME=vocab \
  ADMIN_USERNAME=admin ADMIN_PASSWORD=admin123456 go run .

# 前端（另开终端）
cd frontend
npm install && npm run dev
```

> ⚠️ 本地必须先在空库里导入 `backend/schema.sql`（`words` 表只在里面定义，Go 迁移不建这张表）。

### 常用命令

```bash
docker compose logs -f backend   # 看后端日志（查词/语音报错、启动报错都在这里）
docker compose logs -f mysql     # 看数据库日志
docker compose down              # 停止服务（数据库数据保留在 volume 里）
docker compose down -v           # 停止并清空数据库数据（慎用）
```

## 目录结构

```
vocabulary-memorization/
├── backend/                   Go 后端源码
│   ├── main.go                  路由注册 + 单词录入/列表/归档/删除/闪卡/统计接口
│   ├── store.go                 所有 SQL 收在这里（repository 层）：分页、排序白名单、统计聚合
│   ├── app.go                   App 结构体 + 各 repository 的窄接口（便于测试替换成 fake）
│   ├── db.go                    MySQL 连接 + 幂等数据库迁移（含历史数据回填、词性合并修复）
│   ├── auth.go                  登录/登出、Bearer Token 中间件、超管引导、用户管理与密码重置
│   ├── dictionary.go            全局词库缓存的读写、出现次数统计、管理员查看与删除
│   ├── settings.go              DeepSeek 查词 + 豆包语音配置的读写与内存缓存
│   ├── deepseek.go              调用 DeepSeek 查词
│   ├── doubao.go / pronunciation.go   豆包语音合成与发音接口
│   ├── translate.go            查词编排：重试调度、拼写校验、释义写回
│   ├── middleware.go / ratelimit.go    panic 恢复中间件、登录/改密失败限流
│   ├── models.go               数据结构、同词性释义合并、SRS 排期算法
│   ├── schema.sql                 数据库表结构
│   ├── static/                    前端构建产物（`frontend/` 构建后生成，不进 git）
│   └── Dockerfile                 多阶段构建：先构建 frontend/ 产物，再编译 Go 二进制
├── frontend/                   Vue 3 + Vite 单页应用源码
│   ├── vite.config.js            开发环境把 /api 代理到本地 Go 后端
│   ├── src/api/client.js         统一 fetch 封装（Bearer 凭证、401 跳转、错误归一化）
│   ├── src/stores/auth.js        Pinia 鉴权 store
│   ├── src/router/index.js       Vue Router（history 模式）+ 鉴权守卫
│   ├── src/components/           AppTopbar、WordCard/WordList、Admin 子组件等（按 admin/auth/layout/profile/word 分目录）
│   ├── src/composables/          分页状态机、滚动加载、主题、查词轮询、语音、点击查询等可复用逻辑
│   ├── src/utils/                时间格式化、次数档位映射、例句高亮等纯函数
│   └── src/views/                首页/闪卡/归档/个人中心/统计/后台管理六个路由页面
├── docker-compose.yml          编排 MySQL + 后端两个容器（构建上下文为仓库根目录）
├── deploy.sh                    一键部署脚本（首次部署用，自动生成 .env）
└── *.md                         文档，见下方「文档」
```

## 文档

| 文档 | 内容 |
|---|---|
| [CONTRIBUTING.md](CONTRIBUTING.md) | 开发者指南：本地启动、环境变量、测试、代码约定、发版流程 |
| [DEPLOYMENT.md](DEPLOYMENT.md) | 当前生产服务器的实际部署与更新流程（rsync、备份、端口、代理镜像等实操细节） |
| [CHANGELOG.md](CHANGELOG.md) | 每个版本的变更记录（Keep a Changelog 格式） |

## License

[MIT](LICENSE) © 2026 alanwhy
