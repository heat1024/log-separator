package logsep

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

func (ls *logsep) signalReceiver(confFile string, waitReload chan struct{}, done *bool) {
	sg := make(chan os.Signal)
	signal.Notify(sg, syscall.SIGHUP, syscall.SIGTERM)

L:
	for {
		switch <-sg {
		case syscall.SIGHUP:
			logrus.Info("Reload LSWS Log Separator service")
			reconfig, err := loadConfig(confFile)
			if err != nil {
				logrus.Errorf("error on read config file [%s]: %s", confFile, err.Error())
				logrus.Debug("use previous configuration")

				continue
			}

			logrus.Debug("config reload complete")

			if err := ls.Close(); err != nil {
				logrus.Errorf("error on stop tailing input log file [%s]: %s", ls.config.InputLogPath, err.Error())
			}
			logrus.Debugf("[%s] tailing stopped", ls.config.InputLogPath)

			// init log separator with new configuration
			err = initLogSep(ls, reconfig)
			if err != nil {
				logrus.Errorf("error on restart with new configuration [%s]: %s", reconfig.InputLogPath, err.Error())
				logrus.Debug("Stop Log Separator service")

				*done = true

				ls.Close()
				waitReload <- struct{}{}

				break L
			}
			logrus.Debug("log tailer started")

			waitReload <- struct{}{}

			logrus.Debug("chan send")
		case syscall.SIGTERM:
			logrus.Info("Stop Log Separator service")
			*done = true

			ls.Close()

			waitReload <- struct{}{}

			break L
		}
	}
}
