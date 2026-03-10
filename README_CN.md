# Kratos Skill

面向 Go-Kratos 服务设计、实现与排障的实用技能包。

English version: [README.md](./README.md).

这个仓库是一个可通过 `npx skills` 安装的单技能仓库。真正的技能入口是根目录下的 [`SKILL.md`](./SKILL.md)。本文件仅面向 Git 仓库读者，不参与技能发现。

## 安装

从 GitHub 直接安装：

```bash
npx skills add viking602/kratos-skill
```

这条命令会进入交互式安装流程。

从 GitHub 全局安装：

```bash
npx skills add viking602/kratos-skill -g
```

从这个仓库的本地克隆安装：

```bash
git clone git@github.com:Viking602/kratos-skill.git
cd kratos-skill
npx skills add .
```

仅查看仓库中可安装的技能，不执行安装：

```bash
npx skills add viking602/kratos-skill --list
```

仅为 Codex 无提示安装这个技能：

```bash
npx skills add viking602/kratos-skill -a codex -s kratos-skill -y
```

## 管理

```bash
npx skills list
npx skills check
npx skills update
npx skills remove kratos-skill -y
```

项目级安装通常会写入 `./.agents/skills/kratos-skill`。如果需要全局安装，使用 `-g`。

## 技能覆盖范围

- `api/**/*.proto`、`errors.proto` 与校验规则
- `make api`、`make errors`、`make validate`
- `internal/{biz,data,service,server}` 的 Kratos 分层边界
- Wire 配置、中间件顺序、认证 selector 与服务发现
- 跨服务 gRPC 调用与 Kratos 风格错误处理

## 仓库结构

```text
.
├── SKILL.md
├── agents/openai.yaml
├── references/
├── examples/
├── evals/
├── best-practices.md
├── troubleshooting.md
└── README_CN.md
```

- `SKILL.md`: 技能元数据与执行指引
- `agents/openai.yaml`: 面向支持 skill 的客户端的 UI 元数据
- `references/`: 按需加载的任务参考材料
- `examples/`: proto 与 Go 示例片段
- `evals/`: 评估用例

## 维护说明

- 技能行为放在 `SKILL.md`；仓库说明放在 `README.md` 和 `README_CN.md`。
- 保持根目录的 `SKILL.md` 与 `agents/openai.yaml` 不变，这样 `npx skills` 才能稳定发现这个仓库。
- 需要补充示例或参考资料时，优先更新关联文件，而不是不断膨胀 `SKILL.md`。
