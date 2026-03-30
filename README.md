# go-route-impact

Go 项目**函数级**路由影响分析 CLI 工具。修改一个函数后，精确告诉你"本次修改影响了哪些 API 接口"。

## 它解决什么问题

在 Go Web 项目中，修改一个底层函数（service 方法、repository 方法），很难快速知道会影响到哪些 HTTP 路由。go-route-impact 通过 **AST 静态分析** 构建函数级调用图，从修改的函数出发 **BFS 反向遍历**，只找到真正调用链上的 controller 方法，映射到精确路由。

```
改 GetGameEnvConfig() 函数体某行 → 调用图追踪 → 2 routes (精确到函数)
```

## 快速开始

```bash
# 安装
go install github.com/wuqiang1985/go-route-impact/cmd/go-route-impact@latest

# 进入你的 Go Web 项目
cd your-project

# 查看所有路由
go-route-impact routes

# 分析某个函数的影响（按函数名）
go-route-impact check --func "services.gameInfoService.GetGameEnvConfig"

# 分析某个函数的影响（按文件+行号）
go-route-impact check --file v3/game_info/services/game_info.go --line 4446

# 分析 git 暂存区变更（★ 核心场景）
go-route-impact git --staged

# 查看函数调用图
go-route-impact graph --func "services.gameInfoService.GetGameEnvConfig"
```

## 命令一览

### `check` — 函数级影响分析

```bash
# 按函数名查找
go-route-impact check --func "services.gameInfoService.GetGameEnvConfig"

# 按文件+行号（自动定位所属函数）
go-route-impact check --file v3/game_info/services/game_info.go --line 4446

# 直接用方法名（全项目匹配）
go-route-impact check --func "GetEnvConfig"
```

输出示例：

```
Changed functions (1):
  • services.gameInfoService.GetGameEnvConfig (v3/game_info/services/game_info.go:4446-4526)

📋 Function Impact (1 function → 2 routes)

Call Chain(s):
  services.gameInfoService.GetGameEnvConfig
  ← controllers.GameInfoController.GetEnvConfig
    ← [ROUTE] GET /api/v3/business/game_info/get-env-config

  services.gameInfoService.GetGameEnvConfig
  ← controllers.GameInfoController.GetEnvConfig
    ← [ROUTE] GET /api/v4/business/game_info/get-env-config

Affected Routes (2):
┌────────┬───────────────────────────────────────────┬─────────────────────────┐
│ METHOD │ PATH                                      │ HANDLER                 │
├────────┼───────────────────────────────────────────┼─────────────────────────┤
│ GET    │ /api/v3/business/game_info/get-env-config │ controller.GetEnvConfig │
│ GET    │ /api/v4/business/game_info/get-env-config │ controller.GetEnvConfig │
└────────┴───────────────────────────────────────────┴─────────────────────────┘
```

### `git` — 分析 git 变更影响

```bash
go-route-impact git --staged              # 暂存区变更
go-route-impact git --uncommitted          # 所有未提交变更
go-route-impact git --branch dev           # 与某分支的差异
go-route-impact git --staged --format json # JSON 格式输出
```

核心流程：`git diff --unified=0` → 变更行号 → AST 定位所属函数 → 调用图 BFS → 路由。

### `routes` — 列出项目所有路由

```bash
go-route-impact routes                  # 终端表格
go-route-impact routes --format json    # JSON
go-route-impact routes --format md      # Markdown
```

### `graph` — 函数调用图

```bash
go-route-impact graph --func "services.gameInfoService.GetGameEnvConfig"
```

输出：

```
services.gameInfoService.GetGameEnvConfig (TARGET)
  ← controllers.GameInfoController.GetEnvConfig
    ← [ROUTE] GET /api/v3/business/game_info/get-env-config
  ← controllers.GameInfoController.GetEnvConfig
    ← [ROUTE] GET /api/v4/business/game_info/get-env-config
```

### `hook` — Git Hook 集成

```bash
go-route-impact hook install    # 安装 pre-commit hook
go-route-impact hook uninstall  # 卸载
```

安装后，每次 `git commit` 自动分析暂存区变更的函数并输出影响的路由。

### `init` — 初始化配置

```bash
go-route-impact init
```

生成 `.route-impact.yaml`：

```yaml
framework: auto        # iris | auto
entry_point: main.go
exclude:
  - vendor/
  - test/
  - testdata/
  - docs/
```

## 全局参数

```bash
go-route-impact --project /path/to/project  # 指定项目根目录（默认当前目录）
go-route-impact --config path/to/config     # 指定配置文件
```

## 工作原理

### 核心算法

```
1. 全量 AST 解析所有 .go 文件
       ↓
2. 构建函数级调用图 (caller → callee)
   • pkg.Function()        → 直接 import 包级函数
   • r.Method()            → receiver 方法
   • r.Field.Method()      → struct field 链式调用 + 类型推断
   • 接口方法 → 自动映射到具体实现
       ↓
3. 路由提取 (Iris Party 树追踪)
   • 每条路由记录 handler 的 FuncID
       ↓
4. git diff → 变更行号 → AST 定位所属函数
       ↓
5. 从变更函数出发，BFS 反向遍历调用图
   • 遇到 controller 方法 → 匹配路由
   • 输出完整调用链
```

### 类型推断策略

轻量级类型推断，不依赖编译，针对项目实际模式：

```go
// 模式 1: struct field 调用链
type GameInfoController struct {
    Service services.GameInfoService  // ← 接口类型
}
func (r *GameInfoController) GetEnvConfig(ctx iris.Context) {
    r.Service.GetGameEnvConfig(...)   // ← 推断 Service 字段类型 → 找到具体实现
}

// 模式 2: 变量赋值 + New 命名约定
service := services.NewGameInfoService(repos)  // → service 类型 = gameInfoService

// 模式 3: 接口 → 实现映射
// GameInfoService (interface) → gameInfoService (concrete struct)
// 同包内自动匹配
```

### 架构图

```
                          ┌─────────────┐
                          │   go.mod    │
                          │ (module path)│
                          └──────┬──────┘
                                 │
                 ┌───────────────┼───────────────┐
                 ▼               ▼               ▼
          ┌─────────────┐ ┌───────────┐  ┌──────────────┐
          │ AST Parser  │ │ Type Infer│  │   Extractor  │
          │ (Full AST)  │ │(struct    │  │  (Iris v12)  │
          │             │ │ fields,   │  │ +HandlerFuncID│
          │             │ │ interface)│  │              │
          └──────┬──────┘ └─────┬─────┘  └──────┬───────┘
                 │              │               │
                 ▼              ▼               ▼
          ┌────────────────────────┐     ┌──────────────┐
          │   Function Call Graph  │     │  Route List  │
          │  (forward + reverse)   │     │ (method/path/│
          │  1249 funcs, 6472 edges│     │  HandlerFuncID)│
          └──────────┬─────────────┘     └──────┬───────┘
                     │                          │
                     └──────────┬───────────────┘
                                ▼
                         ┌─────────────┐
                         │  Analyzer   │
                         │(BFS reverse │
                         │ call graph) │
                         └──────┬──────┘
                                ▼
                         ┌─────────────┐
                         │   Output    │
                         │(table/json/ │
                         │ markdown)   │
                         └─────────────┘
```

## 项目结构

```
go-route-impact-v2/
├── cmd/go-route-impact/         # CLI 入口（cobra 命令）
│   ├── main.go                  # 根命令 + 全局 flags
│   ├── check.go                 # check --func / --file+--line
│   ├── git.go                   # git --staged/--uncommitted
│   ├── routes.go                # routes 列出所有路由
│   ├── graph.go                 # graph --func 调用图
│   ├── hook.go                  # hook install/uninstall
│   └── init.go                  # init 生成配置
├── internal/
│   ├── callgraph/               # 函数级调用图
│   │   ├── builder.go           # AST 扫描、提取调用目标、接口解析
│   │   └── graph.go             # 正向+反向调用图、BFS 查找
│   ├── typeinfer/               # 轻量类型推断
│   │   └── infer.go             # struct field 类型、变量赋值类型
│   ├── astutil/                 # AST 工具
│   │   ├── parser.go            # 全量 AST 并行解析（16 并发）
│   │   ├── resolve.go           # go.mod 解析、文件↔包路径转换
│   │   └── locate.go            # 行号 → 所属函数定位
│   ├── extractor/               # 路由提取器（插件架构）
│   │   ├── extractor.go         # RouteExtractor 接口
│   │   ├── registry.go          # 框架注册表
│   │   └── iris/iris.go         # Iris v12 提取器（记录 handler FuncID）
│   ├── analyzer/                # 核心协调器
│   │   ├── analyzer.go          # 组合 callgraph + extractor
│   │   └── impact.go            # 函数级 BFS 影响分析
│   ├── gitutil/                 # Git 工具
│   │   ├── diff.go              # git diff --unified=0 → 变更行号 → 函数映射
│   │   └── hook.go              # hook 安装 / 卸载
│   ├── config/config.go         # .route-impact.yaml 加载
│   └── output/                  # 输出格式
│       ├── table.go             # 终端彩色表格 + 调用链展示
│       ├── json.go              # JSON
│       └── markdown.go          # Markdown
├── pkg/model/model.go           # FuncID, Route, CallChain, ImpactResult
├── go.mod
└── Makefile
```

## 技术选型

| 需求 | 选择 | 理由 |
|------|------|------|
| CLI | [cobra](https://github.com/spf13/cobra) | Go CLI 事实标准 |
| AST 解析 | go/ast + go/parser | 标准库，零外部依赖 |
| go.mod 解析 | [golang.org/x/mod](https://pkg.go.dev/golang.org/x/mod) | 官方库 |
| 终端输出 | [fatih/color](https://github.com/fatih/color) | 彩色终端 |
| 配置 | [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml) | YAML 解析 |

**不依赖 `go list` 和编译**，纯 AST 静态分析，适用于有私有依赖无法本地编译的项目。

## 性能

- 全量 AST 并行解析（16 并发信号量）
- 约 300 文件项目：1249 个函数、6472 条调用边，< 3 秒完成分析
- 内存：调用图常驻，首次构建后后续查询即时返回

## 开发

```bash
# 构建
make build

# 安装到 GOPATH/bin
make install

# 运行测试
make test

# 代码检查
make vet
make fmt
```

## 支持的框架

| 框架 | 状态 | 说明 |
|------|------|------|
| Iris | ✅ 已实现 | Party 树 + mvc.Configure + router.Get/Post + HandlerFuncID |
| Gin | ✅ 已实现 | gin.Default/New + Group 嵌套 + GET/POST/... + 函数注册追踪 |

框架通过 go.mod 自动检测，也可在 `.route-impact.yaml` 中手动指定。路由提取器采用插件架构（`RouteExtractor` 接口），如需支持其他框架可自行扩展。

## License

MIT
