package erl

import (
	"log"
	"os"
	"sync"
)

type ILogger interface {
	Println(v ...any)
	Printf(format string, v ...any)
}

var Logger ILogger = log.New(os.Stdout, "erl-go", log.Ldate|log.Ltime|log.Lmicroseconds)

var debugLog = false

var debugLogMutex sync.RWMutex

func DebugLogEnabled() bool {
	defer debugLogMutex.RUnlock()
	debugLogMutex.RLock()
	return debugLog
}

func SetDebugLog(v bool) {
	defer debugLogMutex.Unlock()
	debugLogMutex.Lock()

	debugLog = v
}

func DebugPrintln(v ...any) {
	if DebugLogEnabled() {
		Logger.Println(v...)
	}
}

func DebugPrintf(format string, v ...any) {
	if DebugLogEnabled() {
		Logger.Printf(format, v...)
	}
}
