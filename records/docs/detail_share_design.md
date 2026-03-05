# 跟进信息详情页截图与分享设计说明

## 概述

在 manager.html 三级页面（客户跟进信息/跟进详情）使用 html2canvas 对详情页截图，通过飞书客户端 `tt.share` 将图片分享到微信、朋友圈、系统（含飞书等）；非飞书环境或分享失败时降级为复制到剪贴板，并提示用户可粘贴到其他应用。

---

## 一、可行性结论

| 环节 | 结论 | 说明 |
|------|------|------|
| html2canvas 对当前详情页截图 | **可行** | 详情页为纯文本 + 内联 SVG，无跨域图片，适合 html2canvas |
| 飞书 tt.share 分享截图 | **可行** | 使用 tt.share({ contentType: "image", image: base64 }) 直接传 base64 图片，可分享到 wx / wx_timeline / system 等 |
| 备选：复制到剪贴板 | **可行** | 非飞书环境或 tt 不可用时，可用 Clipboard API 将截图复制为图片 |

---

## 二、页面与 DOM 结构

- **三级页面**：`level === 'detail'` 时的整块内容，即 `pages/manager.html` 中跟进详情根节点 `<div v-if="level === 'detail'" class="page">`。
- **包含**：顶部导航（返回 +「跟进详情」+ 分享按钮）、客户卡片（头像、客户名、全部跟进、共 N 次跟进）、时间线列表（每条：日期、时间、方式、AI 标签、跟进事项、联系人、预期目标、实际结果、存在风险、下一步计划）。
- **截图目标**：该 `.page` 根节点（通过 `ref="detailPageRef"` 获取），当前无跨域图片，html2canvas 兼容性良好。

---

## 三、技术要点

### 3.1 html2canvas 截图

- **脚本**：引入 `https://html2canvas.hertzen.com/dist/html2canvas.min.js`。
- **选择器**：对三级页面根节点（`detailPageRef`）调用 `html2canvas(el, { scale: 2 })`。
- **输出**：`canvas.toDataURL('image/png')` 得到 Data URL；飞书 tt.share 支持带 `data:image/png;base64,...` 前缀的字符串，可直接传入，无需 strip 前缀。

### 3.2 飞书 tt.share 参数说明（分享图片）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| channelType | string[] | 否 | 分享渠道。默认：飞书 `["wx","wx_timeline"]`，Lark `["system"]`。可选值：`wx` 微信、`wx_timeline` 微信朋友圈、`system` 系统分享（飞书 V4.5.0+）。仅 1 个渠道时直接分享不弹选择面板。Lark 仅支持 `system`。 |
| contentType | string | **是** | 内容类型：`text` / `image` / `url`。分享截图为 `"image"`。 |
| title | string | 否 | 标题，仅当 contentType=`url` 时必填。 |
| content | string | 否 | 正文，当 contentType=`text` 时不能为空。 |
| image | string | 否 | Base64 图片。当 contentType=`"image"` 时**不能为空**。支持带 `data:image/png;base64,...` 前缀的字符串，可直接传 `canvas.toDataURL('image/png')`。 |

**调用示例（截图分享）：**

```javascript
tt.share({
  channelType: ["wx", "wx_timeline", "system"],
  contentType: "image",
  image: canvas.toDataURL("image/png"),
  success(res) { console.log(JSON.stringify(res)); },
  fail(res) { console.log("share fail: " + JSON.stringify(res)); }
});
```

### 3.3 备选：复制到剪贴板

- **流程**：canvas → `canvas.toBlob('image/png')` → `ClipboardItem` → `navigator.clipboard.write([item])`。
- **提示文案**：已复制到剪贴板，可粘贴到微信、飞书或其他应用。

---

## 四、实现要点（已实现）

1. **引入 html2canvas**：在 manager.html 中增加上述 script 引用。
2. **三级页面根节点**：增加 `ref="detailPageRef"`。
3. **分享入口**：在跟进详情页导航栏右侧增加「分享」按钮（加载完成时显示）。
4. **截图与分享逻辑**：
   - 点击「分享」→ `html2canvas(detailPageRef, { scale: 2 })` → 得到 canvas。
   - 若存在 `tt.share`：调用 `tt.share({ channelType: ['wx','wx_timeline','system'], contentType: 'image', image: canvas.toDataURL('image/png') })`；失败时降级为复制到剪贴板并提示。
   - 若无 `tt`：直接复制到剪贴板并提示「已复制到剪贴板，可粘贴到微信、飞书或其他应用」。
5. **错误处理**：html2canvas 失败提示「生成截图失败，请重试」；复制失败提示环境限制或请重试。

---

## 五、风险与注意点

- **tt.share 的 image 格式**：官方文档明确接口会处理带前缀的 Data URL，实现时直接传 `canvas.toDataURL('image/png')` 即可。
- **长列表**：若单客户跟进条数很多，整页高度大，html2canvas 可能较慢或内存占用高，可考虑「仅截首屏」或「分屏截多张」的折中。
- **CSP/跨域**：若将来详情页出现头像等跨域图片，需为图片配置 CORS 或使用代理同源化，否则 canvas 可能被污染或报错。

---

## 六、参考

- 飞书客户端 API 文档：<https://open.feishu.cn/document/client-docs/gadget/-web-app-api/open-ability/share/thirdShare>
- html2canvas：<https://html2canvas.hertzen.com/>
