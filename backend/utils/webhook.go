package utils

import (
    "bytes"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "net/url"
    "strings"
    "time"
    "GO_C2/config"
    "GO_C2/db"
)

type wecomMarkdownPayload struct { MsgType string `json:"msgtype"`; Markdown wecomMarkdownContainer `json:"markdown"` }
type wecomMarkdownContainer struct { Content string `json:"content"` }

func SendWebhookMarkdown(markdownContent string) error {
    if config.GlobalConfig == nil || !config.GlobalConfig.WebhookEnable { return nil }
    baseURL := strings.TrimSpace(config.GlobalConfig.WebhookURL)
    if baseURL == "" { return nil }
    finalURL := buildWeComURL(baseURL, strings.TrimSpace(config.GlobalConfig.WebhookKey))
    payload := wecomMarkdownPayload{ MsgType: "markdown", Markdown: wecomMarkdownContainer{ Content: markdownContent } }
    data, err := json.Marshal(payload); if err != nil { return err }
    client := &http.Client{ Timeout: 5 * time.Second }
    req, err := http.NewRequest(http.MethodPost, finalURL, bytes.NewReader(data)); if err != nil { return err }
    req.Header.Set("Content-Type", "application/json")
    resp, err := client.Do(req); if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 { return fmt.Errorf("webhook status: %s", resp.Status) }
    return nil
}

func BuildBeaconRegistrationMarkdown(beacon *db.Beacon) string {
    timeStr := time.Now().Format("2006-01-02 15:04:05")
    lines := []string{
        fmt.Sprintf("实时新增 Beacon 上线 <font color=\"warning\">1 台</font>\n"),
        fmt.Sprintf("> 时间: <font color=\"comment\">%s</font>", timeStr),
        fmt.Sprintf("> 主机名: <font color=\"comment\">%s</font>", safe(beacon.HostName)),
        fmt.Sprintf("> 用户: <font color=\"comment\">%s</font>", safe(beacon.UserName)),
        fmt.Sprintf("> IP: <font color=\"comment\">%s</font>", safe(beacon.IP)),
        fmt.Sprintf("> 进程: <font color=\"comment\">%s</font>", safe(beacon.ProcessName)),
        fmt.Sprintf("> PID: <font color=\"comment\">%d</font>", beacon.ProcessID),
        fmt.Sprintf("> 架构: <font color=\"comment\">%s</font>", safe(beacon.Arch)),
        fmt.Sprintf("> UUID: <font color=\"comment\">%s</font>", safe(beacon.UUID)),
    }
    return strings.Join(lines, "\n")
}

func SendBeaconRegistrationMarkdownNotification(beacon *db.Beacon) {
    if beacon == nil { return }
    if config.GlobalConfig == nil || !config.GlobalConfig.WebhookEnable || strings.TrimSpace(config.GlobalConfig.WebhookURL) == "" { return }
    markdown := BuildBeaconRegistrationMarkdown(beacon)
    go func() { if err := SendWebhookMarkdown(markdown); err != nil { log.Printf("Webhook 推送失败: %v", err) } }()
}

func safe(s string) string { if s == "" { return "N/A" }; return s }

func buildWeComURL(baseURL, key string) string {
    if strings.Contains(baseURL, "?key=") { return baseURL }
    if strings.TrimSpace(key) == "" { return baseURL }
    sep := "?"
    if strings.Contains(baseURL, "?") {
        if !strings.Contains(baseURL, "key=") { sep = "&" } else { return baseURL }
    }
    return baseURL + sep + "key=" + url.QueryEscape(key)
}

func BuildJobCreatedMarkdown(beacon *db.Beacon, task string, summary string) string {
    timeStr := time.Now().Format("2006-01-02 15:04:05")
    lines := []string{
        fmt.Sprintf("任务下发通知 <font color=\"warning\">%s</font>\n", safe(task)),
        fmt.Sprintf("> 时间: <font color=\"comment\">%s</font>", timeStr),
        fmt.Sprintf("> 主机名: <font color=\"comment\">%s</font>", safe(beacon.HostName)),
        fmt.Sprintf("> 用户: <font color=\"comment\">%s</font>", safe(beacon.UserName)),
        fmt.Sprintf("> IP: <font color=\"comment\">%s</font>", safe(beacon.IP)),
        fmt.Sprintf("> 进程: <font color=\"comment\">%s</font>", safe(beacon.ProcessName)),
        fmt.Sprintf("> PID: <font color=\"comment\">%d</font>", beacon.ProcessID),
        fmt.Sprintf("> 架构: <font color=\"comment\">%s</font>", safe(beacon.Arch)),
        fmt.Sprintf("> UUID: <font color=\"comment\">%s</font>", safe(beacon.UUID)),
    }
    if summary != "" { lines = append(lines, fmt.Sprintf("> 摘要: <font color=\"comment\">%s</font>", safe(summary))) }
    return strings.Join(lines, "\n")
}

func SendJobCreatedMarkdownNotification(beacon *db.Beacon, task string, summary string) {
    if beacon == nil { return }
    if config.GlobalConfig == nil || !config.GlobalConfig.WebhookEnable || strings.TrimSpace(config.GlobalConfig.WebhookURL) == "" { return }
    markdown := BuildJobCreatedMarkdown(beacon, task, summary)
    go func() { if err := SendWebhookMarkdown(markdown); err != nil { log.Printf("Webhook 任务下发推送失败: %v", err) } }()
}

func BuildJobCompletedMarkdown(beacon *db.Beacon, task string, status string, result string) string {
    timeStr := time.Now().Format("2006-01-02 15:04:05")
    lines := []string{
        fmt.Sprintf("任务完成通知 <font color=\"warning\">%s</font>\n", safe(task)),
        fmt.Sprintf("> 状态: <font color=\"comment\">%s</font>", safe(status)),
        fmt.Sprintf("> 时间: <font color=\"comment\">%s</font>", timeStr),
        fmt.Sprintf("> 主机名: <font color=\"comment\">%s</font>", safe(beacon.HostName)),
        fmt.Sprintf("> 用户: <font color=\"comment\">%s</font>", safe(beacon.UserName)),
        fmt.Sprintf("> IP: <font color=\"comment\">%s</font>", safe(beacon.IP)),
        fmt.Sprintf("> 进程: <font color=\"comment\">%s</font>", safe(beacon.ProcessName)),
        fmt.Sprintf("> PID: <font color=\"comment\">%d</font>", beacon.ProcessID),
        fmt.Sprintf("> 架构: <font color=\"comment\">%s</font>", safe(beacon.Arch)),
        fmt.Sprintf("> UUID: <font color=\"comment\">%s</font>", safe(beacon.UUID)),
    }
    if result != "" { lines = append(lines, fmt.Sprintf("> 结果: <font color=\"comment\">%s</font>", safe(result))) }
    return strings.Join(lines, "\n")
}

func SendJobCompletedMarkdownNotification(beacon *db.Beacon, task string, status string, result string) {
    if beacon == nil { return }
    if config.GlobalConfig == nil || !config.GlobalConfig.WebhookEnable || strings.TrimSpace(config.GlobalConfig.WebhookURL) == "" { return }
    markdown := BuildJobCompletedMarkdown(beacon, task, status, result)
    go func() { if err := SendWebhookMarkdown(markdown); err != nil { log.Printf("Webhook 任务完成推送失败: %v", err) } }()
}

func BuildBeaconDeletedMarkdown(beacon *db.Beacon) string {
    timeStr := time.Now().Format("2006-01-02 15:04:05")
    lines := []string{
        "Beacon 删除通知\n",
        fmt.Sprintf("> 时间: <font color=\"comment\">%s</font>", timeStr),
        fmt.Sprintf("> 主机名: <font color=\"comment\">%s</font>", safe(beacon.HostName)),
        fmt.Sprintf("> 用户: <font color=\"comment\">%s</font>", safe(beacon.UserName)),
        fmt.Sprintf("> IP: <font color=\"comment\">%s</font>", safe(beacon.IP)),
        fmt.Sprintf("> 进程: <font color=\"comment\">%s</font>", safe(beacon.ProcessName)),
        fmt.Sprintf("> PID: <font color=\"comment\">%d</font>", beacon.ProcessID),
        fmt.Sprintf("> 架构: <font color=\"comment\">%s</font>", safe(beacon.Arch)),
        fmt.Sprintf("> UUID: <font color=\"comment\">%s</font>", safe(beacon.UUID)),
    }
    return strings.Join(lines, "\n")
}

func SendBeaconDeletedMarkdownNotification(beacon *db.Beacon) {
    if beacon == nil { return }
    if config.GlobalConfig == nil || !config.GlobalConfig.WebhookEnable || strings.TrimSpace(config.GlobalConfig.WebhookURL) == "" { return }
    markdown := BuildBeaconDeletedMarkdown(beacon)
    go func() { if err := SendWebhookMarkdown(markdown); err != nil { log.Printf("Webhook 删除通知推送失败: %v", err) } }()
}


