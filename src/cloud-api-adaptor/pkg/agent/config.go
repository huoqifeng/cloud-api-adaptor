package agent

import (
	toml "github.com/pelletier/go-toml/v2"
)

const (
	ServerAddr             = "unix:///run/kata-containers/agent.sock"
	GuestComponentsProcs   = "none"
	DefaultAgentConfigPath = "/run/peerpod/agent-config.toml"
	DefaultAuthJsonPath    = "/run/peerpod/auth.json"
)

type AgentConfig struct {
	ServerAddr            string `toml:"server_addr"`
	AaKbcParams           string `toml:"aa_kbc_params"`
	ImageRegistryAuthFile string `toml:"image_registry_auth_file"`
	GuestComponentsProcs  string `toml:"guest_components_procs"`
}

func CreateConfigFile(aaKBCParams string) (string, error) {
	config := AgentConfig{
		ServerAddr:            ServerAddr,
		AaKbcParams:           aaKBCParams,
		ImageRegistryAuthFile: DefaultAuthJsonPath,
		GuestComponentsProcs:  GuestComponentsProcs,
	}
	bytes, err := toml.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
