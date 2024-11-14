// internal/config/config.go
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
    Server struct {
        Host string `json:"host"`
        Port int    `json:"port"`
    } `json:"server"`
    
    Database struct {
        Path string `json:"path"`
    } `json:"database"`
    
    Environment string `json:"environment"` // dev, prod
    LogLevel    string `json:"log_level"`  // debug, info, warn, error
}

func getConfigPath() string {
    env := os.Getenv("TIG_ENV")
    if env == "" {
        env = "development"
    }
    return fmt.Sprintf("config/config.%s.json", env)
}


func Load(path string) (*Config, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    var config Config
    if err := json.NewDecoder(file).Decode(&config); err != nil {
        return nil, err
    }

    return &config, nil
}