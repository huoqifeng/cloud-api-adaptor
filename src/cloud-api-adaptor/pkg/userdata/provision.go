package userdata

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	daemon "github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/pkg/forwarder"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/aws"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/azure"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/docker"
	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v2"
)

const (
	InitdataMeta   = "/run/peerpod/initdata.meta"
	AuthJsonPath   = "/run/peerpod/auth.json"
	CheckSumPath   = "/run/peerpod/checksum.txt"
	ConfigParent   = "/run/peerpod"
	DaemonJsonName = "daemon.json"
)

var logger = log.New(log.Writer(), "[userdata/provision] ", log.LstdFlags|log.Lmsgprefix)

var StaticFiles = []string{"/run/peerpod/aa.toml", "/run/peerpod/cdh.toml", "/run/peerpod/policy.rego"}

type Config struct {
	parentPath   string
	fetchTimeout int
}

func NewConfig(fetchTimeout int) *Config {
	return &Config{ConfigParent, fetchTimeout}
}

type WriteFile struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
}

type CloudConfig struct {
	WriteFiles []WriteFile `yaml:"write_files"`
}

type InitData struct {
	Algorithom string            `toml:"algorithm"`
	Version    string            `toml:"version"`
	Data       map[string]string `toml:"data,omitempty"`
}

type UserDataProvider interface {
	GetUserData(ctx context.Context) ([]byte, error)
	GetRetryDelay() time.Duration
}

type DefaultRetry struct{}

func (d DefaultRetry) GetRetryDelay() time.Duration {
	return 5 * time.Second
}

type AzureUserDataProvider struct{ DefaultRetry }

func (a AzureUserDataProvider) GetUserData(ctx context.Context) ([]byte, error) {
	url := azure.AzureUserDataImdsUrl
	logger.Printf("provider: Azure, userDataUrl: %s\n", url)
	return azure.GetUserData(ctx, url)
}

type AWSUserDataProvider struct{ DefaultRetry }

func (a AWSUserDataProvider) GetUserData(ctx context.Context) ([]byte, error) {
	url := aws.AWSUserDataImdsUrl
	logger.Printf("provider: AWS, userDataUrl: %s\n", url)
	return aws.GetUserData(ctx, url)
}

type DockerUserDataProvider struct{ DefaultRetry }

func (a DockerUserDataProvider) GetUserData(ctx context.Context) ([]byte, error) {
	url := docker.DockerUserDataUrl
	logger.Printf("provider: Docker, userDataUrl: %s\n", url)
	return docker.GetUserData(ctx, url)
}

func newProvider(ctx context.Context) (UserDataProvider, error) {

	// This checks for the presence of a file and doesn't rely on http req like the
	// azure, aws ones, thereby making it faster and hence checking this first
	if docker.IsDocker(ctx) {
		return DockerUserDataProvider{}, nil
	}
	if azure.IsAzure(ctx) {
		return AzureUserDataProvider{}, nil
	}

	if aws.IsAWS(ctx) {
		return AWSUserDataProvider{}, nil
	}

	return nil, fmt.Errorf("unsupported user data provider")
}

func retrieveCloudConfig(ctx context.Context, provider UserDataProvider) (*CloudConfig, error) {
	var cc CloudConfig

	// Use retry.Do to retry the getUserData function until it succeeds
	// This is needed because the VM's userData is not available immediately
	err := retry.Do(
		func() error {
			ud, err := provider.GetUserData(ctx)
			if err != nil {
				return fmt.Errorf("failed to get user data: %w", err)
			}

			// We parse user data now, b/c we want to retry if it's not valid
			parsed, err := parseUserData(ud)
			if err != nil {
				return fmt.Errorf("failed to parse user data: %w", err)
			}
			cc = *parsed

			// Valid user data, stop retrying
			return nil
		},
		retry.Context(ctx),
		retry.Delay(provider.GetRetryDelay()),
		retry.LastErrorOnly(true),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, err error) {
			logger.Printf("Retry attempt %d: %v\n", n, err)
		}),
	)

	return &cc, err
}

func parseUserData(userData []byte) (*CloudConfig, error) {
	var cc CloudConfig
	err := yaml.UnmarshalStrict(userData, &cc)
	if err != nil {
		return nil, err
	}
	return &cc, nil
}

func parseDaemonConfig(content []byte) (*daemon.Config, error) {
	var dc daemon.Config
	err := json.Unmarshal(content, &dc)
	if err != nil {
		return nil, err
	}
	return &dc, nil
}

func writeFile(path string, bytes []byte) error {
	// Ensure the parent directory exists
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	err = os.WriteFile(path, bytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}
	logger.Printf("Wrote %s\n", path)
	return nil
}

func processCloudConfig(cfg *Config, cc *CloudConfig) error {
	for _, wf := range cc.WriteFiles {
		path := wf.Path
		bytes := []byte(wf.Content)
		if strings.HasPrefix(path, cfg.parentPath) { // handle files in "/run/peerpod"
			if strings.HasSuffix(path, DaemonJsonName) { // handle daemon.json file
				if bytes == nil {
					return fmt.Errorf("failed to find daemon config entry in cloud config")
				}
				daemonConfig, err := parseDaemonConfig(bytes)
				if err != nil {
					return fmt.Errorf("failed to parse daemon config %s: %w", path, err)
				}
				if err = writeFile(path, bytes); err != nil {
					return fmt.Errorf("failed to write daemon config file %s: %w", path, err)
				}
				if daemonConfig.AuthJson != "" { // handle auth json file
					bytes := []byte(daemonConfig.AuthJson)
					if err = writeFile(AuthJsonPath, bytes); err != nil {
						return fmt.Errorf("failed to write auth json file %s: %w", AuthJsonPath, err)
					}
				}
			} else { // handle other config files
				if err := writeFile(path, bytes); err != nil {
					return fmt.Errorf("failed to write config file %s: %w", path, err)
				}
			}
		} else {
			return fmt.Errorf("failed to write config file, path %s does not in folder %s", path, cfg.parentPath)
		}
	}
	return nil
}

func calculateUserDataHash() error {
	initToml, err := os.ReadFile(InitdataMeta)
	if err != nil {
		return err
	}
	var initdata InitData
	err = toml.Unmarshal(initToml, &initdata)
	if err != nil {
		return err
	}

	checksumStr := ""
	var byteData []byte
	for _, file := range StaticFiles {
		if _, err := os.Stat(file); err == nil {
			logger.Printf("calculateUserDataHash and reading file %s\n", file)
			bytes, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("Error reading file %s: %v", file, err)
			}
			byteData = append(byteData, bytes...)
		}
	}

	switch initdata.Algorithom {
	case "sha256":
		hash := sha256.Sum256(byteData)
		checksumStr = hex.EncodeToString(hash[:])
	case "sha384":
		hash := sha512.Sum384(byteData)
		checksumStr = hex.EncodeToString(hash[:])
	case "sha512":
		hash := sha512.Sum512(byteData)
		checksumStr = hex.EncodeToString(hash[:])
	default:
		return fmt.Errorf("Error creating initdata hash, the algorothom %s not supported", initdata.Algorithom)
	}

	err = os.WriteFile(CheckSumPath, []byte(checksumStr), 0644) // the hash in CheckSumPath will also be used by attester
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", CheckSumPath, err)
	}

	return nil
}

func ProvisionFiles(cfg *Config) error {
	bg := context.Background()
	duration := time.Duration(cfg.fetchTimeout) * time.Second
	ctx, cancel := context.WithTimeout(bg, duration)
	defer cancel()

	// some providers provision config files via process-user-data
	// some providers rely on cloud-init provisin config files
	// all providers need calculate the hash value for attesters usage
	provider, err := newProvider(ctx)
	if provider != nil {
		cc, err := retrieveCloudConfig(ctx, provider)
		if err != nil {
			return fmt.Errorf("failed to retrieve cloud config: %w", err)
		}

		if err = processCloudConfig(cfg, cc); err != nil {
			return fmt.Errorf("failed to process cloud config: %w", err)
		}
	} else {
		logger.Printf("unsupported user data provider %s, we calculate initdata hash only.\n")
	}

	if err = calculateUserDataHash(); err != nil {
		return fmt.Errorf("failed to calculate initdata hash: %w", err)
	}

	return nil
}
