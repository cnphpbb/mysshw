package cmd

import (
	"fmt"
	"os"
	"strings"

	"mysshw/config"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Config file example:
/*
# server group 1
- name: server group 1
  children:
  - { name: server 1, user: root, host: 192.168.1.2 }
  - { name: server 2, user: root, host: 192.168.1.3 }
  - { name: server 3, user: root, host: 192.168.1.4 }
# server group 2
- name: server group 2
  children:
  - { name: server 1, user: root, host: 192.168.2.2 }
  - { name: server 2, user: root, host: 192.168.3.3 }
  - { name: dev server fully configured, user: appuser, host: 192.168.8.35, port: 22, password: 123456 }
  - { name: dev server with key path, user: appuser, host: 192.168.8.35, port: 22, keypath: /root/.ssh/id_rsa }
  - { name: dev server with passphrase key, user: appuser, host: 192.168.8.35, port: 22, keypath: /root/.ssh/id_rsa, passphrase: abcdefghijklmn}
  - { name: dev server without port, user: appuser, host: 192.168.8.35 }
*/

// YMLCmd provides functionality to migrate from old SSHW YAML config to new mysshw TOML config
var YMLCmd = &cobra.Command{
	Use:   "yml",
	Short: "Migrate from sshw YAML config to mysshw TOML config file",
	Long: `Parse and migrate content from the .sshw.yml file of the original sshw project to the .toml config file of the mysshw project.

Example usage:
  mysshw yml -f ~/.sshw.yml

This command will read the specified YAML config file, convert it to TOML format, and append it to the current mysshw config file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Process config file path
		cfgPath, _ := cmd.Flags().GetString("cfg")
		if cfgPath != "" {
			config.CFG_PATH = cfgPath
		}

		// Read YAML file
		ymlContent, err := os.ReadFile(ymlFilePath)
		if err != nil {
			return fmt.Errorf("mysshw:: failed to read YAML file: %v", err)
		}

		// Parse YAML content
		var ymlGroups []YMLServerGroup
		if err := yaml.Unmarshal(ymlContent, &ymlGroups); err != nil {
			return fmt.Errorf("mysshw:: YAML parsing error: %v", err)
		}

		// Convert to TOML config
		result, err := convertYAMLToTOML(ymlGroups)
		if err != nil {
			return fmt.Errorf("mysshw:: conversion failed: %v", err)
		}

		// Append to config file
		if err := appendToConfigFile(result); err != nil {
			return fmt.Errorf("mysshw:: failed to append to config file: %v", err)
		}

		// Prompt user
		fmt.Println("mysshw:: Config file has been successfully updated!")
		fmt.Println("mysshw:: Please restart mysshw service to apply the new configuration.")

		return nil
	},
}

var ymlFilePath string

func init() {
	// Configure command line parameters
	YMLCmd.Flags().StringVarP(&ymlFilePath, "file", "f", "", "Path to sshw project's YAML config file")
	YMLCmd.MarkFlagRequired("file") // Mark file path parameter as required
}

// YMLServerGroup represents server group structure parsed from YAML file
type YMLServerGroup struct {
	Name     string      `yaml:"name"`
	Children []YMLServer `yaml:"children"`
}

// YMLServer represents server information parsed from YAML file
type YMLServer struct {
	Name       string `yaml:"name"`
	Alias      string `yaml:"alias,omitempty"`
	Host       string `yaml:"host"`
	User       string `yaml:"user,omitempty"`
	Port       int    `yaml:"port,omitempty"`
	KeyPath    string `yaml:"keypath,omitempty"`
	Passphrase string `yaml:"passphrase,omitempty"`
	Password   string `yaml:"password,omitempty"`
}

// convertYAMLToTOML converts YAML configuration to TOML format
func convertYAMLToTOML(ymlGroups []YMLServerGroup) (string, error) {
	var result string

	// Iterate through each server group
	for _, group := range ymlGroups {
		// Add group comment
		result += fmt.Sprintf("# %s\n", group.Name)
		// Add group configuration
		result += fmt.Sprintf("[[nodes]]\ngroups = \"%s\"\n\n", group.Name)

		// Iterate through each server in the group
		for i, server := range group.Children {
			// Handle the first server
			if i == 0 {
				result += "[[nodes.ssh]]\n"
			} else {
				result += "[[nodes.ssh]]\n"
			}

			// Add server configuration
			result += fmt.Sprintf("alias = '%s'\n", escapeTomlString(server.Alias))
			result += fmt.Sprintf("host = '%s'\n", escapeTomlString(server.Host))
			result += fmt.Sprintf("name = '%s'\n", escapeTomlString(server.Name))

			// Add optional fields
			if server.User != "" {
				result += fmt.Sprintf("user = '%s'\n", escapeTomlString(server.User))
			}

			if server.Port > 0 {
				result += fmt.Sprintf("port = %d\n", server.Port)
			} else if server.Port == 0 {
				// Default port 22
				result += "port = 22\n"
			}

			if server.Password != "" {
				result += fmt.Sprintf("password = '%s'\n", escapeTomlString(server.Password))
			}

			if server.KeyPath != "" {
				result += fmt.Sprintf("keypath = '%s'\n", escapeTomlString(server.KeyPath))
			}

			if server.Passphrase != "" {
				result += fmt.Sprintf("passphrase = '%s'\n", escapeTomlString(server.Passphrase))
			}

			result += "\n"
		}

		result += "\n"
	}

	return result, nil
}

// escapeTomlString escapes special characters in TOML strings
func escapeTomlString(s string) string {
	// Replace single quotes and backslashes with escaped versions
	result := s
	result = strings.ReplaceAll(result, "\\", "\\\\")
	result = strings.ReplaceAll(result, "'", "\\'")
	return result
}

// appendToConfigFile appends content to the config file
func appendToConfigFile(content string) error {
	// Get full config file path
	cfgPath, err := config.GetCfgPath(config.CFG_PATH)
	if err != nil {
		return err
	}

	// Check if file exists
	fileInfo, err := os.Stat(cfgPath)
	fileExists := err == nil && !fileInfo.IsDir()

	// Open file in append mode
	file, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// If file exists, add a blank line before appending new content
	if fileExists {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}

	// Write the converted TOML content
	if _, err := file.WriteString(content); err != nil {
		return err
	}

	return nil
}
