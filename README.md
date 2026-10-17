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
| 📘 cover_id 字典 | 本地 `cover_id → 剧名` 字典（0 网络依赖、可热更新、各平台 pattern 自动提取） |
| 🔄 资源站替换 | 可配置多个资源站，自动从资源站搜索影视并返回替代链接 |
| 🛠️ 自动更新 | Cron 定时 + 国内镜像测速 + git pull + go build |
| ⚙️ 配置化 | YAML 配置文件，资源站列表独立配置 |
| 🔐 API Key | 手动更新接口可选密钥保护 |

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
curl http://127.0.0.1:8080/
# => {"code":404,"msg":"页面未找到"}

# 无 url 参数
curl http://127.0.0.1:8080/api/clean/
# => {"code":404,"msg":"无url参数"}

# 真实 API - 清洗腾讯视频链接
curl "http://127.0.0.1:8080/api/clean/?url=https%3A%2F%2Fv.qq.com%2Fx%2Fcover%2Fabc123.html"

# 版本
curl http://127.0.0.1:8080/api/version
```

---

## 📡 API 接口

### `GET /api/clean/?url=<待清洗URL>`

| 参数 | 必填 | 说明 |
|---|---|---|
| `url` | 是 | 用户提交的官方视频平台链接 |

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

### 📘 官方站 cover_id → 剧名 字典管理

字典文件 `data/cover_ids.json` 首次启动自动生成，各平台预置 host + regex pattern 骨架。支持以下管理接口：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/admin/dict/` | 所有平台统计（平台名 / 条目数 / hosts） |
| GET | `/api/admin/dict/:platform` | 列出某平台全部 `cover_id → 剧名` |
| POST | `/api/admin/dict/` | 新增 / 更新 一条（JSON: `{"platform":"腾讯视频","cover_id":"abc123","title":"庆余年"}`） |
| DELETE | `/api/admin/dict/:platform/:cover_id` | 删除一条 |
| POST | `/api/admin/dict/reload` | 热重载磁盘上的字典文件 |

**解析链位置**：字典命中是第 3 层（wd手动 → B站 API → **字典** → HTML meta → 豆瓣搜索），0 网络依赖秒级返回。

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
│   ├── metadata.go         # B站 API + 豆瓣兜底
│   ├── coverid_dict.go     # 官方站 cover_id → 剧名 字典引擎
│   ├── resource.go         # 资源站搜索
│   └── updater.go          # 自动更新（镜像测速 + git pull）
├── config/
│   └── config.go           # 配置加载
├── model/
│   ├── types.go            # 数据结构
│   └── version.go          # 版本号
├── utils/
│   └── utils.go            # HTTP 工具
├── data/
│   ├── config.yaml         # 主配置
│   ├── sites.yaml          # 资源站列表
│   ├── mappings.yaml       # 官方站→资源站优先映射
│   └── cover_ids.json      # 官方站 cover_id → 剧名 字典数据
├── static/
│   └── 404.html            # 可选的伪装 HTML 页面
└── go.mod
```

---

## ⚙️ 配置说明

### `data/config.yaml`

```yaml
server:
  port: 8080
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
```

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
