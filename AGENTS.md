# asoul 工程约束

这些规则适用于仓库内的所有实现、修复和文档更新。

## 文件与状态安全

- 入口收到的 Skill ID 必须经过 `skill.ValidateID`。任何目标路径都要校验仍位于目标根目录内，并考虑符号链接。
- 删除、导入或覆盖前必须识别未纳管碰撞、已登记内容变化和哈希失败。默认保护用户文件，只有显式 `force` 才能覆盖；`force` 不能绕过路径边界。
- 部署使用暂存目录和可恢复备份，先写状态，提交成功后才清理备份。状态写入失败必须回滚目录并返回错误。
- 全局状态、项目状态和共享目标目录使用 `github.com/gofrs/flock`。不要在锁外执行 `Load → 修改 → Save`。
- 保留已有文件权限；配置和备份不允许从私有权限变成公开可读。

## 交互与接口

- TUI 的 Git、网络、全量扫描和写操作必须通过 `tea.Cmd`；可取消操作使用独立 context，取消后忽略迟到结果。
- 渲染、光标和快捷键必须从同一个可见列表读取选中对象，尤其是 Skills 与 Updates 页面。
- TUI 按键必须遵守 `doc/TUI交互约定.md` 的焦点层级模型：横向键只在当前聚焦层内循环，绝不跨级；`Tab`/`Shift+Tab` 负责进入/退出下级；改变一级 Tab 必须经过统一的 `setActiveTab()`。修改按键时同步更新页脚、帮助与 README 文案。
- CLI 批量命令必须输出合法 JSON（包括空结果），失败项要包含在结构化结果中并返回非零错误。
- 同一 Git URL 的批量检查只刷新一次缓存；相对本地路径按进程当前目录解析。

## 国际化

- 所有用户可见文案必须经 `internal/i18n` 的 `i18n.T` 渲染，详细规范见 `doc/i18n规范.md`（单一事实来源）。
- 新增或修改文案必须在同一提交内同时更新 `internal/i18n/en.go` 与 `internal/i18n/zh_cn.go`，键集合保持一致。
- `internal/app` 等业务层禁止 import `internal/i18n`；本地化只在 TUI/CLI 展示层进行，`--json` 输出保持英文规范值。
- 状态/来源等枚举值不得直接渲染，必须走 `status.managed.*`、`upstream.status.*`、`source.type.*` 键。
- cobra 的命令帮助与 flag 描述保持英文；`internal/config/wizard.go` 的刻意双语输出豁免。
- 禁止在 `internal/i18n/` 与 `internal/config/wizard.go` 之外出现中日韩字符串字面量（由守卫测试强制）。

## 验证

提交前运行：

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build -o /tmp/asoul-build ./cmd/asoul
```

测试不得依赖真实外网；文件安全、权限、并发和 TUI 交互修复应有针对性的回归用例。
