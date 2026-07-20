package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"sync"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

type NotificationService struct {
	store *store.Store
}

func NewNotificationService(s *store.Store) *NotificationService {
	return &NotificationService{store: s}
}

type NotificationPayload struct {
	Event     string    `json:"event"`
	BuildID   int64     `json:"build_id"`
	BuildNum  int       `json:"build_number"`
	ProjectID int64     `json:"project_id"`
	Project   string    `json:"project"`
	Status    string    `json:"status"`
	Trigger   string    `json:"trigger"`
	Branch    string    `json:"branch,omitempty"`
	Commit    string    `json:"commit,omitempty"`
	Duration  int64     `json:"duration_ms,omitempty"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func (ns *NotificationService) SendBuildNotifications(build *store.Build, project *store.Project) error {
	return ns.SendBuildEvent(build, project, "build.completed")
}

// SendBuildEvent delivers the shared channel configuration at lifecycle points.
// Events include build.started and build.completed; per-channel conditions decide
// which statuses should be delivered.
func (ns *NotificationService) SendBuildEvent(build *store.Build, project *store.Project, eventType string) error {
	channels, err := ns.store.ListEnabledNotificationChannels()
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}

	payload := &NotificationPayload{
		Event:     eventType,
		BuildID:   build.ID,
		BuildNum:  build.Number,
		ProjectID: build.ProjectID,
		Project:   project.Name,
		Status:    build.Status,
		Trigger:   build.Trigger,
		Branch:    build.Branch,
		Commit:    build.CommitSHA,
		Message:   fmt.Sprintf("Build #%d for %s %s", build.Number, project.Name, build.Status),
		Timestamp: time.Now(),
	}

	if build.DurationMs != nil {
		payload.Duration = *build.DurationMs
	}

	var deliveries sync.WaitGroup
	for _, channel := range channels {
		if !ns.matchesConditions(channel, payload) {
			continue
		}
		deliveries.Add(1)
		go func(target *store.NotificationChannel) {
			defer deliveries.Done()
			ns.sendToChannel(target, payload, build.ID)
		}(channel)
	}
	// Each enabled matching channel is independent and executes concurrently.
	// The runner invokes this method asynchronously, so waiting here guarantees
	// that all delivery results are persisted without delaying the build itself.
	deliveries.Wait()

	return nil
}

func (ns *NotificationService) matchesConditions(channel *store.NotificationChannel, payload *NotificationPayload) bool {
	if channel.Conditions == "" || channel.Conditions == "{}" {
		return true
	}

	var conditions struct {
		Statuses []string `json:"statuses"`
	}

	if err := json.Unmarshal([]byte(channel.Conditions), &conditions); err != nil {
		return true
	}

	if len(conditions.Statuses) == 0 {
		return true
	}

	for _, status := range conditions.Statuses {
		if status == payload.Status {
			return true
		}
	}

	return false
}

func (ns *NotificationService) sendToChannel(channel *store.NotificationChannel, payload *NotificationPayload, buildID int64) {
	payloadJSON, _ := json.Marshal(payload)
	event, err := ns.store.CreateNotificationEvent(channel.ID, &buildID, payload.Event, string(payloadJSON))
	if err != nil {
		return
	}

	var errMsg string
	var success bool

	switch channel.Type {
	case store.NotificationChannelWeb:
		// The persisted event is the delivery. Authenticated browsers consume it
		// through the in-app notification feed.
		success = true
	case store.NotificationChannelEmail:
		success, errMsg = ns.sendEmail(channel, payload)
	case store.NotificationChannelFeishu:
		success, errMsg = ns.sendFeishu(channel, payload)
	case store.NotificationChannelWebhook:
		success, errMsg = ns.sendWebhook(channel, payload)
	case store.NotificationChannelDiscord:
		success, errMsg = ns.sendDiscord(channel, payload)
	case store.NotificationChannelWeCom:
		success, errMsg = ns.sendWeCom(channel, payload)
	case store.NotificationChannelTelegram:
		success, errMsg = ns.sendTelegram(channel, payload)
	default:
		errMsg = "unknown channel type"
	}

	now := time.Now()
	if success {
		ns.store.UpdateNotificationEventStatus(event.ID, "delivered", nil, &now)
	} else {
		ns.store.UpdateNotificationEventStatus(event.ID, "failed", &errMsg, nil)
	}
}

func (ns *NotificationService) sendDiscord(channel *store.NotificationChannel, payload *NotificationPayload) (bool, string) {
	var config struct {
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false, fmt.Sprintf("invalid config: %v", err)
	}
	if config.WebhookURL == "" {
		return false, "missing webhook URL"
	}
	color := 0x39a778
	if payload.Status == "failed" {
		color = 0xc95050
	} else if payload.Status == "running" {
		color = 0x168ab4
	}
	message := map[string]interface{}{"embeds": []interface{}{map[string]interface{}{
		"title": fmt.Sprintf("%s · Build #%d", payload.Project, payload.BuildNum), "description": payload.Message, "color": color,
		"fields": []map[string]string{{"name": "Branch", "value": payload.Branch, "inline": "true"}, {"name": "Status", "value": payload.Status, "inline": "true"}},
	}}}
	return postJSON(config.WebhookURL, message)
}

func (ns *NotificationService) sendWeCom(channel *store.NotificationChannel, payload *NotificationPayload) (bool, string) {
	var config struct {
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false, fmt.Sprintf("invalid config: %v", err)
	}
	if config.WebhookURL == "" {
		return false, "missing webhook URL"
	}
	content := fmt.Sprintf("<font color=\"info\">%s</font>\n> Build: <font color=\"comment\">#%d</font>\n> Status: **%s**\n> Branch: %s", payload.Project, payload.BuildNum, payload.Status, payload.Branch)
	return postJSON(config.WebhookURL, map[string]interface{}{"msgtype": "markdown", "markdown": map[string]string{"content": content}})
}

func (ns *NotificationService) sendTelegram(channel *store.NotificationChannel, payload *NotificationPayload) (bool, string) {
	var config struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false, fmt.Sprintf("invalid config: %v", err)
	}
	if config.BotToken == "" || config.ChatID == "" {
		return false, "missing bot token or chat ID"
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken)
	text := fmt.Sprintf("<b>%s</b>\nBuild #%d: <b>%s</b>\nBranch: <code>%s</code>", payload.Project, payload.BuildNum, payload.Status, payload.Branch)
	return postJSON(url, map[string]string{"chat_id": config.ChatID, "text": text, "parse_mode": "HTML"})
}

func postJSON(url string, body interface{}) (bool, string) {
	data, err := json.Marshal(body)
	if err != nil {
		return false, err.Error()
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return false, fmt.Sprintf("http post failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("channel returned status: %d", resp.StatusCode)
	}
	return true, ""
}

func (ns *NotificationService) sendEmail(channel *store.NotificationChannel, payload *NotificationPayload) (bool, string) {
	var config struct {
		SMTPHost     string   `json:"smtp_host"`
		SMTPPort     int      `json:"smtp_port"`
		SMTPUser     string   `json:"smtp_user"`
		SMTPPassword string   `json:"smtp_password"`
		From         string   `json:"from"`
		To           []string `json:"to"`
	}

	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false, fmt.Sprintf("invalid config: %v", err)
	}

	if config.SMTPHost == "" || len(config.To) == 0 {
		return false, "missing SMTP host or recipients"
	}

	msg := fmt.Sprintf(`From: %s
To: %s
Subject: [%s] Build #%d %s

Project: %s
Build: #%d
Status: %s
Trigger: %s
Branch: %s
Commit: %s
Duration: %dms
Time: %s

Message: %s
`,
		config.From,
		joinStrings(config.To, ","),
		payload.Project,
		payload.BuildNum,
		payload.Status,
		payload.Project,
		payload.BuildNum,
		payload.Status,
		payload.Trigger,
		payload.Branch,
		payload.Commit,
		payload.Duration,
		payload.Timestamp.Format(time.RFC1123),
		payload.Message,
	)

	auth := smtp.PlainAuth("", config.SMTPUser, config.SMTPPassword, config.SMTPHost)
	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	if err := smtp.SendMail(addr, auth, config.From, config.To, []byte(msg)); err != nil {
		return false, fmt.Sprintf("send mail failed: %v", err)
	}

	return true, ""
}

func (ns *NotificationService) sendFeishu(channel *store.NotificationChannel, payload *NotificationPayload) (bool, string) {
	var config struct {
		WebhookURL string `json:"webhook_url"`
	}

	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false, fmt.Sprintf("invalid config: %v", err)
	}

	if config.WebhookURL == "" {
		return false, "missing webhook URL"
	}

	var statusEmoji string
	switch payload.Status {
	case "success":
		statusEmoji = "✅"
	case "failed":
		statusEmoji = "❌"
	case "running":
		statusEmoji = "🔄"
	default:
		statusEmoji = "📌"
	}

	feishuMsg := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"config": map[string]interface{}{
				"wide_screen_mode": true,
			},
			"elements": []interface{}{
				map[string]interface{}{
					"tag": "div",
					"text": map[string]interface{}{
						"content": fmt.Sprintf("**%s** [%s] Build #%d %s", statusEmoji, payload.Project, payload.BuildNum, payload.Status),
						"tag":     "lark_md",
					},
				},
				map[string]interface{}{
					"tag": "div",
					"fields": []interface{}{
						map[string]interface{}{
							"is_short": true,
							"text": map[string]interface{}{
								"content": fmt.Sprintf("**Trigger**: %s", payload.Trigger),
								"tag":     "lark_md",
							},
						},
						map[string]interface{}{
							"is_short": true,
							"text": map[string]interface{}{
								"content": fmt.Sprintf("**Branch**: %s", payload.Branch),
								"tag":     "lark_md",
							},
						},
						map[string]interface{}{
							"is_short": true,
							"text": map[string]interface{}{
								"content": fmt.Sprintf("**Commit**: %s", truncateString(payload.Commit, 12)),
								"tag":     "lark_md",
							},
						},
						map[string]interface{}{
							"is_short": true,
							"text": map[string]interface{}{
								"content": fmt.Sprintf("**Duration**: %dms", payload.Duration),
								"tag":     "lark_md",
							},
						},
					},
				},
			},
		},
	}

	data, _ := json.Marshal(feishuMsg)
	resp, err := http.Post(config.WebhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return false, fmt.Sprintf("http post failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("feishu returned status: %d", resp.StatusCode)
	}

	return true, ""
}

func (ns *NotificationService) sendWebhook(channel *store.NotificationChannel, payload *NotificationPayload) (bool, string) {
	var config struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Secret  string            `json:"secret"`
	}

	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false, fmt.Sprintf("invalid config: %v", err)
	}

	if config.URL == "" {
		return false, "missing webhook URL"
	}

	if config.Method == "" {
		config.Method = "POST"
	}

	data, _ := json.Marshal(payload)
	req, err := http.NewRequest(config.Method, config.URL, bytes.NewBuffer(data))
	if err != nil {
		return false, fmt.Sprintf("create request failed: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range config.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("http request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("webhook returned status: %d", resp.StatusCode)
	}

	return true, ""
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
