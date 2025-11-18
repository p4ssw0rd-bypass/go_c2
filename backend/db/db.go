package db

import (
    "database/sql"
    "time"
    "unicode/utf8"

    _ "github.com/go-sql-driver/mysql"
    "golang.org/x/crypto/bcrypt"

    "GO_C2/config"
    "strings"
    "log"
)

var DB *sql.DB

func Init() error {
    var (
        db  *sql.DB
        err error
    )
    // 强制 MySQL
    mc := config.GlobalConfig.Database.MySQL
    if mc.Port == 0 { mc.Port = 3306 }
    dsn := mc.User + ":" + mc.Password + "@tcp(" + mc.Host + ":" + itoa(mc.Port) + ")/" + mc.DBName
    params := mc.Params
    if params == "" { params = "charset=utf8mb4&parseTime=true&loc=Local" }
    dsn = dsn + "?" + params
    db, err = sql.Open("mysql", dsn)
    if err != nil { return err }
    if config.GlobalConfig.Database.MaxOpenConns > 0 { db.SetMaxOpenConns(config.GlobalConfig.Database.MaxOpenConns) }
    if config.GlobalConfig.Database.MaxIdleConns > 0 { db.SetMaxIdleConns(config.GlobalConfig.Database.MaxIdleConns) }
    if config.GlobalConfig.Database.ConnMaxLifetime > 0 { db.SetConnMaxLifetime(time.Duration(config.GlobalConfig.Database.ConnMaxLifetime) * time.Second) }

    if err = db.Ping(); err != nil { return err }
    DB = db
    return createTablesMySQL()
}

// SQLite 分支移除，统一使用 MySQL

func createTablesMySQL() error {
    _, err := DB.Exec(`
    CREATE TABLE IF NOT EXISTS beacons (
        id INT AUTO_INCREMENT PRIMARY KEY,
        ip VARCHAR(255) NOT NULL,
        ipv6 VARCHAR(255) NULL,
        mac VARCHAR(64) NULL,
        hostname VARCHAR(255) NOT NULL,
        username VARCHAR(255) NOT NULL,
        process_name VARCHAR(255) NOT NULL,
        process_path TEXT NOT NULL,
        process_id INT NOT NULL,
        arch VARCHAR(50) NOT NULL,
        os_version VARCHAR(255) NULL,
        os_uuid VARCHAR(100) NOT NULL,
        uuid VARCHAR(100) NOT NULL UNIQUE,
        first_time DATETIME NOT NULL,
        last_seen DATETIME NOT NULL,
        job LONGTEXT,
        job_result LONGTEXT,
        sleep_seconds INT NOT NULL DEFAULT 10,
        remark TEXT NULL,
        current_task VARCHAR(64) NULL
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`)
    if err != nil { return err }

    // 创建upload_sessions表 - 管理上传会话
    _, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS upload_sessions (
            id INT AUTO_INCREMENT PRIMARY KEY,
            uuid VARCHAR(100) NOT NULL,
            filename VARCHAR(255) NOT NULL,
            target_path TEXT NOT NULL,
            total_chunks INT NOT NULL,
            total_size BIGINT NOT NULL,
            status ENUM('pending', 'in_progress', 'completed', 'failed') DEFAULT 'pending',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            INDEX idx_uuid (uuid),
            INDEX idx_status (status),
            FOREIGN KEY (uuid) REFERENCES beacons(uuid) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
    `)
    if err != nil {
        return err
    }

    // 创建upload_chunks表 - 管理分块信息
    _, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS upload_chunks (
            id INT AUTO_INCREMENT PRIMARY KEY,
            session_id INT NOT NULL,
            chunk_index INT NOT NULL,
            chunk_data LONGTEXT NOT NULL,
            chunk_offset BIGINT NOT NULL,
            chunk_size INT NOT NULL,
            status ENUM('pending', 'sent', 'acknowledged', 'failed') DEFAULT 'pending',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            sent_at TIMESTAMP NULL,
            acknowledged_at TIMESTAMP NULL,
            INDEX idx_session_id (session_id),
            INDEX idx_chunk_index (chunk_index),
            INDEX idx_status (status),
            UNIQUE KEY unique_session_chunk (session_id, chunk_index),
            FOREIGN KEY (session_id) REFERENCES upload_sessions(id) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
    `)
    if err != nil {
        return err
    }

    // 创建task_queue表 - 任务队列
    _, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS task_queue (
            id INT AUTO_INCREMENT PRIMARY KEY,
            uuid VARCHAR(100) NOT NULL,
            task_type TINYINT NOT NULL,
            task_data LONGTEXT NOT NULL,
            priority INT DEFAULT 0,
            status ENUM('pending', 'processing', 'completed', 'failed') DEFAULT 'pending',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            started_at TIMESTAMP NULL,
            completed_at TIMESTAMP NULL,
            retry_count INT DEFAULT 0,
            max_retries INT DEFAULT 3,
            INDEX idx_uuid (uuid),
            INDEX idx_status (status),
            INDEX idx_priority (priority),
            INDEX idx_created_at (created_at),
            FOREIGN KEY (uuid) REFERENCES beacons(uuid) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
    `)
    if err != nil {
        return err
    }

    // 创建download_sessions表 - 下载会话管理
    _, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS download_sessions (
            id INT AUTO_INCREMENT PRIMARY KEY,
            uuid VARCHAR(100) NOT NULL,
            filename VARCHAR(255) NOT NULL,
            file_path TEXT NOT NULL,
            total_chunks INT DEFAULT 0,
            status ENUM('pending', 'in_progress', 'completed', 'failed') DEFAULT 'pending',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            INDEX idx_uuid (uuid),
            INDEX idx_status (status),
            FOREIGN KEY (uuid) REFERENCES beacons(uuid) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
    `)
    if err != nil {
        return err
    }

    // 创建download_chunks表 - 下载分块管理
    _, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS download_chunks (
            id INT AUTO_INCREMENT PRIMARY KEY,
            session_id INT NOT NULL,
            chunk_index INT NOT NULL,
            chunk_data LONGTEXT NOT NULL,
            chunk_offset BIGINT NOT NULL,
            chunk_size INT NOT NULL,
            status ENUM('pending', 'completed', 'failed') DEFAULT 'pending',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            completed_at TIMESTAMP NULL,
            INDEX idx_session_id (session_id),
            INDEX idx_chunk_index (chunk_index),
            INDEX idx_status (status),
            UNIQUE KEY unique_session_chunk (session_id, chunk_index),
            FOREIGN KEY (session_id) REFERENCES download_sessions(id) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
    `)
    if err != nil {
        return err
    }

    _, err = DB.Exec(`
    CREATE TABLE IF NOT EXISTS admin_users (
        id INT AUTO_INCREMENT PRIMARY KEY,
        username VARCHAR(100) NOT NULL UNIQUE,
        password VARCHAR(255) NOT NULL
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`)
    if err != nil { return err }

    _, err = DB.Exec(`
    CREATE TABLE IF NOT EXISTS task_history (
        id INT AUTO_INCREMENT PRIMARY KEY,
        beacon_uuid VARCHAR(100) NOT NULL,
        task_type VARCHAR(50) NOT NULL,
        command TEXT NOT NULL,
        status VARCHAR(50) NOT NULL DEFAULT 'pending',
        result LONGTEXT,
        created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
        INDEX idx_th_beacon_uuid (beacon_uuid),
        CONSTRAINT fk_th_beacon FOREIGN KEY (beacon_uuid) REFERENCES beacons(uuid) ON DELETE CASCADE ON UPDATE CASCADE
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`)
    if err != nil { return err }
    // ensure default admin exists and schema compatibility
    if err := ensureDefaultAdmin(); err != nil { return err }
    return ensureSchemaMySQL()
}

func ensureDefaultAdmin() error {
    // if there is any user, skip
    var count int
    if err := DB.QueryRow("SELECT COUNT(1) FROM admin_users").Scan(&count); err != nil { return err }
    if count > 0 { return nil }
    // create default admin/admin123
    hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
    if err != nil { return err }
    _, err = DB.Exec("INSERT INTO admin_users (username, password) VALUES (?, ?)", "admin", string(hash))
    return err
}

// user management
func GetUserByUsername(username string) (*AdminUser, error) {
    u := &AdminUser{}
    err := DB.QueryRow("SELECT id, username, password FROM admin_users WHERE username = ?", username).Scan(&u.ID, &u.Username, &u.Password)
    if err != nil { return nil, err }
    return u, nil
}

func ListUsers() ([]*AdminUser, error) {
    rows, err := DB.Query("SELECT id, username, password FROM admin_users ORDER BY id ASC")
    if err != nil { return nil, err }
    defer rows.Close()
    var users []*AdminUser
    for rows.Next() {
        u := &AdminUser{}
        if err := rows.Scan(&u.ID, &u.Username, &u.Password); err != nil { return nil, err }
        users = append(users, u)
    }
    return users, nil
}

func CreateUser(username, password string) error {
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil { return err }
    _, err = DB.Exec("INSERT INTO admin_users (username, password) VALUES (?, ?)", username, string(hash))
    return err
}

func UpdateUserPassword(username, password string) error {
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil { return err }
    _, err = DB.Exec("UPDATE admin_users SET password = ? WHERE username = ?", string(hash), username)
    return err
}

func DeleteUser(username string) error {
    _, err := DB.Exec("DELETE FROM admin_users WHERE username = ?", username)
    return err
}

func configType() string { return "mysql" }

func itoa(n int) string {
    if n == 0 { return "0" }
    var buf [20]byte
    i := len(buf)
    nn := n
    for nn > 0 {
        i--; buf[i] = byte('0' + nn%10); nn /= 10
    }
    return string(buf[i:])
}

// CRUD & helpers copied
func CreateBeacon(beacon *Beacon) error {
    if beacon.SleepSeconds <= 0 { beacon.SleepSeconds = 10 }
    processPath := []byte(beacon.ProcessPath)
    if !utf8.Valid(processPath) { processPath = []byte(string([]rune(beacon.ProcessPath))) }
    _, err := DB.Exec(`
    INSERT INTO beacons (
        ip, ipv6, mac, hostname, username, process_name, process_path, process_id,
        arch, os_version, os_uuid, uuid, first_time, last_seen, job, job_result, sleep_seconds, remark, current_task
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        beacon.IP, beacon.IPv6, beacon.MAC, beacon.HostName, beacon.UserName, beacon.ProcessName,
        string(processPath), beacon.ProcessID, beacon.Arch, beacon.OSVersion, beacon.OSUUID,
        beacon.UUID, beacon.FirstTime, beacon.LastSeen, beacon.Job, beacon.JobResult, beacon.SleepSeconds, beacon.Remark, beacon.CurrentTask)
    return err
}

func GetBeaconByUUID(uuid string) (*Beacon, error) {
    beacon := &Beacon{}
    err := DB.QueryRow(`
    SELECT id,
           ip,
           COALESCE(ipv6, ''),
           COALESCE(mac, ''),
           hostname,
           username,
           process_name,
           process_path,
           process_id,
           arch,
           COALESCE(os_version, ''),
           os_uuid,
           uuid,
           first_time,
           last_seen,
           COALESCE(job, ''),
           COALESCE(job_result, ''),
           sleep_seconds,
           COALESCE(remark, ''),
           COALESCE(current_task, '')
    FROM beacons WHERE uuid = ?`, uuid).Scan(
        &beacon.ID, &beacon.IP, &beacon.IPv6, &beacon.MAC, &beacon.HostName, &beacon.UserName,
        &beacon.ProcessName, &beacon.ProcessPath, &beacon.ProcessID,
        &beacon.Arch, &beacon.OSVersion, &beacon.OSUUID, &beacon.UUID, &beacon.FirstTime,
        &beacon.LastSeen, &beacon.Job, &beacon.JobResult, &beacon.SleepSeconds, &beacon.Remark, &beacon.CurrentTask)
    if err != nil { return nil, err }
    processPath := []byte(beacon.ProcessPath)
    if !utf8.Valid(processPath) { beacon.ProcessPath = string([]rune(beacon.ProcessPath)) }
    return beacon, nil
}

func UpdateBeaconLastSeen(uuid string) error {
    _, err := DB.Exec("UPDATE beacons SET last_seen = ? WHERE uuid = ?", time.Now(), uuid)
    return err
}

func UpdateBeaconJob(uuid string, job string) error {
    _, err := DB.Exec("UPDATE beacons SET job = ? WHERE uuid = ?", job, uuid)
    return err
}

func UpdateBeaconJobResult(uuid string, result string) error {
    _, err := DB.Exec("UPDATE beacons SET job_result = ? WHERE uuid = ?", result, uuid)
    return err
}

func ListBeacons() ([]*Beacon, error) {
    rows, err := DB.Query(`
    SELECT id,
           ip,
           COALESCE(ipv6, ''),
           COALESCE(mac, ''),
           hostname,
           username,
           process_name,
           process_path,
           process_id,
           arch,
           COALESCE(os_version, ''),
           os_uuid,
           uuid,
           first_time,
           last_seen,
           COALESCE(job, ''),
           COALESCE(job_result, ''),
           sleep_seconds,
           COALESCE(remark, ''),
           COALESCE(current_task, '')
    FROM beacons`)
    if err != nil { return nil, err }
    defer rows.Close()
    var beacons []*Beacon
    for rows.Next() {
        beacon := &Beacon{}
        if err := rows.Scan(&beacon.ID, &beacon.IP, &beacon.IPv6, &beacon.MAC, &beacon.HostName, &beacon.UserName,
            &beacon.ProcessName, &beacon.ProcessPath, &beacon.ProcessID, &beacon.Arch,
            &beacon.OSVersion, &beacon.OSUUID, &beacon.UUID, &beacon.FirstTime, &beacon.LastSeen, &beacon.Job,
            &beacon.JobResult, &beacon.SleepSeconds, &beacon.Remark, &beacon.CurrentTask); err != nil { return nil, err }
        processPath := []byte(beacon.ProcessPath)
        if !utf8.Valid(processPath) { beacon.ProcessPath = string([]rune(beacon.ProcessPath)) }
        beacons = append(beacons, beacon)
    }
    return beacons, nil
}

func CreateTaskHistory(history *TaskHistory) error {
    _, err := DB.Exec(`
    INSERT INTO task_history (beacon_uuid, task_type, command, status, result, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?)`,
        history.BeaconUUID, history.TaskType, history.Command, history.Status,
        history.Result, history.CreatedAt, history.UpdatedAt)
    return err
}

func UpdateTaskHistory(id int64, status, result string) error {
    _, err := DB.Exec(`
    UPDATE task_history
    SET status = ?, result = ?, updated_at = CURRENT_TIMESTAMP
    WHERE id = ?`, status, result, id)
    return err
}

func GetTaskHistoryByBeaconUUID(beaconUUID string) ([]*TaskHistory, error) {
    rows, err := DB.Query(`
    SELECT id, beacon_uuid, task_type, command, status, result, created_at, updated_at
    FROM task_history
    WHERE beacon_uuid = ?
    ORDER BY updated_at DESC, id DESC`, beaconUUID)
    if err != nil { return nil, err }
    defer rows.Close()
    var histories []*TaskHistory
    for rows.Next() {
        h := &TaskHistory{}
        if err := rows.Scan(&h.ID, &h.BeaconUUID, &h.TaskType, &h.Command,
            &h.Status, &h.Result, &h.CreatedAt, &h.UpdatedAt); err != nil { return nil, err }
        histories = append(histories, h)
    }
    return histories, nil
}

func GetAllTaskHistory() ([]*TaskHistory, error) {
    rows, err := DB.Query(`
    SELECT id, beacon_uuid, task_type, command, status, result, created_at, updated_at
    FROM task_history
    ORDER BY updated_at DESC, id DESC`)
    if err != nil { return nil, err }
    defer rows.Close()
    var histories []*TaskHistory
    for rows.Next() {
        h := &TaskHistory{}
        if err := rows.Scan(&h.ID, &h.BeaconUUID, &h.TaskType, &h.Command,
            &h.Status, &h.Result, &h.CreatedAt, &h.UpdatedAt); err != nil { return nil, err }
        histories = append(histories, h)
    }
    return histories, nil
}

func DeleteBeaconByUUID(uuid string) error {
    if configType() == "mysql" {
        if _, err := DB.Exec("DELETE FROM task_history WHERE beacon_uuid = ?", uuid); err != nil { return err }
    }
    result, err := DB.Exec("DELETE FROM beacons WHERE uuid = ?", uuid)
    if err != nil { return err }
    rowsAffected, err := result.RowsAffected()
    if err != nil { return err }
    if rowsAffected == 0 { return sql.ErrNoRows }
    return nil
}

// ensureSchemaMySQL: add missing columns for backward compatibility
func ensureSchemaMySQL() error {
    // Prefer direct ALTER with IF NOT EXISTS (MySQL 8.0+), otherwise ignore duplicate errors
    alters := []string{
        "ALTER TABLE beacons ADD COLUMN IF NOT EXISTS sleep_seconds INT NOT NULL DEFAULT 10",
        "ALTER TABLE beacons ADD COLUMN IF NOT EXISTS ipv6 VARCHAR(255) NULL",
        "ALTER TABLE beacons ADD COLUMN IF NOT EXISTS mac VARCHAR(64) NULL",
        "ALTER TABLE beacons ADD COLUMN IF NOT EXISTS os_version VARCHAR(255) NULL",
        "ALTER TABLE beacons ADD COLUMN IF NOT EXISTS remark TEXT NULL",
    }
    for _, stmt := range alters {
        if _, err := DB.Exec(stmt); err != nil {
            // Fallback for older MySQL: retry without IF NOT EXISTS, ignore duplicate column errors
            if strings.Contains(strings.ToLower(err.Error()), "syntax") || strings.Contains(strings.ToLower(err.Error()), "exists") {
                stmtNoIf := strings.ReplaceAll(stmt, " IF NOT EXISTS", "")
                if _, err2 := DB.Exec(stmtNoIf); err2 != nil {
                    if !strings.Contains(strings.ToLower(err2.Error()), "duplicate column name") {
                        log.Printf("schema alter failed: %s -> %v", stmtNoIf, err2)
                        return err2
                    }
                }
            } else if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
                log.Printf("schema alter failed: %s -> %v", stmt, err)
                return err
            }
        }
    }
    // add current_task
    if _, err := DB.Exec("ALTER TABLE beacons ADD COLUMN IF NOT EXISTS current_task VARCHAR(64) NULL"); err != nil {
        if _, err2 := DB.Exec("ALTER TABLE beacons ADD COLUMN current_task VARCHAR(64) NULL"); err2 != nil {
            if !strings.Contains(strings.ToLower(err2.Error()), "duplicate column name") { return err2 }
        }
    }
    return nil
}

func columnExists(table, column string) (bool, error) {
    dbname := config.GlobalConfig.Database.MySQL.DBName
    row := DB.QueryRow("SELECT COUNT(1) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?", dbname, table, column)
    var cnt int
    if err := row.Scan(&cnt); err != nil { return false, err }
    return cnt > 0, nil
}

// 任务队列相关函数
func CreateUploadSession(uuid, filename, targetPath string, totalChunks int, totalSize int64) (int64, error) {
    result, err := DB.Exec(`
        INSERT INTO upload_sessions (uuid, filename, target_path, total_chunks, total_size, status)
        VALUES (?, ?, ?, ?, ?, 'pending')
    `, uuid, filename, targetPath, totalChunks, totalSize)
    if err != nil {
        return 0, err
    }
    return result.LastInsertId()
}

func AddUploadChunk(sessionID int64, chunkIndex int, chunkData string, chunkOffset int64, chunkSize int) error {
    _, err := DB.Exec(`
        INSERT INTO upload_chunks (session_id, chunk_index, chunk_data, chunk_offset, chunk_size, status)
        VALUES (?, ?, ?, ?, ?, 'pending')
    `, sessionID, chunkIndex, chunkData, chunkOffset, chunkSize)
    return err
}

func AddTaskToQueue(uuid string, taskType byte, taskData string, priority int) error {
    _, err := DB.Exec(`
        INSERT INTO task_queue (uuid, task_type, task_data, priority, status)
        VALUES (?, ?, ?, ?, 'pending')
    `, uuid, taskType, taskData, priority)
    return err
}

func GetNextPendingTask(uuid string) (*TaskItem, error) {
    var task TaskItem
    err := DB.QueryRow(`
        SELECT id, uuid, task_type, task_data, priority, status, created_at
        FROM task_queue 
        WHERE uuid = ? AND status = 'pending'
        ORDER BY priority DESC, created_at ASC
        LIMIT 1
    `, uuid).Scan(&task.ID, &task.UUID, &task.TaskType, &task.TaskData, &task.Priority, &task.Status, &task.CreatedAt)
    if err != nil {
        return nil, err
    }
    return &task, nil
}

func MarkTaskAsProcessing(taskID int64) error {
    _, err := DB.Exec(`
        UPDATE task_queue 
        SET status = 'processing', started_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `, taskID)
    return err
}

func MarkTaskAsCompleted(taskID int64) error {
    _, err := DB.Exec(`
        UPDATE task_queue 
        SET status = 'completed', completed_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `, taskID)
    return err
}

func MarkTaskAsFailed(taskID int64, retryCount int) error {
    _, err := DB.Exec(`
        UPDATE task_queue 
        SET status = 'failed', retry_count = ?
        WHERE id = ?
    `, retryCount, taskID)
    return err
}

func GetNextPendingChunk(sessionID int64) (*ChunkItem, error) {
    var chunk ChunkItem
    err := DB.QueryRow(`
        SELECT id, session_id, chunk_index, chunk_data, chunk_offset, chunk_size, status
        FROM upload_chunks 
        WHERE session_id = ? AND status = 'pending'
        ORDER BY chunk_index ASC
        LIMIT 1
    `, sessionID).Scan(&chunk.ID, &chunk.SessionID, &chunk.ChunkIndex, &chunk.ChunkData, &chunk.ChunkOffset, &chunk.ChunkSize, &chunk.Status)
    if err != nil {
        return nil, err
    }
    return &chunk, nil
}

func GetChunkByIndex(sessionID int64, chunkIndex int) (*ChunkItem, error) {
    var chunk ChunkItem
    err := DB.QueryRow(`
        SELECT id, session_id, chunk_index, chunk_data, chunk_offset, chunk_size, status
        FROM upload_chunks 
        WHERE session_id = ? AND chunk_index = ?
    `, sessionID, chunkIndex).Scan(&chunk.ID, &chunk.SessionID, &chunk.ChunkIndex, &chunk.ChunkData, &chunk.ChunkOffset, &chunk.ChunkSize, &chunk.Status)
    if err != nil {
        return nil, err
    }
    return &chunk, nil
}

func MarkChunkAsSent(chunkID int64) error {
    _, err := DB.Exec(`
        UPDATE upload_chunks 
        SET status = 'sent', sent_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `, chunkID)
    return err
}

func MarkChunkAsAcknowledged(chunkID int64) error {
    _, err := DB.Exec(`
        UPDATE upload_chunks 
        SET status = 'acknowledged', acknowledged_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `, chunkID)
    return err
}

func UpdateUploadSessionStatus(sessionID int64, status string) error {
    _, err := DB.Exec("UPDATE upload_sessions SET status = ?, updated_at = NOW() WHERE id = ?", status, sessionID)
    return err
}

// 数据模型
type TaskItem struct {
    ID        int64
    UUID      string
    TaskType  byte
    TaskData  string
    Priority  int
    Status    string
    CreatedAt time.Time
}

type ChunkItem struct {
    ID          int64
    SessionID   int64
    ChunkIndex  int
    ChunkData   string
    ChunkOffset int64
    ChunkSize   int
    Status      string
}

// 下载会话相关函数
func CreateDownloadSession(uuid, filename, filePath string) (int64, error) {
    result, err := DB.Exec(`
        INSERT INTO download_sessions (uuid, filename, file_path, status, created_at, updated_at)
        VALUES (?, ?, ?, 'pending', NOW(), NOW())`,
        uuid, filename, filePath)
    if err != nil {
        return 0, err
    }
    return result.LastInsertId()
}

func AddDownloadChunk(sessionID int64, chunkIndex int, chunkData string, chunkOffset int64, chunkSize int) error {
    _, err := DB.Exec(`
        INSERT INTO download_chunks (session_id, chunk_index, chunk_data, chunk_offset, chunk_size, status, created_at)
        VALUES (?, ?, ?, ?, ?, 'pending', NOW())`,
        sessionID, chunkIndex, chunkData, chunkOffset, chunkSize)
    return err
}

func GetDownloadChunk(sessionID int64, chunkIndex int) (*DownloadChunk, error) {
    chunk := &DownloadChunk{}
    err := DB.QueryRow(`
        SELECT id, session_id, chunk_index, chunk_data, chunk_offset, chunk_size, status, created_at
        FROM download_chunks 
        WHERE session_id = ? AND chunk_index = ?`,
        sessionID, chunkIndex).Scan(
        &chunk.ID, &chunk.SessionID, &chunk.ChunkIndex, &chunk.ChunkData,
        &chunk.ChunkOffset, &chunk.ChunkSize, &chunk.Status, &chunk.CreatedAt)
    if err != nil {
        return nil, err
    }
    return chunk, nil
}

func GetDownloadProgress(sessionID int64) (*DownloadProgress, error) {
    progress := &DownloadProgress{}
    
    // 获取会话信息
    err := DB.QueryRow(`
        SELECT total_chunks, status FROM download_sessions WHERE id = ?`, sessionID).
        Scan(&progress.TotalChunks, &progress.Status)
    if err != nil {
        return nil, err
    }
    
    // 获取已完成的块数
    err = DB.QueryRow(`
        SELECT COUNT(*) FROM download_chunks 
        WHERE session_id = ? AND status = 'completed'`, sessionID).
        Scan(&progress.CompletedChunks)
    if err != nil {
        return nil, err
    }
    
    progress.SessionID = sessionID
    progress.Progress = float64(progress.CompletedChunks) / float64(progress.TotalChunks) * 100
    
    return progress, nil
}

func CompleteDownloadSession(sessionID int64) error {
    _, err := DB.Exec(`
        UPDATE download_sessions 
        SET status = 'completed', updated_at = NOW() 
        WHERE id = ?`, sessionID)
    return err
}

func MarkDownloadChunkAsCompleted(chunkID int64) error {
    _, err := DB.Exec(`
        UPDATE download_chunks 
        SET status = 'completed', completed_at = NOW() 
        WHERE id = ?`, chunkID)
    return err
}

// 获取beacon的所有下载会话
func GetDownloadSessionsByUUID(uuid string) ([]*DownloadSession, error) {
    rows, err := DB.Query(`
        SELECT id, uuid, filename, file_path, total_chunks, status, created_at, updated_at
        FROM download_sessions 
        WHERE uuid = ?
        ORDER BY created_at DESC`, uuid)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var sessions []*DownloadSession
    for rows.Next() {
        session := &DownloadSession{}
        err := rows.Scan(&session.ID, &session.UUID, &session.Filename, &session.FilePath,
            &session.TotalChunks, &session.Status, &session.CreatedAt, &session.UpdatedAt)
        if err != nil {
            return nil, err
        }
        sessions = append(sessions, session)
    }
    return sessions, nil
}

// 更新下载会话的分块数量
func UpdateDownloadSessionChunks(sessionID int64, totalChunks int) error {
    _, err := DB.Exec(`
        UPDATE download_sessions 
        SET total_chunks = ?, updated_at = NOW() 
        WHERE id = ?`, totalChunks, sessionID)
    return err
}


