# 飞书移动端跟进记录列表页设计说明

## 1. 页面概述

| 项目 | 说明 |
|:---|:---|
| **页面名称** | 跟进记录列表页 / 客户跟进历史详情页 |
| **目标平台** | 飞书移动端（iOS & Android） |
| **核心功能** | 展示客户-事项维度的跟进记录聚合，支持查看完整跟进历史 |
| **数据来源** | PostgreSQL `follow_records` 表 |

---

## 2. 列表页设计（FollowRecordList）

### 2.1 页面布局

```
┌─────────────────────────────┐
│  ←  跟进记录          [+]   │  ← 导航栏
├─────────────────────────────┤
│ 🔍 搜索客户或事项...         │  ← 搜索栏（可选）
├─────────────────────────────┤
│                             │
│ ┌─────────────────────────┐ │
│ │ 阿里巴巴                │ │  ← 客户名称（customer_name）
│ │ 续约谈判                │ │  ← 事项（follow_content 摘要）
│ │ 2024-01-15              │ │  ← 最近跟进时间（仅年月日）
│ └─────────────────────────┘ │
│                             │
│ ┌─────────────────────────┐ │
│ │ 腾讯科技                │ │
│ │ 新产品方案演示          │ │
│ │ 2024-01-14              │ │
│ └─────────────────────────┘ │
│                             │
│         ...                 │
│                             │
└─────────────────────────────┘
```

### 2.2 列表项（List Item）规范

| 元素 | 字段映射 | 样式说明 |
|:---|:---|:---|
| **主标题** | `customer_name` | 16px, 500字重, #1F2329（主文本色） |
| **副标题** | `follow_content` 前20字 | 14px, 400字重, #646A73（次要文本） |
| **时间** | `follow_time` 格式化日期 | 12px, #8F959E, 右对齐 |

### 2.3 时间显示规则（仅年月日）

| 时间范围 | 显示格式 |
|:---|:---|
| 当年 | "MM-DD"（如 01-15） |
| 往年 | "YYYY-MM-DD"（如 2023-12-01） |

### 2.4 数据聚合逻辑

列表按 **客户 + 事项** 维度聚合，显示该组合下**最近一次**跟进记录的时间：

```sql
-- 聚合查询：按客户+事项分组，取最近跟进时间
SELECT DISTINCT ON (customer_id, follow_content)
    id,
    customer_id,
    customer_name,
    follow_content,
    follow_time
FROM follow_records
WHERE user_id = $1
ORDER BY customer_id, follow_content, follow_time DESC;
```

---

## 3. 详情页设计（FollowRecordTimeline）

### 3.1 页面布局

```
┌─────────────────────────────┐
│  ←  跟进历史                │  ← 导航栏
├─────────────────────────────┤
│                             │
│ ┌─────────────────────────┐ │
│ │      客户信息卡片        │ │
│ │  ┌─────┐ 阿里巴巴       │ │
│ │  │头像 │  续约谈判       │ │  ← 事项名称（当前分组key）
│ │  └─────┘  共3次跟进     │ │
│ └─────────────────────────┘ │
│                             │
│ ┌─────────────────────────┐ │
│ │      时间轴区域          │ │
│ │                         │ │
│ │    ●────────────────    │ │
│ │    │ 2024-01-15        │ │
│ │    │ 14:30  面谈       │ │
│ │    │ ─────────────     │ │
│ │    │ 联系人：张三       │ │
│ │    │ 职务：采购经理     │ │
│ │    │                   │ │
│ │    │ 预期目标：         │ │
│ │    │ 完成年度续约       │ │
│ │    │                   │ │
│ │    │ 实际结果：         │ │
│ │    │ 客户基本同意方案   │ │
│ │    │ 需内部审批         │ │
│ │    │                   │ │
│ │    │ 存在风险：         │ │
│ │    │ 竞争对手低价介入   │ │
│ │    │                   │ │
│ │    │ 下一步计划：       │ │
│ │    │ 周三前发送正式报价 │ │
│ │    │                   │ │
│ │    ●────────────────    │ │
│ │    │ 2024-01-10        │ │
│ │    │ 10:00  电话       │ │
│ │    │ ─────────────     │ │
│ │    │ 联系人：李四       │ │
│ │    │ ...               │ │
│ │    │                   │ │
│ │    ●────────────────    │ │
│ │    │ 2024-01-05        │ │
│ │    │ 16:00  微信       │ │
│ │    │ ...               │ │
│ │                         │ │
│ └─────────────────────────┘ │
│                             │
│      [ + 添加跟进记录 ]      │  ← 底部固定按钮
│                             │
└─────────────────────────────┘
```

### 3.2 时间轴节点规范

| 元素 | 字段映射 | 样式说明 |
|:---|:---|:---|
| **时间轴节点** | - | 12px圆点，#3370FF（最新）/#8F959E（历史） |
| **连接线** | - | 2px实线，#DEE0E3 |
| **跟进时间** | `follow_time` | 14px, 500字重, #1F2329 |
| **具体时间** | `follow_time` | 12px, #8F959E |
| **跟进方式** | `follow_method` | 12px Tag标签（详情页内显示） |

### 3.3 时间轴卡片内容

每个时间节点展开显示完整字段：

| 显示名称 | 数据库字段 | 展示格式 |
|:---|:---|:---|
| 联系人 | `contact_person` | 14px, #1F2329 |
| 联系人职务 | `contact_role` | 12px, #646A73，与姓名同行 |
| 预期目标 | `follow_goal` | 14px, #1F2329，多行文本 |
| 实际结果 | `follow_result` | 14px, #1F2329，多行文本 |
| 存在风险 | `risk_content` | 14px, #FA5151（警示红），为空时隐藏整行 |
| 下一步计划 | `next_plan` | 14px, #1F2329，支持自动识别序号（1. 2. 3.） |

### 3.4 交互设计

| 操作 | 响应 |
|:---|:---|
| 点击时间节点 | 展开/收起该节点详情（默认展开最新一条） |
| 下拉刷新 | 重新加载该客户-事项的所有跟进记录 |

---

## 4. 数据查询示例

### 4.1 列表页查询（聚合）

```sql
-- 获取客户-事项维度的聚合列表
WITH latest_records AS (
    SELECT DISTINCT ON (customer_id, LEFT(follow_content, 50))
        id,
        customer_id,
        customer_name,
        LEFT(follow_content, 50) as matter_summary,
        follow_time,
        ROW_NUMBER() OVER (PARTITION BY customer_id ORDER BY follow_time DESC) as rn
    FROM follow_records
    WHERE user_id = $1
    ORDER BY customer_id, LEFT(follow_content, 50), follow_time DESC
)
SELECT * FROM latest_records
ORDER BY follow_time DESC
LIMIT 20 OFFSET $2;
```

### 4.2 详情页查询（时间轴）

```sql
-- 获取指定客户-事项的所有跟进记录（时间轴）
SELECT 
    id,
    customer_name,
    contact_person,
    contact_role,
    contact_phone,
    follow_time,
    follow_method,
    follow_goal,
    follow_result,
    risk_content,
    next_plan,
    created_at
FROM follow_records
WHERE customer_id = $1 
  AND user_id = $2
  AND LEFT(follow_content, 50) = $3  -- 事项匹配
ORDER BY follow_time DESC;
```

---

## 5. 关键状态处理

| 场景 | 处理方式 |
|:---|:---|
| 该客户-事项仅1条记录 | 时间轴显示单节点，隐藏"共X次跟进"或显示"共1次跟进" |
| 风险内容为空 | 该字段整行隐藏，不显示"无" |
| 联系人信息缺失 | 显示"未知联系人"，电话隐藏 |

---

# 与飞书网页应用对接说明

本文档描述 sale_logs（销售日志页面）与飞书网页应用的对接设计，包括 OAuth 登录、用户标识、API 接口及配置要求。

---

## 1. 概述

sale_logs 是运行在飞书内的网页应用（`records/pages/index.html`），用户通过飞书 OAuth 登录后，可查看和录入跟进记录。与 sale_agent（销售助手机器人）共用同一套数据，通过 **union_id** 统一用户标识。

| 组件 | 说明 |
|------|------|
| 前端页面 | `records/pages/index.html`，Vue 3 单页应用 |
| 后端 API | Page API（`/sale/api` 前缀） |
| 飞书应用 | config.yml 中 `feishu.sale_logs`，需在飞书开放平台创建「网页应用」 |

---

## 2. 飞书开放平台配置

### 2.1 创建网页应用

1. 登录 [飞书开放平台](https://open.feishu.cn/)
2. 创建应用，选择「网页应用」
3. 获取 **App ID** 和 **App Secret**

### 2.2 安全设置

在应用「安全设置」中配置：

- **重定向 URL**：必须与 `config.yml` 中 `feishu.sale_logs.redirect_uri` **完全一致**
  - 示例：`https://gw.whir.net/sale/logs/index.html`
  - 协议、域名、路径、末尾斜杠均需一致

### 2.3 权限

确保应用已开通：

- 获取用户 union_id（用于跨应用统一用户标识）
- 获取用户基本信息（姓名、头像等）

---

## 3. 配置项（config.yml）

```yaml
feishu:
  sale_logs:
    app_id: "你的应用 App ID"
    app_secret: "你的应用 App Secret"
    redirect_uri: "https://你的域名/sale/logs/index.html"  # 与飞书开放平台配置完全一致

server:
  api_prefix: /sale/api      # API 路径前缀
  web_prefix: /sale/logs     # 静态页面路径前缀
  jwt_secret: "32+ 位随机字符串"  # 生产环境必填
  allow_x_user_id_fallback: true  # JWT 失效时是否允许 x-user-id 回退
```

---

## 4. OAuth 登录流程

### 4.1 授权入口

用户未登录时，页面显示「飞书登录」按钮，点击跳转至飞书授权页：

```
https://open.feishu.cn/open-apis/authen/v1/authorize?app_id={app_id}&redirect_uri={redirect_uri}&state=
```

- `app_id`：来自 `config.js`（由 `configJSHandler` 动态注入）
- `redirect_uri`：与飞书开放平台配置一致，前端通过 `getFeishuRedirectUri()` 获取

### 4.2 回调与 Token 兑换

授权成功后，飞书重定向回 `redirect_uri?code=xxx`。前端：

1. 从 URL 解析 `code`
2. 调用 `POST {api_prefix}/feishu/auth`，请求体：`{ "code": "xxx", "redirect_uri": "xxx" }`
3. 后端调用飞书 `POST /open-apis/authen/v1/access_token`，用 `app_id`、`app_secret`、`code`、`redirect_uri` 兑换 user_access_token
4. 从 token 响应或 `GET /open-apis/authen/v1/user_info` 获取用户信息
5. 使用 **union_id** 作为 user_id，创建/更新 users 表，签发 JWT
6. 返回 `{ success: true, data: { userId, name, avatar, token } }`

### 4.3 前端存储

- `localStorage.userId`：union_id
- `localStorage.authToken`：JWT

---

## 5. 用户标识：union_id

程序统一使用 **union_id** 作为用户标识：

- **union_id**：同一用户在同一开发商下不同应用中相同，用于跨应用统一
- **open_id**：按应用区分，同一用户在不同应用中不同，**不作为用户标识**

sale_logs（网页）与 sale_agent（机器人）通过 union_id 关联同一用户，实现数据互通。

---

## 6. API 接口

### 6.1 认证方式

请求需携带以下之一：

- `Authorization: Bearer {jwt_token}`（推荐）
- `x-user-id: {union_id}`（当 `allow_x_user_id_fallback: true` 时作为回退）

### 6.2 接口列表

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/feishu/auth` | 飞书 OAuth 登录，用 code 换 JWT |
| GET | `/user/info` | 获取当前用户信息 |
| GET | `/records` | 获取跟进记录列表 |
| POST | `/records` | 新建跟进记录 |
| PUT | `/records/:id` | 更新跟进记录 |
| DELETE | `/records/:id` | 删除跟进记录 |

### 6.3 动态配置（config.js）

页面加载时请求 `config.js`，获取：

```javascript
window.APP_CONFIG = {
  apiPrefix: "/sale/api",
  feishuAppId: "cli_xxx",
  feishuRedirectUri: "https://xxx/sale/logs/index.html"
}
```

由服务端 `configJSHandler` 根据 `config.yml` 动态生成。

---

## 7. 页面访问路径

- 静态页面根路径：`{web_prefix}/`，如 `https://域名/sale/logs/`
- 主页面：`index.html`，完整 URL 如 `https://域名/sale/logs/index.html`
- 在飞书工作台配置该 URL 作为应用主页，用户从飞书内打开

---

## 8. 注意事项

1. **redirect_uri 一致性**：飞书开放平台、config.yml、前端构造的 OAuth URL 三者必须完全一致，否则会返回 20014 等错误。
2. **清除缓存**：修改 OAuth 或用户标识逻辑后，用户需清除 localStorage 并重新登录，否则可能沿用旧的 JWT（含过期的 open_id）。
3. **iframe 内打开**：若页面在飞书 iframe 中打开，飞书登录按钮需使用 `target="_top"` 以正确跳转。
4. **H5 SDK**：页面引入飞书 H5 JS SDK，在飞书内嵌环境中可尝试 `h5sdk.biz.util.getAuthCode` 获取 code；若不可用，则依赖 OAuth 重定向方式。
