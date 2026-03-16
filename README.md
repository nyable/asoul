# asoul - AI Agent SKILLS 管理工具

`asoul` 是一个用于管理 AI Agent SKILLS 的命令行工具，它能够帮助你方便地安装、卸载、更新和分组管理你的各类 AI Agent SKILLS。它同时提供了纯命令行操作风格和基于终端的友好交互式界面（TUI）。

## ✨ 特性

- **支持交互模式**：通过 `-i` 参数进入基于 Bubbletea 构建的强大、美观的交互式终端 UI。
- **SKILLS 安装与管理**：支持将配置好的 SKILL 模块快速复制或**软链接**到目标项目中。
- **分组化管理**：您可以自由地给已有的 SKILLS 划分分组（如：“文档处理”、“代码编写”），支持以分组为单位批量安装和卸载。
- **多输出格式**：支持普通文本（Text）或 JSON 格式输出，方便集成到自动化脚本和 CI/CD 流程中。

---

## 编译

```bash
go build -o output/asoul.exe ./cmd/asoul
```

## 🚀 安装与初始化


首先，确保你已经安装了 Go (>= 1.24.2)，然后通过以下命令获取本工具：

```bash
go install github.com/nyable/asoul/cmd/asoul@latest
```

安装完成后，首先需要进行初始化：

```bash
asoul init
```

执行该命令后，asoul 会在你的用户主目录生成必要的配置：
- 配置文件路径：`~/.asoul/config.yaml`
- 默认 SKILLS 路径：`~/.asoul/skills/`

---

## 📖 核心命令

你可以使用 `asoul -h` 或 `asoul help` 查看完整命令帮助。

### 1. 交互模式 (Interactive Mode)

你可以随时附加 `--interactive` 或 `-i` 进入交互模式。在交互模式下可以通过方向键、Enter 进行选择和管理：

```bash
asoul -i
```

### 2. 浏览 SKILLS (`list` / `ls`)

列出全局可用的所有 SKILLS：
```bash
asoul list
```

列出指定目标项目中已安装的 SKILLS：
```bash
asoul ls -t ./my-project
```

### 3. 安装 SKILLS (`install` / `i`)

将 SKILLS 安装到指定项目：
```bash
# 按名称安装单个或多个 SKILL
asoul install docx pdf -t ./my-project

# 按分组安装
asoul install -g 文档处理 -t ./my-project

# 使用软链接模式（而非文件复制）安装
asoul install docx -l -t ./my-project
```

### 4. 卸载 SKILLS (`uninstall` / `rm`)

从指定项目中移除 SKILLS：
```bash
# 按名称移除
asoul uninstall docx -t ./my-project

# 按分组移除
asoul rm -g 文档处理 -t ./my-project
```

---

## 🗂️ 分组管理 (`group` / `g`)

`asoul group` 命令允许您管理 SKILL 集合，从而实现一键操作多个强关联的 SKILL。

- **列出所有分组**
  ```bash
  asoul group list
  ```
- **查看分组详情**
  ```bash
  asoul group show <name>
  ```
- **创建分组**
  ```bash
  asoul group create "文档处理" -d "包含各类文档操作的 SKILL" -s "docx,pdf,xlsx"
  ```
- **修改分组**
  ```bash
  asoul group update "文档处理" -a "pptx,txt" -r "xlsx" -d "修改后的描述"
  ```
- **删除分组**
  ```bash
  asoul group delete "文档处理"
  ```

---

## ⚙️ 配置管理 (`config` / `cfg`)

查看和修改 `asoul` 的全局配置：

- **查看当前配置**
  ```bash
  asoul config
  ```
- **修改配置项**（例如修改默认的 SKILLS 源目录）
  ```bash
  asoul config set skills_path /path/to/my/custom/skills
  ```

---

## 🔧 全局参数

- `-i, --interactive`: 开启交互模式
- `-o, --output`: 设置输出格式，支持 `text` 和 `json` (默认: `text`)

示例：将列表以 JSON 格式输出
```bash
asoul list -o json
```

## 🛠 开发依赖

- **Cobra**: 命令行解析构建框架
- **Bubbletea / Bubbles / Lipgloss**: TUI (终端交互界面) 技术栈
- **Pterm**: 命令行漂亮输出工具
- **yaml.v3**: YAML 配置读写
