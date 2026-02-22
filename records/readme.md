# 对话式销售日志录入系统

基于设计文档 `design.md` 实现的对话式销售日志录入系统，支持通过飞书机器人进行自然语言交互，自动记录和整理销售跟进信息。

## 系统特性

- **自然对话交互**：支持口语化、非结构化输入
- **多客户并行跟进**：单次会话中可处理多个客户的跟进记录
- **规则驱动状态机**：确保数据一致性和可回放性
- **智能语义理解**：使用大模型进行信息提取和对话生成
- **会话中断恢复**：支持长时间会话的中断和恢复
- **PostgreSQL存储**：可靠的数据持久化

## 项目结构

```
records/
├── main.go                    # 应用入口
├── go.mod                     # Go模块定义
├── config.yml                 # 配置文件
├── design.md                  # 详细设计文档
├── sql/                       # SQL 脚本目录
│   └── schema.sql            # 数据库架构定义
├── records/internal                  # 内部包
│   ├── config/               # 配置管理
│   ├── database/             # 数据库连接和迁移
│   ├── models/               # 数据模型
│   ├── repository/           # 数据访问层
│   ├── ai/                   # AI模型集成
│   ├── engine/               # 规则引擎
│   ├── orchestrator/         # 对话编排器
│   ├── feishu/              # 飞书集成
│   └── server/              # 服务器
└── pkg/                     # 公共包
    └── logger/              # 日志组件
```

## 核心组件

### 1. 规则引擎 (Rule Engine)
- 状态推导：根据客户数据确定当前状态
- 写入权限控制：确保数据写入的合法性
- 聚焦客户选择：智能选择当前处理的客户

### 2. 对话编排器 (Turn Orchestrator)
- 单轮对话处理：完整的对话轮次处理流程
- 运行态管理：维护会话的运行时状态
- 语义分析集成：调用AI模型进行语义理解

### 3. AI客户端
- 语义分析：提取用户输入中的结构化信息
- 对话生成：根据当前状态生成自然回复

### 4. 飞书集成
- 消息接收和发送
- 用户信息获取
- WebSocket长连接维护

## 数据模型

### 核心表结构
- `users`: 用户信息
- `customers`: 客户信息
- `sessions`: 会话记录
- `dialogs`: 对话运行态快照
- `follow_records`: 跟进记录

### 状态机设计
- **客户状态**: CUSTOMER_NAME → FOLLOW_METHOD → FOLLOW_CONTENT → FOLLOW_GOAL → FOLLOW_RESULT → NEXT_PLAN → COMPLETE
- **会话阶段**: COLLECTING → CONFIRMING → OUTPUTTING → EXIT

## 安装和部署

### 1. 环境要求
- Go 1.22+
- PostgreSQL 12+
- 飞书应用配置
- OpenAI API密钥

### 2. 数据库初始化
**系统会在启动时自动完成数据库初始化，无需手动执行 SQL 脚本。**

只需要创建数据库：
```bash
# 创建数据库
createdb sales_log
```

系统启动时会自动：
- 创建所有必要的表结构
- 创建索引优化查询性能
- 创建触发器自动更新时间戳
- 验证表结构的完整性

详细说明请参考：
- [SQL_ORGANIZATION.md](SQL_ORGANIZATION.md) - SQL 文件组织和维护指南
- [SQL_IMPROVEMENTS.md](SQL_IMPROVEMENTS.md) - SQL 文件组织改进总结
- [SQL_QUICK_REFERENCE.md](SQL_QUICK_REFERENCE.md) - SQL 快速参考指南
- [SQL_SEPARATION_COMPLETE.md](SQL_SEPARATION_COMPLETE.md) - SQL 分离改进完成总结

### 3. 配置文件
复制 `config.yml` 并修改相应配置：
- 数据库连接信息
- 飞书应用凭证
- OpenAI API配置

### 4. 编译和运行
```bash
# 安装依赖
go mod tidy

# 编译
go build -o sales-log main.go

# 运行（推荐使用快速启动脚本）
chmod +x start.sh
./start.sh

# 或直接运行
./sales-log
```

## 配置说明

### 数据库配置
```yaml
database:
  host: localhost
  port: 5432
  user: postgres
  password: your_password
  dbname: sales_log
```

### 飞书配置
```yaml
feishu:
  app_id: your_app_id
  app_secret: your_app_secret
  verification_token: your_verification_token
  encrypt_key: your_encrypt_key
```

### AI模型配置
```yaml
ai:
  openai:
    api_key: your_openai_api_key
    base_url: https://api.openai.com/v1
    model_name: gpt-4
```

## 使用方式

### 1. 飞书机器人交互
- 用户在飞书中与机器人对话
- 机器人引导用户复盘客户跟进情况
- 系统自动提取和记录结构化信息

### 2. 对话流程
1. **信息收集阶段 (COLLECTING)**：收集客户跟进的各项信息
2. **确认阶段 (CONFIRMING)**：复述并确认收集的信息
3. **输出阶段 (OUTPUTTING)**：生成最终的跟进记录
4. **结束阶段 (EXIT)**：完成本次对话

### 3. 支持的信息字段
- 客户名称 (必填)
- 跟进时间 (必填)
- 联系人信息 (可选)
- 跟进方式 (必填)
- 跟进内容/项目 (必填)
- 本次目标 (必填)
- 本次结果 (必填)
- 风险/阻塞点 (可选)
- 下一步计划 (必填)

## 监控和维护

### 健康检查
```bash
curl http://localhost:8080/health
```

### 数据库初始化测试
```bash
# 测试数据库自动初始化功能
chmod +x test_db_init.sh
./test_db_init.sh
```

### 日志查看
系统支持结构化日志输出，可配置输出到文件或标准输出。

### 错误处理
- 语义分析失败不影响对话继续
- 数据库错误有重试机制
- 飞书连接断开自动重连

## 开发和扩展

### 添加新的AI模型
实现 `ai.Client` 接口：
```go
type Client interface {
    SemanticAnalysis(ctx context.Context, userInput, stage, focusCustomer, expectedField string) (*models.SemanticAnalysisResult, error)
    GenerateDialogue(ctx context.Context, stage, focusCustomer, userInput, historyContext string) (string, error)
}
```

### 扩展规则引擎
在 `engine/rule_engine.go` 中添加新的规则逻辑。

### 添加新的消息平台
实现 `feishu.Client` 和 `feishu.MessageHandler` 接口。

## 注意事项

1. **数据一致性**：系统严格遵循规则引擎的约束，确保数据的一致性
2. **对话连续性**：优先保证对话不中断，语义失败不影响用户体验
3. **状态可回放**：所有状态变化都有完整的审计轨迹
4. **并发安全**：使用会话级乐观锁防止并发冲突

## 故障排除

### 常见问题
1. **数据库连接失败**：检查数据库配置和网络连接
2. **飞书消息接收失败**：检查应用配置和网络连接
3. **AI模型调用失败**：检查API密钥和网络连接
4. **状态机异常**：查看日志中的规则引擎错误信息

### 日志级别
- `DEBUG`: 详细的执行信息
- `INFO`: 一般操作信息
- `WARN`: 警告信息
- `ERROR`: 错误信息
- `FATAL`: 致命错误

## 许可证

本项目遵循设计文档中的架构约束和业务规则，确保系统的可靠性和可维护性。