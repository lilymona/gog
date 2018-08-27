package logging

import (
	"flag"
	"fmt"
	"log"
	"os"
)

const (
	verboseError = iota
	verboseWarning
	verboseInfo
	verboseDebug
)

var verbose int

func init() {
	flag.IntVar(&verbose, "v", verboseDebug, "The log veboseness")
}

func Errorf(format string, args ...interface{}) {
	if verbose < verboseError {
		return
	}
	Printf("ERROR", format, args...)
}

func Fatalf(format string, args ...interface{}) {
	if verbose < verboseError {
		return
	}
	Printf("FATAL", format, args...)
	os.Exit(1)
}

func Warningf(format string, args ...interface{}) {
	if verbose < verboseWarning {
		return
	}
	Printf("WARNING", format, args...)
}

func Infof(format string, args ...interface{}) {
	if verbose < verboseInfo {
		return
	}
	Printf("INFO", format, args...)
}

func Debugf(format string, args ...interface{}) {
	if verbose < verboseDebug {
		return
	}
	Printf("DEBUG", format, args...)
}

func Printf(level string, format string, args ...interface{}) {
	log.Printf("[%s]: %s", level, fmt.Sprintf(format, args...))
}
