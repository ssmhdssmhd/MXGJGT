# MXGJGT

> **MXGJGT** 是一个基于 Go + Gin 开发的视频链接清洗 / 伪装后端服务。
> 访问域名返回伪装 404 页面（`{"code":404,"msg":"页面未找到"}`），只有命中隐藏路由 `/api/clean/?url=` 才会进入真实 API，识别腾讯视频 / 爱奇艺 / 优酷 / 芒果TV / 搜狐视频 / 乐视视频 / B站 等官方链接，自动提取影视名 + 集数，并从配置的资源站列表中搜索替代链接返回。

---

## ✨ 核心特性

| 功能 | 说明 |
|---|---|
| 🕶️ 伪装模式 | 根路径和任意未知路径返回 `404` 伪装 JSON 或 HTML 页面 |
| 🔍 七大平台识别 | 腾讯视频、爱奇艺、优酷、芒果TV、搜狐视频、乐视视频、哔哩哔哩 |
| 🎬 影视名 + 集数提取 | 支持从 URL 和官方页面 HTML meta 双重提取 |
| 🖥️ 官替（官方资源替换） | 官方链接 → 解析剧名+集数 → 资源站匹配 → 输出替换播放地址 |
| 🔄 三层降级方案 | 方案1 无头浏览器渲染（主）→ 方案2 客户端传参 → 方案3 参数提示（保底） |
| 🛠️ 自动更新 | Cron 定时 + 国内镜像测速 + git pull + go build |
| ⚙️ 配置化 | YAML 配置文件，资源站列表独立配置 |
| 🔐 API Key | 手动更新接口可选密钥保护 |

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
curl "http://127.0.0.1:9991/api/clean/?wd=%E7%8B%82%E9%A3%8B&episode=%E7%AC%AC5%E9%9B%9F"

# 版本
curl http://127.0.0.1:9991/api/version
```

> 无头浏览器内核可通过环境变量指定：`MXGJGT_CHROME=/path/to/chrome-headless-shell ./mxgjgt`

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
  "msg": "success",
  "data": {
    "original":   "https://v.qq.com/x/cover/abc123.html",
    "title":      "庆余年",
    "episode":    "01",
    "site":       "腾讯视频",
    "replacement": "https://www.douban.com/subject/12345678/",
    "results":    [
      {"title": "庆余年 第一季", "url": "https://...", "site": "豆瓣电影"}
    ]
  }
}
```

**失败响应（伪装）：**

```json
{"code": 404, "msg": "无url参数"}
{"code": 400, "msg": "无法识别的链接格式"}
```

### `GET /api/version`

返回当前版本、构建信息、更新配置。

### `POST /api/update`

手动触发更新（需要在 `config.yaml` 的 `security.api_key` 设置后携带 `X-API-Key` 或 `?key=`）。

---

## 📂 项目结构

```
MXGJGT/
├── main.go                 # 入口
├── handler/                # HTTP 路由 + 版本
│   ├── api.go
│   └── version.go
├── service/                # 业务逻辑
│   ├── parser.go           # 官方网站识别 + 影视名/集数提取
│   ├── metadata.go         # 第三方元数据 API（B站/腾讯/豆瓣）+ 解析链兜底
│   ├── browser.go          # 方案1：无头浏览器渲染抓取剧名/集数
│   ├── resource.go         # 资源站搜索
│   └── updater.go          # 自动更新（镜像测速 + git pull）
├── config/
│   └── config.go           # 配置加载
├── model/
│   └── types.go            # 数据结构
├── utils/
│   └── utils.go            # HTTP 工具
├── data/
│   ├── config.yaml         # 主配置
│   └── sites.yaml          # 资源站列表
├── static/
│   └── 404.html            # 可选的伪装 HTML 页面
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

1. **Cron 调度**：按 `config.yaml` 的 `updater.interval` 定时执行
2. **镜像测速**：并行 `git ls-remote` 测试所有镜像速度，选最快的
3. **Git Pull**：使用最优镜像 URL 执行 `git pull`
4. **重新编译**：`go build -o mxgjgt .`（使用 `goproxy.cn` 国内代理）
5. **手动触发**：`POST /api/update`（加 API Key 鉴权）

> 当前环境 Git 仓库需要先 clone 再 pull，或在已有仓库目录中运行。

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
