# asoul

`asoul` 是一个管理和分发 AI Agent SKILLS 的命令行工具。

## 编译

需要 Go 1.24.2 或更高版本。

```bash
go build -o output/asoul.exe ./cmd/asoul
```

## 安装与初始化

也可以直接通过 go install 安装到你的系统环境中：

```bash
go install github.com/nyable/asoul/cmd/asoul@latest
```

首次使用时，如果配置文件不存在，`asoul` 会自动在 `~/.asoul/` 下生成默认的 `config.yaml` 配置文件和 `skills` 存储目录。

## 使用

主要命令分为 `list`（浏览）、`install`（安装）、`uninstall`（卸载）、`group`（分组管理）和 `config`（配置管理）。支持追加 `-i` 进入交互选单模式。

### 交互模式

附加 `-i` 参数即可进入 TUI 交互界面：

```bash
asoul -i
```

### 浏览 SKILLS

列出源目录中可用的所有 SKILLS：
```bash
asoul list
```

列出指定目标项目中已安装的 SKILLS：
```bash
asoul ls -t ./my-project
```

### 安装 SKILLS

按名称将 SKILLS 复制到目标项目：
```bash
asoul install docx pdf -t ./my-project
```

按预设分组安装：
```bash
asoul install -g 文档处理 -t ./my-project
```

以软链接模式安装（不复制文件）：
```bash
asoul install docx -l -t ./my-project
```

### 卸载 SKILLS

从目标项目中移除指定的 SKILLS 或分组：
```bash
asoul uninstall docx -t ./my-project
asoul rm -g 文档处理 -t ./my-project
```

### 分组管理

```bash
# 列出内部分组
asoul group list

# 创建分组关联多个 SKILL
asoul group create "常用技能" -d "日常所需" -s "docx,pdf,xlsx"

# 更新和删除分组
asoul group update "常用技能" -a "txt" -r "xlsx"
asoul group delete "常用技能"
```

### 配置管理

查看和配置 `asoul` 的运行参数：

```bash
asoul config

# 更改源 SKILLS 的寻找路径
asoul config set skills_path /path/to/my/skills
```

### 全局参数

- `-i, --interactive`: 开启交互模式
- `-o, --output`: 切换输出格式，支持 `text`（默认） 和 `json`
