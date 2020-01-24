package logsep

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/hpcloud/tail"
	"github.com/sirupsen/logrus"
)

type logsep struct {
	config        *config
	posFile       *os.File
	posReader     *bufio.Reader
	posWriter     *bufio.Writer
	currentPos    int64
	latestFile    int64
	realFileInode uint64
	tail          *tail.Tail
}

func initLogSep(ls *logsep, conf *config) error {
	// open position file with read, write and append mode and set owner. if file not exist, create it.
	pf, err := os.OpenFile(conf.PosFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		logrus.Errorf("cannot open position file [%s]: %s", conf.PosFile, err.Error())
	}

	pr := bufio.NewReader(pf)
	pw := bufio.NewWriter(pf)

	buff, err := ioutil.ReadAll(pr)
	if err != nil {
		logrus.Errorf("cannot read position file [%s]: %s", conf.PosFile, err.Error())
	}

	position := strings.Split(string(buff), "\t")

	// if position file was empty or not correct, initialize for keep going
	if len(buff) == 0 || len(position) != 3 {
		logrus.Debugf("position file [%s] is not correct. recreate pos file", conf.PosFile)

		buff = []byte(fmt.Sprintf("%s\t0\t0", conf.InputLogPath))
		position = strings.Split(string(buff), "\t")
	}

	lf, err := strconv.ParseInt(position[2], 16, 64)
	if err != nil {
		logrus.Errorf("current position file template is not correct. use current file: %s", err.Error())
		lf = 0
	}

	// get inode from current log file
	finode, err := getInodeFromFile(conf.InputLogPath)
	if err != nil {
		logrus.Errorf("failed to get inode from real log file [%s]: %s", conf.InputLogPath, err.Error())
	}

	// if inode matched, get latest position from position file. if not, set position to 0
	cr := int64(0)
	if uint64(lf) == finode {
		if cr, err = strconv.ParseInt(position[1], 16, 64); err != nil {
			logrus.Errorf("position data is not correct. set position to 0: %s", err.Error())
			cr = 0
		}
	}

	// start tailing logsep log file
	t, err := tail.TailFile(conf.InputLogPath, tail.Config{Follow: true, ReOpen: true, Location: &tail.SeekInfo{Offset: ls.currentPos, Whence: 0}})
	if err != nil {
		// if start tailing failed, abort program
		return fmt.Errorf("error on tailing input logs. stop log separator: %s", err.Error())
	}

	// when first time ls setted
	if ls == nil {
		ls = &logsep{
			config:        conf,
			posFile:       pf,
			posReader:     pr,
			posWriter:     pw,
			currentPos:    cr,
			latestFile:    lf,
			realFileInode: finode,
			tail:          t,
		}
	}

	return nil
}

func getInodeFromFile(path string) (uint64, error) {
	// checking inode is same in position file and real log file
	posStat, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("failed to get inode from real log file [%s]: %s", path, err.Error())
	}

	return posStat.Sys().(*syscall.Stat_t).Ino, nil
}

// Start function is start log-separator service
func Start(confFile string) error {
	var ls *logsep
	waitReload := make(chan struct{})
	done := false
	lastError := error(nil)

	logrus.Info("Start LSWS Log Separator service")

	c, err := loadConfig(confFile)
	if err != nil {
		return err
	}

	err = initLogSep(ls, c)
	if err != nil {
		return err
	}

	defer ls.Close()

	// strat os signal handler
	go ls.signalReceiver(confFile, waitReload, &done)

	for {
		for line := range ls.tail.Lines {
			if line.Err != nil {
				logrus.Errorf("error on read new line: %s", line.Err.Error())

				break
			}

			// update position file when read new line from input log
			ls.updatePositionFile()

			logStruct := strings.SplitN(line.Text, ":", 2)
			if len(logStruct) != 2 {
				logrus.Error("log not match with log separator format.")

				continue
			}

			logName := strings.TrimSpace(logStruct[0])
			logDir := string("")
			logFileName := string("")
			logValue := strings.TrimSpace(logStruct[1]) + "\n"

			// findout separated log pa
			for _, output := range c.OutputLog {
				if logName == output.Name {
					logDir = output.Path
					logFileName = output.File
					if len(logFileName) == 0 {
						logFileName = logName
					}
					break
				}
			}

			if len(logDir) > 0 && len(logFileName) > 0 {
				logPath := fmt.Sprintf("%s/%s", logDir, logFileName)

				var dir os.FileInfo
				var logFile *os.File

				// check log directory is exist
				if dir, err = os.Stat(logDir); err != nil {
					// make data directory recursive
					if err := os.MkdirAll(logDir, 0750); err != nil {
						logrus.Errorf("cannot make log directory [%s]: %s", logDir, err.Error())
					}
					if err := os.Chmod(logDir, 0750); err != nil {
						logrus.Errorf("cannot set mod 0750 to [%s]: %s", logDir, err.Error())
					}
				} else if !dir.IsDir() {
					logrus.Errorf("[%s] is exist but it is not a directory", logDir)
				}

				// open logFile with write and append mode and set owner. if file not exist, create it.
				if logFile, err = os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644); err != nil {
					logrus.Errorf("cannot open log file [%s]: %s", logPath, err.Error())
				}

				_, err = logFile.WriteString(logValue)
				if err != nil {
					logrus.Errorf("cannot write log to [%s]: %s", logPath, err.Error())
				}

				logFile.Close()
			}
		}

		<-waitReload

		if done {
			logrus.Info("done")
			break
		} else {
			logrus.Info("reload")
		}
	}

	return lastError
}

func (ls *logsep) updatePositionFile() {
	if curPos, err := ls.tail.Tell(); err != nil {
		logrus.Errorf("cannot get current position from [%s]: %s", ls.config.InputLogPath, err.Error())
	} else {
		if inode, err := getInodeFromFile(ls.config.InputLogPath); err != nil {
			logrus.Errorf("failed to get inode from real log file [%s]: %s", ls.config.InputLogPath, err.Error())
		} else {
			ls.realFileInode = inode
		}
		posValue := fmt.Sprintf("%s\t%016x\t%016x", ls.config.InputLogPath, curPos, ls.realFileInode)

		// rewind position file's fd pointer
		if newOffset, err := ls.posFile.Seek(0, 0); newOffset != 0 || err != nil {
			logrus.Errorf("position file fd rewind failed: %s", err.Error())
		}

		// truncate position file to size 0
		if err := ls.posFile.Truncate(0); err != nil {
			logrus.Errorf("got error on truncate position file [%s]: %s", ls.config.PosFile, err.Error())
		}

		// write current position data to position file
		if _, err := ls.posWriter.WriteString(posValue); err != nil {
			logrus.Errorf("got error on write position file [%s]: %s", ls.config.PosFile, err.Error())
		}
		if err := ls.posWriter.Flush(); err != nil {
			logrus.Errorf("got error on write flush position file [%s]: %s", ls.config.PosFile, err.Error())
		}
	}
}

func (ls *logsep) Close() error {
	logrus.Debug("try to stop tailer")
	if ls.tail != nil {
		if err := ls.tail.Stop(); err != nil {
			return err
		}
	}
	logrus.Debug("tailer stopped")

	logrus.Debugf("try to close [%s]]", ls.config.PosFile)
	if ls.posFile != nil {
		if err := ls.posFile.Close(); err != nil {
			return err
		}
	}
	logrus.Debugf("close [%s]", ls.config.PosFile)

	return nil
}
