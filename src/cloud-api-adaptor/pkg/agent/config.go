package agent

import (
	"fmt"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	DefaultAaConfigPath = "/run/peerpod/aa.toml"
)

type TokenConfigs struct {
	TokenCfg struct {} `toml:"token_configs"`
	CocoAs struct {
		URL string `toml:"url"`
	} `toml:"token_configs.coco_as"`
	Kbs struct {
		URL  string `toml:"url"`
		Cert string `toml:"cert"`
	} `toml:"token_configs.kbs"`
}

func parseAAKBCParams(aaKBCParams string) (string, error) {
	parts := strings.SplitN(aaKBCParams, "::", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("Invalid aa-kbs-params input: %s", aaKBCParams)
	}
	_, url := parts[0], parts[1]
	return url, nil
}

func CreateConfigFile(aaKBCParams string, certStr string) (string, error) {
	url, err := parseAAKBCParams(aaKBCParams)
	if err != nil {
		return "", err
	}

	config := TokenConfigs{}
	config.CocoAs.URL = ""
	config.Kbs.URL = url
	config.Kbs.Cert = certStr

	bytes, err := toml.Marshal(config)
	if err != nil {
		return "", err
	}
	tomlString := strings.ReplaceAll(string(bytes), "['", "[")
	tomlString = strings.ReplaceAll(tomlString, "']", "]")
	return tomlString, nil
}
