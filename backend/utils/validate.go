package utils

import (
    "log"
    "github.com/google/uuid"
    "GO_C2/db"
    "GO_C2/config"
)

func ValidateBeaconRegisterInfo(info *db.BeaconRegisterInfo) bool {
    log.Printf("验证注册信息:")
    log.Printf("- HostName: %s", info.HostName)
    log.Printf("- UserName: %s", info.UserName)
    log.Printf("- ProcessName: %s", info.ProcessName)
    log.Printf("- ProcessPath: %s", info.ProcessPath)
    log.Printf("- ProcessID: %d", info.ProcessID)
    log.Printf("- Arch: %s", info.Arch)
    log.Printf("- OSUUID: %s", info.OSUUID)
    if info.HostName == "" || info.UserName == "" || info.ProcessName == "" || info.ProcessPath == "" || info.ProcessID <= 0 || info.Arch == "" || info.OSUUID == "" { return false }
    return true
}

func ValidateUUID(id string) bool { _, err := uuid.Parse(id); return err == nil }

func ValidateClientToken(token string) bool {
    expected := ""
    if config.GlobalConfig != nil { expected = config.GlobalConfig.ClientToken }
    valid := token == expected
    log.Printf("验证Token 长度[%d] 结果: %v", len(token), valid)
    return valid
}


