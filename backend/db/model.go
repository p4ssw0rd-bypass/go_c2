package db

import (
    "encoding/json"
    "time"
)

type Beacon struct {
    ID          int64     `json:"id"`
    IP          string    `json:"ip"`
    IPv6        string    `json:"ipv6"`
    MAC         string    `json:"mac"`
    HostName    string    `json:"hostname"`
    UserName    string    `json:"username"`
    ProcessName string    `json:"process_name"`
    ProcessPath string    `json:"process_path"`
    ProcessID   int       `json:"process_id"`
    Arch        string    `json:"arch"`
    OSVersion   string    `json:"os_version"`
    OSUUID      string    `json:"os_uuid"`
    UUID        string    `json:"uuid"`
    FirstTime   time.Time `json:"first_time,omitempty"`
    LastSeen    time.Time `json:"last_seen,omitempty"`
    Job         string    `json:"job"`
    JobResult   string    `json:"job_result"`
    SleepSeconds int      `json:"sleep_seconds"`
    Remark      string    `json:"remark"`
    CurrentTask string    `json:"current_task"`
}

func (b *Beacon) MarshalJSON() ([]byte, error) {
    type Alias Beacon
    return json.Marshal(&struct {
        *Alias
        ProcessPath string `json:"process_path"`
    }{Alias: (*Alias)(b), ProcessPath: decodeUTF8Path(b.ProcessPath)})
}

func decodeUTF8Path(path string) string { return string([]rune(path)) }

type BeaconRegisterInfo struct {
    HostName    string `json:"hostname"`
    UserName    string `json:"username"`
    ProcessName string `json:"process_name"`
    ProcessPath string `json:"process_path"`
    ProcessID   int    `json:"process_id"`
    Arch        string `json:"arch"`
    OSUUID      string `json:"os_uuid"`
}

type TaskHistory struct {
    ID         int64     `json:"id"`
    BeaconUUID string    `json:"beacon_uuid"`
    TaskType   string    `json:"task_type"`
    Command    string    `json:"command"`
    Status     string    `json:"status"`
    Result     string    `json:"result"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}

type AdminUser struct {
    ID       int64  `json:"id"`
    Username string `json:"username"`
    Password string `json:"password"`
}

// 下载会话相关模型
type DownloadSession struct {
    ID          int64     `json:"id"`
    UUID        string    `json:"uuid"`
    Filename    string    `json:"filename"`
    FilePath    string    `json:"file_path"`
    TotalChunks int       `json:"total_chunks"`
    Status      string    `json:"status"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type DownloadChunk struct {
    ID          int64     `json:"id"`
    SessionID   int64     `json:"session_id"`
    ChunkIndex  int       `json:"chunk_index"`
    ChunkData   string    `json:"chunk_data"`
    ChunkOffset int64     `json:"chunk_offset"`
    ChunkSize   int       `json:"chunk_size"`
    Status      string    `json:"status"`
    CreatedAt   time.Time `json:"created_at"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type DownloadProgress struct {
    SessionID        int64   `json:"session_id"`
    TotalChunks      int     `json:"total_chunks"`
    CompletedChunks  int     `json:"completed_chunks"`
    Progress         float64 `json:"progress"`
    Status           string  `json:"status"`
}


