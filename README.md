# MXGJGT

> **MXGJGT** 是一个基于 Go + Gin 开发的视频链接清洗 / 伪装后端服务。
> 访问域名返回伪装 404 页面（默认 nginx 风格 `404 Not Found`），只有命中配置的调用路径（默认 `/api/clean/`）才会进入真实 API。输入腾讯视频 / 爱奇艺 / 优酷 / 芒果TV / 搜狐视频 / 乐视视频 / B站 等官方链接，自动解析影视名 + 集数（**官替**），并从配置的资源站中匹配、输出**替换后的播放地址**；解析失败时可传 `wd` / `episode` 直接指定剧名与集数。另提供 **M3U8 广告过滤（去插播）**：按站点规则剔除 m3u8 里的插播广告片段，输出仍指向源站 CDN 的干净索引。后台 `/mx/` 登录后可可视化管理资源站、平台映射、广告过滤规则、调用路径与在线更新。

---

## ✨ 核心特性

| 功能 | 说明 |
|---|---|
| 🕶️ 伪装模式 | 未知路径返回 nginx 风格 `404 Not Found` 伪装，仅命中调用路径才响应 |
| 🔍 七大平台识别 | 腾讯视频、爱奇艺、优酷、芒果TV、搜狐视频、乐视视频、哔哩哔哩 |
| 🎬 影视名 + 集数提取 | 支持从 URL 和官方页面 HTML meta 双重提取 |
| 🖥️ 官替（官方资源替换） | 官方链接 → 解析剧名+集数 → 资源站匹配 → 输出替换播放地址 |
| 🚫 M3U8 广告过滤（去插播） | 按站点规则剔除 m3u8 插播广告片段，输出仍指向源站 CDN（零代理、零转发） |
| 🔗 `qcb` 字段 | JSON 返回新增 `qcb`（去插播后的播放地址），播放器直接用即走去插播 |
| 🛣️ 自定义调用路径 | 「系统设置 · 路径设置」可自定义 `/api/clean/` 调用路径，支持多路径 |
| 🔄 三层降级方案 | 方案1 无头浏览器渲染（主）→ 方案2 客户端传参 → 方案3 参数提示（保底） |
| 🖥️ 网页后台 | 域名 `/mx/` 登录进入侧边栏仪表盘后台，可视化管理资源站 / 平台映射 / 广告过滤 / 调用路径 / 在线更新，支持全局搜索 |
| 🎞️ 内嵌播放 | 后台资源详情可直接内嵌播放（播放页解析为直链 m3u8，hls.js 播放），无需跳转新窗口 |
| 🔖 版本检查 | 后台「系统 / 更新」显示 当前版本 + 远程最新版本，支持手动检查更新 |
| 🛠️ 自动更新 | 查 GitHub Releases → 下载对应平台二进制 → 原子替换 → 自重启 |
| ⚙️ 配置化 | YAML 配置文件，资源站列表独立配置 |
| 🔐 API Key | 手动更新接口可选密钥保护；后台管理接口需登录 |

---

## 🎯 官替 · 三层降级方案

官替 = 输入官方视频站链接 → 解析出 **剧名 + 集数** → 在资源站中匹配 → 输出 **替换后的播放地址**。三层降级，逐级保底：

| 方案 | 名称 | 说明 |
|---|---|---|
| **方案1（主）** | 无头浏览器渲染 | Chrome Headless 渲染官方页面 DOM，抓取剧名/集数。覆盖爱奇艺 / 优酷 / 芒果等对后端强反爬、纯 HTTP 拿不到真实标题的站点 |
| **方案2** | 客户端直接传参 | 调用方可直接传 `wd=剧名&episode=第X集`，完全跳过官方解析，直接走资源站替换 |
| **方案3（保底）** | 解析失败提示 | 浏览器不可用 / 解析失败时自动回退到 HTTP 解析链（B站API → 腾讯API → HTML meta → 豆瓣兜底）；仍失败则提示补充 `wd` 参数 |

> 无头浏览器为**可选项**：未探测到 Chrome/Chromium 或 `browser.enabled=false` 时，自动降级到方案2/3，无硬依赖。

### 匹配阶段：关键词降级重试

拿到剧名后，资源站匹配会**由严到松逐级重试**关键词，避免官方剧名带上「空格 + 季/部/语言」后缀导致 0 命中：

```
原名 → 去空格 → 去「第X季 / 第X部 / 粤语 / 国语 / 普通话」等后缀 → 取第一个空格/分隔符前的主名
```

> 例：腾讯接口返回「**开心锤锤 第一季**」，资源站只收录「开心锤锤世界」。直接搜原名 0 条；
> 降级到「开心锤锤」后命中，正常输出替换播放地址。

全部候选都没命中时返回 `404`，并在 `msg` 里列出**已尝试过的关键词**，便于判断是「剧名识别错」还是「资源站没收录」：

```json
{"code": 404, "msg": "未在资源站中匹配到资源（已尝试关键词：开心锤锤 第一季 / 开心锤锤第一季 / 开心锤锤）"}
```

### 资源站优先级：平台映射

匹配到多个资源站的结果时，按后台「平台映射」里配置的**资源站顺序**取第一个命中的；该平台没有配置映射（或映射的站点没命中）时，回退到搜索结果的第一条。

```
腾讯视频 → [非凡资源, 吉吉资源 M3U8]   ⇒ 两者都命中时用「非凡资源」
腾讯视频 → [吉吉资源 M3U8, 非凡资源]   ⇒ 两者都命中时用「吉吉资源 M3U8」
```

> 映射里写资源站 **ID**（如 `jjm3u8`），也接受站点名称。填了不存在的 ID 不会报错，会自动回退兜底。

### 解析链顺序

```
B站专用 API → 腾讯视频 API → HTML meta 抓取 → 无头浏览器渲染（方案1）→ 豆瓣兜底搜索 → 参数提示（方案3）
```

---

## 🚀 快速开始

### 1. 编译

```bash
cd MXGJGT
go build -o mxgjgt .
```

### 2. 运行

```bash
# 使用默认配置
./mxgjgt

# 或指定 data 目录
MXGJGT_DATA_DIR=./data ./mxgjgt
```

### 3. 测试

```bash
# 伪装 404
curl http://127.0.0.1:9991/
# => {"code":404,"msg":"页面未找到"}

# 无 url 参数
curl http://127.0.0.1:9991/api/clean/
# => {"code":404,"msg":"无url参数"}

# 方案1 - 清洗官方链接（无头渲染抓剧名）
curl "http://127.0.0.1:9991/api/clean/?url=https%3A%2F%2Fwww.iqiyi.com%2Fa_1fkgtbddd2x.html"

# 方案2 - 客户端直接传参
curl "http://127.0.0.1:9991/api/clean/?wd=%E7%8B%82%E9%A3%99&episode=%E7%AC%AC5%E9%9B%86"

# 版本
curl http://127.0.0.1:9991/api/version
```

> 无头浏览器内核可通过环境变量指定：`MXGJGT_CHROME=/path/to/chrome-headless-shell ./mxgjgt`

---

## 🖥️ 网页后台（/mx/）

浏览器访问 `http://<域名>:9991/mx/` 即进入登录页，登录后可视化维护资源站、平台映射与系统更新，无需再用 curl。

后台采用**侧边栏仪表盘**布局：左侧白色圆角侧边栏（顶部 MXLOGO 蝴蝶徽标）承载主导航，右侧为内容区，全屏紫粉渐变背景 + 半透明白色卡片；窄屏（< 960px）自动转为横向顶栏。顶栏含**按时间问候语**（凌晨/上午/中午/下午/晚上）、**全局搜索**（在已启用资源站中并发查询并汇总）与通知 / 帮助 / 用户图标。

侧边栏菜单：

| 菜单 | 说明 |
|---|---|
| 🏠 首页 | 概览统计卡片（资源站数 / 已启用 / 平台映射数 / 当前版本）+ 快捷入口 |
| 📡 资源站管理 | 资源站列表与增删改查 |
| 🔗 平台映射 | 官方平台 → 优先资源站 |
| 🚫 M3U8 广告过滤 | 站点规则（增删改）+ 探测 / 测试 / 调用日志 |
| 🛣️ 系统设置 | 路径设置：自定义 `/api/clean/` 调用路径 |
| ⚙️ 系统 · 更新 | 版本 / 构建时间 / 更新仓库 / 定时更新状态 |

功能明细：

| 功能 | 说明 |
|---|---|
| 🔐 登录 | 默认账号 `admin` / `mxgjgt`（见下方 `admin` 配置，**请务必修改**） |
| 📡 资源站管理 | 列表 / 新增 / 编辑 / 启用停用 / 删除，含「苹果CMS 自动分析」 |
| 🧪 测试 | 用关键词向该资源站发一次真实请求，返回 状态 / 耗时 / 命中条数 / 分类数 / 结果样例 |
| 🔍 搜索 | 在该资源站内搜索关键词，列出 标题 / 分类 / 备注 / 资源ID，人工核对资源质量；顶栏全局搜索可跨多个已启用站点并发查询并汇总 |
| 🎬 详情 / 内嵌播放 | 在搜索结果点「详情」查看剧集列表；点「内嵌播放」直接在本页播放（播放页自动解析为直链 m3u8，hls.js 播放，失败自动降级「新窗口」），亦可「新窗口」跳转或「复制」地址 |
| 🔄 同步 | 重新拉取该资源站分类列表并写回 `cid`（不改动搜索地址 / 详情地址） |
| 🔗 平台映射 | 勾选每个官方平台优先使用的资源站 |
| 🚫 M3U8 广告过滤 | 站点规则 CRUD（解析方式 / 检测方式 / 索引模式 / 码率 / 匹配前缀与正则 / 时长指纹 / 片段级正则）；「探测」给出广告区间与指纹建议、「测试」返回保留/剔除与过滤后播放列表、「调用日志」查看最近 200 次去插播调用 |
| 🛣️ 系统设置 · 路径设置 | 增删多条调用路径（`http://` / `https://`），任一启用路径命中即生效；全部停用时回退默认 `/api/clean` |
| ⚙️ 系统 / 更新 | 查看 当前版本 / 远程最新版本 / 构建时间 / 更新仓库 / 定时更新状态；「检查更新」只查不更、「立即更新」下载替换并自重启（仅在远程版本更高时执行） |

- 登录态由 Cookie 会话维持，有效期 `admin.session_hours` 小时
- `/api/admin/*` 全部要求登录后访问，未登录返回 `401`
- `admin.enabled: false` 时 `/mx/` 与 `/api/admin/*` 统一返回伪装 `404`
- 后台页面位于 [handler/mxadmin.html](handler/mxadmin.html)，登录鉴权见 [handler/adminauth.go](handler/adminauth.go)
- Logo / favicon / 侧边栏徽标已**内嵌进二进制**（[handler/assets/](handler/assets) + [handler/adminassets.go](handler/adminassets.go)），
  由 `/mx/assets/logo.png`、`/mx/assets/favicon.png`、`/mx/assets/logo-mark.png` 自托管，不依赖 `raw.githubusercontent.com` 等外部图床
- 内嵌播放依赖的 **hls.js v1.5.20**（Apache-2.0）同样内嵌进二进制（[handler/assets/hls.min.js](handler/assets/hls.min.js)），
  由 `/mx/assets/hls.js` 自托管、**按需懒加载**，Safari 走原生 HLS 分支不加载该库，Chrome/Edge/Firefox 才拉取

---

## 📡 API 接口

### `GET /api/clean/?url=<待清洗URL>[&wd=<剧名>&episode=<集数>]`

| 参数 | 必填 | 说明 |
|---|---|---|
| `url` | 否* | 官方视频平台链接（方案1/3 解析剧名+集数） |
| `wd` / `title` | 否* | 剧名（方案2，客户端直接传参；与 `url` 同时传则优先采用） |
| `episode` / `jj` | 否 | 集数，如 `第5集` / `05` |

> \* `url` 与 `wd` 至少提供一个。仅传 `wd` 时走方案2，无需官方解析。
>
> - 方案1：`/api/clean/?url=https://www.iqiyi.com/a_xxx.html`（无头渲染抓剧名）
> - 方案2：`/api/clean/?wd=狂飙&episode=第5集`（直接传参）

**成功响应：**

```json
{
  "code": 200,
  "jm":   "狂飙",
  "jj":   "第5集",
  "url":  "https://svip.jijiplay1.com/watch/xxxx",
  "qcb":  "https://<域名>/api/clean/?type=m3u8&url=https%3A%2F%2Fsvip.jijiplay1.com%2Fwatch%2Fxxxx",
  "msg":  "https://svip.jijiplay1.com/watch/xxxx",
  "time": "2026-10-18 12:00:00",
  "kfz":  "射手沫蝴蝶"
}
```

| 字段 | 说明 |
|---|---|
| `code` | `200` 成功 / `404` 未匹配到资源 / `400` 参数或解析错误 |
| `jm` | 剧名 |
| `jj` | 集数 |
| `url` / `msg` | 替换后的播放地址 |
| `qcb` | **去插播后的播放地址**：`<主调用路径>/?type=m3u8&url=<播放地址>`，播放器直接用即自动剔除插播广告 |
| `time` | 响应时间 |
| `kfz` | 制作方（射手沫蝴蝶） |

> `url` 指向 m3u8 / 带 `type=m3u8` 时，同一调用路径**切换为「去插播」模式**：拉取索引 → 剔除广告片段 → 直接返回过滤后的 m3u8 文本（仍指向源站 CDN）。过滤失败自动 `302` 回源。

**失败响应（伪装）：**

```json
{"code": 404, "msg": "无url参数"}
{"code": 404, "msg": "未在资源站中匹配到资源"}
{"code": 400, "msg": "无法识别的链接格式"}
```

### `GET /api/search/?wd=<剧名>[&episode=<集数>]`

按剧名直接在资源站搜索并返回替换地址（无需官方链接）。

| 参数 | 必填 | 说明 |
|---|---|---|
| `wd` | 是 | 剧名 |
| `episode` | 否 | 集数，如 `第5集` / `05` |

响应格式同 `/api/clean/`（成功返回 `jm` / `jj` / `url`）。

### `GET /api/version`

返回当前版本、构建信息、更新配置。

### `POST /api/update`

手动触发更新：拉取 GitHub Releases 最新版本 → 下载对应平台二进制 → 原子替换 → 自重启。

已登录网页后台时免鉴权；否则需在 `security.api_key` 设值后携带 `X-API-Key` 或 `?key=`。

### 后台管理 `/api/admin/*`

> 以下接口均需先在网页后台（`/mx/`）登录，未登录返回 `401`；`admin.enabled: false` 时统一返回伪装 `404`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/admin/sites` | 列出全部资源站 |
| POST | `/api/admin/site/auto` | 苹果CMS 接口自动分析（返回候选配置） |
| GET | `/api/admin/site/test?id=&wd=` | 资源站连通性测试（关键词默认「庆余年」），返回 状态/耗时/命中条数/分类数/样例 |
| GET | `/api/admin/site/search?id=&wd=` | 在指定资源站内搜索关键词，返回结果列表 |
| GET | `/api/admin/site/detail?id=&rid=` | 获取指定资源详情与剧集播放列表（后台详情 / 内嵌播放用） |
| GET | `/api/admin/stream?page=` | 解析采集站播放页 → 直链 m3u8（供后台内嵌播放，hls.js 直连） |
| GET | `/api/admin/update/check` | 只读检查远程最新版本：返回 `current / latest / has_update / release / published / repo / enabled / error` |
| POST | `/api/admin/site/sync?id=` | 同步资源站分类：重新拉取分类列表并写回 `cid` |
| POST | `/api/admin/site` | 新增资源站 |
| PUT | `/api/admin/site/:id` | 更新资源站 |
| DELETE | `/api/admin/site/:id` | 删除资源站 |
| GET | `/api/admin/mappings` | 查看 官方站→资源站 映射 |
| POST | `/api/admin/mapping` | 设置 官方站→资源站 映射 |
| GET | `/api/admin/filter/sites` | 列出 M3U8 广告过滤站点规则 |
| POST | `/api/admin/filter/site` | 新增过滤规则 |
| PUT | `/api/admin/filter/site/:id` | 更新过滤规则（`id` 支持 ID 或名称） |
| DELETE | `/api/admin/filter/site/:id` | 删除过滤规则 |
| POST | `/api/admin/filter/probe` | 探测 m3u8：返回广告区间 / 片段标注 / 时长指纹建议（`url` 支持 m3u8 或播放页） |
| POST | `/api/admin/filter/test` | 过滤测试：返回 保留/剔除 / 广告区间 / 检测方式 / 过滤后播放列表预览 |
| GET | `/api/admin/filter/logs` | 最近 200 次去插播调用日志 |
| GET | `/api/admin/cleanpaths` | 查看调用路径设置 |
| POST | `/api/admin/cleanpaths` | 保存调用路径设置（立即生效） |
| POST | `/api/admin/reload` | 热重载配置 |

> `id` 支持资源站 ID 或名称。未找到返回 `404`；测试/同步失败返回 `code: 400` + 原因。

---

## 📂 项目结构

```
MXGJGT/
├── main.go                 # 入口
├── handler/                # HTTP 路由
│   ├── api.go              # 核心接口（clean/search/version/update）+ clean 路径动态分发 + nginx 404 伪装
│   ├── filter.go           # M3U8 广告过滤后台 API + 系统设置·路径设置 API
│   ├── admin.go            # 后台管理 API（资源站 / 映射 CRUD + 测试/搜索/同步）
│   ├── adminauth.go        # 网页后台登录鉴权 + 会话
│   ├── adminassets.go      # 后台 logo / favicon / 侧边栏徽标（go:embed 内嵌）
│   ├── assets/             # 内嵌静态资源（logo.png / favicon.png / logo-mark.png / hls.min.js）
│   ├── mxadmin.html        # 网页后台页面（侧边栏仪表盘：首页 / 资源站 / 映射 / 广告过滤 / 系统设置 / 系统·更新）
│   └── version.go          # 版本兼容导出
├── service/                # 业务逻辑
│   ├── parser.go           # 官方网站识别 + 影视名/集数提取
│   ├── metadata.go         # 第三方元数据 API（B站/腾讯/豆瓣）+ 解析链兜底
│   ├── browser.go          # 方案1：无头浏览器渲染抓取剧名/集数
│   ├── resource.go         # 资源站搜索 + 苹果CMS 采集
│   ├── acms.go             # 苹果CMS 接口自动分析
│   ├── filter.go           # 去插播核心引擎（解析 / 广告识别原语 / 码率选择 / 重写 / 缓存 / 日志）
│   ├── cache.go            # 搜索结果缓存
│   └── updater.go          # 自动更新（GitHub Releases 下载 + 原子替换 + 自重启）
├── config/
│   └── config.go           # 配置加载 / 热加载 / 持久化（含 cleanpaths / filter_sites）
├── model/
│   ├── types.go            # 数据结构（Response 含 qcb）
│   ├── filter.go           # 广告过滤数据结构
│   └── version.go          # 版本常量
├── utils/
│   └── utils.go            # HTTP 工具
├── data/
│   ├── config.yaml         # 主配置
│   ├── sites.yaml          # 资源站列表
│   ├── mappings.yaml       # 官方站→资源站映射
│   ├── cleanpaths.yaml     # 自定义调用路径（系统设置 · 路径设置）
│   └── filter_sites.yaml   # M3U8 广告过滤站点规则
├── static/
│   └── 404.html            # 可选的自定义伪装 HTML（默认内置 nginx 风格 404）
├── .github/workflows/
│   └── release.yml         # GitHub Actions 云端编译发布
└── go.mod
```

---

## ⚙️ 配置说明

### `data/config.yaml`

```yaml
server:
  port: 9991
  mode: release              # gin 模式: debug / release / test
  not_found_html: ""         # 自定义伪装 404 HTML 路径

updater:
  enabled: false
  interval: "0 0 3 * * *"    # 每天凌晨 3 点
  branch: main
  repo_url: "https://github.com/your/repo.git"
  mirrors:                   # 国内加速镜像
    - "https://ghproxy.com/{origin}"
    - "https://gh-proxy.com/{origin}"

security:
  api_key: ""                # 设值后 /api/update 需鉴权

# 网页后台（浏览器访问 http://<域名>:9991/mx/ 进入登录页）
admin:
  enabled: true              # 关闭后 /mx/ 与 /api/admin/* 均走伪装 404
  path: "/mx"                # 后台入口路径
  username: "admin"          # 登录用户名
  password: "mxgjgt"         # 登录密码（请务必修改）
  session_hours: 24          # 登录态有效期（小时）

# 无头浏览器渲染（方案1 主方案）
browser:
  enabled: true              # 总开关；未配置时默认启用
  chrome_path: ""            # 留空自动探测，也可用环境变量 MXGJGT_CHROME 指定
  timeout_sec: 20            # 单次渲染超时秒数
```

> Chrome 探测顺序：`config.browser.chrome_path` → 环境变量 `MXGJGT_CHROME` → 常见安装路径 → `PATH`。找不到时自动降级到方案2/3。

### `data/sites.yaml`

配置你的资源站搜索源：

```yaml
- name: "示例资源站"
  url: "https://example.com/search?kw={keyword}"   # {keyword} 会被替换
  encode: "utf8"
  selector: ".list-item"                           # goquery 选择器
  title_attr: ".title-text"                        # 或 ".title-text@href"
  link_attr: ".title-text a@href"
  enabled: true
```

---

## 🔄 自动更新机制

1. **Cron 调度**：`updater.enabled: true` 时按 `interval` 定时执行
2. **查最新版**：调用 GitHub Releases API，比对当前 `model.Version`
3. **下载产物**：按当前 `GOOS/GOARCH` 选对应二进制，可走 `updater.mirrors` 镜像加速
4. **原子替换**：旧文件改名为 `.bak`，新文件落位并 `chmod 0755`
5. **自重启**：执行新二进制并退出旧进程
6. **手动触发**：`POST /api/update`（登录网页后台免鉴权，否则需 API Key）

---

## 📦 云端编译（GitHub Actions）

推送 `v*` tag 或手动触发 `Release` 工作流，即可在云端构建并发布多平台产物：

```bash
git tag v0.0.13 && git push origin v0.0.13
```

- 矩阵构建：Linux `amd64`/`arm64`、Windows `amd64`、macOS `amd64`/`arm64`
- 产物自动打包 `zip` + `SHA256.txt`，挂到 GitHub Release
- 版本号经 `-ldflags -X mxgjgt/model.Version=` 注入

---

## 🌐 支持的官方平台

| 平台 | 主域名 | 识别域名 |
|---|---|---|
| 腾讯视频 | `v.qq.com` | `v.qq.com`, `film.qq.com`, `m.v.qq.com` |
| 爱奇艺 | `iqiyi.com` | `iqiyi.com`, `www.iqiyi.com`, `m.iqiyi.com` |
| 优酷 | `youku.com` | `youku.com`, `www.youku.com`, `v.youku.com` |
| 芒果TV | `mgtv.com` | `mgtv.com`, `www.mgtv.com`, `m.mgtv.com` |
| 搜狐视频 | `tv.sohu.com` | `tv.sohu.com`, `sohu.com` |
| 乐视视频 | `letv.com` | `letv.com`, `www.letv.com` |
| 哔哩哔哩 | `bilibili.com` | `bilibili.com`, `www.bilibili.com`, `b23.tv` |

---

## 📝 版本历史

见 [update.log](update.log)

## 📄 License

MIT
