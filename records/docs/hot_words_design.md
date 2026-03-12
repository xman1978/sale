# 销售日志热词提取与展示设计说明

## 1. 设计目标

销售人员每天会录入大量客户跟进日志，这些日志中包含重要的市场情报，例如：

* 客户关注的产品
* 客户提出的业务需求
* 客户存在的业务痛点
* 影响交易推进的阻力

由于日志是非结构化文本，管理者难以快速发现趋势。

本系统通过 **LLM + 数据统计分析**，自动从销售日志中提取高频商业关键词，并进行可视化展示，实现：

1. 发现 **市场热点产品**
2. 识别 **客户核心需求**
3. 识别 **客户业务痛点**
4. 分析 **交易阻碍因素**

最终形成 **销售情报看板（Sales Intelligence Dashboard）**。

---

# 2. 数据来源

销售日志来源于表 `follow_records`。

日志文本通过 SQL 拼接形成完整语义上下文。**仅选取尚未在 `sales_keyword_records` 中有关键词记录的跟进**（避免重复抽取），可选按 `created_at >= since` 做时间过滤。

```sql
SELECT id,
  '跟进事项：'||COALESCE(follow_content,'')||
  E'\n跟进结果：'||COALESCE(follow_result,'')||
  E'\n下一步计划：'||COALESCE(next_plan,'')||
  E'\n风险：'||COALESCE(risk_content,'')
  AS log_text
FROM follow_records
WHERE id NOT IN (SELECT follow_id FROM sales_keyword_records)
  -- 可选: AND created_at >= $1
ORDER BY created_at ASC
```

拼接后的文本示例：

```
跟进事项：演示万象公文系统，与客户IT部门沟通
跟进结果：客户对电子签章和移动审批比较感兴趣
下一步计划：安排技术对接API接口
风险：客户预算尚未审批
```

该文本将作为 **LLM 信息抽取输入**。

---

# 3. 系统整体架构

系统采用 **离线分析架构**。

```
销售日志
   │
   │ SQL抽取
   ▼
日志文本
   │
   │ LLM 信息抽取
   ▼
结构化关键词
   │
   │ 同义词归一
   ▼
关键词统计
   │
   ▼
热词库
   │
   ▼
前端可视化
```

核心模块：

1. **日志抽取模块**
2. **LLM 信息抽取模块**
3. **关键词归一模块**
4. **统计聚合模块**
5. **热词存储模块**
6. **可视化展示模块**

---

# 4. LLM 热词提取设计

LLM 的任务是从销售日志中提取四类商业信息：

| 类型                    | 含义      |
| --------------------- | ------- |
| products              | 产品或技术名称 |
| business_requirements | 客户业务需求  |
| pain_points           | 客户业务痛点  |
| transaction_friction  | 销售阻碍因素  |

---

## 4.1 LLM 提示词

### system prompt
帮我写一个销售日志热词提取和展示的设计说明。 

销售日志提取：
select '跟进事项：'||follow_content||'\n跟进结果：'||follow_result||'\n下一步计划：'||next_plan||'\n风险：'||COALESCE(next_plan,'') from follow_records

热词提取的提示词：
你是企业销售情报信息抽取专家。
任务：从【销售日志】中提取结构化商业信息，并输出 JSON 。
规则：
1. 只提取日志中明确出现的信息，禁止推测。
2. 每个元素必须是“短语级关键词”，禁止完整句子。
3. 自动去除无意义词，例如：沟通、跟进、交流、客户、讨论、推进。
4. 自动去重，相同含义保留一个。
5. 若没有信息必须返回 []，禁止 null 或解释。
JSON 格式：
{
  "products": [{"term": "", "count": 0}],
  "business_requirements": [{"term": "", "count": 0}],
  "pain_points": [{"term": "", "count": 0}],
  "transaction_friction": [{"term": "", "count": 0}]
}
字段定义：
1. products
日志中出现的具体产品或技术名称，例如：软件产品、系统平台、开源框架、技术组件。
示例：
飞书、钉钉、企业微信、DeepSeek、Kubernetes、泛微OA、万象公文
排除：
系统、平台、软件、方案（泛化词）
2. business_requirements
客户明确提出的功能和业务需求，例如：私有化部署、API接口对接、国产化适配、多系统集成、移动审批、电子签章集成
3. pain_points
客户当前业务中的问题或低效环节，例如：系统稳定性差、审批流程过慢、人工录入效率低、数据孤岛、系统维护成本高
4. transaction_friction
阻碍销售推进或签约的因素，例如：预算不足、价格敏感、决策链过长、合规审计要求、已有供应商、招标流程未启动
同义词合并规则：
1. 语义相同必须合并
2. 输出行业通用表达
3. 合并后计数加 1
输出要求：
 - 禁止输出 Markdown 格式内容
 - 禁止输出解释文本
 - 禁止输出多余字段
 - 禁止输出 null

---

### user prompt

销售日志：

{{logs}}

---

# 5. 同义词归一设计

LLM 虽然会进行一定归一，但仍需要 **规则归一层**。

例如：

| 原词     | 归一      |
| ------ | ------- |
| OA系统   | OA      |
| 办公OA   | OA      |
| 电子签章系统 | 电子签章    |
| API对接  | API接口对接 |
| 接口打通   | API接口对接 |

实现方式：

```
synonym_dictionary
```

示例：

```
OA系统 -> OA
办公OA -> OA
接口打通 -> API接口对接
系统对接 -> API接口对接
```

处理流程：

```
关键词 -> 查词典 -> 替换 -> 统计
```

---

# 6. 热词统计逻辑

系统按 **时间窗口**统计热词。

例如：

* 最近7天
* 最近30天
* 最近90天

统计时按关键词的 **出现次数**（`count` 字段）求和，而非按行数计数。统计 SQL 示例：

```sql
WITH ranked AS (
  SELECT category, term, SUM(count) AS frequency,
         ROW_NUMBER() OVER (PARTITION BY category ORDER BY SUM(count) DESC) AS rn
  FROM sales_keyword_records
  WHERE create_time >= NOW() - INTERVAL '30 days'
  GROUP BY category, term
)
SELECT category, term, frequency FROM ranked WHERE rn <= 50 ORDER BY category, rn
```

**统计结果保留策略**：仅保留**当日**统计。每次执行统计前会删除 `sales_hot_words_stats` 中 `run_time::date` 非当天的记录，再写入本次结果，因此表中只存在当天一批数据，不做历史留痕。

---

# 7. 数据表设计

## 7.1 关键词明细表

```
sales_keyword_records
```

字段：

| 字段          | 类型        | 说明 |
| ----------- | --------- | -- |
| id          | UUID      | 主键 |
| follow_id   | UUID      | 销售日志 ID（引用 follow_records.id） |
| category    | varchar(64) | 分类（products / business_requirements / pain_points / transaction_friction） |
| term        | varchar(255) | 关键词 |
| count       | int       | 该词在本条日志中的出现次数（与 LLM 返回 JSON 一致） |
| create_time | timestamptz | 创建时间 |

**数据策略**：该表**不清空**；每次流水线只对「尚未在本表中出现过的 `follow_id`」做抽取并插入，避免同一跟进重复写入（方案 B 增量）。

示例：

| follow_id | category              | term    | count |
| --------- | --------------------- | ------- | ----- |
| uuid-101  | products              | 万象公文   | 1     |
| uuid-101  | business_requirements | API接口对接 | 2     |
| uuid-101  | transaction_friction  | 预算不足   | 1     |

---

## 7.2 同义词词典

```
sales_keyword_synonyms
```

| 字段          | 类型      |
| ----------- | ------- |
| source_term | varchar |
| target_term | varchar |

---

## 7.3 统计结果表

每次执行热词统计任务时，**仅保留当日统计**：先删除表中 `run_time::date` 非当天的记录，再将当次生成的统计结果写入。不做历史留痕，前端只展示当日热词。

表名：

```
sales_hot_words_stats
```

字段：

| 字段              | 类型        | 说明 |
| --------------- | --------- | -- |
| id              | UUID      | 主键 |
| run_time        | timestamptz | 统计任务执行时间（标识当次生成批次） |
| time_window_days| int       | 时间窗口天数（7 / 30 / 90） |
| category        | varchar(64) | 分类（同上） |
| term            | varchar(255) | 关键词 |
| frequency       | int       | 出现频次（来自 sales_keyword_records 的 SUM(count)） |
| rank            | int       | 该分类下当次统计的排名（1-based） |
| create_time     | timestamptz | 记录创建时间 |

约束与索引：

* 唯一约束：`(run_time, time_window_days, category, term)`，避免同一次运行重复写入同一词条。
* 索引：`(run_time, time_window_days)`、`(category, run_time)`。

示例（当日 30 天窗口的统计结果片段）：

| run_time            | time_window_days | category              | term    | frequency | rank |
| ------------------- | ---------------- | --------------------- | ------- | --------- | ---- |
| 2025-03-11 00:15:00 | 30               | products              | 万象公文   | 56        | 1    |
| 2025-03-11 00:15:00 | 30               | products              | DeepSeek | 42        | 2    |
| 2025-03-11 00:15:00 | 30               | business_requirements | 私有化部署  | 38        | 1    |

使用方式：

* **当日展示**：按 `run_time::date = 当天` 取各时间窗口的统计，按 `category`、`rank` 排序得到当日热词榜。接口与页面仅提供当日数据，无历史日期选择。

---

# 8. 热词计算策略

为了避免短期数据波动，可以使用 **TF 统计权重**。

简单热度公式：

```
hot_score = frequency
```

高级方案：

```
hot_score = frequency * log(最近增长率)
```

用于发现 **突然爆发的需求**。

例如：

| 关键词   | 上周 | 本周 | 热度 |
| ----- | -- | -- | -- |
| 国产化适配 | 3  | 18 | 高  |
| 私有化部署 | 8  | 9  | 中  |

---

# 9. 可视化展示设计

前端建议展示以下组件。

---

## 9.1 产品热词榜

```
万象公文        56
DeepSeek       42
泛微OA         31
企业微信       28
Kubernetes     22
```

作用：

发现 **市场关注的产品和技术趋势**

---

## 9.2 客户需求热词

```
私有化部署
API接口对接
多系统集成
电子签章集成
移动审批
```

作用：

指导 **产品规划**

---

## 9.3 客户痛点

```
数据孤岛
审批流程慢
人工录入效率低
系统维护成本高
```

作用：

帮助销售 **优化话术和解决方案**

---

## 9.4 成交阻力

```
预算不足
已有供应商
决策链过长
招标未启动
```

作用：

优化 **销售策略**

---

## 9.5 热词趋势图（可选扩展）

```
时间维度
────────────
API接口对接
私有化部署
国产化适配
```

用于分析 **需求趋势变化**。当前实现仅保留当日统计，多日趋势对比需在统计表保留历史或另行存储后再实现。

---

# 10. 数据更新策略

推荐 **每日离线计算**（如每日 00:05 定时执行一次）。

流程：

```
定时触发（如 00:05）
  │
  ├─ 若当日已有统计则跳过（RunIfNotGeneratedToday）
  │
  ├─ 抽取「尚未在 sales_keyword_records 中出现的」销售日志（方案 B 增量，不重复抽取）
  │    可选：仅 created_at >= since 的日志（IncrementalSince）
  │
  ├─ LLM 热词抽取（批量，如每批 20 条）
  │
  ├─ 同义词归一
  │
  ├─ 写入关键词表（sales_keyword_records，仅 INSERT，不清表）
  │
  ├─ 删除统计表中非当日数据（DeleteStatsNotForDate），再按时间窗口聚合并写入 sales_hot_words_stats（仅保留当日）
  │
  └─ 热词展示由 records/pages/hot_words.html 通过 API 拉取当日统计
```

### 10.1 动态页面与 API

* **目的**：在飞书内通过 H5 页直接查看**当日**热词榜，无需单独部署前端服务。
* **页面**：`records/pages/hot_words.html` 为**动态页面**，标题为「今日热词」，打开时请求接口拉取当日统计并渲染；**无日期选择器**，仅展示当天数据。
* **接口**：`GET {api_prefix}/hotwords/stats` 固定返回**当日**各时间窗口（7/30/90 天）的统计结果（JSON），不支持 `date` 参数。
* **日期列表**：`GET {api_prefix}/hotwords/run_dates` 仅返回当日日期列表（单元素），用于兼容前端。
* **数据来源**：从统计结果表（sales_hot_words_stats）按当日 `run_time` 取各时间窗口的统计。
* **飞书适配**：页面适配飞书内置浏览器（视口、同源请求），便于在飞书文档/群聊中打开或嵌入。

---

# 11. 性能优化

为了降低 LLM 成本与避免重复数据：

### 1 批量日志

一次处理 20 条日志，合并为一段文本送 LLM。

```
logs = log1 + log2 + log3
```

### 2 增量处理（方案 B）

只处理**尚未在 sales_keyword_records 中有关键词记录的**跟进，避免同一跟进被重复抽取、重复插入。

```
WHERE id NOT IN (SELECT follow_id FROM sales_keyword_records)
```

可选：配合 `IncrementalSince`，仅处理 `created_at >= since` 的日志，进一步缩小范围。

### 3 不重复写入

已有关键词记录的 `follow_id` 不会再次出现在抽取列表中，因此不会对同一日志重复调用 LLM 或重复 INSERT，相当于“缓存提取结果”。

---

# 12. 未来扩展

未来可增加：

### 1 销售机会预测

根据关键词预测：

* 成交概率
* 项目阶段

### 2 行业趋势分析

按行业统计：

```
医疗
金融
政府
制造
```

### 3 竞争对手分析

自动识别：

```
泛微
致远
蓝凌
```

---

# 总结

该系统通过 **LLM + 统计分析**，实现销售日志的智能情报提取：

核心能力：

1. 自动提取 **产品、需求、痛点、交易阻力**
2. 同义词归一
3. 热词统计
4. 销售情报可视化

最终帮助企业：

* 发现市场需求
* 优化销售策略
* 指导产品规划

---
