package comm

import (
	"os"
)

var (
	exitCode = 127 // Error for default.
)

func IsFileExists(inpath string) bool {
	stat, err := os.Stat(inpath)
	return nil == err && !stat.IsDir()
}

func QuitProgram() {
	os.Exit(exitCode)
}

func SetExitCode(ec int) {
	exitCode = ec
}
