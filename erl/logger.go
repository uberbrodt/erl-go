package erl

import (
	"log"
	"os"
)

type ILogger interface {
	Println(v ...any)
	Printf(format string, v ...any)
}

var Logger ILogger = log.New(os.Stdout, "erl-go", log.Ldate|log.Ltime)

var DebugLog = false

func debugPrintln(v ...any) {
	if DebugLog {
		Logger.Println(v...)
	}
}

func debugPrintf(format string, v ...any) {
	if DebugLog {
		Logger.Printf(format, v...)
	}
}
