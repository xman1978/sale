# 销售助手系统（CRM）程序实现文档

---

> **依据**：本实现文档基于 `crm/design.md` 设计说明编写，将设计规格拆解为可执行任务、模块结构、数据模型与验证步骤，供开发与 Agent 实施时使用。
>
> **实施建议**：实施前用 `writing-plans` 将各阶段拆为 2–5 分钟可执行任务；执行时用 `executing-plans` 或 subagent 逐任务实现并校验。

---

## 1. 文档目的与范围

### 1.1 目的

- 将 `design.md` 中的架构、组件、数据模型与流程转化为**可落地的实现任务与约定**。
- 明确**模块/目录结构**、**接口契约**、**持久化 Schema**、**配置与占位符**及**每阶段验收方式**。
- 不重复设计说明中的业务语义，仅给出实现侧的结构化指引。

### 1.2 适用范围

- 后端/全栈开发实现 CRM 框架
- Agent 按阶段执行任务并自验
- 与 design.md 冲突时以 design.md 为准

**实现语言**：**Go**。目录与路径以本实现文档第 3 节「模块与目录结构（Go 实现）」为准。

### 1.3 文档位置

- 设计说明：`crm/design.md`
- 本实现文档：`crm/impl.md`
- 细分功能设计：建议 `docs/plans/YYYY-MM-DD-<feature>-design.md`

---

## 2. 实现阶段与任务拆解

以下阶段与 design.md 第 12 章「实施阶段建议」对应，每阶段下列出可执行子任务与验收方式。

### 阶段一：LLM Gateway 与配置加载

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 1.1 | 实现 YAML 配置加载：读取 `config/config.yml`，支持 `CRM_CONFIG` 指定路径 | `internal/config/loader.go` | 加载后能读取 `llm.baseURL`、`llm.apiKey`、`llm.name` |
| 1.2 | 实现环境变量覆盖：`CRM_CONFIG_CONTENT` 提供 YAML 字符串，合并后覆盖冲突项 | 同上 | 设置 `CRM_CONFIG_CONTENT` 后对应项被覆盖 |
| 1.3 | 实现变量替换：`{env:VAR}`、`{file:path}`、`{prompt:name}` | `internal/config/vars.go` 或内置于 loader | `{env:API_KEY}` 替换为环境变量值 |
| 1.4 | 实现 OpenAI 兼容 HTTP 客户端：POST `{baseURL}/v1/chat/completions`，透传 model、messages、temperature、max_tokens、tools | `internal/llm/openai.go` | `curl` 或单元测试调用兼容 API 返回有效 JSON |
| 1.5 | 从配置绑定：baseURL、apiKey、name、temperature、max_tokens、timeout、limit.context、limit.output | 同上 | 请求体与配置一致 |

**阶段一验收**：配置加载成功且变量替换生效；调用 LLM 端点返回 200 及合法 message/usage。

---

### 阶段二：数据模型与 Session/Context Manager

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 2.1 | 实现 user 表（见 2.4 节 Schema）及唯一约束 external_id | migrations 或 schema 文件 | 插入/查询 user 正常 |
| 2.2 | 实现 session、task、message 表（见 2.4 节）及外键/索引 | 同上 | 按 session_id、task_id 查询 message 正确 |
| 2.3 | 实现 Session & Context Manager：get_or_create_session(user_id, session_id?) | `internal/session/context.go` | 无 session_id 时创建新 Session；有则加载 |
| 2.4 | 实现 load_messages(session_id, task_id)：task_id 为 null 时加载会话级消息 | 同上 | 多 user_id 下 session/message 互不交叉 |
| 2.5 | 实现 append_message(session_id, task_id, message) | 同上 | 写入 message 表且 task_id 正确 |
| 2.6 | 实现 token_estimate(messages) → number（可用 tiktoken 或近似） | `internal/llm/tokens.go` 或 session 内 | 与 limit.context 比较可用于超限判断 |

**阶段二验收**：多 user_id 写入 Session/Task/Message，查询严格按 user 隔离；Token 估算可用。

---

### 阶段三：Task State Manager 与 task 工具

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 3.1 | 实现 Task State Manager：get_active_task(session_id)、get_paused_tasks(session_id) | `internal/task/state_manager.go` | 从 task 表按 status 查询正确 |
| 3.2 | 实现 get_pending_tasks_from_prev_session(user_id, current_session_id)（见 design 9.8） | 同上 | 返回前次会话未完成任务，排序与 LIMIT 5 正确 |
| 3.3 | 实现 task_start(skill_id)：有 active 则 pause → 插入新 task(processing) → 设 active_task | `internal/tools/task_tools.go` | 调用后 active_task 为新任务 |
| 3.4 | 实现 task_pause()：当前 task status→paused，清空 active_task | 同上 | 调用后 active_task 为 null |
| 3.5 | 实现 task_resume(skill_id|task_id)：当前会话优先，否则前序会话；status→processing，设 active_task | 同上 | 可跨会话恢复 |
| 3.6 | 实现 task_complete(skill_id, result?)、task_cancel(task_id) | 同上 | 状态与 active_task 更新正确 |
| 3.7 | 超时自动取消：UPDATE task SET status=cancelled WHERE ... AND COALESCE(paused_at, started_at) < now()-N 天 | 在 Session 加载或独立 job 中 | 超 3 天任务被取消 |
| 3.8 | Orchestrator 骨架：接收 user_id/session_id/message → 调 SCM、TSM → 占位调用 LLM | `internal/orchestrator/orchestrator.go` | 单轮能走通并写回 message |

**阶段三验收**：task_start → 有 active_task；task_pause → 状态更新；task_resume 可恢复；超时任务被取消。

---

### 阶段四：Registry（Tool、Skill、Agent）与 Skill 注入

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 4.1 | Tool Registry：加载内置 task_*、占位 webfetch/rest_api/websearch；name、description、parameters、Execute(args, context) | `internal/registry/tool_registry.go` | 能按 name 取 Tool 定义并执行 |
| 4.2 | Skill Registry：扫描 `config/skills/<name>/SKILL.md`，解析 frontmatter name/description，构建 skill_id→内容映射 | `internal/registry/skill_registry.go` | 配置 skills 限制时仅加载指定 Skill |
| 4.3 | Agent Registry：从 config.yml 的 agent 与 `config/agents/*.md` 合并，解析 prompt、tools、default_agent | `internal/registry/agent_registry.go` | default_agent 解析正确 |
| 4.4 | 组装 system：Agent.prompt + 替换 {{SKILL_LIST}}（Skill 元数据列表）+ 替换 {{TASK_CONTEXT}} | 在 Orchestrator 内 | 占位符被替换为文本 |
| 4.5 | 若有 active_task：追加该 Skill 的 SKILL.md 全文 + task.state 到 system | 同上 | Agent 调用某 Skill 后，该 Skill 内容出现在 prompt 中 |

**阶段四验收**：Agent 调用 task_start(skill_id) 后，下一轮 system 中含对应 SKILL.md 与 TASK_CONTEXT。

---

### 阶段五：Tool 框架、webfetch、rest_api、websearch、MCP 与执行超时

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 5.1 | Tool 执行层：根据 tool_calls 调用 Registry 中 Tool，Execute(args, context)；ToolContext 含 user_id、session_id、message_id | `internal/orchestrator/tool_runner.go` | 执行结果可追加到 messages |
| 5.2 | 对每次 Execute 施加超时（可配置，默认 60s）；超时返回可读错误并继续 loop | 同上 | 单次 Tool 超时后不挂起 Orchestrator |
| 5.3 | 实现 webfetch：url(必填)、method、headers；返回清洗后文本；allowed_domains/禁止内网 | `internal/tools/webfetch.go` | Agent 可调用并得到网页文本 |
| 5.4 | 实现 rest_api：url、method、headers、body；安全与超时同 design 6.2 | `internal/tools/rest_api.go` | Agent 可调用并得到响应体 |
| 5.5 | 实现 websearch：基于 DuckDuckGo，query(必填)；返回摘要与链接列表；超时与安全同 design 6.3 | `internal/tools/websearch.go` | Agent 可调用并得到搜索结果 |
| 5.6 | MCP 配置解析与连接：mcp.*.type=local/remote，启动时拉取 tools，以 mcp_name+tool_name 注册 | `internal/registry/mcp.go` 或内置于 Tool Registry | MCP 暴露的 Tool 可被 Agent 调用 |

**阶段五验收**：Agent 可调用 webfetch、rest_api、websearch 及 MCP 工具并得到返回；Tool 超时后返回错误、流程继续。

---

### 阶段六：context_compress、中期记忆与 entity_extract

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 6.1 | 实现 context_compress：输入 messages、context_limit、threshold_ratio；超限时调用 LLM 做摘要+抽取要点细节（design 8.7、6.4） | `internal/tools/context_compress.go` | 压缩后 messages 变短且保留关键信息 |
| 6.2 | 触发条件：current_tokens/context_limit >= threshold_ratio 时在组装 messages 前调用 | Session/Orchestrator | 超阈值时触发压缩 |
| 6.3 | 实现 user_task_memory 表（见 2.4）；task_complete 时写入 success；失败/中断时可写 failure | migrations + 写入逻辑 | 中期记忆可按 user_id 查询 |
| 6.4 | 实现 entity_extract(user_message, task_state?) → entities（规则或轻量 LLM，输出 design 5.8.3 JSON） | `internal/orchestrator/entity_extract.go` 或内置 | 输出含 entities、confidence |
| 6.5 | Orchestrator 步骤 5.5：组装 system 前调用 entity_extract；用实体对 user_task_memory 筛选，注入中期记忆块 | 在 Orchestrator 组装 system 处 | 按实体筛选后的中期记忆注入提示 |

**阶段六验收**：短期/短期+中期 Token 超阈值时触发压缩；压缩为摘要+要点；实体提取后按实体筛选注入中期记忆。

---

### 阶段七：端到端与双轨协议、隐式暂停

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 7.1 | 实现 5.8.0 双轨输出解析：从 LLM 输出中解析单段 JSON，含 assistant_response、agent_action（task_operation、tool_calls） | `internal/orchestrator/response_parser.go` | 解析成功则执行 task_operation 与 tool_calls |
| 7.2 | 根据 agent_action.task_operation 调用 task_start/pause/resume/complete/cancel，再继续 tool_calls 循环 | Orchestrator | 行为与 design 5.8.1 对应关系一致 |
| 7.3 | 新会话：加载前次未完成任务与本次自动取消任务，注入 TASK_CONTEXT；首条回复提醒用户选择继续/取消 | 同上 | 新会话首条不先 start，先提醒未完成 |
| 7.4 | 隐式暂停（可选）：存在 active_task 且未调用 task_pause 时，若回复与当前 Skill 不符则自动 pause（design 10.1 步骤 9.5） | 同上 | 可配置开关；检测到跑题可自动暂停 |
| 7.5 | **主程序 main.go**：入口为 `main.go`，默认启动后通过 adapter 层与飞书等对接；由命令行参数（如 `-cli`）控制是否进入 CLI 交互 | `main.go`、`internal/adapter/feishu.go` 等 | 默认 `go run .` 启动主程序、挂载配置的 Adapter；不加 `-cli` 不进入终端读行 |
| 7.6 | **CLI 交互（可选）**：实现 CLI Adapter（`internal/adapter/cli.go`），当带 `-cli` 时从 stdin 读行、调用 Orchestrator、将回复写回 stdout；支持 exit/quit、可选 /new | `internal/adapter/cli.go` | 运行 `go run . -cli` 后可多轮输入输出，便于测试 |
| 7.7 | 主程序支持 `-config`、`-user`、`-new-session`、`-verbose` 等（可选）；`-cli` 仅在此处控制是否进入命令行交互 | `main.go` | 命令行参数生效，默认主程序、可选 CLI |

**阶段七验收**：自由对话 + Agent 决策；跑题→task_pause 或后验暂停；回归→task_resume；新会话提醒未完成；**默认主程序通过 adapter 对接飞书**；**带 `-cli` 时可进入 CLI 窗口多轮对话测试**。

---

### 阶段八：长期记忆（可选）

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 8.1 | 知识库接口：向量检索 + BM25（或仅向量），按 query/实体过滤，返回片段与 Token 上限截断 | `internal/memory/long_term.go` | 与任务/实体最匹配的内容可注入 |
| 8.2 | Orchestrator 组装 system 时：用 entity/query 检索长期记忆，追加长期记忆块 | 同上 + Orchestrator | 长期记忆块出现在 system 中 |
| 8.3 | 可选：暴露 memory_retrieve_long 给 Agent，按需拉取 | Tool Registry | Agent 可调用并得到检索结果 |

**阶段八验收**：长期记忆按需注入；可选 memory_retrieve_long 可用。

---

### 阶段九：业务 Skill 与 Rule Engine（可选）

| 序号 | 任务描述 | 产出/路径 | 验收 |
|------|----------|-----------|------|
| 9.1 | 实现至少一个 Skill（如 work-log）：SKILL.md 含 name、description、步骤说明；遵守 5.8.0.1 输出模板 | `config/skills/work-log/SKILL.md` 等 | Agent 调用后按 Skill 引导完成流程 |
| 9.2 | task.state 在任务内更新与恢复时注入 | 已由 TSM + 组装逻辑覆盖 | 恢复时 state 正确 |
| 9.3 | 与 records Rule Engine 集成（若需要）：Skill 内可调用规则或子流程 | 按 records 约定接入 | 按 design 11 可复用 records |

**阶段九验收**：至少一个 Skill 端到端可用；task.state 与恢复正确。

---

## 3. 模块与目录结构（Go 实现）

**技术栈**：Go 1.22+，YAML 配置，OpenAI 兼容 API。项目根为 `crm/`，模块名为 `crm`。

```
crm/
  design.md                 # 设计说明（已有）
  impl.md                   # 本实现文档
  go.mod
  go.sum
  main.go                   # 主程序入口（见 3.1：默认 adapter 对接，-cli 时进入命令行交互）
  config/                   # 配置目录（主配置文件与扩展资源）
    config.yml              # 主配置文件（design 4.1 对应）
    prompts/                # 提示词 .md/.yml/.txt
    skills/                 # skills/<name>/SKILL.md
    agents/                 # agents/<name>.md
  internal/                 # 内部代码（含 Tool 实现）
    config/
      loader.go             # YAML 加载、合并、CRM_CONFIG_CONTENT 覆盖
      vars.go               # {env:}, {file:}, {prompt:} 变量替换
    llm/
      openai.go             # OpenAI 兼容 HTTP 客户端
      tokens.go             # Token 估算
    models/                 # 实体与 DTO（与 design 第 9 章一致）
      user.go
      session.go
      task.go
      message.go
      user_task_memory.go
    session/
      context.go            # Session & Context Manager
    task/
      state_manager.go      # Task State Manager
    registry/
      tool_registry.go      # Tool + MCP 注册与按名执行
      skill_registry.go     # Skill 扫描与按 skill_id 取内容
      agent_registry.go     # Agent 配置与 default_agent
      mcp.go                # MCP 连接与 tools 注册（可选）
    tools/                  # Tool 实现（全部放在 internal 下）
      task_tools.go         # task_start/pause/resume/complete/cancel
      webfetch.go
      rest_api.go
      websearch.go         # 基于 DuckDuckGo 的网页搜索
      context_compress.go
      task_naming.go        
    orchestrator/
      orchestrator.go       # 单轮编排
      tool_runner.go        # tool_calls 执行与超时
      response_parser.go    # 5.8.0 双轨 JSON 解析
      entity_extract.go     # 实体提取
    memory/
      medium_term.go        # user_task_memory 读写与按实体筛选
      long_term.go          # 知识库检索（可选）
    adapter/
      types.go              # 统一入参：user_id, session_id, message, channel
      feishu.go             # 飞书 Bot 对接（飞书机器人 WebSocket 接口，默认主程序通过 adapter 层接入）
      web.go                # WebSocket 对接，OAuth2 验证（可选）
      cli.go                # 命令行窗口交互 Adapter（仅 -cli 时启用，便于测试）
  migrations/               # SQL 迁移（或 embed schema）
    *.sql
```

- **主配置文件**：`config/config.yml`，放在 **config 目录**下；加载时支持环境变量 `CRM_CONFIG` 指定路径、`CRM_CONFIG_CONTENT` 覆盖。
- **Tool**：所有 Tool 实现（task_tools、webfetch、rest_api、websearch、context_compress、task_naming 等）均放在 **internal/tools/** 下。
- **主程序入口**：**`crm/main.go`**。默认情况下启动主程序，通过 **adapter 层**与飞书等渠道对接；仅当通过**命令行参数**（如 `-cli`）显式指定时，才进入命令行交互窗口，用于本地测试。

### 3.1 主程序与 Adapter 层（默认）

- **入口**：`main.go`（项目根 `crm/main.go`）。
- **默认行为**：启动后不进入 CLI，而是根据配置或命令行选择**挂载的 Adapter**，与外部渠道对接：
  - **飞书**：`internal/adapter/feishu.go`，通过飞书机器人 WebSocket 接口接收消息、解析 user_id/session_id、调用 Orchestrator、将回复回写飞书。
    - **飞书 WebSocket 对接参考（官方）**：[使用长连接接收事件](https://open.feishu.cn/document/server-docs/event-subscription-guide/event-subscription-configure-/request-url-configuration-case)。
  - **Web**：`internal/adapter/web.go`（可选），WebSocket 协议，OAuth2 验证。
- 各 Adapter 将渠道协议转为统一入参（user_id、session_id、message、channel），交给同一 Orchestrator，回写时通过 channel 将助手回复发回对应渠道。
- 运行示例：`go run .` 或 `./crm`，可配合 `-config config/config.yml` 等参数；具体挂载哪些 Adapter 由配置或启动参数决定。

### 3.2 命令行窗口交互（CLI，可选）

为便于本地测试与联调，提供**命令行窗口交互**：仅当传入**命令行参数**（如 `-cli`）时，主程序进入 CLI 模式，在终端中逐行输入用户消息，将助手回复打印到 stdout。

**进入方式：**

- 入口仍为 `main.go`，通过参数控制模式。例如：`go run . -cli` 或 `./crm -cli`。
- 未带 `-cli` 时，按默认主程序逻辑启动，通过 adapter 层与飞书等对接，**不**进入终端读行循环。

**CLI 模式下的交互流程：**

1. 检测到 `-cli` 后，加载 `config/config.yml`，初始化 LLM、Registry、Session/Task 等，然后**仅**使用 **CLI Adapter**（`internal/adapter/cli.go`）。
2. 为当前终端固定测试用 **user_id**（如 `cli-{ulid}`），可选 `-user` 指定；**session_id** 单次运行固定或通过 `-new-session` 新建。
3. 循环：打印提示符 → 从 stdin 读一行 → 若为 `exit`/`quit`/`/q` 则退出；可选 `/new` 新会话；将 (user_id, session_id, message) 经 CLI Adapter 交 Orchestrator，把回复打印到 stdout。

**CLI Adapter 契约**：与 design 5.0.3 一致，输出 user_id、session_id、message、channel（回写即打印到终端）。

**测试建议：** 带 `-cli` 运行后可多轮输入输出，验证 task_start、Skill 注入、跑题/回归等；可选 `-verbose` 打印 task_operation/tool_calls；支持 `@work-log` 等快捷 task_start（design 10.4）。

---

## 4. 数据模型与持久化（Schema 摘要）

以下与 design.md 第 9 章一致，实现时可直接转为 DDL。

### 4.1 user

| 字段 | 类型 | 说明 |
|------|------|------|
| id | ulid | 主键 |
| external_id | string nullable | 外部用户 ID，唯一 |
| name | string nullable | 显示名 |
| avatar | string nullable | 头像 URL |
| status | enum | active |
| created_at | timestamp | |
| updated_at | timestamp | |
| metadata | jsonb | 扩展 |

**唯一约束**：external_id（或 (source, external_id)）。

### 4.2 session

| 字段 | 类型 | 说明 |
|------|------|------|
| id | ulid | 主键 |
| user_id | ulid | FK user.id |
| status | enum | active, closed |
| title | string nullable | |
| created_at | timestamp | |
| updated_at | timestamp | |
| metadata | jsonb | 扩展 |

**索引**：user_id, (user_id, updated_at)。

### 4.3 task

| 字段 | 类型 | 说明 |
|------|------|------|
| id | ulid | 主键 |
| session_id | ulid | FK session.id |
| skill_id | string | 如 work-log |
| title | string nullable | task_naming 写入 |
| status | enum | processing, paused, interrupted, completed, cancelled |
| state | jsonb | 任务中间数据 |
| started_at | timestamp | |
| paused_at | timestamp nullable | |
| completed_at | timestamp nullable | |
| cancelled_at | timestamp nullable | |
| cancelled_reason | string nullable | user, expired |
| metadata | jsonb | 扩展 |

**约束**：同一 session_id 下同时最多一条 status=processing。  
**索引**：(session_id, status)。

### 4.4 message

| 字段 | 类型 | 说明 |
|------|------|------|
| id | ulid | 主键 |
| session_id | ulid | FK session.id |
| task_id | ulid nullable | null=会话级 |
| role | enum | user, assistant, system, tool |
| content | text | |
| tool_calls | jsonb nullable | |
| tool_call_id | string nullable | |
| created_at | timestamp | |

**索引**：session_id, task_id。

### 4.5 user_task_memory（中期记忆）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | ulid | 主键 |
| user_id | ulid | FK user.id |
| task_id | ulid nullable | |
| session_id | ulid nullable | |
| skill_id | string | |
| result_type | enum | success, failure |
| summary | text | |
| failure_step | string nullable | |
| created_at | timestamp | |
| metadata | jsonb nullable | 含 entity_id/entity_type 等 |

**索引**：user_id, (user_id, created_at)，及按 metadata 的实体筛选（视存储支持）。

---

## 5. 配置与占位符约定

### 5.1 配置键（与 design 4.4 对齐）

- **llm**：baseURL, apiKey, name, temperature, max_tokens, top_p, timeout, limit.context, limit.output
- **compaction**：auto, prune, threshold_ratio
- **task**：expire_days(3), remind_days(2)
- **user**：rate_limit_per_min, max_sessions_per_user, max_concurrent_per_user
- **permission**：webfetch, rest_api, websearch, skill.* 等
- **default_agent**：字符串
- **agent**：<agent_id>: { description, prompt, model?, tools, permission? }
- **skills**：数组限制启用 Skill
- **mcp**：<name>: { type, url?|command?, enabled?, ... }

### 5.2 提示词占位符（组装时替换）

- **{{SKILL_LIST}}**：Skill 元数据列表 + 调用方式 task_start(skill_id)
- **{{TASK_CONTEXT}}**：当前任务、已暂停、可恢复、已自动取消
- **{{ACTIVE_TASK}}**、**{{PAUSED_TASKS}}**、**{{PENDING_FROM_PREV_SESSION}}**、**{{AUTO_CANCELLED}}**（或合并为 TASK_CONTEXT）
- **{{MEDIUM_MEMORY}}**：按实体筛选后的中期记忆块
- **{{LONG_MEMORY}}**：长期记忆检索结果块
- **{{MESSAGES}}**：context_compress 时待压缩内容

---

## 6. 接口契约摘要（实现时必守）

### 6.1 Adapter 输出（给 Orchestrator）

- **user_id**：ulid，必填
- **session_id**：ulid | null（null 表示新会话）
- **message**：{ content, idempotency_key? }
- **channel**：回写通道（协议相关）

### 6.2 Session & Context Manager

- get_or_create_session(user_id, session_id?) → Session
- load_messages(session_id, task_id?) → Message[]（task_id null=会话级）
- append_message(session_id, task_id?, message) → void
- token_estimate(messages) → number

### 6.3 Task State Manager

- get_active_task(session_id) → Task | null
- get_paused_tasks(session_id) → Task[]
- get_pending_tasks_from_prev_session(user_id, current_session_id) → Task[]

### 6.4 task 工具（原子执行）

- task_start({ skill_id }) → 新 task 信息
- task_pause() → void
- task_resume({ skill_id } | { task_id }) → 恢复的 task
- task_complete({ skill_id, result? }) → void
- task_cancel({ task_id }) → void

### 6.5 LLM 双轨输出（5.8.0）

- 单段 JSON：`assistant_response` + `agent_action`（intent, task_operation, tool_calls, memory_write?, confidence）
- task_operation.type：start | pause | resume | complete | cancel | none
- Orchestrator 解析后执行 task_* 与 tool_calls，不依赖自然语言解析

### 6.6 Tool 执行

- execute(args, context) → string（或 Promise<string>）
- context：user_id, session_id, message_id, agent?, worktree?
- 每次调用带超时（如 60s），超时返回错误并继续流程

---

## 7. 单轮流程（Orchestrator）步骤核对

实现时按 design 10.1 顺序实现并勾选：

1. [ ] Adapter 解析 user_id（未识别则拒绝）
2. [ ] get_or_create_session；新会话时：超时取消、加载前次未完成、注入可恢复与已取消
3. [ ] 按 active_task 加载消息：无则会话级；有则混合策略（全局 2–3 轮 + 任务历史 + 关键摘要）
4. [ ] 加载 active_task、paused_tasks
5. [ ] 可选 @skill_id 解析 → 直接 task_start(skill_id)
6. [ ] Token 统计；超限触发 context_compress
7. [ ] **5.5** entity_extract → 按实体筛选中期记忆、构造长期检索 query
8. [ ] 构建 system：Agent.prompt + {{SKILL_LIST}} + {{TASK_CONTEXT}} + [SKILL.md+state] + 中期块 + 长期块
9. [ ] 获取 tools 列表（task_*、webfetch、rest_api、websearch、MCP…）
10. [ ] 调用 LLM；解析双轨 JSON；执行 task_operation 与 tool_calls 循环（Tool 超时保护）
11. [ ] **9.5** 隐式暂停：有 active_task 且未 task_pause 且回复与 Skill 不符 → 自动 pause
12. [ ] 持久化 messages（带 task_id）、更新 task，返回回复给 Adapter

---

## 8. 验收检查表（与 design 2.4 对齐）

| 验证项 | 预期 | 阶段 |
|--------|------|------|
| 配置加载 | 读取 config/config.yml，变量替换生效，CRM_CONFIG_CONTENT 可覆盖 | 一 |
| Agent 与 LLM | 请求到达 OpenAI 兼容端点，temperature/max_tokens 透传正确 | 一 |
| 能力调用同级 | Agent 可调用 Skill（task_start 绑定）、Tool、MCP；Tool/MCP 原子执行 | 四、五 |
| 任务切换 | task_start/pause/resume/complete/cancel 正确更新状态与 active_task | 三、七 |
| 新会话 | 加载前次未完成，超 3 天取消，提醒继续/取消 | 三、七 |
| 多用户 | 不同 user_id 的 Session/Task/Message 互不交叉 | 二 |
| Skill 注入 | active_task.skill_id 对应时，SKILL.md 注入 Agent 上下文 | 四 |
| 压缩触发 | 短期/短期+中期超阈值触发 context_compress；摘要+要点细节 | 六 |
| 主程序默认行为 | 运行 `go run .` 或 `./crm` 时通过 adapter 层与飞书等对接，不进入 CLI | 七 |
| CLI 交互测试 | 运行 `go run . -cli` 后可从终端多轮输入、收到助手回复；支持 exit/quit、可选 /new | 七 |

---

## 9. 参考

- 设计说明：`crm/design.md`
- 能力与路由：design 2.1、2.5
- 核心组件 JSON：design 5.0.2
- 提示词与 5.8.0 双轨协议：design 5.7、5.8
- 数据模型详表：design 第 9 章
- 记忆与压缩：design 第 8 章、6.4、8.7
