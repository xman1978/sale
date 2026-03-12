# 无向量知识问答系统 — Go 实现文档（impl.md）

本文档依据 [design.md](./design.md) 中的系统架构与算法，规划 **Go 语言** 下的实现阶段与编码规范，用于指导代码编写与迭代。
实施建议：实施前用 writing-plans 将各阶段拆为 2–5 分钟可执行任务；执行时用 executing-plans 或 subagent 逐任务实现并校验。
---

# 1. 文档说明与设计依据

| 项目 | 说明 |
|------|------|
| 设计文档 | [knowledge/design.md](./design.md) |
| 编码语言 | Go（≥ 1.21） |
| 架构对应 | 设计文档第 3 节「系统总体架构」 |
| API 接口设计 | 设计文档第 31 节「API 接口设计」（OAuth2、路由、请求/响应约定） |
| Web 页面设计 | 设计文档第 32 节「Web 页面交互设计」（查询、对话、执行过程、上传、文档信息） |

实现顺序按 **自底向上、先存储后业务**：Storage → Processing Pipeline（Converter、Chunker、Relation、Insight、Compressor）→ Knowledge Graph 读写 → Query Engine → API（§31）→ 后台任务与运维；Web 前端（§32）可随 API 完成后对接或独立迭代。



---

# 2. Go 项目结构

推荐目录布局（与设计分层对应）：

```text
knowledge/                    # 本系统根目录（或置于 repo 子模块）
├── main.go                   # 主进程入口：API + 后台任务
├── config.yml                # 主配置文件（敏感项可用环境变量覆盖）
├── prompt.yml                # LLM 提示词（对齐 design.md §30：图片转 Markdown、Intent Parser、PageIndex 修复、节点摘要、节点评分、问答、知识压缩、历史会话压缩、查询重写、文档大纲生成、关系识别、Tag 提取、Insight 生成 等）
├── internal/
│   ├── config/               # 配置加载（环境变量、YAML）
│   ├── ingest/               # 接入层：上传、路由到 Converter
│   ├── converter/            # 文档转换：PDF/Word/HTML/图片 → Markdown
│   ├── chunker/              # PageIndex 分块：TOC、结构解析、语义分割、节点与摘要
│   ├── relation/             # 关系构建：候选、LLM 关系识别、写入 node_relations
│   ├── insight/              # 洞察生成（可选先 stub）
│   ├── compressor/           # 对话历史压缩（可选先 stub）
│   ├── graph/                # 图读写封装：BFS 扩展、邻居查询（基于 storage）
│   ├── query/                # Query Engine
│   │   ├── intent.go         # Intent Parser
│   │   ├── retrieval.go      # Metadata + Graph + Keyword 检索编排
│   │   ├── rank.go           # LLM Ranking
│   │   └── answer.go         # Answer Generation
│   ├── api/                  # HTTP API Layer
│   └── worker/               # 后台任务：关系构建、洞察、对话压缩
├── storage/                  # 存储层（与 pkg 同级）
│   ├── postgres/             # PostgreSQL：documents, nodes, node_relations
│   └── docs/                 # Markdown 文档文件 storage/docs/{doc_id}.md
├── pkg/
│   ├── logger/               # 统一日志
│   ├── llm/                  # LLM 客户端抽象（OpenAI/兼容 API）
│   └── uuid/                 # 或使用 github.com/google/uuid
├── logs/                     # 日志文件存储目录
├── pages/                    # Web 静态页面统一存储
├── migrations/               # SQL 迁移（documents, nodes, node_relations）
├── go.mod
├── go.sum
├── config.yml.example        # 配置示例（复制为 config.yml 使用）
└── README.md
```

**设计对应关系**：

| 设计文档模块 | 实现路径 |
|--------------|----------|
| Storage Layer（PostgreSQL + Markdown） | `storage/postgres` + `storage/docs` |
| Processing Pipeline | `converter`、`chunker`、`relation`、`insight`、`compressor` |
| Knowledge Graph（nodes/edges 读写） | `internal/graph` + `storage/postgres` |
| Query Engine | `internal/query` |
| API Layer（design §31） | `internal/api`：OAuth2、`/api/v1/*` 路由、统一响应与错误码 |
| Ingest Layer | `internal/ingest` |
| 后台任务 | `internal/worker` |
| Web 页面（design §32） | `pages/`：静态页面统一存放；检索/问答/执行过程/上传/文档信息页 |

---

# 3. Go 编码规范

## 3.1 通用约定

- **风格**：遵循 [Effective Go](https://go.dev/doc/effective_go) 与 `gofmt`，使用 `golangci-lint`（或项目既定 linter）做 CI。
- **包名**：小写单词，无下划线；`internal/` 下包仅本项目使用，不对外暴露。
- **错误**：使用 `fmt.Errorf("...: %w", err)` 包装，业务可定义 `internal/errors` 中的 sentinel 或类型以便 `errors.Is`/`errors.As`。
- **上下文**：IO、RPC、LLM 调用等接收 `context.Context`，并传递取消与超时。
- **并发**：需并发时用 `errgroup` 或显式 goroutine + channel，避免裸 `go` 无回收；LLM 调用注意限流（如 semaphore 或 worker 池）。

## 3.2 日志与配置

- **日志**：统一通过 `pkg/logger` 或 `log/slog` 输出，包含 request_id/ doc_id 等关键 ID，不直接使用 `log.Printf`。
- **配置**：集中从 `internal/config` 读取；敏感信息（DB URL、API Key）从环境变量或密钥服务读取，不入库。
- **LLM 提示词**：所有 LLM 调用的 system/user 提示词单独放在根目录 `prompt.yml` 中（按场景分 key，如 intent_parser、ranking、answer、relation_recognize、outline_fallback 等），由 `internal/config` 或 `pkg/llm` 加载；代码中不硬编码长段 prompt 文本。

## 3.3 数据库与事务

- **驱动**：使用 `github.com/jackc/pgx/v5`（或项目已选型）访问 PostgreSQL。
- **事务**：多表写入（如 document + nodes + relations）在同一事务中完成，失败整体回滚；只读查询尽量短事务或无事务。

## 3.4 测试

- **单元测试**：核心逻辑（分块、层级推断、BFS、评分公式）必须有单元测试，表驱动优先。
- **集成测试**：Storage、Query Engine 与 DB 的集成用 testcontainers 或内嵌 DB，避免依赖外部实例。
- **Mock**：LLM 与外部服务通过接口注入，便于 mock（如 `pkg/llm.Interface`）。

---

# 4. 实现阶段规划

以下阶段与 design.md 中的架构与章节一一对应，按顺序实现可减少返工。

---

## 阶段 0：基础与存储层（对应设计 §3 存储、§8 数据库）

**目标**：可运行的进程骨架、配置、日志、PostgreSQL 表结构、Markdown 文档存储抽象。

**任务**：

| 序号 | 任务 | 设计依据 | 交付 |
|------|------|----------|------|
| 0.1 | 初始化 Go 模块与目录结构 | §2 项目结构 | `go.mod`、根目录 `main.go`、`internal/` 与 `storage/`、`pkg/`、`logs/` 占位 |
| 0.2 | 配置与日志 | §17 可靠性（日志） | `internal/config`、`pkg/logger`，支持 YAML + 环境变量 |
| 0.3 | PostgreSQL 迁移 | §8.2–8.4 表结构 | `migrations/` 下 `documents`、`nodes`、`node_relations` 建表及索引（含 GIN(tags)、from_node/to_node） |
| 0.4 | 存储接口与实现 | §8 ER、§16 性能（索引） | `storage/postgres`：Document/Node/Relation 的 CRUD；`storage/docs`：按 `storage/docs/{doc_id}.md` 读写 Markdown 文件 |

**验收**：启动服务可连接 DB、执行迁移；能插入一条 document 与对应 nodes，并能从文件系统读写 `{doc_id}.md`。

---

## 阶段 1：接入与文档转换（对应设计 §4 文档接入、§4.2 文档转换）

**目标**：支持 PDF/Word/HTML/图片/Markdown 接入，并统一输出为 Markdown 落盘。

**任务**：

| 序号 | 任务 | 设计依据 | 交付 |
|------|------|----------|------|
| 1.1 | Ingest 入口 | §3 Ingest Layer | `internal/ingest`：接收上传或 URL，按 `original_format` 路由到对应 Converter |
| 1.2 | PDF 转换 | §4.1（转图片后多模态） | 使用 Go 调用多模态模型或外部服务，输出 Markdown；或集成现有 PDF→文本库再转 MD |
| 1.3 | Word/HTML 转换 | §4.1（go 开源库） | 选用 Go 库（如 unidoc、golang.org/x/net/html）转 Markdown，落盘为 `storage/docs/{doc_id}.md` |
| 1.4 | 图片转换 | §4.1 多模态 | 与 PDF 共用一个多模态调用路径，输出说明性 Markdown |
| 1.5 | Markdown 直通 | §4.1 | 直接写入 `storage/docs/{doc_id}.md`，并写入 `documents` 表一条记录 |

**验收**：每种格式至少一个样本能入库并在 `storage/docs/{doc_id}.md` 得到合法 Markdown；`documents` 表有对应 `doc_id`、`original_format`、`ingest_ts`。

---

## 阶段 2：PageIndex 分块（对应设计 §5 文档分块、§19 结构化分块、§22 自动目录）

**目标**：从 Markdown（或 TOC+正文）生成章节树与节点，写入 `nodes` 表并生成 short/long summary。

**任务**：

| 序号 | 任务 | 设计依据 | 交付 |
|------|------|----------|------|
| 2.1 | 结构解析（Markdown # / ## / ###） | §19.3、§22.4 编号模式 | `internal/chunker/structure.go`：解析标题层级，支持阿拉伯数字、英文/罗马、中文数字等编号模式 |
| 2.2 | TOC 检测与兜底大纲 | §22.2–22.3、§22.5.1 | `internal/chunker/toc.go`：TOC 检测（前 N 页）+ 结构提取；无 TOC 时调用 LLM 生成全文大纲（标题尽量原文），再建树 |
| 2.3 | 语义分割 | §19.4 | `internal/chunker/semantic.go`：段落 > 2000 字时按句边界拆分，保留完整句子与上下文 |
| 2.4 | 节点生成与 importance | §5.2、§19.5–19.6、§8.3 | 每个块生成 node（title, short_summary, long_summary, importance, page_start/page_end, char_start/char_end）；importance 按标题权重+关键词密度+层级计算 |
| 2.5 | 摘要生成 | §19.2、§14 小模型 | 使用 small LLM 生成 short_summary/long_summary，写入 `nodes` |
| 2.6 | 与存储对接 | §5.1 流程 | 分块流程：读取 doc → 解析结构/TOC → 语义分割 → 生成节点 → 写 DB；文档 > 500 字触发分块（设计 §5） |

**验收**：给定一份 Markdown，能生成一棵节点树并写入 `nodes`，且 `parent_id`、`page_start`/`page_end`、`importance`、摘要均符合设计；支持无 TOC 时通过 LLM 大纲兜底建目录。

---

## 阶段 3：知识关系与图谱写入（对应设计 §6 关系系统、§20 关系构建算法）

**目标**：在节点基础上按标签/同文档等缩小候选，用 LLM 识别关系并写入 `node_relations`。

**任务**：

| 序号 | 任务 | 设计依据 | 交付 |
|------|------|----------|------|
| 3.1 | 候选节点生成 | §20.3 | `internal/relation/candidate.go`：按 tags 重叠、同 doc_id、关键词等查询，每批约 200（LIMIT 200） |
| 3.2 | 关系识别（LLM） | §20.4、§6.1 关系类型 | `internal/relation/recognize.go`：输入 nodeA/nodeB 的 short_summary，调用 small LLM，输出 relation_type（same_topic/supports/contradicts/references/elaborates/summary_of）与 confidence |
| 3.3 | 关系过滤与写入 | §20.5、§8.4 | confidence > 0.6 才写入 `node_relations`；批量 insert，事务提交 |
| 3.4 | 批处理与后台任务 | §20.6–20.7、§15 后台 | `internal/worker/relation.go`：每批约 100 nodes，定时（如 30 分钟）或事件触发，调用 relation 构建流程 |

**验收**：对已有 nodes 运行一次关系构建，能生成并持久化 `node_relations`；关系类型与设计 §6.1 一致，且带 confidence/evidence。

---

## 阶段 4：图遍历与 Query Engine 基础（对应设计 §9–§11、§21 三层检索、§29 Query Engine）

**目标**：实现 BFS 图扩展、Metadata 检索、Keyword 检索，以及 Query Engine 编排（不含 LLM Ranking/Answer 可在阶段 5 接齐）。

**任务**：

| 序号 | 任务 | 设计依据 | 交付 |
|------|------|----------|------|
| 4.1 | 图读写与 BFS | §10、§21.4、§29.5 | `internal/graph/expand.go`：BFS depth=2，max_neighbors=50，基于 `node_relations` 查邻居；使用 §23 的索引（from_node/to_node） |
| 4.2 | Metadata 检索 | §21.3、§29.4 | `internal/query/metadata.go`：按 tags（数组重叠）、importance 排序，LIMIT 200，得到 seed nodes |
| 4.3 | Keyword 检索 | §29.2 并行三路 | `internal/query/keyword.go`：对 title/short_summary 做关键词匹配（或简单全文），返回节点 ID 列表 |
| 4.4 | Intent Parser | §29.3 | `internal/query/intent.go`：调用 LLM 从用户问题解析 intent、keywords、tags（JSON 输出） |
| 4.5 | 检索编排与去重 | §24.1–24.3、§29.6 | `internal/query/retrieval.go`：并行执行 Tag(Metadata)/Graph/Keyword 三路，合并去重，得到候选池（约 300 节点） |

**验收**：给定 query，能解析出 intent/tags/keywords；能通过 Metadata + Graph + Keyword 得到合并去重后的候选节点列表。

---

## 阶段 5：LLM Ranking 与 Answer Generation（对应设计 §11 评分、§21.5–21.6、§29.7–29.9）

**目标**：对候选节点做相关性评分，取 Top 20，再调用大模型生成可解释回答。

**任务**：

| 序号 | 任务 | 设计依据 | 交付 |
|------|------|----------|------|
| 5.1 | LLM Ranking | §11、§29.7 | `internal/query/rank.go`：输入 query + 候选的 short_summary/tags/importance，small LLM 输出 score(0–1)；按 Score = 0.4*relevance + 0.2*importance + 0.2*relation_score + 0.2*recency 综合（或先实现 relevance，其余用 DB 字段） |
| 5.2 | TopK 控制 | §29.8 | 取 ranked 前 20 个节点（可配置），避免上下文爆炸 |
| 5.3 | Answer Generation | §29.9 | `internal/query/answer.go`：大模型输入 query + top 节点的 long_summary/source，输出回答 + 引用 node_id + 推理说明；Prompt 约束「仅用提供节点、必须引用 node_id」 |
| 5.4 | Query Engine 串联 | §9、§21.7 | `internal/query/engine.go`：Intent → Metadata/Graph/Keyword 检索 → 合并 → Rank → Top20 → Answer，返回最终回答与引用列表 |

**验收**：端到端一次 query 能得到：解析 intent、候选池、评分、Top20、最终回答及引用节点；回答可解释（含 node_id/source）。

---

## 阶段 6：API 与后台任务（对应设计 §3 API Layer、§15 后台任务、§31 API 接口设计）

**目标**：对外 HTTP API（符合 design §31）、后台定时/事件任务（关系构建、洞察、对话压缩）；可选交付 Web 前端（§32）。

**任务**：

| 序号 | 任务 | 设计依据 | 交付 |
|------|------|----------|------|
| 6.1 | API 路由与 OAuth2 鉴权 | §18、§31.1–31.2 | `internal/api`：Base URL `/api/v1/`；OAuth2 token 端点 `POST /oauth/token`（授权码/客户端凭证）；请求头 `Authorization: Bearer <access_token>` 校验；HTTPS、token 不落日志 |
| 6.2 | 接口与统一响应 | §31.3–31.4 | 文档上传 `POST /api/v1/documents`、文档详情 `GET /api/v1/documents/{id}`、问答 `POST /api/v1/query`、流式 `POST /api/v1/query/stream`、对话列表 `GET /api/v1/conversations`；统一响应 `code/message/data`；401/403 及分页 `page/page_size/total` |
| 6.3 | 文档上传与入库 | §4、阶段 1 | 调用 Ingest + Converter，写 `documents` 与 `storage/docs/{doc_id}.md`，并触发分块（或异步） |
| 6.4 | 问答接口 | §9、阶段 5 | 接收 query（及可选 conversation_id），调用 Query Engine，返回 answer + 引用节点列表；可选返回中间步骤（§32 执行过程展示） |
| 6.5 | 后台 Worker | §15 | `internal/worker`：关系构建（30 分钟）、洞察生成（2 小时）、对话压缩（实时或按需）；可选用 cron 或任务队列 |
| 6.6 | Web 前端（可选） | §32 | `pages/` 检索/知识浏览、对话问答、执行过程面板、文档上传、文档列表与详情页；与上述 API 对接，OAuth2 登录 |

**验收**：通过 API 上传文档并触发分块；通过 API 发起问答并得到带引用的回答；鉴权与响应格式符合 §31；后台任务可按配置周期执行且不阻塞 API；若做 Web，五大功能可正常使用。

---

## 阶段 7：可靠性与安全（对应设计 §17 可靠性、§18 安全、§16 性能）

**目标**：任务失败重试、事务一致性、LLM fallback、审计日志、性能与缓存。

**任务**：

| 序号 | 任务 | 设计依据 | 交付 |
|------|------|----------|------|
| 7.1 | 任务重试与限流 | §17 | 关系构建/洞察等失败入重试队列；LLM 调用限流与超时控制 |
| 7.2 | 事务与一致性 | §17 | 多表写入同一事务；关键路径审计日志（谁在何时操作了哪条数据） |
| 7.3 | LLM 降级 | §17 | 大模型不可用时 fallback 策略（如仅返回检索到的节点摘要，不生成长答） |
| 7.4 | 安全与审计 | §18 | 文档权限校验、API 鉴权、敏感配置不入库、访问审计日志 |
| 7.5 | 性能与缓存 | §16、§29.12 | short_summary 检索、relation 索引已用；可增加结果缓存（query hash → answer，TTL 5min）、邻居缓存等 |

**验收**：关键操作有审计日志；单次查询延迟与设计目标（如 Metadata 5ms、Graph 10ms、Ranking 200ms、Answer 1s）可测量并可优化。

---

# 5. 配置与常量（与设计对齐）

建议在 `internal/config` 或根目录 `config.yml` 中集中定义，便于调优：

| 配置项 | 设计依据 | 建议默认 |
|--------|----------|----------|
| `toc_check_page_num` | §22.3 | 20 |
| `max_page_num_each_node` | §22.3 | 10 |
| `max_token_num_each_node` | §22.3 | 20000 |
| `chunk_trigger_min_chars` | §5 | 500 |
| `semantic_split_max_chars` | §19.4 | 2000 |
| `graph_expand_depth` | §10、§29.5 | 2 |
| `graph_max_nodes` | §10 | 500 |
| `graph_max_neighbors` | §29.5、§23.3 | 50 |
| `metadata_seed_limit` | §21.3、§29.4 | 200 |
| `candidate_pool_size` | §29.6 | 300 |
| `ranking_top_k` | §29.8、§21.7 | 20 |
| `relation_confidence_threshold` | §20.5 | 0.6 |
| `relation_batch_size` | §20.7 | 100 |
| `conversation_compress_token_threshold` | §13 | 4096 |

---

# 6. 实现阶段总览

| 阶段 | 名称 | 设计章节 | 主要产出 |
|------|------|----------|----------|
| 0 | 基础与存储层 | §3、§8 | 项目骨架、DB 迁移、documents/nodes/node_relations 读写、Markdown 文件存储 |
| 1 | 接入与文档转换 | §4 | Ingest、PDF/Word/HTML/图片/Markdown → Markdown 落盘 |
| 2 | PageIndex 分块 | §5、§19、§22 | 结构解析、TOC/兜底大纲、语义分割、节点与摘要、写 nodes |
| 3 | 知识关系与图谱 | §6、§20 | 候选生成、LLM 关系识别、写 node_relations、后台关系构建 |
| 4 | 图遍历与 Query 基础 | §9–§11、§21、§29.1–29.6 | BFS、Metadata/Graph/Keyword 检索、Intent、检索编排 |
| 5 | Ranking 与 Answer | §11、§21.5–21.7、§29.7–29.9 | LLM Ranking、TopK、Answer Generation、Query Engine 串联 |
| 6 | API 与后台任务 | §3、§15、§31、§32 | HTTP API（OAuth2、§31 路由与统一响应）、上传/问答/流式接口、Worker 定时任务；可选 Web 前端（§32 五大功能） |
| 7 | 可靠性与安全 | §16–§18 | 重试、事务、fallback、鉴权、审计、缓存 |

按上述阶段顺序实现，可保证依赖清晰、与 design.md 一致，并便于分步验收与迭代。

---

# 7. prompt.yml Key 约定（对应设计 §30 LLM Prompt）

为便于实现与提示词管理，对 `prompt.yml` 推荐按 **设计文档 §30.x 编号** 定义 key，示例：

```yaml
# 图片 → Markdown（design §30.1）
image_to_markdown:
  system: |
    ...
  user: |
    ...

# Intent Parser（design §30.2）
intent_parser:
  system: |
    ...
  user: |
    ...

# PageIndex 结构修复（design §30.3）
pageindex_fix:
  system: |
    ...
  user: |
    ...

# 节点摘要生成（design §30.4）
node_summary:
  system: |
    ...
  user: |
    ...

# 节点评分（design §30.5，注意包含 reason 字段）
node_ranking:
  system: |
    ...
  user: |
    ...

# 问答生成（design §30.6）
answer_generation:
  system: |
    ...
  user: |
    ...

# 知识压缩（design §30.7）
knowledge_compress:
  system: |
    ...
  user: |
    ...

# 历史会话压缩（design §30.8）
conversation_compress:
  system: |
    ...
  user: |
    ...

# 查询重写（design §30.9）
query_rewrite:
  system: |
    ...
  user: |
    ...

# 文档大纲生成（design §30.10）
document_outline:
  system: |
    ...
  user: |
    ...

# 关系识别（design §30.11）
relation_recognize:
  system: |
    ...
  user: |
    ...

# Tag 提取（design §30.12）
tag_extract:
  system: |
    ...
  user: |
    ...

# Insight 生成（design §30.13）
insight_generation:
  system: |
    ...
  user: |
    ...
```

各模块在调用 LLM 时，应通过统一的 `pkg/llm` 封装按上述 key 读取对应 `system` / `user` 提示词，以保证实现与 `design.md` 中 Prompt 设计一一对应、便于后续统一调优。

---

# 8. API 接口与 Web 页面实现（对应 design §31、§32）

本节将 design.md 第 31 节「API 接口设计」与第 32 节「Web 页面交互设计」落实为实现要点与交付约定，便于与阶段 6 对齐验收。

---

## 8.1 API 接口实现要点（design §31）

**概述与约定**

* **Base URL**：`/api/v1/`，所有业务接口挂在该前缀下。
* **协议**：REST 风格；请求/响应 Body `Content-Type: application/json`，字符编码 UTF-8。
* **实现位置**：`internal/api`（路由注册、中间件、handler）；OAuth2 可集中为 `internal/api/auth` 或引入成熟库（如 go-oauth2）。

**OAuth2（§31.2）**

* **端点**：`POST /oauth/token`，根据 `grant_type` 处理 authorization_code、client_credentials 等；可选 `GET /oauth/authorize`（授权码模式）。
* **鉴权中间件**：从 Header `Authorization: Bearer <access_token>` 解析并校验 token，失败返回 401；按 scope 做细粒度权限时返回 403。
* **安全**：仅 HTTPS；access_token 建议短有效期（如 2 小时）；refresh_token 与 revoke 策略按 §31.2 实现；token 不写入日志。

**接口列表与实现（§31.3）**

| 方法 | 路径 | 说明 | 实现要点 |
|------|------|------|----------|
| POST | /api/v1/documents | 上传/登记文档 | 接收 multipart 或 JSON（含 source_path/元数据），调 Ingest，返回 doc_id 与状态 |
| GET | /api/v1/documents/{id} | 文档状态与元信息 | 查 documents 表，返回设计 §8.2 字段；可选带节点数/关系数 |
| POST | /api/v1/query | 提交问题，返回回答 | Body：query、可选 conversation_id；调 Query Engine；返回 answer、references；可选返回 steps（§32 执行过程） |
| POST | /api/v1/query/stream | 流式回答 | SSE 或 chunked 流式输出；同上，回答边生成边推送 |
| GET | /api/v1/conversations | 对话历史列表 | 分页参数 page、page_size；返回会话列表及简要信息 |

**请求与响应约定（§31.4）**

* **统一成功响应**：`{"code":0,"message":"success","data":{...}}`。
* **错误**：401 未提供/无效 token；403 无权限；4xx/5xx 与业务错误码统一结构（如 code 非 0、message 可读）。
* **分页**：列表接口返回 `data` 内包含 `items`、`total`，请求参数 `page`（从 1 开始）、`page_size`。

---

## 8.2 Web 页面实现要点（design §32）

**目录与技术选型**

* **静态页面目录**：Web 页面的静态页面统一存储在项目根目录下的 **`pages/`** 目录。
* **前端框架**：Web 页面使用 html 静态页面实现，与 §31 API 对接（OAuth2、Bearer、统一响应）。

**角色与原则**

Web 端为架构中 Client 的一种形态，通过 §31 的 API（OAuth2）与后端交互。实现原则：与 API 一一对应；执行过程可解释；支持文档全生命周期（上传 → 处理 → 查询）。

**功能模块与页面（§32.2）**

| 功能 | 页面/区域 | 实现要点 | 依赖 API |
|------|-----------|----------|----------|
| 查询相关内容 | 检索/知识浏览页 | 关键词与筛选、结果列表与详情、跳转原文或节点 | GET 文档/节点列表或搜索接口（若 §31 扩展） |
| 对话问答 | 问答/对话页 | 输入框、发送、多轮对话展示、流式回答逐字展示 | POST /api/v1/query、POST /api/v1/query/stream |
| 显示执行过程 | 执行过程面板 | 展示 Intent、检索阶段、命中节点、排序与生成状态 | 由 /api/v1/query 响应中 steps/candidates 等字段或后续专用接口 |
| 上传文档 | 文档上传页/入口 | 文件选择、可选元数据、提交后显示处理中/完成/失败 | POST /api/v1/documents |
| 查看文档信息 | 文档列表与详情页 | 列表分页、状态与元数据；详情页单文档信息与节点结构 | GET /api/v1/documents、GET /api/v1/documents/{id} |

**关键流程**

* **文档流程**：上传 → 轮询或推送处理状态 → 完成后在文档列表/详情页查看；失败展示错误信息。
* **问答流程**：输入问题 → 请求 /api/v1/query（或 stream）→ 若有中间结果则更新执行过程面板 → 展示最终回答与引用。

**技术选型与部署**

Web 静态资源与页面统一存放在 **`pages/`** 目录。需统一 OAuth2 登录（授权码模式）与 Bearer 注入，与 §31 约定一致。
