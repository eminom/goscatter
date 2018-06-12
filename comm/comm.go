package comm

import (
	"os"
)

func IsFileExists(inpath string) bool {
	stat, err := os.Stat(inpath)
	return nil == err && !stat.IsDir()
}
