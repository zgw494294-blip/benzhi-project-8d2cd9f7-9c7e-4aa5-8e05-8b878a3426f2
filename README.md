# 生物样本冷链接收放行台

这是一个面向样本接收专员和质量审核员的冷链证据治理服务。系统把案卷、样本容器、温度探头、运输分段、风险裁决和最终接收凭据串成可追溯流程，支持退回补证、复审和凭据完整性核验。

## 构建

```text
go build ./cmd/coldchain-server
```

## 运行

```text
go run ./cmd/coldchain-server -addr=127.0.0.1:19081
```

默认监听 `127.0.0.1:19081`。也可以使用 `-addr=127.0.0.1:<port>`，或设置 `PORT` 端口号（服务会绑定到 `127.0.0.1:<PORT>`）。服务提供浏览器工作台 `GET /` 和同源 JSON API，数据默认保存在 `./data`，可用 `-data` 指定目录。

## 自检与测试

完整 HTTP 冒烟流程会实际启动服务、创建案卷、登记基础资料、提交证据、完成审核并签发凭据：

```text
go run ./cmd/coldchain-server -selfcheck -addr=127.0.0.1:19081
```

运行单元测试：

```text
go test ./...
```

## 工作流能力

工作台和 JSON API 支持案卷状态、参与方、交接窗口、窗口阶段（`windowPhase`）和临期分钟（`dueWithinMinutes`）组合筛选及稳定分页（`page`、`pageSize`），返回过滤集合的总数、状态计数、最新更新时间、窗口剩余分钟、同发送方/接收方的重叠冲突摘要和下一页信息。容器、探头和运输分段可批量原子登记，失败会返回条目位置且不改变版本。

创建案卷会校验不区分大小写的编号唯一性，并支持 `Idempotency-Key` 绑定成功或冲突结果；重叠交接窗口保留建案并在临期队列中预警。基础资料批量登记会校验样本类别阈值、探头证书、序列号、精度和校准覆盖，详情与风险响应提供按证书排序的覆盖台账；公开证据接口要求起止及中间采样点，并跨修订拒绝重复读数。

风险接口返回合并后的运输覆盖缺口、下一段建议边界、原始读数上下文、持续时长、最大偏离、稳定指纹、整改闭环和复审基线差异，并可按类型、严重度、修订号和时间交集过滤。`decisions` 支持整批原子裁决及稳定未决清单，退回后的证据使用 `remediationNote` 记录整改说明，并可用 `remediatesFindingIDs` 明确关联所覆盖的基线发现。

案卷详情和列表返回 `windowConflicts` 与 `windowConflictDigest`；接收专员可调用 `/conflict-review` 提交带 `expectedVersion` 的接受或解除决定。`/probe-replace` 保留旧探头和历史修订，新增探头的校准台账会参与风险复算。`/evidence-precheck` 在不写入案卷的前提下返回逐段采样统计、覆盖缺口、下一段建议和指纹，正式 `/evidence` 可携带 `precheckFingerprint` 或 `precheckToken`。

审核 `/return` 可同时提交 `tasks`（或 `remediationTasks`）整改责任清单，复审门禁会检查责任人、截止时间和必需证据类型；风险响应包含任务状态。凭据批量核验支持 `Idempotency-Key`，失败项可通过 `POST /api/credentials` 记录操作人、结论和说明，`GET /api/credential-verifications` 或 `/api/credentials/reviews` 查询，记录和审计链会在重启后恢复。

放行凭据可通过案卷 ID、凭据 ID 或 `credentialNumber` 核验。`GET /api/credentials` 支持重复查询参数或逗号分隔值，单批最多 50 项，保持输入顺序返回每项 `checks` 和有效、无效、未放行、不存在汇总。核验过程会重新计算清单摘要并检查案卷审计哈希链，所有列表、风险、核验、清单和审计查询均为只读操作。
