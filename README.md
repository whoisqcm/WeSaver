<div align="center">

# WeSaver

**微信公众号文章批量备份工具**

*Batch backup tool for WeChat Official Account articles*

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Windows-0078D6?style=flat-square&logo=windows&logoColor=white)](https://www.microsoft.com/windows)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/whoisqcm/WeSaver?style=flat-square&color=blue)](https://github.com/whoisqcm/WeSaver/releases/latest)
[![Download](https://img.shields.io/github/downloads/whoisqcm/WeSaver/total?style=flat-square&color=brightgreen)](https://github.com/whoisqcm/WeSaver/releases/latest)

<img src="https://img.shields.io/badge/WeChat-Article_Archiver-07C160?style=for-the-badge&logo=wechat&logoColor=white" alt="WeChat Article Archiver"/>

<br/>

[**>>> 下载最新版 / Download Latest Release <<<**](https://github.com/whoisqcm/WeSaver/releases/latest)

---

[功能特性](#-功能特性) | [快速开始](#-快速开始) | [使用指南](#-使用指南) | [构建](#%EF%B8%8F-从源码构建) | [项目结构](#-项目结构)

</div>

---

## 功能特性

> **一键备份公众号历史文章，支持多种格式导出，离线阅读无忧。**

| 功能 | 说明 |
|------|------|
| **自动抓取 Token** | 通过本地代理自动捕获微信认证 Token，无需手动抓包 |
| **批量下载** | 支持按页数或文章数量批量抓取公众号全部历史文章 |
| **多格式导出** | HTML 原文 / Markdown / Excel 详情表，自由选择 |
| **离线阅读优化** | 导出的 HTML 自动清理外部脚本、修复懒加载图片，可直接双击打开 |
| **断点续传** | 中断后重新运行可跳过已下载的文章 |
| **评论抓取** | 可选同时抓取文章精选评论 |
| **原生桌面窗口** | 基于 WebView2 的原生 Windows 窗口，无需浏览器 |
| **速度调节** | 慢速 / 标准 / 高速三档，配合并发数灵活控制 |
| **安全代理** | 仅监听 `127.0.0.1`，退出自动恢复系统代理设置 |

---

## Quick Start (English)

> **Batch backup WeChat Official Account articles to HTML / Markdown / Excel — offline-ready.**

### Features

- **Auto Token Capture** — local proxy intercepts WeChat auth token automatically
- **Batch Download** — crawl full article history by page count or article limit
- **Multi-format Export** — HTML (offline-optimized) / Markdown / Excel spreadsheet
- **Resume Support** — skip already-downloaded articles on re-run
- **Comment Fetching** — optionally grab featured comments
- **Native Desktop UI** — WebView2 window on Windows, browser fallback otherwise
- **Speed Control** — slow / normal / fast profiles with adjustable concurrency
- **Safe Proxy** — listens on `127.0.0.1` only, restores system proxy on exit

### Requirements

- **Windows 10/11** (WebView2 runtime, usually pre-installed)
- **WeChat for PC** (to trigger token capture)
- **Go 1.24+** (for building from source only)

### Download

> **[Download WeSaver.exe from Latest Release](https://github.com/whoisqcm/WeSaver/releases/latest)**
>
> Just download and double-click — no installation required.

---

## 快速开始

### 系统要求

| 项目 | 要求 |
|------|------|
| 操作系统 | Windows 10 / 11 |
| 运行时 | WebView2 Runtime（Win10+ 通常已预装） |
| 微信 | PC 版微信（用于触发 Token 抓取） |
| 编译环境 | Go 1.24+（仅从源码构建时需要） |

### 下载与运行

> **[点击这里下载最新版 WeSaver.exe](https://github.com/whoisqcm/WeSaver/releases/latest)** — 下载后双击运行，无需安装。

或从源码构建：

```powershell
# 克隆仓库
git clone https://github.com/whoisqcm/WeSaver.git
cd WeSaver

# 构建
.\build.ps1

# 运行
.\wesaver.exe
```

---

## 使用指南

### 工作流程

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  1. 抓 Token │ ──→ │  2. 校验有效  │ ──→ │  3. 配置参数  │ ──→ │  4. 执行任务  │
│  Start Proxy │     │  Validate    │     │  Configure   │     │  Run Task    │
└─────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
```

### Step 1: 获取 Token

**方式 A — 自动抓取（推荐）：**

1. 点击界面上的 **「开始抓 token」**
2. 首次使用会弹出证书安装确认，点击「是」
3. 在 **微信 PC 端**打开任意公众号的历史文章列表
4. Token 自动回填到输入框

**方式 B — 手动粘贴：**

1. 使用 Fiddler / Charles 等工具抓取 `mp/profile_ext` 开头的请求 URL
2. 粘贴到 Token 链接输入框
3. 点击 **「校验 token」** 确认有效

### Step 2: 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| 任务名称 | 自动填充 | 输出目录名称 |
| 输出目录 | `output` | 文件保存根目录 |
| 抓取页数 | 1 | 每页约 10 篇文章 |
| 抓取数量 | 0（不截断） | 截断上限，0 表示不截断（实际范围由页数决定） |
| 抓取速度 | 慢速 | 慢速 / 标准 / 高速 |
| 线程并发 | 2 | 同时下载的文章数 |
| 详情采样率 | 0.25 | 下载文章详情的比例（0~1） |

### Step 3: 选择导出格式

- [x] **HTML 原文** — 离线优化，图片可见，无外部依赖
- [x] **Markdown** — 适合知识库、笔记软件
- [x] **详情 Excel** — 文章标题、链接、发布时间等元数据汇总
- [ ] **抓取评论** — 可选，会增加请求量

### Step 4: 执行

点击 **「执行任务」**，日志区实时显示进度。完成后自动打开输出目录。

---

## 输出目录结构

```
output/
└── 公众号名称_20250304_020000/
    ├── raw/
    │   ├── html/          ← HTML 原文（离线可读）
    │   ├── md/            ← Markdown 转换
    │   └── assets/        ← 资源文件
    ├── data/              ← Excel 数据表
    ├── manifest.json      ← 抓取清单
    └── 公众号抓取信息.md    ← 任务摘要
```

---

## 从源码构建

### 前置条件

```powershell
# 确认 Go 版本
go version   # 需要 1.24+
```

### 构建

```powershell
# 使用构建脚本（推荐）
.\build.ps1

# 或手动构建
go build -ldflags "-s -w -H windowsgui" -o wesaver.exe .
```

### 运行测试

```powershell
go test ./... -v
```

---

## 项目结构

```
WechatSaver/
├── main.go                          # 程序入口，WebView2 窗口
├── build.ps1                        # Windows 构建脚本
├── go.mod / go.sum                  # Go 依赖管理
│
├── internal/
│   ├── api/
│   │   └── client.go                # HTTP 客户端，文章下载
│   │
│   ├── export/
│   │   ├── service.go               # 文件保存（HTML/MD/Excel）
│   │   ├── htmlclean.go             # HTML 离线优化清理
│   │   ├── htmlclean_test.go        # 清理模块测试
│   │   ├── excel.go                 # Excel 导出
│   │   └── paths.go                 # 输出路径管理
│   │
│   ├── models/
│   │   ├── article.go               # 文章数据模型
│   │   ├── options.go               # 任务配置与速度档位
│   │   └── token.go                 # Token 解析
│   │
│   ├── pipeline/
│   │   └── pipeline.go              # 抓取流水线（列表→详情→导出）
│   │
│   ├── proxy/
│   │   ├── capture.go               # 本地代理与 Token 捕获
│   │   ├── cert.go                  # CA 证书生成与管理（Windows）
│   │   └── wininet.go               # 系统代理设置（Windows API）
│   │
│   ├── repo/
│   │   └── task_repo.go             # SQLite 任务持久化
│   │
│   └── ui/
│       ├── server.go                # HTTP API 服务器
│       ├── api_helper.go            # Token 校验辅助
│       └── static/
│           └── index.html           # 前端界面
│
└── .gitignore
```

---

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.24 |
| 桌面窗口 | [go-webview2](https://github.com/jchv/go-webview2) |
| HTML→Markdown | [html-to-markdown](https://github.com/JohannesKaufmann/html-to-markdown) |
| Excel 导出 | [excelize](https://github.com/xuri/excelize) |
| 数据存储 | [modernc.org/sqlite](https://modernc.org/sqlite) (pure Go) |
| 系统代理 | Windows WinInet API (syscall) |
| 证书管理 | Windows Crypt32 API (syscall) |

---

## 注意事项

> [!WARNING]
> - Token 有时效性，过期后需重新抓取
> - 高速模式 + 高并发可能触发微信风控，建议使用默认慢速
> - 首次使用会安装本地 CA 证书用于 Token 捕获，需确认系统弹窗
> - 本工具仅供个人学习与文章备份使用

> [!TIP]
> - 建议先用 1 页测试，确认正常后再批量抓取
> - 「断点续传」默认开启，中断后重跑不会重复下载
> - 导出的 HTML 已优化为离线可读，双击即可在浏览器中查看

---

## License

MIT License - see [LICENSE](LICENSE) for details.

---

<div align="center">

**Made with :heart: for archiving knowledge**

</div>
