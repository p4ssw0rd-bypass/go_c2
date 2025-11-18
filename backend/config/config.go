package config

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
)

type Config struct {
    DBPath        string `json:"db_path"`
    AdminUser     string `json:"admin_user"`
    AdminPass     string `json:"admin_pass"`
    ClientToken   string `json:"client_token"`
    JWTSecret     string `json:"jwt_secret"`
    WebhookURL    string `json:"webhook_url"`
    WebhookKey    string `json:"webhook_key"`
    WebhookEnable bool   `json:"webhook_enable"`

    APIEndpoint   string `json:"api_endpoint"`

    Database        DatabaseConfig        `json:"database"`
    Server          ServerConfig          `json:"server"`
    Routes          RoutesConfig          `json:"routes"`
    Encoding        EncodingConfig        `json:"encoding"`
    TrafficDisguise TrafficDisguiseConfig `json:"traffic_disguise"`
}

type DatabaseConfig struct {
    Type string `json:"type"`
    MySQL MySQLConfig `json:"mysql"`
    MaxOpenConns    int `json:"max_open_conns"`
    MaxIdleConns    int `json:"max_idle_conns"`
    ConnMaxLifetime int `json:"conn_max_lifetime"`
}

type MySQLConfig struct {
    Host     string `json:"host"`
    Port     int    `json:"port"`
    User     string `json:"user"`
    Password string `json:"password"`
    DBName   string `json:"dbname"`
    Params   string `json:"params"`
}

type ServerConfig struct {
    Host         string `json:"host"`
    Port         int    `json:"port"`
    ReadTimeout  int    `json:"read_timeout"`
    WriteTimeout int    `json:"write_timeout"`
    MaxBodySize  int    `json:"max_body_size"`
    EnableHTTPS  bool   `json:"enable_https"`
    HTTPSPort    int    `json:"https_port"`
    CertFile     string `json:"cert_file"`
    KeyFile      string `json:"key_file"`
    HostHeader   string `json:"host_header"`
    Listeners    []ListenerConfig `json:"listeners"`
}

// ListenerConfig 额外监听配置（可多条）
type ListenerConfig struct {
    Host       string `json:"host"`
    Port       int    `json:"port"`
    HostHeader string `json:"host_header"`
    Type       string `json:"type"` // admin | beacon | both
}

type RoutesConfig struct {
    AdminPrefix    string `json:"admin_prefix"`
    APIPrefix      string `json:"api_prefix"`
    BeaconEndpoint string `json:"beacon_endpoint"`
    RegisterPath   string `json:"register_path"`
    LoginPath      string `json:"login_path"`
}

type EncodingConfig struct {
    CustomBase64Table string `json:"custom_base64_table"`
    UseCustomBase64   bool   `json:"use_custom_base64"`
}

type TrafficDisguiseConfig struct {
    Enable bool   `json:"enable"`
    Prefix string `json:"prefix"`
    Suffix string `json:"suffix"`
}

var GlobalConfig = &Config{}

func Init() error {
    if err := loadConfigFromFile("config.json"); err != nil {
        log.Printf("Warning: Failed to load config.json, using defaults: %v", err)
        if err := saveConfigToFile("config.json"); err != nil {
            log.Printf("Warning: Failed to create default config.json: %v", err)
        }
    }
    if GlobalConfig.APIEndpoint != "" {
        GlobalConfig.Routes.BeaconEndpoint = GlobalConfig.APIEndpoint
    }
    return nil
}

func loadConfigFromFile(filename string) error {
    // 尝试多个可能的路径
    paths := []string{
        filename,                    // 当前工作目录
        "./" + filename,            // 当前工作目录
        "../" + filename,           // 上级目录
        "../../" + filename,        // 上上级目录
        "./config/" + filename,     // config子目录
        "../config/" + filename,    // 上级config目录
    }
    
    var data []byte
    var err error
    
    for _, path := range paths {
        data, err = ioutil.ReadFile(path)
        if err == nil {
            log.Printf("成功加载配置文件: %s", path)
            break
        }
    }
    
    if err != nil {
        return fmt.Errorf("无法找到配置文件 %s，尝试过的路径: %v", filename, paths)
    }
    
    return json.Unmarshal(data, GlobalConfig)
}

func saveConfigToFile(filename string) error {
    data, err := json.MarshalIndent(GlobalConfig, "", "  ")
    if err != nil { return err }
    return ioutil.WriteFile(filename, data, 0644)
}

func SaveConfig() error { return saveConfigToFile("config.json") }

func GetServerAddress() string {
    if len(GlobalConfig.Server.Listeners) > 0 {
        l := GlobalConfig.Server.Listeners[0]
        return fmt.Sprintf("%s:%d", l.Host, l.Port)
    }
    return fmt.Sprintf("%s:%d", GlobalConfig.Server.Host, GlobalConfig.Server.Port)
}
func GetHTTPSServerAddress() string { return fmt.Sprintf("%s:%d", GlobalConfig.Server.Host, GlobalConfig.Server.HTTPSPort) }
func IsHTTPSEnabled() bool { return GlobalConfig.Server.EnableHTTPS }


