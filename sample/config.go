package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Lark struct {
		AppID     string `yaml:"appId"`
		AppSecret string `yaml:"appSecret"`
	} `yaml:"lark"`
}

func LoadConfig(path string) (*Config, error) {
	configBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	config := &Config{}
	err = yaml.Unmarshal(configBytes, config)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return config, nil
}
