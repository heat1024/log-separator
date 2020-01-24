package main

import (
	"os"

	logrus_stack "github.com/Gurpartap/logrus-stack"
	logsep "github.com/heat1024/log-separator/log-separator"
	"github.com/sirupsen/logrus"
)

var confFile string

const (
	defaultConfigPath = "./config.toml"
	location          = "Asia/Tokyo"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	stackLevels := []logrus.Level{logrus.PanicLevel, logrus.FatalLevel}
	logrus.AddHook(logrus_stack.NewHook(stackLevels, stackLevels))

	confFile = os.Getenv("LOGSEP_CONFIG_PATH")
	if len(confFile) == 0 {
		confFile = defaultConfigPath
	}
}

func main() {
	logsep.Start(confFile)
}
