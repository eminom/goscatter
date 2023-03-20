package comm

import (
	"os"
)

type ExitCode int

const (
	ExitCode_Success      ExitCode = 0
	ExitCode_GeneralError          = 1
	ExitCode_ReqTimeout            = 2
	ExitCode_NoSuchFile            = 3
)

var (
	exitCode ExitCode = ExitCode_GeneralError
)

func IsFileExists(inpath string) bool {
	stat, err := os.Stat(inpath)
	return nil == err && !stat.IsDir()
}

func GetFileSize(inpath string) int64 {
	stat, err := os.Stat(inpath)
	if err == nil {
		return stat.Size()
	}
	return 0
}

func QuitProgram() {
	os.Exit(int(exitCode))
}

func SetExitCode(ec ExitCode) {
	exitCode = ec
}
