// Package utils 提供通用的工具函数
// 包含时间戳生成、签名计算、重试机制等辅助功能
package utils

import (
	"crypto/hmac"    // HMAC 哈希算法
	"crypto/sha256" // SHA256 哈希算法
	"encoding/hex"  // 十六进制编码
	"fmt"          // 格式化输出
	"time"         // 时间处理
)

// GenerateTimestamp 生成当前时间的 Unix 时间戳字符串
// 返回:
//   string - 以秒为单位的时间戳字符串
func GenerateTimestamp() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}

// GenerateSignature 使用 HMAC-SHA256 算法生成签名
// 参数:
//   secret - 签名密钥
//   data - 待签名的数据
// 返回:
//   string - 十六进制编码的签名结果
func GenerateSignature(secret, data string) string {
	// 创建 HMAC-SHA256 哈希器
	h := hmac.New(sha256.New, []byte(secret))
	// 写入待签名数据
	h.Write([]byte(data))
	// 返回十六进制编码的签名结果
	return hex.EncodeToString(h.Sum(nil))
}

// Retry 执行带有指数退避的重试机制
// 参数:
//   attempts - 最大重试次数
//   sleep - 初始等待时间，每次重试会翻倍
//   fn - 要执行的函数，返回 error 表示需要重试
// 返回:
//   error - 最后一次执行失败的错误，如果成功则返回 nil
func Retry(attempts int, sleep time.Duration, fn func() error) error {
	var err error
	// 循环尝试最多 attempts 次
	for i := 0; i < attempts; i++ {
		// 执行函数
		if err = fn(); err == nil {
			// 成功，直接返回
			return nil
		}
		// 失败，等待一段时间后重试
		time.Sleep(sleep)
		// 指数退避：下次等待时间翻倍
		sleep *= 2
	}
	// 所有重试都失败，返回最后一次的错误
	return err
}
