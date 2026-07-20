---
sidebar_position: 7
---

# Notifications

buildworld supports multiple notification channels to alert you about build status changes.

## Supported Channels

| Channel | Description |
|---------|-------------|
| Email | Send notifications via SMTP |
| Feishu | Send notifications to Feishu webhook |
| Webhook | Send HTTP POST/GET/PUT requests |

## Configuration

### Email Channel

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

### Feishu Channel

```json
{
  "webhook_url": "https://open.feishu.cn/open-apis/bot/v2/hook/xxxx"
}
```

### Webhook Channel

```json
{
  "url": "https://api.example.com/webhook",
  "method": "POST",
  "headers": {
    "Authorization": "Bearer your-token"
  }
}
```

## Conditions

Filter notifications by build status:

```json
{
  "statuses": ["success", "failed"]
}
```

Leave empty `{}` to receive notifications for all statuses.

## Notification Payload

The payload sent to webhooks:

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

## Web UI Management

1. Go to **Notifications** in the navigation
2. Click **New Channel**
3. Select channel type
4. Configure the channel
5. Set conditions (optional)
6. Click **Save**

## API Endpoints

```bash
# List channels
GET /api/notifications/channels

# Create channel
POST /api/notifications/channels
{
  "name": "My Channel",
  "type": "email",
  "config": "{...}",
  "conditions": "{}",
  "description": "",
  "enabled": true
}

# Get channel
GET /api/notifications/channels/{id}

# Update channel
PUT /api/notifications/channels/{id}

# Delete channel
DELETE /api/notifications/channels/{id}

# List events
GET /api/notifications/channels/{id}/events
```

## Next Steps

- [Pipeline Guide](/pipelines) - Create pipelines
- [Configuration](/configuration) - Configure the server