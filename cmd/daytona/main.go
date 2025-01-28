// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	golog "log"

	"github.com/daytonaio/daytona/internal/util"
	"github.com/daytonaio/daytona/pkg/agent"
	"github.com/daytonaio/daytona/pkg/agent/config"
	"github.com/daytonaio/daytona/pkg/agent/toolbox"
	log "github.com/sirupsen/logrus"
)

func main() {
	setLogLevel()

	agentMode := config.ModeProject

	if hostModeFlag {
		agentMode = config.ModeHost
	}

	c, err := config.GetConfig(agentMode)
	if err != nil {
		panic(err)
	}
	c.ProjectDir = filepath.Join(os.Getenv("HOME"), c.ProjectName)

	if projectDir := os.Getenv("DAYTONA_PROJECT_DIR"); projectDir != "" {
		c.ProjectDir = projectDir
	}

	if _, err := os.Stat(c.ProjectDir); os.IsNotExist(err) {
		if err := os.MkdirAll(c.ProjectDir, 0755); err != nil {
			panic(fmt.Errorf("failed to create project directory: %w", err))
		}
	}

	var agentLogWriter io.Writer
	if c.LogFilePath != nil {
		logFile, err := os.OpenFile(*c.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
		agentLogWriter = logFile
	}

	toolBoxServer := &toolbox.Server{
		ProjectDir: c.ProjectDir,
	}

	agent := agent.Agent{
		Config: c,
		//	Git:    git,
		//	Ssh:              sshServer,
		Toolbox: toolBoxServer,
		//	Tailscale:        tailscaleServer,
		LogWriter: agentLogWriter,
		//	TelemetryEnabled: telemetryEnabled,
	}

	agent.Start()
}

var hostModeFlag bool

func setLogLevel() {
	agentLogLevel := os.Getenv("AGENT_LOG_LEVEL")
	if agentLogLevel != "" {
		level, err := log.ParseLevel(agentLogLevel)
		if err != nil {
			log.Errorf("Invalid log level: %s, defaulting to info level", agentLogLevel)
			level = log.InfoLevel
		}
		log.SetLevel(level)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}

func init() {
	logLevel := log.WarnLevel

	logLevelEnv, logLevelSet := os.LookupEnv("LOG_LEVEL")

	if logLevelSet {
		var err error
		logLevel, err = log.ParseLevel(logLevelEnv)
		if err != nil {
			logLevel = log.WarnLevel
		}
	}

	log.SetLevel(logLevel)

	golog.SetOutput(&util.DebugLogWriter{})
}
