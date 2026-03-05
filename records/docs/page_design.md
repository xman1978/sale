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

列表**仅按客户名分组**，与 manager 页一致：每客户一行，不展示跟进事项与日期。

```
┌─────────────────────────────┐
│  ←  跟进记录          [+]   │  ← 导航栏
├─────────────────────────────┤
│ 🔍 搜索客户...               │  ← 搜索栏（按客户名过滤）
├─────────────────────────────┤
│                             │
│ ┌─────────────────────────┐ │
│ │ 阿里巴巴    更新时间 12-25 14:30 │  ← 客户名 + 右侧「更新时间」
│ │ 点击查看跟进             │ │  ← 固定副标题
│ └─────────────────────────┘ │
│                             │
│ ┌─────────────────────────┐ │
│ │ 腾讯科技    更新时间 12-20 10:00 │
│ │ 点击查看跟进             │ │
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
| **右侧更新时间** | `last_record_at`（最近一次增加跟进记录的时间） | 12px, #3370FF（主色蓝），无记录显示「暂无」；与 manager 列表项样式一致 |
| **副标题** | 固定文案「点击查看跟进」 | 14px, 400字重, #646A73（次要文本）；列表不显示跟进事项 |

### 2.3 数据聚合逻辑（前端）

列表数据由前端从「全量跟进记录」按 **客户** 维度聚合（与 manager 一致）：

- 对 `follow_records` 按 `customer_id` 去重，得到「客户列表」`groupsList`。
- 每组保留 `customer_id`、`customer_name`、`last_record_at`（该客户最近一次记录的 `created_at`，无则回退 `follow_time`）；排序按 `last_record_at` **降序**。
- 搜索仅按客户名过滤（占位符「搜索客户...」）。

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
│ │  │头像 │  全部跟进       │ │  ← 固定文案（该客户下全部记录）
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
| **跟进方式** | `follow_method` | 12px Tag 标签（详情页内显示） |
| **AI 生成** | `ai === true` | 当记录为对话方式收集时，在时间卡片右上角显示「AI 生成」标签（12px，灰色背景） |

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

### 4.1 列表页数据（index.html）

列表页直接使用 **GET /records** 拉取当前用户全部跟进记录；前端按 `customer_id` 聚合为 `groupsList`（每客户一行，按该客户最近跟进时间排序），不依赖后端聚合接口。

### 4.2 详情页查询（时间轴）

当前实现：详情页按**客户**展示该客户下**全部**跟进记录（不限跟进事项），前端用 `customer_id` 过滤。

```sql
-- 获取指定客户的全部跟进记录（时间轴）
SELECT 
    id,
    customer_id,
    customer_name,
    contact_person,
    contact_role,
    contact_phone,
    follow_time,
    follow_method,
    follow_content,
    follow_goal,
    follow_result,
    risk_content,
    next_plan,
    ai,
    created_at
FROM follow_records
WHERE customer_id = $1 AND user_id = $2
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
5. **飞书内 SDK 异步注入**：在飞书客户端内打开时，`window.h5sdk` 可能晚于页面脚本加载。前端在飞书环境下会先等待 `waitForH5Sdk`（约 3.5 秒轮询），再调用 `getAuthCode`，避免因 SDK 未就绪而一直显示「请使用飞书登录」。

---

# 页面修改记录

## 跟进记录按客户名展示（2025-03）

### 变更概述

将跟进记录的展示从「按客户名 + 跟进事项」改为「仅按客户名」：列表每客户一行，点进详情后展示该客户下全部跟进记录（含所有跟进事项）；在添加/编辑页中跟进事项为必填且可更改；详情时间线中每条记录展示该条的跟进事项。

### 1. 列表与详情逻辑

| 项目 | 修改前 | 修改后 |
|:---|:---|:---|
| **列表聚合** | 按 `(customer_id, follow_content)` 去重，每「客户+事项」一行 | 按 `customer_id` 去重，每客户一行（取该客户最新跟进时间的一条作入口） |
| **详情数据** | 只显示同客户、同跟进事项的记录 | 显示该客户下全部跟进记录（不限跟进事项） |
| **详情页头部** | 显示当前分组的单一「跟进事项」 | 显示「全部跟进」 |

涉及文件：

- **index.html**：`filteredRecords` 去重 key 改为仅 `customer_id`；`detailRecords` 只按 `customer_id` 过滤；DetailPage 头部 `customer-matter` 改为「全部跟进」。
- **manager.html**：二级列表由后端 `/manager/users/:id/groups` 仅返回客户名列表；请求详情时只传 `customer_name`；详情头部显示「全部跟进」。

### 2. 详情时间线：显示跟进事项

在**详细跟进记录页面**的每条时间线记录卡片中，增加「跟进事项」展示（`record.follow_content`），便于区分同一客户下不同事项的多次跟进。

- **index.html**：DetailPage 时间线卡片在 timeline-header 下方增加「跟进事项」区块（`v-if="record.follow_content"`）。
- **manager.html**：同上，每条 timeline-item 内增加跟进事项 section。

### 3. 添加/编辑页：跟进事项必填且可更改

| 场景 | 修改前 | 修改后 |
|:---|:---|:---|
| **添加记录** | 从详情页进入时客户名与跟进事项均为只读 | 仅客户名只读，跟进事项始终可编辑（仍必填） |
| **编辑记录** | 跟进事项为只读（disabled） | 跟进事项可编辑，且为必填（提交前校验「请填写跟进事项」）；标签为「跟进事项 *」 |
| **后端 PUT** | `updateRecordRequest` 不含 `follow_content`，无法更新 | 增加 `FollowContent` 字段，PUT 时写入并持久化 |

涉及文件：

- **index.html**：AddModal 中跟进事项输入框去掉 `isFromDetail` 的 disabled；EditModal 中跟进事项去掉 disabled、标签改为「跟进事项 *」、submit 中增加 `follow_content` 必填校验。
- **page_api.go**：`updateRecordRequest` 增加 `FollowContent string`；PUT 分支中设置 `record.FollowContent = &fc` 后再调用 `UpdateFollowRecord`。

### 4. 后端 API 变更（manager）

| 接口 | 修改前 | 修改后 |
|:---|:---|:---|
| **GET /manager/users/:id/groups** | 返回 `(customer_name, follow_content)` 组合列表 | 仅按客户名去重，返回 `customer_name` 列表（无 follow_content） |
| **GET /manager/users/:id/records** | 必填 `customer_name` 与 `follow_content` | 仅必填 `customer_name`；`follow_content` 可选，不传或为空时返回该客户下全部记录 |

涉及文件：

- **repository.go**：`ListCustomerFollowGroupsForManager` 查询改为 `SELECT DISTINCT customer_name ... ORDER BY customer_name`；`ListFollowRecordsForManager` 的 WHERE 增加对 `follow_content` 的可选条件（`COALESCE($3, '') = ''` 时不过滤事项）。
- **manager_api.go**：groups 返回的 map 仅含 `customer_name`；records 仅在校验 `customer_name` 为空时返回 400。

---

## 后续更新（2025-03）

### 1. follow_records 表增加 ai 字段

| 项目 | 说明 |
|:---|:---|
| **数据库** | `follow_records` 表新增列 `ai BOOLEAN NOT NULL DEFAULT false`。已有库需执行：`ALTER TABLE sale.follow_records ADD COLUMN IF NOT EXISTS ai BOOLEAN NOT NULL DEFAULT false;` |
| **含义** | 标识该条跟进是否通过**对话方式**（销售助手）收集：对话写入为 `true`，页面手动录入为 `false`。 |
| **后端** | 模型 `FollowRecord` 增加字段 `AI`；`CreateFollowRecord`/`UpdateFollowRecord` 及所有相关 SELECT 包含 `ai`；对话流程（output_worker、turn_orchestrator）写入时设 `AI: true`，`CreateFollowRecordForPage` 设 `AI: false`。 |
| **前端** | 详情页时间线卡片右上角：当 `record.ai === true` 时显示「AI 生成」标签（index.html、manager.html 均已支持）。列表/详情 API 返回中包含 `ai` 字段。 |

### 2. 编辑跟进记录：允许编辑客户名与跟进事项

| 项目 | 说明 |
|:---|:---|
| **前端** | 编辑弹窗（EditModal）中「客户名」「跟进事项」由只读改为可编辑；保存前校验必填；标签为「客户名 *」「跟进事项 *」。 |
| **后端** | `updateRecordRequest` 增加 `CustomerName string`；PUT 处理时设置 `record.CustomerName = req.CustomerName`，与 `FollowContent` 一并持久化。 |

### 3. 列表实现与数据流（index.html）

| 项目 | 说明 |
|:---|:---|
| **列表数据** | 使用计算属性 `groupsList`：从 `records` 按 `customer_id` 分组，每组 `{ customer_id, customer_name }`，按该客户最近跟进时间排序；搜索按客户名过滤。 |
| **列表展示** | 每行仅显示客户名 + 副标题「点击查看跟进」，不显示跟进事项与日期。 |
| **详情数据** | `detailRecords` 仅按 `selectedCustomer.customer_id` 过滤，展示该客户下全部跟进记录；详情头部分组文案为「全部跟进」。 |
| **注意** | setup 的 return 中暴露 `groupsList`（不再使用已移除的 `filteredRecords`），否则会导致列表异常。 |

### 4. 飞书内打开时的登录逻辑

| 项目 | 说明 |
|:---|:---|
| **问题** | 在飞书客户端内打开时，H5 SDK（`window.h5sdk`）可能异步注入，一进页就调 `getAuthCode` 易拿不到 code，从而一直显示「请使用飞书登录」。 |
| **实现** | 新增 `waitForH5Sdk(timeoutMs)`：在飞书环境下若暂无 `window.h5sdk`，则轮询等待（约 3.5 秒）。`getAuthCode` 在飞书环境下先 `await waitForH5Sdk(3500)`，再在存在 `h5sdk` 时调用 `initH5Bridge()` 与 `h5sdk.biz.util.getAuthCode`。 |
| **配置** | 若仍失败，需确认 `config.js`/APP_CONFIG 中已配置 `feishuAppId`（及可选 `feishuRedirectUri`），且飞书应用后台已配置可信域名与重定向 URL。 |

---

## 列表按最近一次跟进记录时间排序与「更新时间」展示（2025-03）

### 变更概述

在 **manager.html** 与 **index.html** 的列表页中，列表按「用户/客户最近一次增加客户跟进记录的时间」倒序排列，并在每项右侧展示该时间为「日志更新时间」或「更新时间」，无记录时显示「暂无」；样式统一为 12px、主色蓝（#3370FF）。

### 1. manager.html

| 层级 | 修改内容 |
|:---|:---|
| **一级：用户列表** | 用户列表按「该用户最近一次增加客户跟进记录的时间」倒序（`last_record_at DESC NULLS LAST`）；每项右侧增加「日志更新时间」，显示该时间，无记录显示「暂无」。 |
| **二级：客户列表** | 客户列表按「该客户最近一次增加客户跟进记录的时间」倒序；每项右侧增加「更新时间」，显示该时间，无记录显示「暂无」。 |
| **样式** | 右侧时间使用 `.list-item-meta`：`font-size: 12px`，`color: var(--primary-color)`（蓝色），与列表项标题同一行、右对齐。 |

**后端**：

- **Repository**：`ManagerUser` 增加 `LastRecordAt *time.Time`；`ListUsersForManager` 的 SQL 增加子查询 `(SELECT MAX(fr.created_at) FROM follow_records fr WHERE fr.user_id = u.id) AS last_record_at`，`ORDER BY last_record_at DESC NULLS LAST, u.name, u.id`。`CustomerFollowGroup` 增加 `LastRecordAt *time.Time`；`ListCustomerFollowGroupsForManager` 改为按 `customer_name` 分组并取 `MAX(created_at) AS last_record_at`，`ORDER BY last_record_at DESC NULLS LAST, customer_name`。
- **manager_api.go**：用户列表返回中增加 `last_record_at`（ISO 或 null）；客户列表（groups）返回中增加 `last_record_at`（ISO 或 null）。

**前端**：一级列表项右侧展示「日志更新时间」+ 格式化时间（`formatDateTime`），二级列表项右侧展示「更新时间」+ 格式化时间；无值时显示「暂无」。复用 `formatDate`、`formatTime`，新增 `formatDateTime`。

### 2. index.html（一级：客户列表）

| 项目 | 说明 |
|:---|:---|
| **排序** | 客户列表（`groupsList`）按「该客户最近一次**增加**客户跟进记录的时间」倒序；取每条记录的 `created_at`（无则回退 `follow_time`）按客户聚合后的最大值作为 `last_record_at`。 |
| **展示** | 每项右侧增加「更新时间」，显示 `last_record_at` 的格式化值，无记录显示「暂无」。 |
| **样式** | 与 manager 一致：`.list-item-meta`，12px，`color: var(--primary-color)`。 |

**前端**：`groupsList` 计算属性中保留每客户的 `last_record_at`，按该字段倒序；ListPage 模板中在 `list-item-header` 右侧增加「更新时间」+ `formatDateTime(group.last_record_at)` 或「暂无」；ListPage 的 methods 中增加 `formatTime`、`formatDateTime`；样式表中增加 `.list-item-meta` 定义。

### 3. 时间含义与数据来源

| 项目 | 说明 |
|:---|:---|
| **「最近一次增加客户跟进记录的时间」** | 以 `follow_records.created_at` 为准（记录写入时间）；manager 后端用 `MAX(created_at)`，index 前端从 GET /records 返回的每条记录的 `created_at` 聚合。 |
| **无记录用户/客户** | 后端排序使用 `NULLS LAST`，无记录项排在列表最后；前端对 null/空显示「暂无」。 |
