package logsep

import (
	"github.com/BurntSushi/toml"
)

type outputlog struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
	File string `toml:"file"`
}

type config struct {
	InputLogPath string      `toml:"input_log"`
	PosFile      string      `toml:"pos_file"`
	LogUser      string      `toml:"user"`
	LogGroup     string      `toml:"group"`
	OutputLog    []outputlog `toml:"output_log"`
}

func loadConfig(path string) (*config, error) {
	var c config
	defaultConfig(&c)

	_, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func defaultConfig(config *config) {
	config.InputLogPath = "/usr/local/lsws/logs/access.log"
	config.PosFile = "/usr/local/lsws/logs/logsep.pos"
	config.LogUser = ""
	config.LogGroup = ""
	config.OutputLog = nil
}
