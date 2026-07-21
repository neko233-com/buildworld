---
sidebar_position: 7
---

# 通知系统

buildworld 支持多种通知渠道，在构建状态变化时提醒您。

## 支持的渠道

| 渠道 | 描述 |
|------|------|
| 邮件 | 通过 SMTP 发送通知 |
| 飞书 | 发送通知到飞书 Webhook |
| Webhook | 发送 HTTP POST/GET/PUT 请求 |

## 配置

### 邮件渠道

```json
{
  "smtp_host": "smtp.example.com",
  "smtp_port": 587,
  "smtp_user": "user@example.com",
  "smtp_password": "secret",
  "from": "buildworld@example.com",
  "to": ["dev@example.com", "ops@example.com"]
}
```

### 飞书渠道

```json
{
  "webhook_url": "https://open.feishu.cn/open-apis/bot/v2/hook/xxxx"
}
```

### Webhook 渠道

```json
{
  "url": "https://api.example.com/webhook",
  "method": "POST",
  "headers": {
    "Authorization": "Bearer your-token"
  }
}
```

## 条件过滤

按构建状态过滤通知：

```json
{
  "statuses": ["success", "failed"]
}
```

留空 `{}` 可接收所有状态的通知。

## 通知载荷

发送到 Webhook 的载荷：

```json
{
  "event": "build_completed",
  "build_id": 123,
  "build_number": 42,
  "project_id": 5,
  "project": "my-app",
  "status": "success",
  "trigger": "manual",
  "branch": "main",
  "commit": "a1b2c3d",
  "duration_ms": 12345,
  "message": "Build #42 for my-app success",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## Web UI 管理

1. 进入导航栏的 **通知** 页面
2. 点击 **新建渠道**
3. 选择渠道类型
4. 配置渠道参数
5. 设置条件（可选）
6. 点击 **保存**

## API 端点

```bash
# 列出渠道
GET /api/notifications/channels

# 创建渠道
POST /api/notifications/channels
{
  "name": "My Channel",
  "type": "email",
  "config": "{...}",
  "conditions": "{}",
  "description": "",
  "enabled": true
}

# 获取渠道
GET /api/notifications/channels/{id}

# 更新渠道
PUT /api/notifications/channels/{id}

# 删除渠道
DELETE /api/notifications/channels/{id}

# 列出事件
GET /api/notifications/channels/{id}/events
```

## 下一步

- [流水线指南](./pipelines) - 创建流水线
- [配置指南](./configuration) - 配置服务器
