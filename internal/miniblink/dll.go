package miniblink

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Wangbull/blink/internal/log"
	"golang.org/x/sys/windows"
)

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// LoadDLL: 优先尝试系统加载，其次使用用户 LOCALAPPDATA 目录释放并加载 DLL
func LoadDLL(dllFile, tempPath string) (*windows.DLL, error) {
	// 先尝试从系统搜索路径直接加载
	if loaded, err := windows.LoadDLL(dllFile); err == nil {
		log.Debug("直接加载DLL: %s", dllFile)
		return loaded, nil
	}

	// 使用 %LOCALAPPDATA%\miniblink\miniblink_<VERSION>_<ARCH>
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		// 回退到传入的 tempPath（避免空字符串）
		localAppData = tempPath
	}
	baseDir := filepath.Join(localAppData, "miniblink")
	dir := filepath.Join(baseDir, fmt.Sprintf("miniblink_%s_%s", VERSION, ARCH))
	releaseFile := filepath.Join(dir, dllFile)

	// 如果已存在且可加载，则直接加载
	if _, err := os.Stat(releaseFile); err == nil {
		if loaded, err := windows.LoadDLL(releaseFile); err == nil {
			log.Debug("直接加载已存在DLL: %s", releaseFile)
			return loaded, nil
		}
		// 存在但加载失败，继续尝试覆盖（可能损坏）
	}

	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}

	// 释放嵌入资源到 tmp，然后原子替换（只有内容不同才替换）
	tmpFile := releaseFile + ".tmp"
	if err := releaseEmbedDLLAtomic(tmpFile, releaseFile); err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("写入 %s 失败：权限不足: %w", dir, err)
		}
		return nil, err
	}

	// 加载并返回错误供上层处理
	loaded, err := windows.LoadDLL(releaseFile)
	if err != nil {
		return nil, fmt.Errorf("加载DLL失败: %w", err)
	}
	log.Debug("已释放并加载DLL: %s", releaseFile)
	return loaded, nil
}

func releaseEmbedDLLAtomic(tmpFile, finalFile string) error {
	r, err := res.Open(fmt.Sprintf("release/%s/miniblink_%s_%s.dll", ARCH, VERSION, ARCH))
	if err != nil {
		return errors.New("无法从内嵌资源读取 DLL: " + err.Error())
	}
	defer r.Close()

	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return errors.New("无法创建临时文件: " + err.Error())
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return errors.New("写入临时文件失败: " + err.Error())
	}
	out.Close()

	// 如果 final 存在且哈希相同，则删除 tmp 并返回
	if _, err := os.Stat(finalFile); err == nil {
		h1, err1 := fileSHA256(tmpFile)
		h2, err2 := fileSHA256(finalFile)
		if err1 == nil && err2 == nil && h1 == h2 {
			_ = os.Remove(tmpFile)
			return nil
		}
	}

	// 原子重命名，失败则用复制回退
	if err := os.Rename(tmpFile, finalFile); err != nil {
		in, err2 := os.Open(tmpFile)
		if err2 != nil {
			return fmt.Errorf("替换文件失败且打开 tmp 失败: %w", err2)
		}
		defer in.Close()
		out2, err3 := os.OpenFile(finalFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err3 != nil {
			return fmt.Errorf("替换文件失败且创建 final 失败: %w", err3)
		}
		if _, err4 := io.Copy(out2, in); err4 != nil {
			out2.Close()
			return fmt.Errorf("替换文件复制失败: %w", err4)
		}
		out2.Close()
		_ = os.Remove(tmpFile)
	}
	return nil
}
