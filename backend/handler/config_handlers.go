package handler

import (
    "log"
    "net/http"
    "github.com/gin-gonic/gin"
    "GO_C2/config"
)

// GetConfigHandler 获取当前配置
func GetConfigHandler(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "webhook_enable": config.GlobalConfig.WebhookEnable,
        "webhook_url":    config.GlobalConfig.WebhookURL,
        "webhook_key":    config.GlobalConfig.WebhookKey,
        "server": gin.H{
            "host":          config.GlobalConfig.Server.Host,
            "port":          config.GlobalConfig.Server.Port,
            "read_timeout":  config.GlobalConfig.Server.ReadTimeout,
            "write_timeout": config.GlobalConfig.Server.WriteTimeout,
            "max_body_size": config.GlobalConfig.Server.MaxBodySize,
            "host_header":   config.GlobalConfig.Server.HostHeader,
            "listeners":     config.GlobalConfig.Server.Listeners,
        },
        "routes": gin.H{
            "admin_prefix":    config.GlobalConfig.Routes.AdminPrefix,
            "api_prefix":      config.GlobalConfig.Routes.APIPrefix,
            "beacon_endpoint": config.GlobalConfig.Routes.BeaconEndpoint,
            "register_path":   config.GlobalConfig.Routes.RegisterPath,
            "login_path":      config.GlobalConfig.Routes.LoginPath,
        },
    })
}

// UpdateConfigHandler 更新配置
func UpdateConfigHandler(c *gin.Context) {
    var newConfig struct {
        AdminPass     string `json:"admin_pass"`
        WebhookURL    string `json:"webhook_url"`
        WebhookKey    string `json:"webhook_key"`
        WebhookEnable bool   `json:"webhook_enable"`
        Server        struct {
            Host         string `json:"host"`
            Port         int    `json:"port"`
            ReadTimeout  int    `json:"read_timeout"`
            WriteTimeout int    `json:"write_timeout"`
            MaxBodySize  int    `json:"max_body_size"`
            HostHeader   string `json:"host_header"`
            Listeners    []config.ListenerConfig `json:"listeners"`
        } `json:"server"`
        Routes struct {
            AdminPrefix    string `json:"admin_prefix"`
            APIPrefix      string `json:"api_prefix"`
            BeaconEndpoint string `json:"beacon_endpoint"`
            RegisterPath   string `json:"register_path"`
            LoginPath      string `json:"login_path"`
        } `json:"routes"`
    }

    if err := c.BindJSON(&newConfig); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config data"})
        return
    }

    if newConfig.AdminPass != "" { config.GlobalConfig.AdminPass = newConfig.AdminPass }
    config.GlobalConfig.WebhookURL = newConfig.WebhookURL
    config.GlobalConfig.WebhookKey = newConfig.WebhookKey
    config.GlobalConfig.WebhookEnable = newConfig.WebhookEnable

    if newConfig.Server.Host != "" { config.GlobalConfig.Server.Host = newConfig.Server.Host }
    if newConfig.Server.Port > 0 { config.GlobalConfig.Server.Port = newConfig.Server.Port }
    if newConfig.Server.ReadTimeout > 0 { config.GlobalConfig.Server.ReadTimeout = newConfig.Server.ReadTimeout }
    if newConfig.Server.WriteTimeout > 0 { config.GlobalConfig.Server.WriteTimeout = newConfig.Server.WriteTimeout }
    if newConfig.Server.MaxBodySize > 0 { config.GlobalConfig.Server.MaxBodySize = newConfig.Server.MaxBodySize }
    if newConfig.Server.Host != "" { config.GlobalConfig.Server.Host = newConfig.Server.Host }
    if newConfig.Server.Port > 0 { config.GlobalConfig.Server.Port = newConfig.Server.Port }
    // HostHeader（允许置空）
    if newConfig.Server.HostHeader != "" || (newConfig.Server.HostHeader == "") {
        config.GlobalConfig.Server.HostHeader = newConfig.Server.HostHeader
    }
    // 多监听（仅更新 beacon 类型，保留现有非 beacon 类型）
    if newConfig.Server.Listeners != nil {
        // 规范化入参：默认 type=beacon
        sanitized := make([]config.ListenerConfig, 0, len(newConfig.Server.Listeners))
        for _, it := range newConfig.Server.Listeners {
            if it.Type == "" { it.Type = "beacon" }
            sanitized = append(sanitized, it)
        }

        preserved := make([]config.ListenerConfig, 0)
        for _, old := range config.GlobalConfig.Server.Listeners {
            if old.Type != "beacon" {
                preserved = append(preserved, old)
            }
        }
        config.GlobalConfig.Server.Listeners = append(preserved, sanitized...)
    }

    if newConfig.Routes.AdminPrefix != "" { config.GlobalConfig.Routes.AdminPrefix = newConfig.Routes.AdminPrefix }
    if newConfig.Routes.APIPrefix != "" { config.GlobalConfig.Routes.APIPrefix = newConfig.Routes.APIPrefix }
    if newConfig.Routes.BeaconEndpoint != "" { config.GlobalConfig.Routes.BeaconEndpoint = newConfig.Routes.BeaconEndpoint }
    if newConfig.Routes.RegisterPath != "" { config.GlobalConfig.Routes.RegisterPath = newConfig.Routes.RegisterPath }
    if newConfig.Routes.LoginPath != "" { config.GlobalConfig.Routes.LoginPath = newConfig.Routes.LoginPath }

    if err := config.SaveConfig(); err != nil {
        log.Printf("Failed to save config: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save configuration"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Configuration updated successfully. Some changes may require server restart to take full effect."})
}

// ReloadConfigHandler 重新加载配置
func ReloadConfigHandler(c *gin.Context) {
    if err := config.Init(); err != nil {
        log.Printf("Failed to reload config: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload configuration"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Configuration reloaded successfully"})
}


