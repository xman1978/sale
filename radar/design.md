# 商机雷达 Agent 设计说明

---

# 一、系统概述

## 1.1 系统目标

AI 商机雷达系统（Opportunity Radar）的目标是：

> 从客户公开数据中自动发现**信息化建设信号**，并结合行业知识与趋势，推断客户未来可能存在的信息化建设**商机场景**。

系统将输出：

- 客户战略方向
- AI 建设阶段
- 重点 AI 场景
- 推荐产品机会
- 商机概率评分

系统定位：

```
销售情报系统
+ AI 推理系统
+ 行业知识系统
```

主要服务对象：

- ToB 销售
- 售前顾问
- 市场团队

---

# 二、核心设计思想

系统核心思想：

```
信号收集 → 信号理解 → 战略推理 → 商机生成
```

逻辑链路：

```
公开信息
   ↓
信号识别
   ↓
客户战略推断
   ↓
行业信息化建设趋势匹配
   ↓
商机场景生成
```

例如：

```
GPU采购
+ AI战略讲话
+ 医院行业

→ 推断

AI应用建设阶段

→ 重点场景

医疗影像AI
病历结构化
AI助手
```

---

# 三、系统整体架构

系统采用 **五层架构**：

```
数据层
↓
信号识别层
↓
客户战略推理层
↓
行业知识推理层
↓
商机生成层
```

完整架构：

```mermaid
flowchart TB
    subgraph dataLayer [数据采集层]
        dataSources[招标采购 | 客户官网 | 新闻 | 政策 | 年报]
    end

    subgraph signalLayer [信号抽取层 Signal]
        signalTech[LLM + NLP]
        signalExtract[提取结构化信号]
        signalItems[技术采购 · 战略方向 · IT建设 · 预算项目]
        signalTech --> signalExtract --> signalItems
    end

    subgraph strategyLayer [客户战略推理层]
        strategyInfer[推断客户信息化建设阶段]
        strategyModel[阶段模型: 数据化 → 算力 → AI平台 → AI应用]
        strategyInfer --> strategyModel
    end

    subgraph industryLayer [行业知识推理层]
        industryKB[行业场景知识库]
        industryItems[行业趋势 · 场景优先级 · 成熟度模型]
        industryKB --> industryItems
    end

    subgraph opportunityLayer [商机生成层]
        oppGen[生成销售机会]
        oppItems[推荐场景 · 推荐产品 · 商机评分 · 机会说明]
        oppGen --> oppItems
    end

    dataLayer --> signalLayer --> strategyLayer --> industryLayer --> opportunityLayer
```



---

# 四、核心功能模块

系统包含 **6个核心模块**。

---

# 4.1 数据采集模块

负责采集客户公开信息。

主要来源：


| 数据类型 | 来源          |
| ---- | ----------- |
| 招标采购 | 政府采购网 / 招标网 |
| 企业信息 | 官网 / 年报     |
| 新闻   | 企业新闻 / 行业媒体 |
| 政策   | 行业监管政策      |
| 公开讲话 | 领导讲话 / 会议纪要 |


采集方式：

```
爬虫
RSS
API
文档解析
```

输出：

```
Document（markdown 格式）
```

---

# 4.2 信号抽取模块（Signal Extraction）

负责从文本中抽取 **商机信号**。

信号类型：


| 信号类型 | 示例       |
| ---- | -------- |
| 算力建设 | GPU服务器采购 |
| 平台建设 | 数据中台     |
| AI战略 | AI提效     |
| 预算项目 | 智能客服系统   |
| 组织变化 | 成立AI中心   |


抽取结构：

```
Signal
```

示例：

```json
{
  "customer": "某三甲医院",
  "signal_type": "infrastructure",
  "technology": "GPU算力",
  "intent": "AI基础设施建设",
  "time": "2025",
  "confidence": 0.86
}
```

---

# 4.3 客户战略推理模块

根据 **信号组合** 推断客户的技术阶段。

阶段模型：


| 阶段   | 特征     |
| ---- | ------ |
| 信息化  | 信息系统建设 |
| 数据化  | 数据平台   |
| 算力化  | GPU采购  |
| AI平台 | AI平台   |
| AI应用 | 业务AI   |


示例：

```
GPU采购
+ AI战略讲话
```

推断：

```
AI应用准备阶段
```

---

# 4.4 行业知识库

系统必须具备 **行业信息化场景知识图谱**。

结构：

```
行业
 ├ 信息化场景
 │
 └ 对应产品
```

示例（医疗行业）：

```
医疗
 ├ AI影像
 ├ 病历结构化
 ├ 临床决策
 └ 智能随访
```

示例（金融行业）：

```
金融
 ├ 智能客服
 ├ 反欺诈
 ├ 投研助手
 └ 风控AI
```

---

# 4.5 商机推理模块

结合：

```
客户信号
+
行业信息化趋势
+
客户成熟度
```

推断：

```
潜在应用
```

示例：

```
医院
+
GPU采购
+
AI提效战略
```

推断：

```
AI影像
病历结构化
智能问答助手
```

---

# 4.6 商机评分模块

对商机进行评分。

评分维度：


| 维度    | 权重  |
| ----- | --- |
| 战略信号  | 0.3 |
| 采购信号  | 0.3 |
| 行业趋势  | 0.2 |
| 客户成熟度 | 0.2 |


输出：

```
Opportunity Score
```

示例：

```
0.82
```

---

# 五、系统数据模型

核心数据对象：

---

## Customer

```
customer_id
customer_name
industry
region
```

---

## Signal

```
signal_id
customer_id
signal_type
technology
intent
time
confidence
source
```

---

## Industry Scenario

```
industry
ai_scenario
priority
related_products
```

---

## Opportunity

```
opportunity_id
customer_id
scenario
score
description
recommended_products
created_time
```

---

# 六、Agent + Tool 设计

系统建议采用 **Agent 架构**。

---

## OpportunityRadarAgent

负责：

```
客户分析
商机推断
机会生成
```

---

## Tool 1

### DataCollectorTool

输入：

```
customer_name
```

输出：

```
documents（markdown 格式内容）
```

---

## Tool 2

### SignalExtractionTool

输入：

```
document（markdown 格式内容）
```

输出：

```
signals（json 格式）
```

---

## Tool 3

### StrategyInferenceTool

输入：

```
signals（json 格式）
```

输出：

```
customer_stage
```

---

## Tool 4

### OpportunityInferenceTool

输入：

```
customer_stage
industry
signals
```

输出：

```
opportunity_scenarios
```

---

## Tool 5

### OpportunityScoringTool

输出：

```
opportunity_score
```

---

# 七、系统输出示例

最终输出示例：

```json
{
  "customer": "某省人民医院",
  "industry": "医疗",
  "signals": [
    "采购GPU服务器",
    "院长提出AI提效",
    "上线数据中台"
  ],
  "customer_stage": "AI应用准备阶段",
  "opportunities": [
    {
      "scenario": "AI影像",
      "score": 0.84
    },
    {
      "scenario": "病历结构化",
      "score": 0.78
    }
  ],
  "recommended_products": [
    "AI应用平台",
    "医疗知识库",
    "智能助手"
  ]
}
```

---

# 八、系统关键能力

系统竞争力来自三个能力：

---

## 1 信号理解能力

系统要理解：

```
采购
战略
政策
```

之间的关系。

---

## 2 行业信息化知识

例如：

```
医疗信息化
金融信息化
政务信息化
```

---

## 3 战略推理能力

例如：

```
算力建设
→ AI应用
```

---

# 九、系统扩展能力

未来可以增加：

### 1 客户 AI 成熟度评分

```
AI Maturity Score
```

---

### 2 商机预测

预测：

```
未来 12 个月采购
```

---

### 3 商机雷达

可视化：

```
客户信息化布局
```

---

# 十、系统价值

系统最终为销售提供：

```
提前发现商机
```

而不是：

```
等客户招标
```

价值：


| 能力   | 价值     |
| ---- | ------ |
| 自动情报 | 减少人工调研 |
| AI推断 | 发现隐藏需求 |
| 机会评分 | 优先跟进   |


---

