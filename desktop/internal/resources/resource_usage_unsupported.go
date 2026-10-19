//go:build !darwin || !cgo

package resources

import (
	"errors"
)

func readProcessSample() (processSample, error) {
	return processSample{}, errors.New("当前平台不支持进程资源统计")
}
