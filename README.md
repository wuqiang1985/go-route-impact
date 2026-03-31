# go-route-impact

Go 项目**函数级**路由影响分析工具 —— 改了一个函数，精确告诉你影响了哪些 API。

## 解决什么问题

Go Web 项目中，修改一个底层方法后，很难快速判断哪些 HTTP 路由会受影响。`go-route-impact` 通过 AST 静态分析构建**函数级调用图**，从修改的函数出发反向追踪，精确定位受影响的路由。

```
修改 service 层的 SaveUserGroup()
  → 调用图追踪到 controller 层的 SaveUserGroup()
    → 定位到 POST /api/auth/manager/user/group/save
```

不依赖编译，纯 AST 分析，适用于有私有依赖无法本地编译的项目。

## 核心价值

1. **Code Review 提效** — PR 里改了 service 层 5 个方法，reviewer 不用人肉追代码就能知道影响了哪 8 条接口，重点测这 8 条就够了
2. **降低线上事故风险** — 改一个公共方法以为只影响自己的接口，实际被 3 个 controller 调用。工具在提交前暴露隐性影响，避免改一处崩三处
3. **缩小测试范围** — QA 不用猜要测哪些接口，工具直接给出精确列表，回归测试从"全量 283 条"缩小到"实际影响的 3 条"
4. **新人上手加速** — 不熟悉代码调用关系时，`graph` 命令直接展示函数的调用链和关联路由，不用读完整个项目就能理解影响面
5. **发布决策有依据** — 上线前跑一下 `git --staged`，输出的路由列表就是本次发布的影响范围，可以直接贴到发布单里

## 使用场景

| 场景 | 命令 | 谁用 |
|------|------|------|
| 提交前自查影响面 | `go-route-impact git --staged` | 开发 |
| PR 评审看变更影响了哪些接口 | `go-route-impact git --branch main` | Reviewer |
| 提测时给 QA 回归范围 | `go-route-impact git --branch dev --format md` | 开发 / QA |
| 不熟悉的代码，改之前先看调用链 | `go-route-impact graph --func "MethodName"` | 开发 |
| 每次 commit 自动提示影响路由 | `go-route-impact hook install` | 团队 |
| 发布单填写影响范围 | `go-route-impact git --branch main --format json` | 运维 / SRE |

## 快速开始

```bash
# 1. 安装
go install github.com/wuqiang1985/go-route-impact/cmd/go-route-impact@latest

# 2. 进入项目根目录（go.mod 所在目录）
cd your-project

# 3. 初始化配置（如果 main.go 不在根目录，需要修改 entry_point）
go-route-impact init
# 编辑 .route-impact.yaml，设置 entry_point: cmd/api/main.go

# 4. 查看所有路由，验证是否正确提取
go-route-impact routes

# 5. 分析 git 变更影响（日常核心用法）
go-route-impact git --uncommitted
```

> **提示**：Gin 项目通过全量扫描提取路由，通常无需修改 `entry_point`。Iris 项目需要 `entry_point` 指向包含 `main()` 的文件。

## 日常使用

### 分析 Git 变更 ★

最常用的场景——提交前看看改动影响了哪些接口：

```bash
go-route-impact git --staged        # 暂存区变更
go-route-impact git --uncommitted    # 所有未提交变更
go-route-impact git --branch dev     # 与某分支的差异
```

输出示例：

```
Changed functions (2):
  • accountSvc.AccountSvc.GetUserAuth (app/api/account/accountSvc/accountSvc.go:45-80)
  • userGroupSvc.UserGroupSvc.SaveUserGroup (app/api/userGroup/userGroupSvc/userGroupSvc.go:40-315)

📋 Function Impact (2 functions → 2 routes)

Call Chain(s):
  userGroupSvc.UserGroupSvc.SaveUserGroup
  ← userGroupCntlr.UserGroup.SaveUserGroup
    ← [ROUTE] POST /api/auth/manager/user/group/save

Affected Routes (2):
┌────────┬───────────────────────────────────┬─────────────────────────┐
│ METHOD │ PATH                              │ HANDLER                 │
├────────┼───────────────────────────────────┼─────────────────────────┤
│ GET    │ /api/account/get_user_auth        │ account.GetUserAuth     │
│ POST   │ /api/auth/manager/user/group/save │ userGroup.SaveUserGroup │
└────────┴───────────────────────────────────┴─────────────────────────┘
```

### 分析指定函数

```bash
# 按函数名（支持模糊匹配）
go-route-impact check --func "SaveUserGroup"

# 按完整路径
go-route-impact check --func "userGroupSvc.UserGroupSvc.SaveUserGroup"

# 按文件 + 行号（自动定位所属函数）
go-route-impact check --file app/api/userGroup/userGroupSvc/userGroupSvc.go --line 50
```

### 查看函数调用图

```bash
go-route-impact graph --func "SaveUserGroup"
```

```
userGroupSvc.UserGroupSvc.SaveUserGroup (TARGET)
  ← userGroupCntlr.UserGroup.SaveUserGroup
    ← [ROUTE] POST /api/auth/manager/user/group/save
```

### 列出所有路由

```bash
go-route-impact routes                   # 终端表格
go-route-impact routes --format json     # JSON 格式
go-route-impact routes --format md       # Markdown 格式
```

### Git Hook 自动分析

```bash
go-route-impact hook install     # 安装 pre-commit hook
go-route-impact hook uninstall   # 卸载
```

安装后每次 `git commit` 自动输出受影响的路由。

## 配置

运行 `go-route-impact init` 生成 `.route-impact.yaml`：

```yaml
framework: auto          # auto | iris | gin
entry_point: main.go     # main 函数所在文件（相对项目根目录）
exclude:                 # 排除目录
  - vendor/
  - test/
  - testdata/
  - docs/
```

| 字段 | 说明 | 默认值 |
|------|------|--------|
| `framework` | Web 框架，`auto` 会从 go.mod 自动检测 | `auto` |
| `entry_point` | main.go 路径，Gin 项目可忽略 | `main.go` |
| `exclude` | 跳过的目录 | vendor, test 等 |

全局参数：

```bash
go-route-impact -p /path/to/project routes   # 指定项目目录
go-route-impact -c myconfig.yaml routes      # 指定配置文件
```

## 支持的框架

| 框架 | 路由注册模式 |
|------|-------------|
| **Gin** | `r.GET()` / `r.Group()` 嵌套 / struct 内嵌 `*gin.Engine` / 方法级注册 |
| **Iris** | `Party` 树 / `mvc.Configure` / `router.Get/Post` |

框架通过 go.mod 自动检测。路由提取器采用插件架构（`RouteExtractor` 接口），可自行扩展。

## 工作原理

```
 ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
 │  AST Parser  │    │  Type Infer  │    │  Extractor   │
 │  全量解析     │    │  类型推断     │    │  路由提取     │
 │  16 并发      │    │  struct field │    │  Gin / Iris  │
 │              │    │  interface   │    │              │
 └──────┬───────┘    └──────┬───────┘    └──────┬───────┘
        │                   │                   │
        ▼                   ▼                   ▼
 ┌─────────────────────────────┐    ┌────────────────────┐
 │  Function Call Graph        │    │  Route Registry     │
 │  正向 + 反向调用图           │    │  路由 → HandlerFuncID│
 └─────────────┬───────────────┘    └─────────┬──────────┘
               │                              │
               └──────────┬───────────────────┘
                          ▼
               ┌─────────────────────┐
               │  Impact Analyzer    │
               │  BFS 反向遍历调用图  │
               │  变更函数 → 路由     │
               └─────────────────────┘
```

**核心流程**：

1. **全量 AST 解析** — 并行解析所有 `.go` 文件，提取函数声明和调用关系
2. **类型推断** — 轻量级推断 struct field 类型、嵌入字段、接口到实现的映射
3. **调用图构建** — 识别 `pkg.Func()`、`r.Method()`、`r.Field.Method()` 三种调用模式
4. **路由提取** — 从 Gin/Iris 路由注册代码中提取路由并关联 handler 函数
5. **影响分析** — 从 git diff 的变更行号定位到函数，BFS 反向追踪到路由

## 项目结构

```
go-route-impact/
├── cmd/go-route-impact/        # CLI（cobra）
├── internal/
│   ├── analyzer/               # 核心协调器 + 影响分析
│   ├── callgraph/              # 函数级调用图构建 + BFS
│   ├── typeinfer/              # 轻量类型推断
│   ├── astutil/                # AST 解析 + 行号定位 + 包路径解析
│   ├── extractor/              # 路由提取器
│   │   ├── gin/                #   Gin 提取器
│   │   └── iris/               #   Iris 提取器
│   ├── gitutil/                # Git diff 解析 + hook
│   ├── config/                 # 配置加载
│   └── output/                 # 输出格式（table / json / markdown）
├── pkg/model/                  # 公共数据结构
└── Makefile
```

## 开发

```bash
make build       # 构建
make install     # 安装到 GOPATH/bin
make test        # 测试
make vet         # 静态检查
```

## License

MIT
