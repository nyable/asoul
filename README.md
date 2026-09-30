# asoul

`asoul` 是一个使用 Go 开发的 Agent Skills 本地管理与分发工具。

## 核心特性

- **三层模型设计**：`Source` (Git / Local / Managed) ➔ `Managed Store` (`<workspace>/skills/`) ➔ `Target` (例如 `~/.codex/skills`, `~/.config/opencode/skills`)。
- **确定性内容 Hash**：基于 `golang.org/x/mod/sumdb/dirhash` 生成 `h1:...` 校验和，精准保护用户修改，杜绝静默覆盖。
- **Git 缓存与零冗余抓取**：Git 仓库自动建立 mirror/bare 本地缓存，避免重复克隆；同仓库多个 Skill 批量检查时自动聚合为单次 `fetch`。
- **安全替换机制 (Staged Replace)**：临时兄弟目录暂存 ➔ 旧目录备份 ➔ 原子重命名 ➔ 原子更新 `catalog.json` ➔ 清除备份，失败自动回滚。
- **双模式与多语言 (i18n) 支持**：功能完备的 CLI（全面支持 `--json` 输出）与基于 Bubble Tea 的交互式终端界面 (TUI)，支持中英双语与跟随系统环境自动探测；TUI 与 CLI 运行时输出均经 `internal/i18n` 本地化，命令帮助与 `--json` 保持英文规范值。

## 安装与编译

需要 Go 1.27+ 环境（`go.mod` 当前声明 Go 1.27.1）：

```bash
go build -o asoul ./cmd/asoul
```

## 首次运行配置向导

当您首次在终端运行 `asoul` 时，会自动检测用户配置目录（如 `~/.config/asoul/`），并交互式引导生成配置文件 `config.jsonc`：
1. **界面语言选择**：支持 English、简体中文及 Auto-detect 自动跟随系统环境变量；亦可随时通过 `asoul config set language zh-CN` 切换。
2. **工作区目录设置**：设定统一管理 Skills 的根目录（默认 `~/.config/asoul`），并提示一键初始化。
3. **标准分发目标配置**：一键启用常见 Agent 目标目录（Codex、OpenCode、Claude 等），也可自定义添加。
4. 您还可以随时通过命令重新启动向导：
   ```bash
   asoul config init
   ```

## 快速上手

### 1. 初始化工作区

```bash
# 在当前目录或指定目录初始化 (默认使用 ~/.config/asoul)
asoul init ~/.config/asoul
cd ~/.config/asoul
```

### 2. 创建受管 Skill

```bash
asoul new oracle-helper
```

### 3. 从外部目录或 Git 仓库添加 Skill

```bash
# 从外部本地目录导入 (深度拷贝，追踪上游变更)
asoul add ../company-skills/code-review

# 从 Git 仓库添加单 Skill
asoul add https://github.com/example/skills.git --path skills/frontend-design

# 自动发现 Git 仓库中所有 Skill
asoul add https://github.com/example/all-skills.git --all
```

### 4. 查看状态与检查上游更新

```bash
# 查看本地管理状态 (clean / modified / unmanaged / invalid / missing)
asoul status

# 检查上游更新
asoul check

# 查看与上游最新版本的 Diff 差异
asoul diff frontend-design

# 更新至最新版本 (本地存在修改时会自动阻断保护，需追加 --force)
asoul update frontend-design
```

### 5. 分发与部署到 Agent Target

`asoul` 支持两种级别的分发：**程序全局级**（按不同 Agent 分发至其用户全局目录）与**项目级**（指定项目路径，支持格式多选与 `auto` 自动探测）。

#### 5.1 项目级分发（Project Level）
```bash
# 自动探测项目中已存在的 Agent 目录并分发 (若均未检测到则回退至 standard)
asoul deploy oracle-helper --project . --format auto

# 分发到项目标准目录 (.agents/skills/，兼容大多数 Agent)
asoul deploy oracle-helper --project /path/to/my-repo --format standard

# 多选格式：同时分发到 standard 和 claude 格式 (.claude/skills/)
asoul deploy oracle-helper --project /path/to/my-repo --format standard,claude

# 从项目中卸载指定格式（省略 --format 则一次性彻底卸载该 Skill 在该项目中的全部格式）
asoul undeploy oracle-helper --project /path/to/my-repo --format claude
asoul undeploy oracle-helper --project /path/to/my-repo

# 查看和管理已缓存的项目路径
asoul project list
asoul project remove /path/to/my-repo
```
*项目级部署状态完整记录在 `<project>/.agents/.asoul-state.json` 中，asoul 自身仅轻量缓存项目路径。*

#### 5.2 程序全局级分发（Program Global Level）
```bash
# 部署到指定 Agent 程序的全局目录
asoul deploy oracle-helper --global claude            # ~/.claude/skills
asoul deploy oracle-helper --global antigravity-cli   # ~/.gemini/antigravity-cli/skills
asoul deploy oracle-helper --global standard          # ~/.agents/skills

# 从全局程序中卸载 (目标被修改时会自动拦截保护，需加 --force 强制执行)
asoul undeploy oracle-helper --global claude
```

#### 5.3 技能分组管理（Skill Group Management）
人为将指定的多个 Skills 归类为一个具名分组（例如 "常用"、"JAVA开发"、"frontend"），便于批量部署：
```bash
# 查看所有技能分组
asoul group list

# 创建新分组
asoul group create 常用
asoul group create JAVA开发

# 为分组添加技能（支持同时添加多个）
asoul group add 常用 git-helper docker-helper
asoul group add JAVA开发 spring-boot-tools maven-helper

# 从分组中移除技能
asoul group remove 常用 docker-helper

# 删除分组（不会删除工作区技能源文件）
asoul group delete 常用
```

### 6. 健康体检与诊断

```bash
asoul doctor
asoul doctor --clean
```

### 7. Agent 供应商与模型配置补全 (Model & Provider Enrichment)

支持自动读取 OpenCode 配置文件（`opencode.json` 或 `opencode.jsonc`），提取自定义 Provider 中的模型定义，从权威数据库 `models.dev` 获取上下文窗口大小、最大输出、定价、推理能力、工具调用等元数据，并根据自定义 JSON 规则自动注入 `options`、`variants` 思考变体等多层级配置：

```bash
# 自动检测 OpenCode 配置文件并丰富模型元数据（默认创建 .bak 备份）
asoul model enrich --agent opencode

# 仅预览变更差异（Diff Preview），不写入磁盘
asoul model enrich --agent opencode --dry-run

# 指定目标配置文件并覆盖已有元数据
asoul model enrich --file ./opencode.jsonc --override

# 使用自定义规则文件注入特定 variants 和参数
asoul model enrich --rules ~/.config/asoul/rules/opencode.json

# 仅按自定义规则重新生成配置，跳过 models.dev 元数据（离线、快速）
asoul model enrich --agent opencode --rules-only --dry-run

# 查询 models.dev 中的单个模型详细规格与定价
asoul model query claude-3-7-sonnet
asoul model query gpt-4o

# 查看当前已激活的模型规则列表
asoul model rules --agent opencode
```

#### 7.1 自定义规则（JSON/JSONC）配置样例
每个渠道单独配置一份规则文件，渠道按**文件名**匹配：全局 `~/.config/asoul/rules/<channel>.json`，项目级 `<workspace>/rules/<channel>.json`（也支持 `.jsonc`）。
文件内**不再声明** `agent` 字段；未知字段会被直接拒绝，避免拼写错误或过期配置静默失效。
每条规则包含四种相互独立的操作，可同时配置、按顺序执行；未指定 `order` 时按默认顺序
`fill → merge → override → remove` 执行：

- `fill`：仅当目标键不存在时写入（补空缺）
- `merge`：递归合并 map，数组做**并集去重**（不覆盖已有元素）
- `override`：递归合并，**强制覆盖**已有值（数组整体替换）
- `remove`：按点分路径删除字段
- `order`（可选）：自定义操作顺序。必须是上述四种的子集、不重复，且覆盖所有实际配置的操作，否则报错

字符串值支持变量注入：`${model_id}`、`${provider_id}`、正则捕获组 `${1}`/`${name}`、
`${config_dir}`、`${workspace}`，以及 `${date}`、`${time}`、`${datetime}`、`${timestamp}`、
`${year}`/`${month}`/`${day}`（时间类默认 UTC）。未知变量原样保留；出于安全考虑不解析环境变量或执行 shell。

```jsonc
{
  "$schema": "https://asoul.dev/schemas/model-rules.json",
  "rules": [
    {
      "name": "custom-gpt-variants",
      "pattern": "^gpt-(.+)$",
      "description": "为所有 GPT 系列模型注入思考变体并补齐默认值",
      "order": ["fill", "merge", "override", "remove"],
      "fill": {
        "family": "gpt-${1}"
      },
      "merge": {
        "variants": {
          "high": { "reasoningEffort": "high" }
        }
      },
      "override": {
        "options": {
          "reasoningEffort": "high"
        },
        "headers": {
          "X-Proxy-Routing": "priority"
        }
      },
      "remove": ["deprecated"]
    }
  ]
}
```

> 配置写回采用注释保留（comment-preserving）方式：未改动的字段会保留原有注释、空行与键顺序，
> 新增字段追加在该对象末尾，不再整体重排或丢失注释。

### 8. 交互式 TUI

在终端中直接运行：

```bash
asoul tui
```
或直接运行 `asoul`（在支持 TTY 的终端环境下默认启动 TUI）。

TUI 具备 CLI 的全部核心能力与交互界面。导航采用**焦点层级**模型：`←`/`→` 只在当前聚焦层级内循环，`Tab` 进入下级，`Shift+Tab`/`Esc` 返回上级（详见 `doc/TUI交互约定.md`）。一级 Tab 栏固定显示在首行，页脚固定在底部，内容区在剩余高度内滚动，不会把 Tab 栏顶出屏幕。

所有耗时操作（部署、更新、检查、克隆/拉取、删除、诊断等）统一显示进度反馈：有可靠计数的显示进度条与 `50% (100/200)` 百分比，无计数的显示旋转动画与流动条；git 自带的 `% (n/m)` 进度会被解析为真实百分比。CLI 在 TTY 下同样原地刷新进度条，`--json`/`quiet`/非 TTY 自动静默。

- **Tab 1: Overview 概览**
  - 工作区技能、上游、部署与健康状态速览；`u` 更新全部待更新，`r` 全量检查上游更新
- **Tab 2: Sources 上游来源**
  - `i`：查看上游源详情；`s`：浏览并添加该来源中的技能
  - `a`：登记上游后自动扫描并打开技能选择窗口（不自动导入）；`e`：编辑；`r`：重读本地缓存（不联网）；`c`：拉取并检查上游、刷新可用技能（包括尚未导入技能的来源）；`u`：更新该来源全部技能；`x` / `del`：移除
  - 未扫描、扫描失败、扫描成功但无技能分别展示。扫描失败/取消或关闭技能选择窗口不会删除已登记来源，`s` 可重试；本页 `d` / `D` 不触发部署/卸载。
- **Tab 3: Skills 技能**（`Tab` 进入子标签：技能 / 分组）
  - `a`：添加 Skill（Git 仓库 URL 或本地目录，支持多 Skill 探测与批量导入）
  - `n`：新建本地受管 Skill；`A`：纳管（Adopt）未受管目录
  - `x` / `del`：删除 Skill（弹窗确认，修改保护）；`X`：按正则/来源/渠道批量筛选删除
  - `i`：查看详情并使用 Glamour 预览 `SKILL.md`；`v`：Diff 差异查看
  - `u`：更新单项（本地修改时提示 `[f]` 强制覆盖）；`U`：批量更新所有可用 Skill
  - `d`：部署技能（`←`/`→` 在**技能列表手动多选**与**按分组批量**之间切换）；`D`：从目标卸载
  - `r`：异步拉取 Git 检查上游更新；`/`：实时搜索过滤
  - **分组子标签**：`a` 新建分组；`e` 打开复选框分配技能；`d` 部署分组；`x` / `del` 删除分组（不影响技能文件）
- **Tab 4: Channels 渠道配置**（`Tab` 进入渠道层，`←` / `→` 切换已配置的 Agent 渠道）
  - 顶部为渠道选择器，标题行内嵌 `[f]` 状态筛选（全部 / 仅启用 / 仅禁用）
  - 每个渠道一行摘要：启用状态圆点（绿=启用 / 红=禁用）+ 配置目录 + `SKILLS:N`；支持数据同步的渠道再显示 `数据同步（同步上游 / 补齐缺失）`
  - 模型配置表仅对支持数据同步的渠道（如 OpenCode）显示：`i` 查看当前高亮行详情（供应商头→供应商详情，模型行→模型详情），`Space` 折叠/展开供应商头
  - `t` 切换启用/禁用，`d` / `D` 部署/卸载技能，`a` / `+` 新增渠道，`x` / `del` 移除，`I` 查看当前渠道完整详情（路径、特性、已部署技能等），`r` 刷新渠道与模型
  - 模型配置：`e` 从 `models.dev` 补全并预览差异，确认后写入（自动备份为 `<原文件名>.yyyyMMddHHmmss`）；`R` 仅按自定义规则重新生成；`o` 切换同步模式；`c` 查看模型配置文件
  - 差异浏览：`/` 搜索、`n` / `N` 下一/上一匹配、`]c` / `[c` 跳转差异块、`gg` / `G` 首/末、`h` / `l` 横向滚动；未搜索时确认页的 `n` 仍表示取消，`Esc` / `q` 始终可取消。
  - 配置差异确认页的 `e` 返回候选草稿编辑；JSON/JSONC 编辑窗口的 `Ctrl+E` 临时打开外部编辑器，保存并关闭后恢复 TUI、校验并重新生成差异，确认后才写入真实文件。`Ctrl+S` 在内置编辑框中校验并预览。无编辑器、启动失败或内容无效时仍可在内置编辑框修复草稿。
- **Tab 5: Projects 项目工程**
  - 登记本地代码工程项目并查看已部署技能；`a` / `+` 添加路径（显式管理，不自动注入当前目录）
  - `Space` / `t`：切换启用/禁用（未启用项目在批量分发时安全跳过）；`f`：过滤（全部 / 启用 / 禁用）
  - `i`：查看项目详情（路径、产物格式、已部署技能）；`d` / `D`：部署/卸载；`x` / `del`：移除
- **Tab 6: System 系统维护**（`Tab` 进入子标签：缓存 / 诊断）
  - 缓存：浏览 Git 本地裸库缓存及占用大小；`p` 清理孤立无引用缓存；`c` 一键清空
  - 诊断：全流程健康检查（Git、工作区、Catalog、文件一致性、残留文件）；`↑`/`↓`、`PgUp`/`PgDn`、`g`/`G` 滚动诊断项；`r` 重新诊断；`c` 清理 staging 残留
- **Tab 7: Settings 设置**
  - `7` 或 `s`：快速直达设置面板
  - `Space`：快速切换显示语言（简体中文、English、自动跟随系统），即时生效并持久化到 `config.jsonc`；按 `e` 可打开选择弹窗
  - `w`：打开**多工作区管理面板**，查看全部已登记的工作区列表、当前激活指示（`●`）与初始化状态（`[已初始化]` / `[未初始化]`），支持：
    - `Enter`：一键切换当前激活的工作区（自动持久化并热重载全界面）
    - `a` / `+`：登记添加新的技能工作区目录；`x` / `d`：注销/移除选中的工作区路径
    - `i`：为当前选中的工作区快速初始化创建 `catalog.json`
  - `I`：为当前工作区根目录初始化 `catalog.json`
  - `r`：重新从磁盘加载配置；`R`：一键恢复推荐默认配置（标准 Agent 渠道与默认工作区）
- **通用快捷键**
  - `1 - 7`：直达对应顶级 Tab；`4` / `m`：渠道；`s`：设置；`[` / `]`：仅在顶级聚焦时前后循环
  - `←` / `→`（`h` / `l`）：在当前聚焦层级内循环，**绝不跨级**；`Tab` / `Shift+Tab`：在当前页层级间循环（一级 ↔ 二级，无二级页无变化）；`Esc`：返回上级
  - `i` / `I`：查看详情统一键——`i` 打开当前选中对象详情（上游 / 技能 / 渠道行 / 项目），`I` 打开当前页面上下文对象（Channels 当前渠道、Settings 初始化工作区）；主列表 `Enter` 不再绑定默认动作
  - `?`：打开**当前页面**的快捷键帮助（内容可滚动，`↑`/`↓`、`PgUp`/`PgDn`、`g`/`G` 滚动，`Esc` 关闭）；主列表 `Esc`：返回/清搜索；`q`：仅主列表退出，弹窗内 `q` 等同 `Esc`
  - 页脚仅列出当前页高频快捷键，不再重复 Tab / 方向键等层级切换说明

## 常用全局选项

- `--root <path>`：显式指定工作区根目录。
- `--config <path>`：指定自定义配置文件路径。
- `--json`：输出标准 JSON 格式数据至 stdout。

### 外部编辑器（Linux / macOS / Windows）

在 asoul 的 `config.jsonc` 中配置可执行文件和参数数组，优先级为 `editor` → `VISUAL` → `EDITOR` → PATH 中的 `nvim` / `vim` / `vi` / `nano`。CLI 的 `upstream add` 仍只登记来源；新增后自动扫描仅适用于 TUI。

```jsonc
"editor": {
  "command": "C:\\Program Files\\Neovim\\bin\\nvim.exe",
  "args": []
}
```

Linux/macOS 可使用 `"command": "nvim"`。VS Code 使用 `"command": "code"`（Windows 也可明确指定 `code.cmd`）和 `"args": ["--wait"]`；GUI 编辑器必须提供等待关闭文件的参数，否则 asoul 会过早返回。不会自动调用 Windows 文件关联或 Notepad。

环境变量命令支持单/双引号分组，反斜杠保持字面值，不支持 shell 展开、管道或重定向。带空格的 Windows 路径推荐使用上述结构化配置。`.cmd` / `.bat` 使用受限 `cmd.exe` 启动，拒绝引号、换行及 `&|<>^%!` 等特殊字符；这些路径/参数请改用 `.exe` 配置。

外部编辑只访问私有临时草稿，不直接修改原文件；Unix 使用私有权限，Windows 使用当前用户及 SYSTEM 的受保护 ACL。配置写入前检查原文件基线，其他进程修改后会拒绝覆盖；保留原配置权限与备份。全文件 JSONC 草稿保留注释及用户选择的换行符，字段修改保留原文件换行风格。
- `--quiet, -q`：静音模式，仅输出核心数据或错误。
- `--non-interactive`：非交互模式，遇到歧义或冲突直接返回明确错误。
- `--force, -f`：强制覆盖本地或目标修改。

涉及删除、覆盖和部署时，asoul 默认保护未纳管目录及内容已变化的目标；只有明确使用 `--force` 才会继续。部署先暂存并更新状态，状态写入失败会回滚目标目录。

## 测试

运行离线端到端测试与全部单元测试：

```bash
go test -v ./...
```

## 设计规范与开发指南

深入了解项目的设计原理、TUI 焦点层级与排版对齐、存储安全机制等，请参阅：
- 📘 [开发规范与踩坑指南](doc/开发规范与踩坑指南.md)
- 🖥️ [TUI 交互约定](doc/TUI交互约定.md)
- 🌐 [国际化（i18n）规范](doc/i18n规范.md)
