//go:build darwin && cgo

package resources

/*
#include <libproc.h>
#include <sys/resource.h>
#include <unistd.h>

static int taskpicker_usage(struct rusage_info_v2 *info) {
	return proc_pid_rusage(getpid(), RUSAGE_INFO_V2, (rusage_info_t *)info);
}
*/
import "C"

import (
	"fmt"
	"time"
)

func readProcessSample() (processSample, error) {
	var info C.struct_rusage_info_v2
	result, err := C.taskpicker_usage(&info)
	if result != 0 {
		return processSample{}, fmt.Errorf("读取进程资源占用失败：%w", err)
	}
	return processSample{
		at:          time.Now(),
		cpuTime:     time.Duration(info.ri_user_time + info.ri_system_time),
		memoryBytes: uint64(info.ri_resident_size),
	}, nil
}
