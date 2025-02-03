// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/daytonaio/daytona/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Add a standard error response struct
type ErrorResponse struct {
	Error string `json:"error"`
}

func SessionExecuteCommand(configDir string) func(c *gin.Context) {
	return func(c *gin.Context) {
		sessionId := c.Param("sessionId")
		if sessionId == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "session id is required"})
			return
		}

		var request SessionExecuteRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		// Validate command is not empty (if not already handled by binding)
		if strings.TrimSpace(request.Command) == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "command cannot be empty"})
			return
		}

		session, ok := sessions[sessionId]
		if !ok {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "session not found"})
			return
		}

		var cmdId *string
		var logFile *os.File

		cmdId = util.Pointer(uuid.NewString())

		command := &Command{
			Id:      *cmdId,
			Command: request.Command,
		}
		session.commands[*cmdId] = command

		logFilePath := command.LogFilePath(session.Dir(configDir))

		if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: fmt.Sprintf("failed to create log directory: %v", err),
			})
			return
		}

		logFile, err := os.Create(logFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: fmt.Sprintf("failed to create log file: %v", err),
			})
			return
		}

		cmdToExec := fmt.Sprintf("%s > %s 2>&1 ; echo \"DTN_EXIT: $?\" >> %s\n", request.Command, logFile.Name(), logFile.Name())

		type execResult struct {
			out      string
			err      error
			exitCode *int
		}
		resultChan := make(chan execResult)

		go func() {
			out := ""
			defer close(resultChan)

			logChan := make(chan []byte)
			errChan := make(chan error)

			go util.ReadLog(context.Background(), logFile, true, logChan, errChan)

			defer logFile.Close()

			for {
				select {
				case logEntry := <-logChan:
					logEntry = bytes.Trim(logEntry, "\x00")
					if len(logEntry) == 0 {
						continue
					}
					exitCode, line := extractExitCode(string(logEntry))
					out += line

					if exitCode != nil {
						sessions[sessionId].commands[*cmdId].ExitCode = exitCode
						resultChan <- execResult{out: out, exitCode: exitCode, err: nil}
						return
					}
				case err := <-errChan:
					if err != nil {
						resultChan <- execResult{out: out, exitCode: nil, err: err}
						return
					}
				}
			}
		}()

		_, err = session.stdinWriter.Write([]byte(cmdToExec))
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: fmt.Sprintf("failed to write command: %v", err),
			})
			return
		}

		if request.Async {
			c.JSON(http.StatusAccepted, SessionExecuteResponse{
				CommandId: cmdId,
			})
			return
		}

		result := <-resultChan
		if result.err != nil {
			c.AbortWithError(http.StatusBadRequest, result.err)
			return
		}

		c.JSON(http.StatusOK, SessionExecuteResponse{
			CommandId: cmdId,
			Output:    &result.out,
			ExitCode:  result.exitCode,
		})
	}
}

func extractExitCode(output string) (*int, string) {
	var exitCode *int

	regex := regexp.MustCompile(`DTN_EXIT: (\d+)\n`)
	matches := regex.FindStringSubmatch(output)
	if len(matches) > 1 {
		code, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, output
		}
		exitCode = &code
	}

	if exitCode != nil {
		output = strings.Replace(output, fmt.Sprintf("DTN_EXIT: %d\n", *exitCode), "", 1)
	}

	return exitCode, output
}
