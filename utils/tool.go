package utils

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

var lockPath = "/root/cni_lock_dir"
var logPath = "/root/test-cni.log"

func WriteLog(log ...string) {
	file, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		file, _ = os.Create(logPath)
	}
	defer file.Close()
	write := bufio.NewWriter(file)
	logRes := ""
	for _, c := range log {
		logRes += c
		logRes += " "
	}
	_, _ = write.WriteString(time.Now().Format("2006-01-02 15:04:05") + ": " + logRes + "\r\n")
	write.Flush()
}

func CreateDir(dirName string) error {
	err := os.MkdirAll(dirName, 0766)
	if err != nil {
		return err
	}
	return nil
}

func CreateFile(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	if data != nil {
		var n int
		n, err = f.Write(data)
		if err == nil && n < len(data) {
			return io.ErrShortWrite
		}
	}
	return nil
}

func FileIsExisted(filename string) bool {
	existed := true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		existed = false
	}
	return existed
}

func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func DeleteFile(path string) error {
	if path == "" {
		return nil
	}
	if FileIsExisted(path) {
		err := os.Remove(path)
		if err != nil {
			return err
		}
	}
	return nil
}

func StringsIn(strs []string, str string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
}

func CopyFile(src, dst string) error {
	processInfo := exec.Command(
		"/bin/sh", "-c",
		fmt.Sprintf("cp %s %s", src, dst),
	)
	_, err := processInfo.Output()
	return err
}

func AcquireLock() (bool, error) {
	err := os.Mkdir(lockPath, 0766)
	if err != nil {
		var e *os.PathError
		errors.As(err, &e)
		if e.Err.Error() == "file exists" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func ReleaseLock() {
	err := os.RemoveAll(lockPath)
	if err != nil {
		WriteLog("release lock error:", err.Error())
	}
}
