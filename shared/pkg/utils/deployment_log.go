package utils

import (
	"errors"
	"os"
	"sync"
	"syscall"
)

const DeploymentLogMaxBytes = 10 * 1024 * 1024
const DeploymentLogTruncatedMessage = "Log truncated: size limit reached."

var deploymentLogMu sync.Mutex

func WriteDeploymentLog(file *os.File, message string) (truncated bool, err error) {
	deploymentLogMu.Lock()
	defer deploymentLogMu.Unlock()
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return false, err
	}
	defer func() {
		err = errors.Join(err, syscall.Flock(int(file.Fd()), syscall.LOCK_UN))
	}()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() == DeploymentLogMaxBytes {
		return true, nil
	}
	marker := "\n" + DeploymentLogTruncatedMessage + "\n"
	remaining := int64(DeploymentLogMaxBytes-len(marker)) - info.Size()
	if remaining < 0 {
		if err := file.Truncate(int64(DeploymentLogMaxBytes - len(marker))); err != nil {
			return false, err
		}
		remaining = 0
	}
	if int64(len(message)) <= remaining {
		_, err = file.WriteString(message)
		return false, err
	}
	_, err = file.WriteString(message[:remaining] + marker)
	return true, err
}

type deploymentLogWriter struct {
	file *os.File
}

func (writer deploymentLogWriter) Write(data []byte) (int, error) {
	if _, err := WriteDeploymentLog(writer.file, string(data)); err != nil {
		return 0, err
	}
	return len(data), nil
}
