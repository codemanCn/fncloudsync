package obs

import (
	"log"
	"os"
)

func NewLogger() *log.Logger {
	return log.New(os.Stdout, "fn-cloudsync ", log.LstdFlags|log.Lmicroseconds)
}
