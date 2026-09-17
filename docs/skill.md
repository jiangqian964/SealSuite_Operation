# 智能分析 Skill 框架（Intelligent Analysis Skill）

## 目的

本 Skill 框架提供一套**通用的智能分析能力**，基于 LLM 推理（优先）+ 规则引擎（兜底），对各类输入数据进行分类、决策和原因推断。

核心分析范式：

```
输入元数据 → [LLM推理 → 规则兜底] → 分类结果 + 决策 + 置信度 + 可解释原因
```

目前已覆盖 / 规划覆盖以下场景：

| 场景 | 名称 | 状态 | 核心目标 |
|------|------|------|---------|
| 场景一 | 文件归类分析 | ✅ 已实现 | 识别系统/软件文件，判断是否应从用户个人文件中排除 |
| 场景二 | DLP 误报分析加白 | ✅ 已实现 | 识别 DLP 泄露事件中的误报，自动加白减少人工审核量 |
| 场景三 | VPN 访问策略分析 | 📋 设计中 | 基于 VPN 访问日志，分析员工/角色/部门的内网访问权限合理性，给出权限优化建议 |

**设计原则（强约束）**：**宁可不处理，也不要误判**。不确定时必须返回 `unknown` 且决策取保守值（不加白、不排除、不回收）。

---

## 通用框架

### 通用输入模型

每个待分析条目均包含元信息，具体字段由场景定义。通用字段包括：

```json
{
  "id": "条目标识",
  "metadata": { "场景特有字段": "..." }
}
```

### 通用输出模型

每个条目返回一条分析结果，通用结构如下：

```json
{
  "id": "条目标识，与输入对应",
  "category": "分类标签，场景自定义枚举",
  "should_action": false,
  "confidence": 0.85,
  "reasons": ["原因1", "原因2"],
  "method": "llm"
}
```

### 通用字段含义

| 字段 | 类型 | 说明 |
|------|------|------|
| `category` | string | 分类标签，各场景自定义枚举值 |
| `should_action` | bool | 决策布尔值（如 should_exclude / should_whitelist / should_revoke），各场景定义具体语义 |
| `confidence` | float | 0~1 的置信度，越不确定越低 |
| `reasons` | string[] | 短句列表，可解释原因（便于审计、可视化与纠错） |
| `method` | string | `llm` 或 `heuristic`（规则兜底） |

### 推理策略

#### 1) 规则兜底（Heuristic）
每个场景内置一套"可解释、保守"的规则引擎：
- 明确正向特征 → 命中分类，返回对应决策
- 明确负向特征 → 排除分类，返回反向决策
- 无法确定 → 返回 `unknown` 且 `should_action=false`

#### 2) LLM 推理（优先）
当 LLM 配置完整且 `prefer_llm=true` 时：
1. 将输入元信息批量组织为 JSON
2. 使用固定 `system_prompt` 约束输出为**纯 JSON**
3. 解析与校验失败时，自动回退规则结果

### 设计原则（强约束）

1. **保守优先**：宁可不处理，也不要误判
2. **不确定即 `unknown`**：分类返回 `unknown`，决策取保守值（`false`）
3. **可解释性**：必须返回 `reasons` 字段说明判断依据
4. **自动回退**：LLM 解析失败或超时时，静默回退规则结果

---

## 场景一：文件归类分析（File Classification）

### 场景说明

将**文件路径、文件名、文件格式（扩展名）、打开该文件的应用名**等元信息交给推理引擎，尽可能准确地识别哪些文件/路径更可能属于操作系统文件、软件配置文件、软件日志文件、缓存/临时文件、软件运行数据，并据此判断该文件**是否应从"用户个人文件"集合中排除**。

### 输入（File Meta）

每个文件条目的元信息结构如下（字段越全，越利于推理；但只要求 `file_path`）：

```json
{
  "file_path": "/Users/alice/Library/Logs/App/app.log",
  "file_name": "app.log",
  "file_ext": ".log",
  "opener_app": "Console",
  "size_bytes": 12345,
  "mtime": "2026-07-07T10:00:00+08:00"
}
```

#### 关键字段说明

- `file_path`：完整路径（含文件名）。这是最重要的判断依据。
- `file_name`：可选。若缺失可由 `file_path` 解析。
- `file_ext`：可选。`.log/.plist/.ini/.json` 等对分类很关键。
- `opener_app`：可选。辅助判断该文件是否属于某软件生态（例如 Console/VSCode/WeChat）。

### 输出（Classification）

每个文件返回一条分类结果：

```json
{
  "file_path": "/Users/alice/Library/Logs/App/app.log",
  "category": "app_log",
  "should_exclude_from_personal": true,
  "confidence": 0.9,
  "reasons": ["位于 macOS Logs 目录（Library/Logs）", "扩展名为 .log"],
  "method": "llm"
}
```

#### 字段含义

- `category`：分类标签（见下）
- `should_exclude_from_personal`：
  - `true`：更可能是系统/软件文件，不应视为用户个人文件
  - `false`：更可能是用户个人文件，或无法确定（保守策略）
- `confidence`：0~1 的确定度。越不确定越低。
- `reasons`：短句列表，可解释原因（便于审计、可视化与纠错）。
- `method`：`llm` 或 `heuristic`（规则兜底）。

### 分类标签（Category）

`category` 取值：

- `user_personal`：用户个人文件（文档、照片、项目资料等）
- `os_system`：操作系统文件（系统目录、系统组件等）
- `app_config`：软件配置文件（preferences、ini、plist、yaml 等）
- `app_log`：软件日志文件（Logs、Crash、*.log 等）
- `cache_temp`：缓存/临时文件（Caches、Temp、*.tmp 等）
- `app_data`：软件运行数据（Application Support、索引、数据库等非直接面向用户的数据）
- `unknown`：无法判断（必须保守：不排除）

### 推理策略

#### 1) 规则兜底（Heuristic）

本场景内置一套"可解释、保守"的规则引擎：

- 明确系统/软件路径特征：如 macOS `~/Library/Logs`、`~/Library/Caches`、`~/Library/Preferences`、`~/Library/Application Support` 等
- 明确日志/缓存特征：`.log/.tmp`、`Logs/Crash` 目录等
- 明确用户个人目录特征：`Desktop/Documents/Downloads/Pictures/...`

规则无法确定时：返回 `unknown` 且 `should_exclude_from_personal=false`。

#### 2) LLM 推理（优先）

当 LLM 配置完整且 `prefer_llm=true` 时：

1. 将文件元信息批量组织为 JSON
2. 使用固定 `system_prompt` 约束输出为**纯 JSON**
3. 解析与校验失败时，自动回退规则结果

---

## 场景二：DLP 误报分析加白（DLP False Positive Whitelisting）

### 场景说明

针对 DLP（数据防泄漏）系统产生的泄露告警事件，通过分析**文件信息、用户信息、泄露渠道、事件类型**等元数据，智能识别误报事件，并给出加白建议（按路径 / 文件名 / 扩展名），从而减少人工审核量。

### 输入（DLP Event Meta）

每个 DLP 事件的元信息结构如下：

```json
{
  "event_id": "evt_123456",
  "file_info_name": "report.docx",
  "file_info_path": "/Users/alice/Documents/project/report.docx",
  "file_info_type": "document",
  "leak_way_app_name": "WeChat",
  "event_type": "dlp_leak",
  "user_id": "u_1001",
  "user_name": "张三",
  "device_id": "dev_2002",
  "event_time": "2026-07-07T10:00:00+08:00",
  "evidence_url": "https://dlp.example.com/evidence/evt_123456"
}
```

#### 关键字段说明

- `file_info_path` / `file_info_name`：文件路径和名称。核心判断依据，可用于匹配路径/文件名/扩展名规则。
- `leak_way_app_name`：泄露渠道应用名。辅助判断是内部工具还是外部应用。
- `user_id` / `user_name`：用户信息。用于辅助判断（如该用户是否为安全团队成员）。
- `event_type`：事件类型。不同类型的误报率差异较大。

### 输出（DLP Analysis Result）

每个 DLP 事件返回一条分析结果：

```json
{
  "event_id": "evt_123456",
  "category": "false_positive_system",
  "should_whitelist": true,
  "confidence": 0.88,
  "reasons": ["文件位于系统缓存目录", "文件扩展名为系统日志格式 .log", "该文件非用户主动创建"],
  "match_type": "extension",
  "match_value": ".log",
  "method": "llm"
}
```

#### 字段含义

- `category`：分类标签（见下）
- `should_whitelist`：
  - `true`：判断为误报，建议加白
  - `false`：判断为真实违规或无法确定（保守策略）
- `confidence`：0~1 的置信度
- `reasons`：短句列表，可解释原因
- `match_type`：建议的白名单匹配类型（`path` / `name` / `extension`），仅当 `should_whitelist=true` 时有意义
- `match_value`：建议的匹配值（对应 `match_type`）
- `method`：`llm` 或 `heuristic`（规则兜底）

### 分类标签（Category）

`category` 取值：

- `true_positive`：真实违规，需人工处理
- `false_positive_system`：系统文件误报（缓存、日志、配置、临时文件等非用户主动创建的文件）
- `false_positive_work`：工作文件误报（正常业务文档、代码文件等，企业内部传输不构成泄露）
- `false_positive_personal`：个人文件误报（用户私人照片、个人文档等，不涉及企业敏感数据）
- `unknown`：无法判断（必须保守：不加白）

### 推理策略

#### 1) 规则兜底（Heuristic）

本场景内置一套"可解释、保守"的规则引擎，按优先级依次检查：

1. **白名单匹配优先**：已在白名单中的路径/文件名/扩展名，直接标记为已加白
2. **系统文件特征**：系统目录（`/System`、`/Library`、`~/Library` 等）、系统扩展名（`.log`、`.plist`、`.tmp`、`.cache` 等）
3. **开发工具特征**：`.git/`、`node_modules/`、`dist/`、`build/` 等构建产物目录
4. **常见办公模板**：`~/.dotfiles/`、`~/Templates/` 等

规则无法确定时：返回 `unknown` 且 `should_whitelist=false`。

#### 2) LLM 推理（优先）

当 LLM 配置完整且 `prefer_llm=true` 时：

1. 将 DLP 事件元信息批量组织为 JSON
2. 使用固定 `system_prompt` 约束输出为**纯 JSON**，包含分类、是否加白、建议的 match_type/match_value
3. 解析与校验失败时，自动回退规则结果

### 白名单机制

白名单支持三种匹配维度，按精度从高到低：

| 匹配类型 | match_type | match_value 示例 | 说明 |
|---------|-----------|----------------|------|
| 精确路径 | `path` | `/Users/alice/Library/Logs/app.log` | 仅匹配完全相同的路径 |
| 文件名 | `name` | `.DS_Store` | 匹配所有同名文件 |
| 扩展名 | `extension` | `.log` | 匹配所有同扩展名文件 |

加白策略：建议优先使用**精度最高**的匹配维度，避免过度加白。

---

## 场景三：VPN 访问策略分析（VPN Access Policy Analysis）

> 📋 **设计草案**：本场景为规划中功能，尚未实现。以下为设计规范，供后续开发参考。

### 场景说明

通过分析 VPN 访问日志，结合**员工角色、部门、访问目标 IP/端口、访问频率、访问时段**等多维数据，智能判断哪些内网访问权限是合理的、哪些是过度授权应回收的、哪些需要人工审核，从而辅助安全团队进行权限治理（Least Privilege 原则）。

### 输入（VPN Access Meta）

每条 VPN 访问记录的元信息结构如下：

```json
{
  "access_id": "vpn_789012",
  "user_id": "u_1001",
  "user_name": "张三",
  "user_role": "backend_engineer",
  "user_department": "研发部-后端组",
  "src_ip": "10.0.0.5",
  "dest_ip": "192.168.1.100",
  "dest_port": 3306,
  "protocol": "tcp",
  "access_time": "2026-07-07T10:00:00+08:00",
  "access_result": "success",
  "app_name": "MySQLWorkbench",
  "duration_seconds": 3600,
  "bytes_transferred": 1024000
}
```

#### 关键字段说明

- `user_role` / `user_department`：用户角色和部门。核心判断依据，用于匹配基准权限矩阵。
- `dest_ip` / `dest_port`：访问目标地址和端口。权限粒度的核心维度。
- `access_time`：访问时间。用于判断是否为工作时间内的正常访问。
- `access_result`：访问结果（success/fail）。失败访问不纳入权限合理性判断。
- `bytes_transferred`：数据传输量。异常大流量可能标记为风险。
- `duration_seconds`：访问持续时长。

### 输出（VPN Policy Analysis Result）

每条权限分析结果（按"用户-目标IP-端口"维度聚合后输出）：

```json
{
  "user_id": "u_1001",
  "dest_ip": "192.168.1.100",
  "dest_port": 3306,
  "category": "access_should_have",
  "should_revoke": false,
  "confidence": 0.92,
  "reasons": [
    "用户为后端工程师，数据库访问权限符合角色基准",
    "近 30 天内有 25 天访问该数据库，频率稳定",
    "访问均发生在工作时段（9:00-18:00）"
  ],
  "suggested_policy": "保留后端工程师对 192.168.1.100:3306 的 MySQL 访问权限",
  "method": "llm"
}
```

#### 字段含义

- `category`：分类标签（见下）
- `should_revoke`：
  - `true`：判断为过度授权，建议回收权限
  - `false`：判断为合理权限或无法确定（保守策略：不回收）
- `confidence`：0~1 的置信度
- `reasons`：短句列表，可解释原因
- `suggested_policy`：建议的策略调整描述（人类可读）
- `method`：`llm` 或 `heuristic`（规则兜底）

### 分类标签（Category）

`category` 取值：

- `access_should_have`：合理权限，应继续保留
- `access_should_remove`：不合理权限，建议回收（如离职员工、转岗后仍有原部门权限、长期未使用的权限）
- `access_needs_review`：无法自动判断，需要人工审核
- `unknown`：无法判断（必须保守：不建议回收）

### 推理策略

#### 1) 规则兜底（Heuristic）

本场景内置一套"可解释、保守"的规则引擎，按优先级依次检查：

1. **基准权限匹配**：基于角色/部门的基准权限矩阵，命中则标记为合理
2. **长期未访问**：超过 N 天（如 90 天）未访问的权限，建议回收
3. **离职/转岗用户**：已离职或转岗用户仍有原权限的，建议回收
4. **非常规时段访问**：大量访问发生在凌晨/非工作时间，标记为需人工审核
5. **异常大流量**：单次访问数据量远超同角色均值，标记为需人工审核

规则无法确定时：返回 `unknown` 且 `should_revoke=false`。

#### 2) LLM 推理（优先）

当 LLM 配置完整且 `prefer_llm=true` 时：

1. 将聚合后的权限访问数据（用户信息 + 目标资源 + 访问统计）组织为 JSON
2. 使用固定 `system_prompt` 约束输出为**纯 JSON**
3. 解析与校验失败时，自动回退规则结果

##### System Prompt 模板

```
你是一名资深的零信任安全架构师，负责基于 VPN 访问日志分析内网访问权限的合理性。

## 任务
分析每个用户对每个内网资源（IP:端口）的访问是否合理，给出「继续授权」或「回收权限」的建议。

## 分析维度
1. **角色/部门匹配度**：用户的角色和部门是否与该资源的典型访问者匹配
2. **访问频率**：近 30 天和 90 天的访问天数和次数
3. **访问时段**：工作时间（9:00-18:00）vs 非工作时间 vs 凌晨（0:00-6:00）
4. **最近访问时间**：最近一次访问距今多久
5. **数据传输量**：总流量大小是否合理

## 输出要求
严格输出 JSON 格式，不要输出任何其他文字。格式如下：
{
  "results": [
    {
      "user_id": "用户ID",
      "dest_ip": "目标IP",
      "dest_port": 目标端口,
      "category": "access_should_have | access_should_remove | access_needs_review | unknown",
      "should_revoke": false,
      "confidence": 0.0-1.0,
      "reasons": ["原因1", "原因2"],
      "suggested_policy": "建议的策略描述（人类可读）"
    }
  ]
}

## 分类规则
- **access_should_have**：访问合理，应继续授权。置信度 >= 0.8
- **access_should_remove**：过度授权，建议回收。置信度 >= 0.9（高风险操作，必须高度确认）
- **access_needs_review**：无法自动判断，需要人工审核。置信度 0.5-0.8
- **unknown**：信息不足，无法判断。置信度 < 0.5，should_revoke = false

## 保守原则（强约束）
- 宁可不回收，也不要误回收
- 信息不足或不确定时，返回 unknown，should_revoke = false
- 只有当有充分证据表明权限确实过度时，才建议回收

## 权限回收判断标准（满足多条时置信度累加）
1. 近 90 天无任何访问记录 → 高度怀疑过度授权（置信度 +0.4）
2. 近 30 天无任何访问记录 → 中度怀疑（置信度 +0.2）
3. 用户已离职/转岗（可从部门路径或角色名称推断） → 高度怀疑（置信度 +0.4）
4. 访问资源与用户角色/部门完全不匹配 → 中度怀疑（置信度 +0.3）
5. 仅在非常时期（如某个项目期间）短暂访问过，之后长期未使用 → 中度怀疑（置信度 +0.25）

## 权限保留判断标准
1. 近 30 天有 >= 5 天访问，且主要在工作时段 → 合理权限（置信度 >= 0.85）
2. 用户角色/部门与资源类型匹配，且访问频率稳定 → 合理权限（置信度 >= 0.8）
3. 虽然访问频率低，但每次访问都在工作时段且数据量合理 → 合理但低频（置信度 0.7-0.8，分类为 needs_review）
```

##### User Prompt 模板

```
请分析以下 VPN 访问统计数据，给出权限建议：

输入数据（每个条目为一个用户对一个目标资源的访问聚合统计）：
{{input_json_array}}

请严格按照 system prompt 中定义的 JSON 格式输出分析结果。
```

##### 输入数据字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `user_id` | string | 用户唯一标识 |
| `user_name` | string | 用户姓名 |
| `user_role` | string | 用户角色（可能多个，用逗号分隔） |
| `user_department` | string | 用户部门路径 |
| `dest_ip` | string | 目标 IP 地址 |
| `dest_port` | int | 目标端口 |
| `protocol` | string | 协议（TCP/UDP/ICMP 等） |
| `first_access_time` | string | 首次访问时间（RFC3339） |
| `last_access_time` | string | 最近访问时间（RFC3339） |
| `access_days_30` | int | 近 30 天访问天数 |
| `access_days_90` | int | 近 90 天访问天数 |
| `total_access_count` | int | 总访问次数 |
| `success_count` | int | 成功次数 |
| `failed_count` | int | 失败次数 |
| `avg_duration_sec` | int | 平均会话时长（秒） |
| `total_bytes` | int64 | 总传输字节数 |
| `work_hour_count` | int | 工作时段（9-18点）访问次数 |
| `off_hour_count` | int | 非工作时段访问次数 |
| `night_count` | int | 凌晨时段（0-6点）访问次数 |
| `resource_tag` | string | 资源标签（可选，如数据库、文件服务器等） |

### 详细数据模型

#### 数据表清单

| 表名常量 | 实际表名 | 说明 |
|---------|---------|------|
| `VPNAccessLogTable` | `vpn_access_logs` | VPN 访问日志（原始数据） |
| `VPNAccessStatsTable` | `vpn_access_stats` | 访问统计聚合（用户+资源维度） |
| `VPNPolicyAnalysisTable` | `vpn_policy_analysis` | 权限分析结果 |
| `VPNBaselinePolicyTable` | `vpn_baseline_policies` | 基准权限矩阵（规则配置） |
| `VPNAnalysisTaskStateTable` | `vpn_analysis_task_state` | 分析任务状态 |

#### 常量定义

```go
// 访问结果
const (
    VPNAccessResultSuccess = "success"
    VPNAccessResultFailed  = "failed"
)

// 分析分类（category）
const (
    VPNAnalysisCategoryShouldHave   = "access_should_have"     // 合理权限，应保留
    VPNAnalysisCategoryShouldRemove = "access_should_remove"   // 过度授权，建议回收
    VPNAnalysisCategoryNeedsReview  = "access_needs_review"    // 需要人工审核
    VPNAnalysisCategoryUnknown      = "unknown"                // 无法判断
)

// 分析状态
const (
    VPNAnalysisStatusPending   = "pending"     // 待分析
    VPNAnalysisStatusAnalyzed  = "analyzed"    // 已分析
    VPNAnalysisStatusReviewed  = "reviewed"    // 已人工审核
)

// 基准权限适用维度
const (
    VPNPolicyScopeUser       = "user"        // 按用户
    VPNPolicyScopeRole       = "role"        // 按角色
    VPNPolicyScopeDepartment = "department"  // 按部门
)

// 资源类型
const (
    VPNResourceTypeIP       = "ip"        // 单个 IP
    VPNResourceTypeCIDR     = "cidr"      // IP 网段
    VPNResourceTypeIPPort   = "ip_port"   // IP+端口
)
```

#### 1. VPNAccessLog（VPN 访问日志）

原始访问日志，从 VPN 系统同步而来。

```go
type VPNAccessLog struct {
    ID              string `json:"id"`                 // 日志唯一 ID
    AccessLogID     string `json:"access_log_id"`      // 源系统日志 ID（去重）
    UserID          string `json:"user_id"`            // 用户 ID
    UserName        string `json:"user_name"`          // 用户名
    UserRole        string `json:"user_role"`          // 用户角色（同步时快照）
    UserDepartment  string `json:"user_department"`    // 用户部门（同步时快照）
    SrcIP           string `json:"src_ip"`             // 源 IP（VPN 分配地址）
    DestIP          string `json:"dest_ip"`            // 目标 IP
    DestPort        int    `json:"dest_port"`          // 目标端口
    Protocol        string `json:"protocol"`           // 协议（tcp/udp/icmp 等）
    AccessTime      string `json:"access_time"`        // 访问时间（RFC3339）
    AccessResult    string `json:"access_result"`      // 访问结果（success/failed）
    AppName         string `json:"app_name"`           // 访问应用名（若可识别）
    DurationSeconds int    `json:"duration_seconds"`   // 会话持续时长（秒）
    BytesUploaded   int64  `json:"bytes_uploaded"`     // 上传字节数
    BytesDownloaded int64  `json:"bytes_downloaded"`   // 下载字节数
    BytesTotal      int64  `json:"bytes_total"`        // 总传输字节数
    DeviceID        string `json:"device_id"`          // 设备 ID（若有）
    DeviceName      string `json:"device_name"`        // 设备名
    ClientIP        string `json:"client_ip"`          // 客户端公网出口 IP
    Country         string `json:"country"`            // 登录地国家
    City            string `json:"city"`               // 登录地城市
    ImportedAt      string `json:"imported_at"`        // 入库时间
    RawJSON         string `json:"raw_json"`           // 原始 JSON（完整保留）
}
```

#### 2. VPNAccessStats（访问统计聚合）

按「用户 + 目标资源」维度聚合的访问统计数据，作为分析的输入基础。

```go
type VPNAccessStats struct {
    ID                   string `json:"id"`                     // 统计记录 ID
    UserID               string `json:"user_id"`                // 用户 ID
    UserName             string `json:"user_name"`              // 用户名
    UserRole             string `json:"user_role"`              // 用户角色
    UserDepartment       string `json:"user_department"`        // 用户部门
    DestIP               string `json:"dest_ip"`                // 目标 IP
    DestPort             int    `json:"dest_port"`              // 目标端口（0 表示全端口）
    Protocol             string `json:"protocol"`               // 协议
    ResourceTag          string `json:"resource_tag"`           // 资源标签（如 DB/WEB/SSH 等）
    FirstAccessTime      string `json:"first_access_time"`      // 首次访问时间
    LastAccessTime       string `json:"last_access_time"`       // 最近访问时间
    AccessDays30         int    `json:"access_days_30"`         // 近 30 天访问天数
    AccessDays90         int    `json:"access_days_90"`         // 近 90 天访问天数
    TotalAccessCount     int    `json:"total_access_count"`     // 累计访问次数
    SuccessCount         int    `json:"success_count"`          // 成功次数
    FailedCount          int    `json:"failed_count"`           // 失败次数
    AvgDurationSeconds   int    `json:"avg_duration_seconds"`   // 平均会话时长（秒）
    TotalBytes           int64  `json:"total_bytes"`            // 累计传输字节数
    WorkHourAccessCount  int    `json:"work_hour_access_count"` // 工作时段访问次数
    OffHourAccessCount   int    `json:"off_hour_access_count"`  // 非工作时段访问次数
    NightAccessCount     int    `json:"night_access_count"`     // 凌晨访问次数（0:00-6:00）
    PeakHourAccessCount  int    `json:"peak_hour_access_count"` // 高峰时段访问次数
    UpdatedAt            string `json:"updated_at"`             // 统计更新时间
}
```

#### 3. VPNPolicyAnalysis（权限分析结果）

分析结果表，存储对每个「用户+资源」维度的分析结论。

```go
type VPNPolicyAnalysis struct {
    ID               string   `json:"id"`                      // 分析结果 ID
    StatsID          string   `json:"stats_id"`                // 关联的统计记录 ID
    UserID           string   `json:"user_id"`                 // 用户 ID
    UserName         string   `json:"user_name"`               // 用户名
    UserRole         string   `json:"user_role"`               // 用户角色
    UserDepartment   string   `json:"user_department"`         // 用户部门
    DestIP           string   `json:"dest_ip"`                 // 目标 IP
    DestPort         int      `json:"dest_port"`               // 目标端口
    Protocol         string   `json:"protocol"`                // 协议
    ResourceTag      string   `json:"resource_tag"`            // 资源标签
    Category         string   `json:"category"`                // 分类标签
    ShouldRevoke     bool     `json:"should_revoke"`           // 是否建议回收
    Confidence       float64  `json:"confidence"`              // 置信度 0~1
    Reasoning        string   `json:"reasoning"`               // 推理原因（JSON 数组序列化，便于存储）
    Reasons          []string `json:"reasons"`                 // 原因列表（运行时使用，不存库）
    SuggestedPolicy  string   `json:"suggested_policy"`        // 建议的策略描述（人类可读）
    Method           string   `json:"method"`                  // 分析方式（llm/heuristic）
    Status           string   `json:"status"`                  // 状态（pending/analyzed/reviewed）
    ReviewedBy       string   `json:"reviewed_by"`             // 审核人
    ReviewedAt       string   `json:"reviewed_at"`             // 审核时间
    ReviewComment    string   `json:"review_comment"`          // 审核意见
    AnalyzedAt       string   `json:"analyzed_at"`             // 分析时间
    Revoked          bool     `json:"revoked"`                 // 是否已执行回收
    RevokedAt        string   `json:"revoked_at"`              // 回收执行时间
    RevokeError      string   `json:"revoke_error"`            // 回收失败原因
}
```

#### 4. VPNBaselinePolicy（基准权限矩阵）

配置各角色/部门应有的基准权限，作为规则兜底的判断依据。

```go
type VPNBaselinePolicy struct {
    ID           string `json:"id"`             // 策略 ID
    PolicyName   string `json:"policy_name"`    // 策略名称
    ScopeType    string `json:"scope_type"`     // 适用范围类型（user/role/department）
    ScopeValue   string `json:"scope_value"`    // 适用范围值（用户ID/角色名/部门名）
    ResourceType string `json:"resource_type"`  // 资源类型（ip/cidr/ip_port）
    DestIP       string `json:"dest_ip"`        // 目标 IP / 网段
    DestPort     int    `json:"dest_port"`      // 目标端口（0 表示所有端口）
    Protocol     string `json:"protocol"`       // 协议（tcp/udp/any）
    ResourceTag  string `json:"resource_tag"`   // 资源标签（业务标签，便于分组）
    Description  string `json:"description"`    // 策略说明
    Priority     int    `json:"priority"`       // 优先级（数字越小优先级越高）
    Enabled      bool   `json:"enabled"`        // 是否启用
    CreatedAt    string `json:"created_at"`     // 创建时间
    UpdatedAt    string `json:"updated_at"`     // 更新时间
}
```

#### 5. VPNAnalysisTaskState（分析任务状态）

调度任务状态管理，与 DLP 和重复设备的模式一致。

```go
type VPNAnalysisTaskState struct {
    ID                       string `json:"id"`                          // 状态记录 ID
    LastSyncAt               string `json:"last_sync_at"`                // 最近同步时间
    LastSyncCount            int    `json:"last_sync_count"`             // 最近同步日志数量
    LastSyncError            string `json:"last_sync_error"`             // 最近同步错误
    LastStatsAt              string `json:"last_stats_at"`               // 最近统计聚合时间
    LastStatsUserCount       int    `json:"last_stats_user_count"`       // 最近统计用户数
    LastStatsResourceCount   int    `json:"last_stats_resource_count"`   // 最近统计资源数
    LastStatsError           string `json:"last_stats_error"`            // 最近统计错误
    LastAnalysisAt           string `json:"last_analysis_at"`            // 最近分析时间
    LastAnalysisCount        int    `json:"last_analysis_count"`         // 最近分析条数
    LastAnalysisError        string `json:"last_analysis_error"`         // 最近分析错误
    SyncScheduleEnabled      bool   `json:"sync_schedule_enabled"`       // 同步定时任务是否启用
    StatsScheduleEnabled     bool   `json:"stats_schedule_enabled"`      // 统计定时任务是否启用
    AnalysisScheduleEnabled  bool   `json:"analysis_schedule_enabled"`   // 分析定时任务是否启用
}
```

#### 设计思路说明

1. **分层设计**：原始日志 → 统计聚合 → 分析结果，三层数据分离，各层职责清晰
2. **聚合粒度**：按「用户 + IP + 端口 + 协议」为最小分析单元，符合最小权限原则
3. **基准权限矩阵**：支持 user/role/department 三种维度，支持 ip/cidr/ip_port 三种资源粒度
4. **与现有风格一致**：沿用 DLP 和重复设备的模式，含任务状态表、RawJSON 兜底、状态流转
5. **审核闭环**：`status` 字段支持 pending → analyzed → reviewed 三态流转，支持人工审核备注
6. **多维度统计**：访问天数、时段分布、流量大小等多维度指标，支撑规则判断和 LLM 推理
7. **回收执行**：`revoked` 字段跟踪回收动作落地状态，支持错误信息记录
8. **资源标签**：`resource_tag` 字段便于按业务系统分组（如 DB/WEB/SSH 等）

---

## 对外接口

### 通用调用模式

#### A) 函数调用（项目内模块）

各场景独立提供 Service 类，遵循统一调用模式：

```python
# 伪代码示意，具体类型由各场景定义
svc = {Scenario}Service()
resp = svc.analyze(
    {Scenario}Request(
        prefer_llm=True,
        items=[...],
    )
)
print(resp.model_dump())
```

#### B) HTTP API（FastAPI 风格）

```
POST /api/{scenario}/analyze
```

请求体通用结构：

```json
{
  "prefer_llm": true,
  "items": [...]
}
```

返回体：对应场景的 `{Scenario}Response`。

---

## LLM 配置（可选）

默认：只用规则兜底也能工作。

启用 LLM：设置以下环境变量（使用 OpenAI 兼容 `/v1/chat/completions` 接口）：

- `LLM_BASE_URL`
- `LLM_API_KEY`
- `LLM_MODEL`

额外约束：

- 超时建议：60s（代码内默认 `timeout_s=60`）
- SSL 校验：默认 `verify_ssl=False`（适配内网/特定三方）

---

## 日志与审计

该 Skill 框架会记录：

- 请求参数（场景类型、条目数量等）
- `system_prompt`、`user_prompt`（便于复现推理）
- LLM 原始响应内容与 usage（若上游返回）
- 端到端耗时（latency）
- 规则命中情况（若走规则兜底）

---

## 误判控制（强约束）

为避免误判造成负面影响，所有场景必须遵守：

1. **不确定就 `unknown` 且不动作**：`should_action` 取保守值 `false`
2. LLM 输出必须是严格 JSON；解析失败立刻回退规则
3. 规则引擎也采用保守策略：无法识别特征时不动作
4. 高风险操作（如权限回收）必须设置比低风险操作（如文件分类）更高的置信度阈值

---

## 你可以继续扩展的点

- **增加新场景**：按本框架的模式新增分析场景（如代码安全审计、日志异常检测等）
- **白名单/黑名单机制**：各场景均可扩展白名单/黑名单规则
- **人工审核闭环**：增加 `needs_review` 状态，置信度低于阈值时进入人工审核队列
- **反馈学习**：记录人工修正结果，用于优化 prompt 或规则
- **批量聚合分析**：目前是单条目分析，可扩展为批量聚合后分析（如 VPN 场景按用户聚合访问日志）
