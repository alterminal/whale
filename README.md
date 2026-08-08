# 🐋 Whale

Whale 是一個以 Go 語言實作的 [Matrix](https://matrix.org/) 聯邦式即時通訊伺服器（Homeserver），目標是打造高效、輕量且易於部署的 Matrix 服務端。

## ✨ 功能特性

- **Matrix 協定支援** — 實作 Matrix 核心 API（Client-Server、Server-Server、Application Service）
- **高效能** — 使用 Go 原生並發模型，低記憶體佔用，高吞吐量
- **聯邦支援** — 完整的 Federation API，可與其他 Matrix 伺服器組成去中心化網路
- **輕量部署** — 單一二進位檔案，無需外部依賴即可執行
- **多種儲存後端** — 支援 SQLite / PostgreSQL 作為持久化儲存

## 🚀 快速開始

### 前置需求

- Go 1.21+

### 編譯

```bash
git clone https://github.com/your-org/whale.git
cd whale
go build -o whale .
```

### 執行

```bash
./whale
```

### 設定

Whale 預設會在 `~/.whale/config.yaml` 讀取設定檔。首次執行時若無設定檔將自動產生預設設定。

```yaml
server:
  name: "localhost"
  port: 8008
  domain: "localhost"

database:
  driver: "postgres"
  dsn: "host=localhost user=whale_dev password=your_password dbname=whale_dev port=5432 sslmode=disable"

federation:
  enabled: true
  port: 8448
```

## 📁 專案結構

```
whale/
├── main.go              # 進入點
├── go.mod               # Go 模組定義
├── client/              # Client-Server API
├── federation/          # Server-Server API
├── room/                # 房間與事件邏輯
├── user/                # 使用者管理
├── storage/             # 資料持久層
└── config/              # 設定管理
```

## 🤝 貢獻

歡迎提交 Issue 與 Pull Request。請先閱讀貢獻指南。

## 📄 授權

本專案採用 [MIT License](LICENSE)。
