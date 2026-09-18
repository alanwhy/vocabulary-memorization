<!-- 阶段记录：模型 GPT-5 (Codex) / 开始 2026-09-18 15:46 CST / 结束 2026-09-18 15:47 CST / 规范版本 @sw/dev-skills@1.1.2 -->

# 后端分包详细设计

## 后端详细设计

### 包与依赖方向

```text
cmd/vocab-server
  ├── internal/app ───────→ internal/model
  └── internal/storage ───→ internal/model

禁止：internal/app → internal/storage
禁止：internal/storage → internal/app
```

- `cmd/vocab-server` 是唯一组合根，创建 DB、repositories、应用与 HTTP server。
- `internal/app` 是 HTTP 适配与当前业务编排层，声明自己消费的 repository 接口。
- `internal/storage` 是 MySQL 适配层，只实现接口，不感知 Handler 或 HTTP DTO。
- `internal/model` 是叶子包，只含共享数据模型和纯规则，不依赖其他项目内部包。

### 内部接口

```go
// internal/app
type Dependencies struct {
    Users      UserStore
    Sessions   SessionStore
    Words      WordStore
    Dictionary DictionaryStore
    Settings   SettingsStore
}

func New(deps Dependencies, cfg Config) *App
func (a *App) Handler(staticDir string) http.Handler
func (a *App) CancelBackground()
func (a *App) WaitBackground(timeout time.Duration) bool
```

- 五个 Store 接口从现有 `app.go` 原样迁移，只把跨包返回值切换为 `internal/model` 类型。
- storage 提供五个 repository 构造结果，由组合根填入 `Dependencies`；不在 app 内部接收 `*sql.DB` 或创建具体实现。
- 进程初始化、后台任务启动和优雅关闭所需的方法只导出给组合根，其他 Handler 和 helper 保持包内私有。
- `Handler(staticDir)` 使用局部 `http.NewServeMux`，集中注册当前 32 条 API pattern 与 1 条 SPA fallback；每条 API 保持“timeout 在鉴权外层”的顺序，最外层只包装一次 panic recover。静态目录可注入，生产仍传 `./static`，测试传 `t.TempDir()`。
- `CancelBackground()` 与 `WaitBackground(timeout)` 分开暴露，确保 HTTP shutdown 仍夹在取消和等待之间。

### 模型边界

- `model` 承载 `Sense`、`Word`、`User`、`UserWithStats`、`WordStats`、`DailyCount`、`WordCloudItem`、`LetterStat`、词典条目和词汇索引条目；storage 必须能构造统计结果中的三个切片类型。
- `MergeSensesByPos`、`SensesEnriched`、`SensesNeedEnrichment`、`NormalizeGlosses`、`ApplySRSScheduling` 作为跨包纯函数导出；实现逻辑不改。
- 分页响应信封、登录请求、设置表单等 HTTP DTO 保留在 `app`。
- JSON 标签必须逐字保持，导出 Go 标识符不得引起响应字段变化。

### 数据库与迁移

- schema 与 SQL 文本不变。
- 去除包级 `var db *sql.DB`，所有连接、迁移和 repository 都显式接收同一个 `*sql.DB`。
- 连接重试、连接池参数和迁移调用顺序保持原样。
- app 与 storage 不互相导入环境读取 helper；可以各自保留窄的私有解析，或由组合根显式传入，但 DB 30 次/2 秒重试、连接池默认 25、翻译并发默认 5、管理员/DeepSeek/TTS/音频默认值不得改变。
- `schema.sql` 仍由 MySQL 容器初始化挂载，不嵌入 Go 二进制。

### 启动与关闭流程

1. `cmd/vocab-server/main.go` 读取环境配置并连接数据库。
2. storage 执行原幂等迁移。
3. 组合根创建 repositories，并注入 app。
4. app 初始化管理员后，storage 立即完成历史 `user_id` 收紧，严格保持当前迁移顺序。
5. app 再加载设置、初始化音频目录、恢复卡死翻译并启动翻译扫描器和限流清理任务。
6. `http.Server` 使用 `app.Handler()`，监听地址和超时保持原值。
7. 收到退出信号后，保持“取消后台任务 → HTTP Shutdown → 限时等待任务”的顺序。

### 单元测试

| 被测对象 | 覆盖点 | 是否已存在测试 |
|---|---|---|
| `internal/model` 纯规则 | 释义合并、强化判定、重要释义规范化、SRS 排期 | 是，迁移现有测试 |
| `internal/storage` SQL helper | LIKE 转义、空值转换、扫描与统计辅助逻辑 | 是，迁移现有测试 |
| `internal/app` 鉴权和 Handler | 登录、会话、管理员权限、禁用账号、错误分支 | 是，迁移现有测试 |
| `internal/app` 外部响应解析 | DeepSeek、豆包解析与异常输入 | 是，迁移现有测试 |
| `internal/app.Handler` | method/path、鉴权链、fake store 调用、SPA fallback | 否，本次新增路由契约测试 |

### 契约变更记录

| 日期 | 改了什么 | 谁提出 | 对方是否已知 |
|---|---|---|---|
| 2026-09-18 | 仅调整 Go 包内接口和构造方式；HTTP 与数据库契约不变 | Alan | 已确认 |
