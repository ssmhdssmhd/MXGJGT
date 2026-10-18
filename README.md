# MXGJGT

> **MXGJGT** 是一个基于 Go + Gin 开发的视频链接清洗 / 伪装后端服务。
> 访问域名返回伪装 404 页面（`{"code":404,"msg":"页面未找到"}`），只有命中隐藏路由 `/api/clean/` 才会进入真实 API。输入腾讯视频 / 爱奇艺 / 优酷 / 芒果TV / 搜狐视频 / 乐视视频 / B站 等官方链接，自动解析影视名 + 集数（**官替**），并从配置的资源站中匹配、输出**替换后的播放地址**；解析失败时可传 `wd` / `episode` 直接指定剧名与集数。另提供网页后台 `/mx/`，登录后可可视化管理资源站、平台映射与在线更新。

---

## ✨ 核心特性

| 功能 | 说明 |
|---|---|
| 🕶️ 伪装模式 | 根路径和任意未知路径返回 `404` 伪装 JSON 或 HTML 页面 |
| 🔍 七大平台识别 | 腾讯视频、爱奇艺、优酷、芒果TV、搜狐视频、乐视视频、哔哩哔哩 |
| 🎬 影视名 + 集数提取 | 支持从 URL 和官方页面 HTML meta 双重提取 |
| 🖥️ 官替（官方资源替换） | 官方链接 → 解析剧名+集数 → 资源站匹配 → 输出替换播放地址 |
| 🔄 三层降级方案 | 方案1 无头浏览器渲染（主）→ 方案2 客户端传参 → 方案3 参数提示（保底） |
| 🖥️ 网页后台 | 域名 `/mx/` 登录后可视化管理资源站 / 平台映射 / 在线更新 |
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

| 功能 | 说明 |
|---|---|
| 🔐 登录 | 默认账号 `admin` / `mxgjgt`（见下方 `admin` 配置，**请务必修改**） |
| 📡 资源站管理 | 列表 / 新增 / 编辑 / 启用停用 / 删除，含「苹果CMS 自动分析」 |
| 🧪 测试 | 用关键词向该资源站发一次真实请求，返回 状态 / 耗时 / 命中条数 / 分类数 / 结果样例 |
| 🔍 搜索 | 在该资源站内搜索关键词，列出 标题 / 分类 / 备注 / 资源ID，人工核对资源质量 |
| 🔄 同步 | 重新拉取该资源站分类列表并写回 `cid`（不改动搜索地址 / 详情地址） |
| 🔗 平台映射 | 勾选每个官方平台优先使用的资源站 |
| ⚙️ 系统 | 查看版本 / 构建时间 / 仓库，重载配置、检查并更新 |

- 登录态由 Cookie 会话维持，有效期 `admin.session_hours` 小时
- `/api/admin/*` 全部要求登录后访问，未登录返回 `401`
- `admin.enabled: false` 时 `/mx/` 与 `/api/admin/*` 统一返回伪装 `404`
- 后台页面位于 [handler/mxadmin.html](handler/mxadmin.html)，登录鉴权见 [handler/adminauth.go](handler/adminauth.go)
- Logo / favicon 已**内嵌进二进制**（[handler/assets/](handler/assets) + [handler/adminassets.go](handler/adminassets.go)），
  由 `/mx/assets/logo.png`、`/mx/assets/favicon.png` 自托管，不依赖 `raw.githubusercontent.com` 等外部图床

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
| `time` | 响应时间 |
| `kfz` | 制作方（射手沫蝴蝶） |

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
| POST | `/api/admin/site/sync?id=` | 同步资源站分类：重新拉取分类列表并写回 `cid` |
| POST | `/api/admin/site` | 新增资源站 |
| PUT | `/api/admin/site/:id` | 更新资源站 |
| DELETE | `/api/admin/site/:id` | 删除资源站 |
| GET | `/api/admin/mappings` | 查看 官方站→资源站 映射 |
| POST | `/api/admin/mapping` | 设置 官方站→资源站 映射 |
| POST | `/api/admin/reload` | 热重载配置 |

> `id` 支持资源站 ID 或名称。未找到返回 `404`；测试/同步失败返回 `code: 400` + 原因。

---

## 📂 项目结构

```
MXGJGT/
├── main.go                 # 入口
├── handler/                # HTTP 路由
│   ├── api.go              # 核心接口（clean/search/version/update）
│   ├── admin.go            # 后台管理 API（资源站 / 映射 CRUD + 测试/搜索/同步）
│   ├── adminauth.go        # 网页后台登录鉴权 + 会话
│   ├── adminassets.go      # 后台 logo / favicon（go:embed 内嵌）
│   ├── assets/             # 内嵌静态资源（logo.png / favicon.png）
│   ├── mxadmin.html        # 网页后台页面（登录 + 管理界面）
│   └── version.go          # 版本兼容导出
├── service/                # 业务逻辑
│   ├── parser.go           # 官方网站识别 + 影视名/集数提取
│   ├── metadata.go         # 第三方元数据 API（B站/腾讯/豆瓣）+ 解析链兜底
│   ├── browser.go          # 方案1：无头浏览器渲染抓取剧名/集数
│   ├── resource.go         # 资源站搜索 + 苹果CMS 采集
│   ├── acms.go             # 苹果CMS 接口自动分析
│   ├── cache.go            # 搜索结果缓存
│   └── updater.go          # 自动更新（GitHub Releases 下载 + 原子替换 + 自重启）
├── config/
│   └── config.go           # 配置加载 / 热加载 / 持久化
├── model/
│   ├── types.go            # 数据结构
│   └── version.go          # 版本常量
├── utils/
│   └── utils.go            # HTTP 工具
├── data/
│   ├── config.yaml         # 主配置
│   ├── sites.yaml          # 资源站列表
│   └── mappings.yaml       # 官方站→资源站映射
├── static/
│   └── 404.html            # 可选的伪装 HTML 页面
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
git tag v0.0.7 && git push origin v0.0.7
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
